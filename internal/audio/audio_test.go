package audio

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lieyan666/firevoicebox/internal/config"
)

func TestExtForMime(t *testing.T) {
	cases := map[string]string{
		"audio/webm;codecs=opus": ".webm",
		"audio/webm":             ".webm",
		"audio/mp4":              ".mp4",
		"audio/x-m4a":            ".mp4",
		"audio/ogg":              ".ogg",
		"audio/mpeg":             ".mp3",
		"audio/wav":              ".wav",
		"weird/thing":            ".bin",
		"":                       ".bin",
	}
	for mime, want := range cases {
		if got := extForMime(mime); got != want {
			t.Errorf("extForMime(%q)=%q want %q", mime, got, want)
		}
	}
}

func TestSaveNative(t *testing.T) {
	dir := t.TempDir()
	s := New(dir, config.TranscodeConfig{Enabled: false})
	if s.Transcoding() {
		t.Fatal("transcoding should be off")
	}

	data := []byte("hello-bytes")
	rel, mime, size, err := s.Save("proj1", "sub1", "audio/webm", data)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if rel != filepath.Join("proj1", "sub1.webm") {
		t.Errorf("rel=%q", rel)
	}
	if mime != "audio/webm" {
		t.Errorf("mime=%q", mime)
	}
	if size != int64(len(data)) {
		t.Errorf("size=%d want %d", size, len(data))
	}

	// File must exist with the right contents.
	got, err := os.ReadFile(s.AbsPath(rel))
	if err != nil || string(got) != string(data) {
		t.Errorf("readback failed: %v %q", err, got)
	}

	// Delete and confirm gone; deleting again is not an error.
	if err := s.Delete(rel); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := os.Stat(s.AbsPath(rel)); !os.IsNotExist(err) {
		t.Errorf("file should be gone")
	}
	if err := s.Delete(rel); err != nil {
		t.Errorf("second delete should be nil, got %v", err)
	}
}

func TestRemoveProject(t *testing.T) {
	dir := t.TempDir()
	s := New(dir, config.TranscodeConfig{})
	if _, _, _, err := s.Save("p", "a", "audio/webm", []byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveProject("p"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "p")); !os.IsNotExist(err) {
		t.Errorf("project dir should be removed")
	}
}
