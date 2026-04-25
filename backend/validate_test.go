package main

// Unit tests for the checkout-form validators added alongside the
// "collect CPF and split address" change. These run pure string logic
// and do not touch the DB.

import "testing"

func TestValidateFullName(t *testing.T) {
	cases := []struct {
		in   string
		ok   bool
		want string
	}{
		{"Andreas Assinaturas", true, "Andreas Assinaturas"},
		{"  andreas   assinaturas  ", true, "andreas assinaturas"},
		{"João da Silva Santos", true, "João da Silva Santos"},
		{"Joao", false, ""},            // single word
		{"", false, ""},                // empty
		{"A B", false, ""},             // surname too short
		{"Andreas  X", false, ""},      // second word too short
	}
	for _, c := range cases {
		got, ok := validateFullName(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("validateFullName(%q) = (%q, %v); want (%q, %v)",
				c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestValidateCPForCNPJ(t *testing.T) {
	cases := []struct {
		in   string
		ok   bool
		want string
	}{
		{"123.456.789-09", true, "12345678909"},
		{"12345678909", true, "12345678909"},
		{"12.345.678/0001-95", true, "12345678000195"},
		{"12345678000195", true, "12345678000195"},
		{"00000000000", false, ""}, // all-zeros rejected
		{"11111111111", false, ""}, // all-ones rejected
		{"123", false, ""},         // too short
		{"abcd", false, ""},        // no digits
		{"", false, ""},
	}
	for _, c := range cases {
		got, ok := validateCPForCNPJ(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("validateCPForCNPJ(%q) = (%q, %v); want (%q, %v)",
				c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestValidateUF(t *testing.T) {
	if got, ok := validateUF("sp"); !ok || got != "SP" {
		t.Errorf("validateUF(sp) = (%q, %v)", got, ok)
	}
	if got, ok := validateUF(" rj "); !ok || got != "RJ" {
		t.Errorf("validateUF( rj ) = (%q, %v)", got, ok)
	}
	if _, ok := validateUF("xx"); ok {
		t.Error("validateUF(xx) should fail")
	}
	if _, ok := validateUF(""); ok {
		t.Error("validateUF(empty) should fail")
	}
	if _, ok := validateUF("saopaulo"); ok {
		t.Error("validateUF(long) should fail")
	}
}
