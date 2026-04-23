package main

import "testing"

func TestParseOrigins_ExactAndWildcard(t *testing.T) {
	raw := "https://nast.com.br, https://*.vercel.app, http://localhost:5173"
	got := parseOrigins(raw)
	if len(got) != 3 {
		t.Fatalf("want 3 matchers, got %d", len(got))
	}
	cases := map[string]bool{
		"https://nast.com.br":                     true,
		"https://preview-42-nast.vercel.app":      true,
		"https://something.vercel.app":            true,
		"http://localhost:5173":                   true,
		"http://localhost:5174":                   false, // wrong port
		"https://evil.com":                        false,
		"https://foovercel.app":                   false, // no leading dot
		"https://nast.vercel.app.attacker.com":    false, // suffix misuse
	}
	for origin, want := range cases {
		if originAllowed(got, origin) != want {
			t.Fatalf("originAllowed(%q) = %v, want %v", origin, !want, want)
		}
	}
}

func TestOriginMatcher_EmptyOrigin(t *testing.T) {
	ms := parseOrigins("https://nast.com.br")
	if originAllowed(ms, "") {
		t.Fatal("empty origin should never be allowed")
	}
}
