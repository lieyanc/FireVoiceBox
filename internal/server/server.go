// Package server wires together the HTTP API and the embedded React frontend.
package server

import (
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"path"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/lieyan666/firevoicebox/internal/audio"
	"github.com/lieyan666/firevoicebox/internal/config"
	"github.com/lieyan666/firevoicebox/internal/store"
	"github.com/lieyan666/firevoicebox/internal/updater"
)

// Server holds shared dependencies for HTTP handlers.
type Server struct {
	cfg     *config.Config
	st      *store.Store
	audio   *audio.Storer
	dist    fs.FS // embedded frontend (the "dist" subtree)
	updater *updater.Updater
}

// New constructs a Server.
func New(cfg *config.Config, st *store.Store, au *audio.Storer, dist fs.FS) *Server {
	s := &Server{cfg: cfg, st: st, audio: au, dist: dist}
	s.updater = updater.New(
		func() updater.Config { return cfg.Update },
		func() string { return cfg.Server.DataDir },
		log.Default(),
		updater.RestartHooks{},
	)
	return s
}

// SetUpdater replaces the default updater. The main program uses this to
// install restart hooks that gracefully stop HTTP and close the database.
func (s *Server) SetUpdater(u *updater.Updater) {
	if u != nil {
		s.updater = u
	}
}

// Handler builds the root http.Handler with all routes mounted.
func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	// Note: client IP is resolved via clientIP() which honours the
	// trusted_proxy config, so we deliberately do not use middleware.RealIP
	// (which would unconditionally trust X-Forwarded-For).
	r.Use(middleware.Recoverer)
	r.Use(middleware.Logger)

	r.Route("/api", func(api chi.Router) {
		// Public endpoints (no auth).
		api.Get("/p/{key}", s.handlePublicProject)
		api.Post("/p/{key}/submissions", s.handleCreateSubmission)

		// Owner authentication.
		api.Post("/admin/login", s.handleLogin)
		api.Post("/admin/logout", s.handleLogout)
		api.Get("/admin/me", s.handleMe)

		// Owner-only project management.
		api.Group(func(o chi.Router) {
			o.Use(s.requireOwner)
			o.Get("/admin/projects", s.handleListProjects)
			o.Post("/admin/projects", s.handleCreateProject)
			o.Patch("/admin/projects/{id}", s.handleUpdateProject)
			o.Delete("/admin/projects/{id}", s.handleDeleteProject)
			o.Get("/admin/version", s.handleAdminVersion)
			o.Get("/admin/update/status", s.handleAdminUpdateStatus)
			o.Post("/admin/update/check", s.handleAdminUpdateCheck)
			o.Post("/admin/update/apply", s.handleAdminUpdateApply)
			o.Post("/admin/update/dismiss", s.handleAdminUpdateDismiss)
		})

		// Project management (owner OR valid manage token).
		api.Get("/manage/projects/{id}", s.handleManageProject)
		api.Get("/manage/projects/{id}/submissions", s.handleManageSubmissions)
		api.Get("/manage/projects/{id}/export", s.handleExport)
		api.Get("/manage/submissions/{id}/audio", s.handleSubmissionAudio)
		api.Delete("/manage/submissions/{id}", s.handleDeleteSubmission)
	})

	// Everything else: serve the embedded SPA.
	r.NotFound(s.serveSPA)
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	})

	return r
}

// serveSPA serves static frontend assets, falling back to index.html so that
// client-side routes (e.g. /r/:id, /admin) resolve correctly.
func (s *Server) serveSPA(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	p := strings.TrimPrefix(r.URL.Path, "/")
	if p == "" {
		p = "index.html"
	}
	if f, err := s.dist.Open(p); err == nil {
		f.Close()
		http.FileServer(http.FS(s.dist)).ServeHTTP(w, r)
		return
	}
	// A request that looks like a static asset (has a file extension) but does
	// not exist should 404 rather than fall back to index.html — otherwise a
	// stale/cache-busted chunk request would receive HTML with a 200 status.
	if path.Ext(p) != "" {
		http.NotFound(w, r)
		return
	}
	// Fallback to index.html for SPA client-side routes.
	index, err := s.dist.Open("index.html")
	if err != nil {
		http.Error(w, "frontend not built", http.StatusNotFound)
		return
	}
	defer index.Close()
	data, err := fs.ReadFile(s.dist, "index.html")
	if err != nil {
		http.Error(w, "frontend not built", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// --- small JSON helpers ---

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v != nil {
		if err := json.NewEncoder(w).Encode(v); err != nil {
			log.Printf("server: encode response: %v", err)
		}
	}
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
