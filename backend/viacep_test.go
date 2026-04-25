package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newFakeViaCep returns a viaCepClient wired to a test HTTP server that
// serves a single deterministic reply. Tests that exercise
// runLabelJob / the admin endpoints use this so they never hit the
// real viacep.com.br endpoint.
func newFakeViaCep(t *testing.T, reply addressDetails) *viaCepClient {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"cep":        "08503000",
			"bairro":     reply.District,
			"localidade": reply.City,
			"uf":         reply.State,
		})
	}))
	t.Cleanup(srv.Close)
	return &viaCepClient{http: srv.Client(), baseURL: srv.URL}
}

// newFakeViaCepFailing returns a client whose single endpoint always
// returns {"erro": true}, simulating a CEP that ViaCEP cannot resolve.
func newFakeViaCepFailing(t *testing.T) *viaCepClient {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"erro": true}`))
	}))
	t.Cleanup(srv.Close)
	return &viaCepClient{http: srv.Client(), baseURL: srv.URL}
}

// defaultFakeViaCep is the value we inject in happy-path tests. Bairro
// / Cidade / UF chosen to look like a recognisable São Paulo address.
func defaultFakeViaCep(t *testing.T) *viaCepClient {
	return newFakeViaCep(t, addressDetails{
		District: "Centro",
		City:     "São Paulo",
		State:    "SP",
	})
}

func TestViaCep_HappyPath(t *testing.T) {
	c := newFakeViaCep(t, addressDetails{District: "Perdizes", City: "São Paulo", State: "SP"})
	out, err := c.Lookup(context.Background(), "05019-000")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if out.District != "Perdizes" || out.City != "São Paulo" || out.State != "SP" {
		t.Fatalf("unexpected addressDetails: %+v", out)
	}
}

func TestViaCep_InvalidCep(t *testing.T) {
	c := newFakeViaCep(t, addressDetails{})
	if _, err := c.Lookup(context.Background(), "123"); err == nil {
		t.Fatal("expected error for short CEP")
	}
}

func TestViaCep_NotFound(t *testing.T) {
	c := newFakeViaCepFailing(t)
	_, err := c.Lookup(context.Background(), "99999999")
	if err != errViaCepNotFound {
		t.Fatalf("expected errViaCepNotFound, got %v", err)
	}
}

func TestViaCep_NormalisesState(t *testing.T) {
	c := newFakeViaCep(t, addressDetails{District: "Sé", City: "São Paulo", State: "sp"})
	out, _ := c.Lookup(context.Background(), "01001-000")
	if out.State != "SP" {
		t.Fatalf("state = %q, want SP (uppercase)", out.State)
	}
}

func TestResolveAddressDetails_UsesOverridesWhenComplete(t *testing.T) {
	// With all three fields pre-filled, no ViaCEP call should happen.
	c := newFakeViaCepFailing(t) // would fail if called
	got, err := resolveAddressDetails(context.Background(), c, "08503-000", addressDetails{
		District: "Centro", City: "Suzano", State: "SP",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.City != "Suzano" {
		t.Fatalf("overrides not preferred: %+v", got)
	}
}

func TestResolveAddressDetails_ViaCepFillsBlanks(t *testing.T) {
	c := newFakeViaCep(t, addressDetails{District: "Jardins", City: "São Paulo", State: "SP"})
	got, err := resolveAddressDetails(context.Background(), c, "01001-000", addressDetails{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.District != "Jardins" || got.City != "São Paulo" || got.State != "SP" {
		t.Fatalf("viacep did not populate: %+v", got)
	}
}

func TestResolveAddressDetails_ViaCepFailureSurfacedWhenNoOverride(t *testing.T) {
	c := newFakeViaCepFailing(t)
	_, err := resolveAddressDetails(context.Background(), c, "99999999", addressDetails{})
	if err == nil {
		t.Fatal("expected error when ViaCEP fails and no overrides given")
	}
}

func TestResolveAddressDetails_PartialOverrideWithCityFallsThroughOnFailure(t *testing.T) {
	c := newFakeViaCepFailing(t)
	got, err := resolveAddressDetails(context.Background(), c, "99999999", addressDetails{
		City: "Suzano", State: "SP",
	})
	if err != nil {
		t.Fatalf("expected tolerance when city+state overridden, got %v", err)
	}
	if got.City != "Suzano" || got.State != "SP" {
		t.Fatalf("overrides lost: %+v", got)
	}
}
