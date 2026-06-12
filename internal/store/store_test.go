package store

import (
	"path/filepath"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	st, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestProjectRoundtrip(t *testing.T) {
	st := newTestStore(t)
	p := &Project{ID: "p1", Title: "Hello", MaxDurationSec: 30, MaxPerIP: 2, ManageToken: "tok"}
	if err := st.CreateProject(p); err != nil {
		t.Fatalf("create: %v", err)
	}
	if p.Slug != "p1" {
		t.Errorf("slug should default to id, got %q", p.Slug)
	}

	got, err := st.GetProject("p1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Title != "Hello" || got.MaxPerIP != 2 || got.Status != StatusOpen {
		t.Errorf("unexpected project: %+v", got)
	}

	bySlug, err := st.GetProjectByIDOrSlug("p1")
	if err != nil || bySlug.ID != "p1" {
		t.Errorf("by slug/id failed: %v %+v", err, bySlug)
	}

	if _, err := st.GetProject("nope"); err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestSubmissionCountAndCascade(t *testing.T) {
	st := newTestStore(t)
	p := &Project{ID: "p1", Title: "T", ManageToken: "tok"}
	if err := st.CreateProject(p); err != nil {
		t.Fatal(err)
	}
	for i, ip := range []string{"1.1.1.1", "1.1.1.1", "2.2.2.2"} {
		sub := &Submission{ID: string(rune('a' + i)), ProjectID: "p1", IP: ip, FilePath: "x"}
		if err := st.CreateSubmission(sub); err != nil {
			t.Fatal(err)
		}
	}
	if n, _ := st.CountSubmissions("p1"); n != 3 {
		t.Errorf("count=%d want 3", n)
	}
	if n, _ := st.CountSubmissionsByIP("p1", "1.1.1.1"); n != 2 {
		t.Errorf("by ip=%d want 2", n)
	}

	// Deleting the project should cascade to submissions.
	if err := st.DeleteProject("p1"); err != nil {
		t.Fatal(err)
	}
	if n, _ := st.CountSubmissions("p1"); n != 0 {
		t.Errorf("after cascade count=%d want 0", n)
	}
}

func TestDeleteSubmissionReturnsRow(t *testing.T) {
	st := newTestStore(t)
	p := &Project{ID: "p1", Title: "T", ManageToken: "tok"}
	_ = st.CreateProject(p)
	_ = st.CreateSubmission(&Submission{ID: "s1", ProjectID: "p1", FilePath: "audio/p1/s1.webm"})

	sub, err := st.DeleteSubmission("s1")
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if sub.FilePath != "audio/p1/s1.webm" {
		t.Errorf("expected file path returned, got %q", sub.FilePath)
	}
	if _, err := st.GetSubmission("s1"); err != ErrNotFound {
		t.Errorf("expected gone, got %v", err)
	}
}
