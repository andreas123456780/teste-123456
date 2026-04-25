package main

// ViaCEP resolver: turn a Brazilian CEP (zip code) into district/city/
// state metadata. SuperFrete requires all three separately, but the
// checkout form historically captured only a single free-text address
// line plus a CEP. Before handing the payload to SuperFrete we query
// https://viacep.com.br/ws/{cep}/json/ to fill in the blanks.
//
// ViaCEP is free, unauthenticated, and returns JSON. We keep timeouts
// short (3s) so a flaky lookup can't eat the webhook budget; if it
// fails the caller falls back to whatever is stored on the order.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type viacepAddress struct {
	Cep        string `json:"cep"`
	Logradouro string `json:"logradouro"`
	Bairro     string `json:"bairro"`
	Localidade string `json:"localidade"`
	Uf         string `json:"uf"`
	Erro       any    `json:"erro"`
}

// addressDetails is the shape runLabelJob needs to populate the
// SuperFrete "to" object.
type addressDetails struct {
	District string
	City     string
	State    string
}

// cepLookupResult is returned by the /api/cep/{cep} endpoint. The
// checkout form uses it to auto-fill the address fields in one shot
// after the customer types a CEP.
type cepLookupResult struct {
	Cep        string `json:"cep"`
	Logradouro string `json:"logradouro"`
	Bairro     string `json:"bairro"`
	Cidade     string `json:"cidade"`
	Uf         string `json:"uf"`
}

type viaCepClient struct {
	http    *http.Client
	baseURL string
}

func newViaCepClient() *viaCepClient {
	return &viaCepClient{
		http:    &http.Client{Timeout: 3 * time.Second},
		baseURL: "https://viacep.com.br",
	}
}

// Lookup queries ViaCEP for the supplied zip. It returns a not-found
// sentinel on invalid or unknown CEPs so callers can surface a clear
// error to operators. Transient HTTP failures are returned verbatim.
var errViaCepNotFound = errors.New("viacep: CEP not found")

func (c *viaCepClient) Lookup(ctx context.Context, cep string) (addressDetails, error) {
	cep = digitsOnly(cep)
	if len(cep) != 8 {
		return addressDetails{}, errors.New("viacep: invalid CEP")
	}
	if c == nil {
		return addressDetails{}, errors.New("viacep: client not initialised")
	}
	url := fmt.Sprintf("%s/ws/%s/json/", strings.TrimRight(c.baseURL, "/"), cep)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return addressDetails{}, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return addressDetails{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return addressDetails{}, fmt.Errorf("viacep: %d %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var v viacepAddress
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return addressDetails{}, fmt.Errorf("viacep: decode: %w", err)
	}
	// ViaCEP signals "not found" with a body like {"erro": true} or
	// {"erro": "true"}. Either way treat it as missing.
	if v.Erro != nil {
		switch e := v.Erro.(type) {
		case bool:
			if e {
				return addressDetails{}, errViaCepNotFound
			}
		case string:
			if strings.EqualFold(e, "true") {
				return addressDetails{}, errViaCepNotFound
			}
		}
	}
	out := addressDetails{
		District: strings.TrimSpace(v.Bairro),
		City:     strings.TrimSpace(v.Localidade),
		State:    strings.ToUpper(strings.TrimSpace(v.Uf)),
	}
	if out.City == "" && out.State == "" && out.District == "" {
		return addressDetails{}, errViaCepNotFound
	}
	return out, nil
}

// LookupFull is a superset of Lookup that also returns the logradouro
// (street name). The checkout form needs it to pre-fill the address
// field after a CEP is typed; label generation only cares about the
// three Lookup() returns, so the cheaper call stays available for
// backend jobs.
func (c *viaCepClient) LookupFull(ctx context.Context, cep string) (cepLookupResult, error) {
	cep = digitsOnly(cep)
	if len(cep) != 8 {
		return cepLookupResult{}, errors.New("viacep: invalid CEP")
	}
	if c == nil {
		return cepLookupResult{}, errors.New("viacep: client not initialised")
	}
	url := fmt.Sprintf("%s/ws/%s/json/", strings.TrimRight(c.baseURL, "/"), cep)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return cepLookupResult{}, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return cepLookupResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return cepLookupResult{}, fmt.Errorf("viacep: %d %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var v viacepAddress
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return cepLookupResult{}, fmt.Errorf("viacep: decode: %w", err)
	}
	if v.Erro != nil {
		switch e := v.Erro.(type) {
		case bool:
			if e {
				return cepLookupResult{}, errViaCepNotFound
			}
		case string:
			if strings.EqualFold(e, "true") {
				return cepLookupResult{}, errViaCepNotFound
			}
		}
	}
	out := cepLookupResult{
		Cep:        cep,
		Logradouro: strings.TrimSpace(v.Logradouro),
		Bairro:     strings.TrimSpace(v.Bairro),
		Cidade:     strings.TrimSpace(v.Localidade),
		Uf:         strings.ToUpper(strings.TrimSpace(v.Uf)),
	}
	if out.Cidade == "" && out.Uf == "" && out.Bairro == "" {
		return cepLookupResult{}, errViaCepNotFound
	}
	return out, nil
}

// handleCepLookup exposes ViaCEP through our own origin so the
// browser doesn't have to deal with CORS. The zip is parsed from the
// last path segment — we never accept query params because a naive
// proxy is a classic SSRF vector. Digits are extracted defensively
// even though only /api/cep/{8-digit-string} should reach here.
func handleCepLookup(client *viaCepClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		const prefix = "/api/cep/"
		cep := strings.TrimPrefix(r.URL.Path, prefix)
		cep = digitsOnly(cep)
		if len(cep) != 8 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "CEP inválido"})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		res, err := client.LookupFull(ctx, cep)
		if errors.Is(err, errViaCepNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "CEP não encontrado"})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "lookup failed"})
			return
		}
		// Browsers will cache this for the life of the form; the
		// server side cache is unnecessary because ViaCEP is fast.
		w.Header().Set("Cache-Control", "public, max-age=86400")
		writeJSON(w, http.StatusOK, res)
	}
}
