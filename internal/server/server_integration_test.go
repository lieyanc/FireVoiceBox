package server

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"io/fs"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/textproto"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/lieyan666/firevoicebox/internal/audio"
	"github.com/lieyan666/firevoicebox/internal/config"
	"github.com/lieyan666/firevoicebox/internal/store"
)

func newIntegrationServer(t *testing.T) *httptest.Server {
	return newIntegrationServerWithDist(t, fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<html>spa</html>")},
	})
}

func newIntegrationServerWithDist(t *testing.T, dist fs.FS) *httptest.Server {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })

	cfg, _, err := config.Load(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.Server.Secret = "itest-secret"
	cfg.Server.MaxUploadMB = 5
	cfg.Admin.Password = "pw"
	au := audio.New(filepath.Join(dir, "audio"), config.TranscodeConfig{})

	srv := New(cfg, st, au, dist)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts
}

func authedClient(t *testing.T) *http.Client {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar}
}

func uploadForm(t *testing.T, fields map[string]string, fileMime string, data []byte) (io.Reader, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range fields {
		_ = mw.WriteField(k, v)
	}
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="audio"; filename="rec.webm"`)
	h.Set("Content-Type", fileMime)
	part, err := mw.CreatePart(h)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write(data)
	_ = mw.Close()
	return &buf, mw.FormDataContentType()
}

func decode[T any](t *testing.T, r *http.Response) T {
	t.Helper()
	var v T
	if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
		t.Fatalf("decode: %v", err)
	}
	r.Body.Close()
	return v
}

func TestCacheHeaders(t *testing.T) {
	ts := newIntegrationServerWithDist(t, fstest.MapFS{
		"index.html":             &fstest.MapFile{Data: []byte("<html>spa</html>")},
		"assets/index-AbC123.js": &fstest.MapFile{Data: []byte(`console.log("ok")`)},
		"favicon.ico":            &fstest.MapFile{Data: []byte("ico")},
	})

	get := func(path string) *http.Response {
		t.Helper()
		res, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}

	assertNoStore := func(path string, wantPrivate bool) {
		t.Helper()
		res := get(path)
		defer res.Body.Close()
		want := cacheControlNoStore
		if wantPrivate {
			want = cacheControlPrivateNoStore
		}
		if got := res.Header.Get("Cache-Control"); got != want {
			t.Fatalf("%s Cache-Control=%q want %q", path, got, want)
		}
		if got := res.Header.Get("Pragma"); got != "no-cache" {
			t.Fatalf("%s Pragma=%q want no-cache", path, got)
		}
		if got := res.Header.Get("Expires"); got != "0" {
			t.Fatalf("%s Expires=%q want 0", path, got)
		}
	}

	assertNoStore("/", false)
	assertNoStore("/admin/projects/abc", false)
	assertNoStore("/api/admin/me", true)

	res := get("/api/client/version")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("client version status=%d", res.StatusCode)
	}
	if got := res.Header.Get("Cache-Control"); got != cacheControlPrivateNoStore {
		t.Fatalf("client version Cache-Control=%q want %q", got, cacheControlPrivateNoStore)
	}
	clientVersion := decode[struct {
		CacheKey string `json:"cache_key"`
	}](t, res)
	if clientVersion.CacheKey == "" {
		t.Fatalf("client version cache_key is empty")
	}

	res = get("/admin?fvb_refresh=test")
	if got := res.Header.Get("Clear-Site-Data"); got != `"cache"` {
		t.Fatalf("Clear-Site-Data=%q want %q", got, `"cache"`)
	}
	res.Body.Close()

	res = get("/assets/index-AbC123.js")
	if got := res.Header.Get("Cache-Control"); got != cacheControlImmutableAsset {
		t.Fatalf("asset Cache-Control=%q want %q", got, cacheControlImmutableAsset)
	}
	res.Body.Close()
}

func TestFullFlow(t *testing.T) {
	ts := newIntegrationServer(t)
	c := authedClient(t)

	// --- login ---
	post := func(cl *http.Client, path, ct string, body io.Reader) *http.Response {
		req, _ := http.NewRequest("POST", ts.URL+path, body)
		if ct != "" {
			req.Header.Set("Content-Type", ct)
		}
		res, err := cl.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}

	if res := post(c, "/api/admin/login", "application/json", strings.NewReader(`{"password":"wrong"}`)); res.StatusCode != 401 {
		t.Fatalf("wrong login status=%d", res.StatusCode)
	}
	if res := post(c, "/api/admin/login", "application/json", strings.NewReader(`{"password":"pw"}`)); res.StatusCode != 200 {
		t.Fatalf("login status=%d", res.StatusCode)
	}

	// --- create project (max_per_ip=2) ---
	res := post(c, "/api/admin/projects", "application/json", strings.NewReader(`{"title":"Grad","max_duration_sec":30,"max_per_ip":2,"slug":"grad"}`))
	if res.StatusCode != 201 {
		t.Fatalf("create status=%d", res.StatusCode)
	}
	proj := decode[store.Project](t, res)
	if proj.Slug != "grad" || proj.ManageToken == "" {
		t.Fatalf("unexpected project %+v", proj)
	}

	// --- public project by slug ---
	res, _ = c.Get(ts.URL + "/api/p/grad")
	if res.StatusCode != 200 {
		t.Fatalf("public status=%d", res.StatusCode)
	}

	upload := func(student string, dur string) int {
		body, ct := uploadForm(t, map[string]string{"student_id": student, "nickname": "N-" + student, "duration_sec": dur}, "audio/webm", []byte("audio-"+student))
		res := post(c, "/api/p/"+proj.ID+"/submissions", ct, body)
		res.Body.Close()
		return res.StatusCode
	}

	// Two uploads OK, third blocked by per-IP cap.
	if s := upload("1", "10"); s != 201 {
		t.Fatalf("upload1 status=%d", s)
	}
	if s := upload("2", "10"); s != 201 {
		t.Fatalf("upload2 status=%d", s)
	}
	if s := upload("3", "10"); s != 429 {
		t.Fatalf("upload3 status=%d want 429", s)
	}

	// --- manage list (owner cookie) ---
	res, _ = c.Get(ts.URL + "/api/manage/projects/" + proj.ID + "/submissions")
	if res.StatusCode != 200 {
		t.Fatalf("manage list status=%d", res.StatusCode)
	}
	subs := decode[[]map[string]any](t, res)
	if len(subs) != 2 {
		t.Fatalf("want 2 subs, got %d", len(subs))
	}
	audioPath, _ := subs[0]["audio_path"].(string)
	subID, _ := subs[0]["id"].(string)

	// --- token access (fresh client, no cookie) ---
	anon := &http.Client{}
	req, _ := http.NewRequest("GET", ts.URL+"/api/manage/projects/"+proj.ID+"/submissions", nil)
	req.Header.Set("X-Manage-Token", proj.ManageToken)
	res, _ = anon.Do(req)
	if res.StatusCode != 200 {
		t.Fatalf("token list status=%d", res.StatusCode)
	}
	res.Body.Close()

	// No auth at all -> 401.
	res, _ = anon.Get(ts.URL + "/api/manage/projects/" + proj.ID + "/submissions")
	if res.StatusCode != 401 {
		t.Fatalf("no-auth status=%d want 401", res.StatusCode)
	}
	res.Body.Close()

	// --- audio range request ---
	req, _ = http.NewRequest("GET", ts.URL+audioPath, nil)
	req.Header.Set("Range", "bytes=0-2")
	res, _ = c.Do(req)
	if res.StatusCode != http.StatusPartialContent {
		t.Fatalf("range status=%d want 206", res.StatusCode)
	}
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if len(b) != 3 {
		t.Fatalf("range body len=%d want 3", len(b))
	}

	// --- export zip ---
	res, _ = c.Get(ts.URL + "/api/manage/projects/" + proj.ID + "/export")
	if ct := res.Header.Get("Content-Type"); ct != "application/zip" {
		t.Fatalf("export content-type=%q", ct)
	}
	zbytes, _ := io.ReadAll(res.Body)
	res.Body.Close()
	zr, err := zip.NewReader(bytes.NewReader(zbytes), int64(len(zbytes)))
	if err != nil {
		t.Fatalf("zip open: %v", err)
	}
	var hasCSV, audioCount = false, 0
	for _, f := range zr.File {
		if f.Name == "metadata.csv" {
			hasCSV = true
		}
		if strings.HasPrefix(f.Name, "audio/") {
			audioCount++
		}
	}
	if !hasCSV || audioCount != 2 {
		t.Fatalf("zip contents: csv=%v audio=%d", hasCSV, audioCount)
	}

	// --- delete submission ---
	req, _ = http.NewRequest("DELETE", ts.URL+"/api/manage/submissions/"+subID, nil)
	res, _ = c.Do(req)
	if res.StatusCode != 200 {
		t.Fatalf("delete status=%d", res.StatusCode)
	}
	res.Body.Close()
	res, _ = c.Get(ts.URL + "/api/manage/projects/" + proj.ID + "/submissions")
	after := decode[[]map[string]any](t, res)
	if len(after) != 1 {
		t.Fatalf("after delete=%d want 1", len(after))
	}

	// --- SPA fallback ---
	res, _ = c.Get(ts.URL + "/admin/projects/" + proj.ID)
	if res.StatusCode != 200 || !strings.Contains(res.Header.Get("Content-Type"), "text/html") {
		t.Fatalf("spa fallback status=%d ct=%q", res.StatusCode, res.Header.Get("Content-Type"))
	}
	res.Body.Close()
}

func TestDurationLimitRejected(t *testing.T) {
	ts := newIntegrationServer(t)
	c := authedClient(t)
	req, _ := http.NewRequest("POST", ts.URL+"/api/admin/login", strings.NewReader(`{"password":"pw"}`))
	req.Header.Set("Content-Type", "application/json")
	res, _ := c.Do(req)
	res.Body.Close()

	req, _ = http.NewRequest("POST", ts.URL+"/api/admin/projects", strings.NewReader(`{"title":"T","max_duration_sec":30,"max_per_ip":0}`))
	req.Header.Set("Content-Type", "application/json")
	res, _ = c.Do(req)
	proj := decode[store.Project](t, res)

	body, ct := uploadForm(t, map[string]string{"student_id": "s", "nickname": "n", "duration_sec": "999"}, "audio/webm", []byte("x"))
	req, _ = http.NewRequest("POST", ts.URL+"/api/p/"+proj.ID+"/submissions", body)
	req.Header.Set("Content-Type", ct)
	res, _ = c.Do(req)
	res.Body.Close()
	if res.StatusCode != 400 {
		t.Fatalf("over-duration status=%d want 400", res.StatusCode)
	}
}

func TestAdminSettingsCanBeUpdated(t *testing.T) {
	ts := newIntegrationServer(t)
	c := authedClient(t)

	req, _ := http.NewRequest("POST", ts.URL+"/api/admin/login", strings.NewReader(`{"password":"pw"}`))
	req.Header.Set("Content-Type", "application/json")
	res, _ := c.Do(req)
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("login status=%d", res.StatusCode)
	}

	res, _ = c.Get(ts.URL + "/api/admin/settings")
	if res.StatusCode != 200 {
		t.Fatalf("settings status=%d", res.StatusCode)
	}
	body := decode[struct {
		Settings config.Config `json:"settings"`
	}](t, res)
	next := body.Settings
	next.Admin.Password = "new-pw"
	next.Server.MaxUploadMB = 7
	next.Server.Secret = ""
	next.Transcode.Enabled = true
	next.Transcode.Format = "wav"
	next.Transcode.Bitrate = "96k"
	next.Transcode.OnError = "reject"
	next.Update.Enabled = true
	next.Update.Channel = "dev"
	next.Update.CheckInterval = 120
	next.Update.Repo = "owner/repo"

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(next); err != nil {
		t.Fatal(err)
	}
	req, _ = http.NewRequest("PUT", ts.URL+"/api/admin/settings", &buf)
	req.Header.Set("Content-Type", "application/json")
	res, _ = c.Do(req)
	if res.StatusCode != 200 {
		t.Fatalf("update settings status=%d", res.StatusCode)
	}
	updated := decode[struct {
		Settings config.Config `json:"settings"`
	}](t, res)
	if updated.Settings.Admin.Password != "new-pw" || updated.Settings.Server.MaxUploadMB != 7 {
		t.Fatalf("settings not updated: %+v", updated.Settings)
	}
	if updated.Settings.Server.Secret == "" {
		t.Fatalf("server secret was not regenerated")
	}

	res, _ = c.Get(ts.URL + "/api/admin/me")
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("session after secret change status=%d", res.StatusCode)
	}

	fresh := authedClient(t)
	req, _ = http.NewRequest("POST", ts.URL+"/api/admin/login", strings.NewReader(`{"password":"pw"}`))
	req.Header.Set("Content-Type", "application/json")
	res, _ = fresh.Do(req)
	res.Body.Close()
	if res.StatusCode != 401 {
		t.Fatalf("old password status=%d want 401", res.StatusCode)
	}
	req, _ = http.NewRequest("POST", ts.URL+"/api/admin/login", strings.NewReader(`{"password":"new-pw"}`))
	req.Header.Set("Content-Type", "application/json")
	res, _ = fresh.Do(req)
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("new password status=%d", res.StatusCode)
	}
}
