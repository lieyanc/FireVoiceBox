package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lieyan666/firevoicebox/internal/config"
)

func testServer() *Server {
	cfg := &config.Config{}
	cfg.Server.Secret = "unit-test-secret"
	return &Server{cfg: cfg}
}

func TestSignVerifyRoundtrip(t *testing.T) {
	s := testServer()
	signed := s.signValue("owner")
	val, ok := s.verifyValue(signed)
	if !ok || val != "owner" {
		t.Fatalf("verify failed: ok=%v val=%q", ok, val)
	}

	// Tampered payload must fail.
	if _, ok := s.verifyValue("admin." + signed[len("owner")+1:]); ok {
		t.Error("tampered value should not verify")
	}
	if _, ok := s.verifyValue("garbage"); ok {
		t.Error("garbage should not verify")
	}

	// A different secret must not validate the cookie.
	other := testServer()
	other.cfg.Server.Secret = "different"
	if _, ok := other.verifyValue(signed); ok {
		t.Error("cookie signed with another secret should not verify")
	}
}

func TestClientIPTrustedProxy(t *testing.T) {
	s := testServer()

	makeReq := func() *http.Request {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = "203.0.113.7:5555"
		r.Header.Set("X-Forwarded-For", "9.9.9.9, 10.0.0.1")
		r.Header.Set("X-Real-IP", "8.8.8.8")
		return r
	}

	// Untrusted: ignore proxy headers, use RemoteAddr host.
	s.cfg.Server.TrustedProxy = false
	if got := s.clientIP(makeReq()); got != "203.0.113.7" {
		t.Errorf("untrusted clientIP=%q want 203.0.113.7", got)
	}

	// Trusted: first XFF entry wins.
	s.cfg.Server.TrustedProxy = true
	if got := s.clientIP(makeReq()); got != "9.9.9.9" {
		t.Errorf("trusted clientIP=%q want 9.9.9.9", got)
	}

	// Trusted but no XFF: fall back to X-Real-IP.
	s.cfg.Server.TrustedProxy = true
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "203.0.113.7:5555"
	r.Header.Set("X-Real-IP", "8.8.8.8")
	if got := s.clientIP(r); got != "8.8.8.8" {
		t.Errorf("clientIP=%q want 8.8.8.8", got)
	}
}
