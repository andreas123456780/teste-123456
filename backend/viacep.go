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
