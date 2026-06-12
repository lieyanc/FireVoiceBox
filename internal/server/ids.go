package server

import (
	"crypto/rand"
	"encoding/base32"
	"strings"
)

var idEncoding = base32.NewEncoding("abcdefghijklmnopqrstuvwxyz234567").WithPadding(base32.NoPadding)

// newID returns a URL-safe random identifier of roughly nChars lowercase
// base32 characters.
func newID(nChars int) string {
	nBytes := (nChars*5 + 7) / 8
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		panic("server: failed to read random bytes: " + err.Error())
	}
	s := idEncoding.EncodeToString(b)
	if len(s) > nChars {
		s = s[:nChars]
	}
	return s
}

// slugify converts arbitrary text into a URL-friendly slug.
func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		case r == '-' || r == ' ' || r == '_':
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	return out
}
