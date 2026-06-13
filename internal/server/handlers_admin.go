package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/lieyan666/firevoicebox/internal/store"
	"github.com/lieyan666/firevoicebox/internal/version"
)

// ownerProjectView is the full project representation returned to the owner,
// including the manage token and submission count.
type ownerProjectView struct {
	*store.Project
	SubmissionCount int `json:"submission_count"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !constantEqual(body.Password, s.cfg.Admin.Password) {
		writeErr(w, http.StatusUnauthorized, "invalid password")
		return
	}
	s.setOwnerSession(w, r)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	s.clearOwnerSession(w)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	if !s.isOwner(r) {
		writeErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"owner": true})
}

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := s.st.ListProjects()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to list projects")
		return
	}
	out := make([]ownerProjectView, 0, len(projects))
	for _, p := range projects {
		n, _ := s.st.CountSubmissions(p.ID)
		out = append(out, ownerProjectView{Project: p, SubmissionCount: n})
	}
	writeJSON(w, http.StatusOK, out)
}

type projectInput struct {
	Title          *string `json:"title"`
	Description    *string `json:"description"`
	Slug           *string `json:"slug"`
	MaxDurationSec *int    `json:"max_duration_sec"`
	MaxPerIP       *int    `json:"max_per_ip"`
	Status         *string `json:"status"`
}

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	var in projectInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if in.Title == nil || *in.Title == "" {
		writeErr(w, http.StatusBadRequest, "title is required")
		return
	}

	p := &store.Project{
		ID:             newID(10),
		Title:          *in.Title,
		MaxDurationSec: 60,
		Status:         store.StatusOpen,
		ManageToken:    newID(24),
	}
	if in.Description != nil {
		p.Description = *in.Description
	}
	if in.MaxDurationSec != nil && *in.MaxDurationSec > 0 {
		p.MaxDurationSec = *in.MaxDurationSec
	}
	if in.MaxPerIP != nil && *in.MaxPerIP >= 0 {
		p.MaxPerIP = *in.MaxPerIP
	}
	if in.Status != nil && (*in.Status == store.StatusOpen || *in.Status == store.StatusClosed) {
		p.Status = *in.Status
	}

	// Resolve a unique slug.
	slug := ""
	if in.Slug != nil {
		slug = slugify(*in.Slug)
	}
	if slug == "" {
		slug = slugify(*in.Title)
	}
	if slug == "" {
		slug = p.ID
	}
	p.Slug = s.uniqueSlug(slug)

	if err := s.st.CreateProject(p); err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to create project")
		return
	}
	n, _ := s.st.CountSubmissions(p.ID)
	writeJSON(w, http.StatusCreated, ownerProjectView{Project: p, SubmissionCount: n})
}

// uniqueSlug returns base, or base with a short suffix if already taken.
func (s *Server) uniqueSlug(base string) string {
	candidate := base
	for i := 0; i < 5; i++ {
		if _, err := s.st.GetProjectByIDOrSlug(candidate); errors.Is(err, store.ErrNotFound) {
			return candidate
		}
		candidate = base + "-" + newID(4)
	}
	return base + "-" + newID(8)
}

func (s *Server) handleUpdateProject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	p, err := s.st.GetProject(id)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "project not found")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "lookup failed")
		return
	}

	var in projectInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if in.Title != nil && *in.Title != "" {
		p.Title = *in.Title
	}
	if in.Description != nil {
		p.Description = *in.Description
	}
	if in.MaxDurationSec != nil && *in.MaxDurationSec > 0 {
		p.MaxDurationSec = *in.MaxDurationSec
	}
	if in.MaxPerIP != nil && *in.MaxPerIP >= 0 {
		p.MaxPerIP = *in.MaxPerIP
	}
	if in.Status != nil && (*in.Status == store.StatusOpen || *in.Status == store.StatusClosed) {
		p.Status = *in.Status
	}
	if in.Slug != nil {
		newSlug := slugify(*in.Slug)
		if newSlug != "" && newSlug != p.Slug {
			p.Slug = s.uniqueSlug(newSlug)
		}
	}

	if err := s.st.UpdateProject(p); err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to update project")
		return
	}
	n, _ := s.st.CountSubmissions(p.ID)
	writeJSON(w, http.StatusOK, ownerProjectView{Project: p, SubmissionCount: n})
}

func (s *Server) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := s.st.GetProject(id); errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "project not found")
		return
	}
	if err := s.st.DeleteProject(id); err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to delete project")
		return
	}
	// Best-effort removal of the project's audio directory.
	_ = s.audio.RemoveProject(id)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleAdminVersion(w http.ResponseWriter, r *http.Request) {
	info := version.Info()
	info["update_channel"] = s.cfg.Update.Channel
	info["update_repo"] = s.cfg.Update.Repo
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "version": info})
}

func (s *Server) handleAdminUpdateStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"status": s.updater.Status(),
	})
}

func (s *Server) handleAdminUpdateCheck(w http.ResponseWriter, r *http.Request) {
	result, err := s.updater.CheckOnly(r.Context())
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":     false,
			"result": result,
			"error":  err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "result": result})
}

func (s *Server) handleAdminUpdateApply(w http.ResponseWriter, r *http.Request) {
	status := s.updater.Status()
	if status.State == "ready" {
		if err := s.updater.ApplyPending(r.Context()); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "applying"})
		return
	}
	if status.State == "checking" || status.State == "downloading" || status.State == "applying" {
		writeErr(w, http.StatusConflict, "update already in progress")
		return
	}
	s.updater.StartUpdate(r.Context())
	writeJSON(w, http.StatusOK, map[string]string{"status": "update_started"})
}

func (s *Server) handleAdminUpdateDismiss(w http.ResponseWriter, r *http.Request) {
	s.updater.DismissPending()
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
