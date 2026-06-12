package server

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/lieyan666/firevoicebox/internal/store"
)

// publicProject is the project view exposed to the recording page.
type publicProject struct {
	ID             string `json:"id"`
	Slug           string `json:"slug"`
	Title          string `json:"title"`
	Description    string `json:"description"`
	MaxDurationSec int    `json:"max_duration_sec"`
	Status         string `json:"status"`
	Accepting      bool   `json:"accepting"`
}

func (s *Server) handlePublicProject(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	p, err := s.st.GetProjectByIDOrSlug(key)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "project not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	writeJSON(w, http.StatusOK, publicProject{
		ID:             p.ID,
		Slug:           p.Slug,
		Title:          p.Title,
		Description:    p.Description,
		MaxDurationSec: p.MaxDurationSec,
		Status:         p.Status,
		Accepting:      p.Status == store.StatusOpen,
	})
}

func (s *Server) handleCreateSubmission(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	p, err := s.st.GetProjectByIDOrSlug(key)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "project not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	if p.Status != store.StatusOpen {
		writeErr(w, http.StatusForbidden, "this collection is closed")
		return
	}

	ip := s.clientIP(r)

	// Enforce per-IP submission cap before doing expensive work.
	if p.MaxPerIP > 0 {
		n, err := s.st.CountSubmissionsByIP(p.ID, ip)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "limit check failed")
			return
		}
		if n >= p.MaxPerIP {
			writeErr(w, http.StatusTooManyRequests, "upload limit reached for your network")
			return
		}
	}

	maxBytes := int64(s.cfg.Server.MaxUploadMB) << 20
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes+(1<<20)) // headroom for form fields
	if err := r.ParseMultipartForm(maxBytes + (1 << 20)); err != nil {
		writeErr(w, http.StatusRequestEntityTooLarge, "upload too large or malformed")
		return
	}

	studentID := strings.TrimSpace(r.FormValue("student_id"))
	nickname := strings.TrimSpace(r.FormValue("nickname"))
	if studentID == "" || nickname == "" {
		writeErr(w, http.StatusBadRequest, "student_id and nickname are required")
		return
	}
	if len(studentID) > 64 || len(nickname) > 64 {
		writeErr(w, http.StatusBadRequest, "student_id or nickname too long")
		return
	}

	duration, _ := strconv.Atoi(r.FormValue("duration_sec"))
	if duration < 0 {
		duration = 0
	}
	if p.MaxDurationSec > 0 && duration > p.MaxDurationSec+1 {
		writeErr(w, http.StatusBadRequest, "recording exceeds the time limit")
		return
	}

	file, header, err := r.FormFile("audio")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "missing audio file")
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		writeErr(w, http.StatusRequestEntityTooLarge, "failed to read upload")
		return
	}
	if len(data) == 0 {
		writeErr(w, http.StatusBadRequest, "empty recording")
		return
	}

	srcMime := header.Header.Get("Content-Type")
	if srcMime == "" {
		srcMime = r.FormValue("mime")
	}

	subID := newID(16)
	relPath, mime, size, err := s.audio.Save(p.ID, subID, srcMime, data)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to store recording")
		return
	}

	sub := &store.Submission{
		ID:          subID,
		ProjectID:   p.ID,
		StudentID:   studentID,
		Nickname:    nickname,
		IP:          ip,
		UserAgent:   r.UserAgent(),
		DurationSec: duration,
		FilePath:    relPath,
		MimeType:    mime,
		SizeBytes:   size,
		CreatedAt:   time.Now(),
	}
	if err := s.st.CreateSubmission(sub); err != nil {
		_ = s.audio.Delete(relPath) // avoid orphaned file
		writeErr(w, http.StatusInternalServerError, "failed to save submission")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"id": subID})
}
