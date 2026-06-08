package main

// Site settings: a small key/value store backing operator-tunable
// frontend behaviour that doesn't fit the product catalog. Today the
// only consumer is the next-drop countdown, but the table is generic
// so future toggles (announcement bar, banner copy, hero CTA) can
// piggyback without another migration.
//
// Public access is read-only and limited to an allowlist of keys (see
// publicSettingKeys) so we never accidentally leak admin-only state.
// Writes always go through the admin-protected handler.

import (
        "context"
        "database/sql"
        "encoding/json"
        "errors"
        "fmt"
        "net/http"
        "strings"
        "time"
)

// settingsStore is intentionally tiny. The schema is just (key, value,
// updated_at) and the value is always a JSON-encoded blob — even for
// scalar booleans — so the public read endpoint can stream payloads
// straight back to the browser without per-key marshalling.
type settingsStore struct {
        db *sql.DB
}

func newSettingsStore(db *sql.DB) *settingsStore {
        return &settingsStore{db: db}
}

// publicSettingKeys is the allowlist of keys exposed by the public
// GET /api/settings endpoint. Anything not in this set is admin-only.
var publicSettingKeys = map[string]bool{
        "countdown": true,
        "secret":    true,
}

// CountdownSettings is the structured payload behind the "countdown"
// key. Visible=false hides the section entirely; TargetAt is an RFC3339
// timestamp; CtaURL is optional (when blank the button hides).
type CountdownSettings struct {
        Visible    bool   `json:"visible"`
        Title      string `json:"title"`
        Subtitle   string `json:"subtitle"`
        TargetAt   string `json:"targetAt"`
        CtaLabel   string `json:"ctaLabel"`
        CtaURL     string `json:"ctaUrl"`
        EndedLabel string `json:"endedLabel"`
}

// defaultCountdown is what the public endpoint returns when no admin
// override has been saved yet. Visible=false keeps the section hidden
// until the operator opts in.
func defaultCountdown() CountdownSettings {
        return CountdownSettings{
                Visible:    false,
                Title:      "PRÓXIMO DROP",
                Subtitle:   "Edição limitada — peças numeradas",
                TargetAt:   "",
                CtaLabel:   "Avise-me",
                CtaURL:     "",
                EndedLabel: "Drop liberado",
        }
}

// get returns the raw JSON for a key, or empty string if it has never
// been written. Caller is responsible for unmarshalling into the right
// shape.
func (s *settingsStore) get(ctx context.Context, key string) (string, error) {
        row := s.db.QueryRowContext(ctx, rb(`SELECT value FROM site_settings WHERE key = ?`), key)
        var raw string
        if err := row.Scan(&raw); err != nil {
                if errors.Is(err, sql.ErrNoRows) {
                        return "", nil
                }
                return "", fmt.Errorf("get setting: %w", err)
        }
        return raw, nil
}

// set upserts the JSON-encoded value for a key.
func (s *settingsStore) set(ctx context.Context, key, value string) error {
        now := time.Now().UTC()
        _, err := s.db.ExecContext(ctx, rb(`INSERT INTO site_settings(key, value, updated_at)
                VALUES (?, ?, ?)
                ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at`),
                key, value, now)
        if err != nil {
                return fmt.Errorf("set setting: %w", err)
        }
        return nil
}

func (s *settingsStore) getCountdown(ctx context.Context) (CountdownSettings, error) {
        raw, err := s.get(ctx, "countdown")
        if err != nil {
                return CountdownSettings{}, err
        }
        out := defaultCountdown()
        if raw == "" {
                return out, nil
        }
        if err := json.Unmarshal([]byte(raw), &out); err != nil {
                // Corrupted row — fall back to defaults rather than 500.
                return defaultCountdown(), nil
        }
        return out, nil
}

func (s *settingsStore) setCountdown(ctx context.Context, c CountdownSettings) error {
        c = sanitizeCountdown(c)
        buf, err := json.Marshal(c)
        if err != nil {
                return fmt.Errorf("marshal countdown: %w", err)
        }
        return s.set(ctx, "countdown", string(buf))
}

// sanitizeCountdown trims whitespace, enforces bounds and validates
// the target timestamp (so the storefront can blindly trust the
// response). Returns a copy with normalized fields.
func sanitizeCountdown(c CountdownSettings) CountdownSettings {
        c.Title = clampString(strings.TrimSpace(c.Title), 80)
        c.Subtitle = clampString(strings.TrimSpace(c.Subtitle), 160)
        c.CtaLabel = clampString(strings.TrimSpace(c.CtaLabel), 40)
        c.CtaURL = clampString(strings.TrimSpace(c.CtaURL), 400)
        c.EndedLabel = clampString(strings.TrimSpace(c.EndedLabel), 80)
        c.TargetAt = strings.TrimSpace(c.TargetAt)
        if c.TargetAt != "" {
                if t, err := time.Parse(time.RFC3339, c.TargetAt); err == nil {
                        c.TargetAt = t.UTC().Format(time.RFC3339)
                } else {
                        // Invalid timestamp — drop it so the frontend treats the
                        // section as "ended" gracefully instead of NaN.
                        c.TargetAt = ""
                }
        }
        if c.Title == "" {
                c.Title = "PRÓXIMO DROP"
        }
        if c.EndedLabel == "" {
                c.EndedLabel = "Drop liberado"
        }
        // CtaLabel only matters when CtaURL is set; clear both together
        // when URL is empty so the frontend has a single signal to gate
        // the button.
        if c.CtaURL == "" {
                c.CtaLabel = ""
        }
        return c
}

