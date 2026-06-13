package updater

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/lieyan666/firevoicebox/internal/version"
)

func TestCheckOnlyResolvesLatestStableReleaseWithoutAPI(t *testing.T) {
	originalVersion := version.Version
	defer func() { version.Version = originalVersion }()
	version.Version = "v1.0.0"

	targetName := testTargetName(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertGitHubNoCacheHeaders(t, r)
		switch r.URL.Path {
		case "/owner/repo/releases/latest/download/" + targetName:
			if r.Method != http.MethodHead {
				t.Fatalf("expected HEAD, got %s", r.Method)
			}
			http.Redirect(w, r, "/owner/repo/releases/download/v1.4.0/"+targetName, http.StatusFound)
		case "/owner/repo/releases/download/v1.4.0/" + targetName:
			if r.Method != http.MethodHead {
				t.Fatalf("expected HEAD, got %s", r.Method)
			}
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	setGitHubReleaseBaseURL(t, server.URL)

	u := testUpdater(Config{
		Channel: "stable",
		Repo:    "owner/repo",
	})

	result, err := u.CheckOnly(context.Background())
	if err != nil {
		t.Fatalf("CheckOnly returned error: %v", err)
	}
	if !result.HasUpdate {
		t.Fatalf("expected update to be available")
	}
	if result.LatestVersion != "v1.4.0" {
		t.Fatalf("expected latest version v1.4.0, got %q", result.LatestVersion)
	}
}

func TestCheckOnlyUsesConfiguredReleaseTag(t *testing.T) {
	originalVersion := version.Version
	defer func() { version.Version = originalVersion }()
	version.Version = "v1.0.0"
	targetName := testTargetName(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/owner/repo/releases/download/v1.4.0/"+targetName {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodHead {
			t.Fatalf("expected HEAD, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	setGitHubReleaseBaseURL(t, server.URL)

	u := testUpdater(Config{
		Channel: "stable",
		Tag:     "v1.4.0",
		Repo:    "owner/repo",
	})

	result, err := u.CheckOnly(context.Background())
	if err != nil {
		t.Fatalf("CheckOnly returned error: %v", err)
	}
	if !result.HasUpdate {
		t.Fatalf("expected update to be available")
	}
	if result.LatestVersion != "v1.4.0" {
		t.Fatalf("expected latest version v1.4.0, got %q", result.LatestVersion)
	}
}

func TestCheckOnlySelectsNewestPrerelease(t *testing.T) {
	originalVersion := version.Version
	originalCommit := version.Commit
	defer func() { version.Version = originalVersion }()
	defer func() { version.Commit = originalCommit }()
	version.Version = "dev-0007-20260401-aaaaaaa"
	version.Commit = "aaaaaaa"
	remoteVersion := "dev-0042-20260425-bbbbbbb"
	targetName := testTargetName(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/owner/repo/releases/download/dev/" + targetName:
			if r.Method != http.MethodHead {
				t.Fatalf("expected HEAD, got %s", r.Method)
			}
			w.WriteHeader(http.StatusOK)
		case "/owner/repo/releases/download/dev/version.json":
			_ = json.NewEncoder(w).Encode(releaseVersionInfo{
				Version:   remoteVersion,
				Commit:    "bbbbbbb",
				BuildTime: "2026-04-25T00:00:00Z",
				Tag:       "dev",
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	setGitHubReleaseBaseURL(t, server.URL)

	u := testUpdater(Config{
		Channel: "dev",
		Repo:    "owner/repo",
	})

	result, err := u.CheckOnly(context.Background())
	if err != nil {
		t.Fatalf("CheckOnly returned error: %v", err)
	}
	if !result.HasUpdate {
		t.Fatalf("expected prerelease update to be available")
	}
	if result.LatestVersion != remoteVersion {
		t.Fatalf("expected latest prerelease, got %q", result.LatestVersion)
	}
}

func TestCheckOnlySkipsDevReleaseForSameCommit(t *testing.T) {
	originalVersion := version.Version
	originalCommit := version.Commit
	defer func() { version.Version = originalVersion }()
	defer func() { version.Commit = originalCommit }()
	version.Version = "dev-0042-20260425-bbbbbbb"
	version.Commit = "bbbbbbb"
	targetName := testTargetName(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/owner/repo/releases/download/dev/" + targetName:
			if r.Method != http.MethodHead {
				t.Fatalf("expected HEAD, got %s", r.Method)
			}
			w.WriteHeader(http.StatusOK)
		case "/owner/repo/releases/download/dev/version.json":
			_ = json.NewEncoder(w).Encode(releaseVersionInfo{
				Version:   "dev-0042-20260425-bbbbbbb",
				Commit:    "bbbbbbb",
				BuildTime: "2026-04-25T00:00:00Z",
				Tag:       "dev",
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	setGitHubReleaseBaseURL(t, server.URL)

	u := testUpdater(Config{
		Channel: "dev",
		Repo:    "owner/repo",
	})

	result, err := u.CheckOnly(context.Background())
	if err != nil {
		t.Fatalf("CheckOnly returned error: %v", err)
	}
	if result.HasUpdate {
		t.Fatalf("did not expect update for same commit: %#v", result)
	}
	if result.LatestVersion != "dev-0042-20260425-bbbbbbb" {
		t.Fatalf("expected latest version from version metadata, got %q", result.LatestVersion)
	}
}

func TestPerformUpdateDownloadsAndVerifiesPrerelease(t *testing.T) {
	originalVersion := version.Version
	originalCommit := version.Commit
	defer func() { version.Version = originalVersion }()
	defer func() { version.Commit = originalCommit }()
	version.Version = "dev-0007-20260401-aaaaaaa"
	version.Commit = "aaaaaaa"

	cfg := Config{
		Channel: "dev",
		Repo:    "owner/repo",
	}
	dataDir := t.TempDir()
	u := New(
		func() Config { return cfg },
		func() string { return dataDir },
		log.New(io.Discard, "", 0),
		RestartHooks{},
	)

	tag := "dev"
	remoteVersion := "dev-0042-20260425-bbbbbbb"
	targetName, err := u.targetName()
	if err != nil {
		t.Fatalf("targetName returned error: %v", err)
	}
	binary := []byte("new binary")
	sum := fmt.Sprintf("%x", sha256.Sum256(binary))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertGitHubNoCacheHeaders(t, r)
		switch r.URL.Path {
		case "/owner/repo/releases/download/" + tag + "/" + targetName:
			if r.Method == http.MethodHead {
				w.WriteHeader(http.StatusOK)
				return
			}
			_, _ = w.Write(binary)
		case "/owner/repo/releases/download/" + tag + "/" + targetName + ".sha256":
			_, _ = w.Write([]byte(sum + "  " + targetName + "\n"))
		case "/owner/repo/releases/download/" + tag + "/version.json":
			_ = json.NewEncoder(w).Encode(releaseVersionInfo{
				Version:   remoteVersion,
				Commit:    "bbbbbbb",
				BuildTime: "2026-04-25T00:00:00Z",
				Tag:       tag,
			})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	setGitHubReleaseBaseURL(t, server.URL)

	u.performUpdate(context.Background())

	status := u.Status()
	if status.State != "ready" {
		t.Fatalf("expected update to be ready, got %q: %s", status.State, status.Error)
	}
	if status.LatestVersion != remoteVersion || u.pendingTag != tag {
		t.Fatalf("expected pending latest tag %q, got status=%q pending=%q", tag, status.LatestVersion, u.pendingTag)
	}
	got, err := os.ReadFile(u.pendingBinaryPath)
	if err != nil {
		t.Fatalf("read pending binary: %v", err)
	}
	if string(got) != string(binary) {
		t.Fatalf("pending binary content mismatch")
	}
}

func TestReleaseTargetNameIncludesOnlyPublishedTargets(t *testing.T) {
	tests := []struct {
		goos    string
		goarch  string
		want    string
		wantErr bool
	}{
		{goos: "linux", goarch: "amd64", want: "firevoicebox-linux-amd64"},
		{goos: "linux", goarch: "arm64", want: "firevoicebox-linux-arm64"},
		{goos: "darwin", goarch: "amd64", want: "firevoicebox-darwin-amd64"},
		{goos: "darwin", goarch: "arm64", want: "firevoicebox-darwin-arm64"},
		{goos: "linux", goarch: "arm", wantErr: true},
		{goos: "windows", goarch: "amd64", wantErr: true},
		{goos: "windows", goarch: "arm64", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.goos+"-"+tt.goarch, func(t *testing.T) {
			got, err := releaseTargetName(tt.goos, tt.goarch)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got target %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("releaseTargetName returned error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected target %q, got %q", tt.want, got)
			}
		})
	}
}

func testTargetName(t *testing.T) string {
	t.Helper()
	targetName, err := releaseTargetName(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Fatalf("releaseTargetName returned error: %v", err)
	}
	return targetName
}

func setGitHubReleaseBaseURL(t *testing.T, baseURL string) {
	t.Helper()
	original := githubReleaseBaseURL
	githubReleaseBaseURL = baseURL
	t.Cleanup(func() {
		githubReleaseBaseURL = original
	})
}

func testUpdater(cfg Config) *Updater {
	return New(
		func() Config { return cfg },
		func() string { return "" },
		log.New(io.Discard, "", 0),
		RestartHooks{},
	)
}

func assertGitHubNoCacheHeaders(t *testing.T, r *http.Request) {
	t.Helper()

	cacheControl := r.Header.Get("Cache-Control")
	for _, directive := range []string{"no-cache", "no-store", "max-age=0"} {
		if !strings.Contains(cacheControl, directive) {
			t.Fatalf("expected Cache-Control to include %q, got %q", directive, cacheControl)
		}
	}
	if got := r.Header.Get("Pragma"); got != "no-cache" {
		t.Fatalf("expected Pragma no-cache, got %q", got)
	}
	if got := r.Header.Get("Expires"); got != "0" {
		t.Fatalf("expected Expires 0, got %q", got)
	}
}
