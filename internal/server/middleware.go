package server

import (
	"net"
	"net/http"
	"strings"

	"github.com/lieyan666/firevoicebox/internal/store"
)

// clientIP extracts the client IP, honouring X-Forwarded-For / X-Real-IP only
// when trusted_proxy is enabled (otherwise those headers are attacker-controlled
// and would let clients bypass per-IP limits).
func (s *Server) clientIP(r *http.Request) string {
	if s.cfg.Server.TrustedProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			// First entry is the original client.
			parts := strings.Split(xff, ",")
			if ip := strings.TrimSpace(parts[0]); ip != "" {
				return ip
			}
		}
		if xr := strings.TrimSpace(r.Header.Get("X-Real-IP")); xr != "" {
			return xr
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// requireOwner is middleware that rejects requests without a valid owner session.
func (s *Server) requireOwner(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.isOwner(r) {
			writeErr(w, http.StatusUnauthorized, "authentication required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// hasProjectAccess reports whether the request may manage the given project,
// either as the owner or by presenting the project's manage token (via the
// X-Manage-Token header or ?key= query parameter).
func (s *Server) hasProjectAccess(r *http.Request, p *store.Project) bool {
	if s.isOwner(r) {
		return true
	}
	token := r.Header.Get("X-Manage-Token")
	if token == "" {
		token = r.URL.Query().Get("key")
	}
	return token != "" && constantEqual(token, p.ManageToken)
}
