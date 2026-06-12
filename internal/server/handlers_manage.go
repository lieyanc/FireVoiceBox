package server

import (
	"archive/zip"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/lieyan666/firevoicebox/internal/store"
)

// submissionView augments a submission with the URL to fetch its audio.
type submissionView struct {
	*store.Submission
	AudioPath string `json:"audio_path"`
}

// resolveProjectAccess loads a project by ID and verifies the request may
// manage it. On failure it writes the response and returns nil.
func (s *Server) resolveProjectAccess(w http.ResponseWriter, r *http.Request, id string) *store.Project {
	p, err := s.st.GetProject(id)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "project not found")
		return nil
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "lookup failed")
		return nil
	}
	if !s.hasProjectAccess(r, p) {
		writeErr(w, http.StatusUnauthorized, "access denied")
		return nil
	}
	return p
}

func (s *Server) handleManageProject(w http.ResponseWriter, r *http.Request) {
	p := s.resolveProjectAccess(w, r, chi.URLParam(r, "id"))
	if p == nil {
		return
	}
	n, _ := s.st.CountSubmissions(p.ID)
	writeJSON(w, http.StatusOK, ownerProjectView{Project: p, SubmissionCount: n})
}

func (s *Server) handleManageSubmissions(w http.ResponseWriter, r *http.Request) {
	p := s.resolveProjectAccess(w, r, chi.URLParam(r, "id"))
	if p == nil {
		return
	}
	subs, err := s.st.ListSubmissions(p.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to list submissions")
		return
	}
	out := make([]submissionView, 0, len(subs))
	for _, sub := range subs {
		out = append(out, submissionView{
			Submission: sub,
			AudioPath:  "/api/manage/submissions/" + sub.ID + "/audio",
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleSubmissionAudio(w http.ResponseWriter, r *http.Request) {
	sub, err := s.st.GetSubmission(chi.URLParam(r, "id"))
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "submission not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	p, err := s.st.GetProject(sub.ProjectID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	if !s.hasProjectAccess(r, p) {
		writeErr(w, http.StatusUnauthorized, "access denied")
		return
	}

	f, err := os.Open(s.audio.AbsPath(sub.FilePath))
	if err != nil {
		writeErr(w, http.StatusNotFound, "audio file missing")
		return
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "stat failed")
		return
	}
	if sub.MimeType != "" {
		w.Header().Set("Content-Type", sub.MimeType)
	}
	// http.ServeContent handles Range requests so wavesurfer can seek.
	http.ServeContent(w, r, filepath.Base(sub.FilePath), fi.ModTime(), f)
}

func (s *Server) handleDeleteSubmission(w http.ResponseWriter, r *http.Request) {
	sub, err := s.st.GetSubmission(chi.URLParam(r, "id"))
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "submission not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	p, err := s.st.GetProject(sub.ProjectID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	if !s.hasProjectAccess(r, p) {
		writeErr(w, http.StatusUnauthorized, "access denied")
		return
	}
	if _, err := s.st.DeleteSubmission(sub.ID); err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to delete submission")
		return
	}
	_ = s.audio.Delete(sub.FilePath)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleExport streams a zip archive of all submissions (audio + metadata.csv).
func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	p := s.resolveProjectAccess(w, r, chi.URLParam(r, "id"))
	if p == nil {
		return
	}
	subs, err := s.st.ListSubmissions(p.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to list submissions")
		return
	}

	filename := p.Slug
	if filename == "" {
		filename = p.ID
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-export.zip"`, filename))

	zw := zip.NewWriter(w)
	defer zw.Close()

	// metadata.csv
	if cw, err := zw.Create("metadata.csv"); err == nil {
		c := csv.NewWriter(cw)
		_ = c.Write([]string{"id", "student_id", "nickname", "ip", "user_agent", "duration_sec", "mime_type", "size_bytes", "created_at", "file"})
		for _, sub := range subs {
			_ = c.Write([]string{
				sub.ID, sub.StudentID, sub.Nickname, sub.IP, sub.UserAgent,
				strconv.Itoa(sub.DurationSec), sub.MimeType, strconv.FormatInt(sub.SizeBytes, 10),
				sub.CreatedAt.Format(time.RFC3339), filepath.Base(sub.FilePath),
			})
		}
		c.Flush()
	}

	// audio files under audio/
	for _, sub := range subs {
		af, err := os.Open(s.audio.AbsPath(sub.FilePath))
		if err != nil {
			continue
		}
		entry, err := zw.Create("audio/" + filepath.Base(sub.FilePath))
		if err != nil {
			af.Close()
			continue
		}
		_, _ = io.Copy(entry, af)
		af.Close()
	}
}
