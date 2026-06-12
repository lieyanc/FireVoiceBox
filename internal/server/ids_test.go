package server

import "testing"

func TestNewIDFormat(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		id := newID(10)
		if len(id) != 10 {
			t.Fatalf("len=%d want 10 (%q)", len(id), id)
		}
		for _, c := range id {
			if !((c >= 'a' && c <= 'z') || (c >= '2' && c <= '7')) {
				t.Fatalf("unexpected char %q in %q", c, id)
			}
		}
		if seen[id] {
			t.Fatalf("collision on %q", id)
		}
		seen[id] = true
	}
}

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Hello World":      "hello-world",
		"  trim  me  ":     "trim-me",
		"Grad 2026!!":      "grad-2026",
		"under_score-dash": "under-score-dash",
		"多个   空格":          "",
		"a---b":            "a-b",
	}
	for in, want := range cases {
		if got := slugify(in); got != want {
			t.Errorf("slugify(%q)=%q want %q", in, got, want)
		}
	}
}