func clampString(s string, max int) string {
        if len(s) <= max {
                return s
        }
        return s[:max]
}

// ----- Secret product settings -----

// SecretSettings controls the storefront secret-product unlock panel.
// Locked=true means the product requires a code to reveal.
// Code is the unlock passphrase (compared case-insensitively by the client).
type SecretSettings struct {
        Locked bool   `json:"locked"`
        Code   string `json:"code"`
}

func defaultSecret() SecretSettings {
        return SecretSettings{Locked: true, Code: "10820"}
}

func (s *settingsStore) getSecret(ctx context.Context) (SecretSettings, error) {
        raw, err := s.get(ctx, "secret")
        if err != nil {
                return SecretSettings{}, err
        }
        out := defaultSecret()
        if raw == "" {
                return out, nil
        }
        if err := json.Unmarshal([]byte(raw), &out); err != nil {
                return defaultSecret(), nil
        }
        return out, nil
}

func (s *settingsStore) setSecret(ctx context.Context, sc SecretSettings) error {
        sc.Code = strings.TrimSpace(sc.Code)
        if sc.Code == "" {
                sc.Code = defaultSecret().Code
        }
        buf, err := json.Marshal(sc)
        if err != nil {
                return fmt.Errorf("marshal secret: %w", err)
        }
        return s.set(ctx, "secret", string(buf))
}

func handleAdminSecret(ctx context.Context, w http.ResponseWriter, r *http.Request, store *settingsStore) {
        switch r.Method {
        case http.MethodGet:
                sc, err := store.getSecret(ctx)
                if err != nil {
                        writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "load failed"})
                        return
                }
                writeJSON(w, http.StatusOK, sc)
        case http.MethodPut, http.MethodPost:
                var sc SecretSettings
                dec := json.NewDecoder(r.Body)
                dec.DisallowUnknownFields()
                if err := dec.Decode(&sc); err != nil {
                        writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
                        return
                }
                if err := store.setSecret(ctx, sc); err != nil {
                        writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "save failed"})
                        return
                }
                out, _ := store.getSecret(ctx)
                writeJSON(w, http.StatusOK, out)
        default:
                writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
        }
}

// ----- HTTP -----

// handlePublicSettings exposes the safe subset of settings keyed by
// `key=...` query param (returns the single payload). With no key it
// returns a {key: value} map for every public-allowlisted entry. This
// lets the storefront fetch all CMS-style state in one round trip.
func handlePublicSettings(store *settingsStore) http.HandlerFunc {
        return func(w http.ResponseWriter, r *http.Request) {
                if r.Method != http.MethodGet {
                        writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
                        return
                }
                ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
                defer cancel()
                key := strings.TrimSpace(r.URL.Query().Get("key"))
                if key != "" {
                        if !publicSettingKeys[key] {
                                writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown setting"})
                                return
                        }
                        payload, err := loadPublicSetting(ctx, store, key)
                        if err != nil {
                                writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "load failed"})
                                return
                        }
                        writeJSON(w, http.StatusOK, payload)
                        return
                }
                // No key — return all public entries in a flat map.
                out := map[string]any{}
                for k := range publicSettingKeys {
                        payload, err := loadPublicSetting(ctx, store, k)
                        if err != nil {
                                writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "load failed"})
                                return
                        }
                        out[k] = payload
                }
                writeJSON(w, http.StatusOK, out)
        }
}

func loadPublicSetting(ctx context.Context, store *settingsStore, key string) (any, error) {
        switch key {
        case "countdown":
                return store.getCountdown(ctx)
        case "secret":
                return store.getSecret(ctx)
        default:
                return nil, fmt.Errorf("unknown public key %q", key)
        }
}

// handleAdminSettings is the operator-side counterpart. GET reads the
// stored value (admin sees everything, public or not); PUT replaces
// it with the JSON body. Each settings key has its own schema, so the
// handler dispatches on `?key=...`.
func handleAdminSettings(store *settingsStore) http.HandlerFunc {
        return func(w http.ResponseWriter, r *http.Request) {
                ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
                defer cancel()
                key := strings.TrimSpace(r.URL.Query().Get("key"))
                if key == "" {
                        writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing key"})
                        return
                }
                switch key {
                case "countdown":
                        handleAdminCountdown(ctx, w, r, store)
                case "secret":
                        handleAdminSecret(ctx, w, r, store)
                default:
                        writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown setting"})
                }
        }
}

func handleAdminCountdown(ctx context.Context, w http.ResponseWriter, r *http.Request, store *settingsStore) {
        switch r.Method {
        case http.MethodGet:
                c, err := store.getCountdown(ctx)
                if err != nil {
                        writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "load failed"})
                        return
                }
                writeJSON(w, http.StatusOK, c)
        case http.MethodPut, http.MethodPost:
                var c CountdownSettings
                dec := json.NewDecoder(r.Body)
                dec.DisallowUnknownFields()
                if err := dec.Decode(&c); err != nil {
                        writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
                        return
                }
                if err := store.setCountdown(ctx, c); err != nil {
                        writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "save failed"})
                        return
                }
                // Echo the sanitized payload so the admin form syncs.
                out, _ := store.getCountdown(ctx)
                writeJSON(w, http.StatusOK, out)
        default:
                writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
        }
}
