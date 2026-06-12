package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strings"
	"time"
)

const sessionCookie = "fvb_session"

// signValue returns "value.signature" where signature is HMAC-SHA256 of value.
func (s *Server) signValue(value string) string {
	mac := hmac.New(sha256.New, []byte(s.cfg.Server.Secret))
	mac.Write([]byte(value))
	return value + "." + hex.EncodeToString(mac.Sum(nil))
}

// verifyValue validates a signed value and returns the payload.
func (s *Server) verifyValue(signed string) (string, bool) {
	i := strings.LastIndexByte(signed, '.')
	if i < 0 {
		return "", false
	}
	value, sig := signed[:i], signed[i+1:]
	expected := s.signValue(value)
	if subtle.ConstantTimeCompare([]byte(signed), []byte(expected)) == 1 {
		_ = sig
		return value, true
	}
	return "", false
}

// setOwnerSession issues a signed owner cookie.
func (s *Server) setOwnerSession(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    s.signValue("owner"),
		Path:     "/",
		HttpOnly: true,
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(7 * 24 * time.Hour / time.Second),
	})
}

// clearOwnerSession removes the owner cookie.
func (s *Server) clearOwnerSession(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
}

// isOwner reports whether the request carries a valid owner session cookie.
func (s *Server) isOwner(r *http.Request) bool {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return false
	}
	val, ok := s.verifyValue(c.Value)
	return ok && val == "owner"
}

func isHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

// constantEqual compares two strings in constant time.
func constantEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
