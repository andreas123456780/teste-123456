package main

import (
	"strings"
	"testing"
	"time"
)

func TestOrderToken_Roundtrip(t *testing.T) {
	key := []byte("test-secret")
	now := time.Now()
	tok := makeOrderToken(key, "ord_abc", now)
	if !strings.Contains(tok, ".") {
		t.Fatalf("token has no separator: %q", tok)
	}
	id, err := parseOrderToken(key, tok, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if id != "ord_abc" {
		t.Fatalf("got %q, want ord_abc", id)
	}
}

func TestOrderToken_WrongKey(t *testing.T) {
	tok := makeOrderToken([]byte("a"), "ord_abc", time.Now())
	if _, err := parseOrderToken([]byte("b"), tok, time.Now()); err == nil {
		t.Fatal("expected error when key differs")
	}
}

func TestOrderToken_Tampered(t *testing.T) {
	key := []byte("k")
	tok := makeOrderToken(key, "ord_abc", time.Now())
	// Flip a single character in the signature portion.
	parts := strings.SplitN(tok, ".", 2)
	if len(parts) != 2 || len(parts[1]) == 0 {
		t.Fatalf("unexpected token shape: %q", tok)
	}
	var b byte = 'a'
	if parts[1][0] == 'a' {
		b = 'b'
	}
	tampered := parts[0] + "." + string(b) + parts[1][1:]
	if _, err := parseOrderToken(key, tampered, time.Now()); err == nil {
		t.Fatal("expected signature mismatch")
	}
}

func TestOrderToken_Expired(t *testing.T) {
	key := []byte("k")
	tok := makeOrderToken(key, "ord_abc", time.Now().Add(-365*24*time.Hour))
	if _, err := parseOrderToken(key, tok, time.Now()); err == nil {
		t.Fatal("expected expired error")
	}
}

func TestOrderToken_Malformed(t *testing.T) {
	if _, err := parseOrderToken([]byte("k"), "nope", time.Now()); err == nil {
		t.Fatal("expected malformed error")
	}
}

func TestOrderToken_NoKey(t *testing.T) {
	if _, err := parseOrderToken(nil, "a.b", time.Now()); err == nil {
		t.Fatal("expected no-key error")
	}
}

func TestMaskEmail(t *testing.T) {
	cases := map[string]string{
		"a@b.com":                    "•@b.com",
		"ab@b.com":                   "••@b.com",
		"andreas@example.com":        "a•••••s@example.com",
		"x":                          "",
	}
	for in, want := range cases {
		if got := maskEmail(in); got != want {
			t.Errorf("maskEmail(%q)=%q want %q", in, got, want)
		}
	}
}
