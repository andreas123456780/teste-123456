package main

// Internal cron-style endpoints.
//
// /api/internal/jobs/process-labels is the fallback path that retries
// label generation for paid orders that didn't get a tracking_code on
// the first try (typically because the synchronous webhook hit a
// SuperFrete timeout or the function was killed before runLabelJob
// finished). Vercel Cron + Fly.io both support hitting an HTTP
// endpoint on a schedule, so we expose this as plain HTTP guarded by a
// shared secret rather than tying ourselves to a specific scheduler.
//
// Auth: callers must present `Authorization: Bearer <secret>` where
// the secret matches INTERNAL_JOB_TOKEN. Vercel Cron sends exactly
// this header when CRON_SECRET is configured, so the same value works
// for both managed and self-hosted setups.

import (
	"context"
	"crypto/subtle"
	"errors"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type internalJobsConfig struct {
	token string
}

func loadInternalJobsConfig() internalJobsConfig {
	// Prefer INTERNAL_JOB_TOKEN; fall back to Vercel's CRON_SECRET so
	// the default Vercel Cron header (Authorization: Bearer
	// $CRON_SECRET) authenticates without extra config.
	tok := strings.TrimSpace(os.Getenv("INTERNAL_JOB_TOKEN"))
	if tok == "" {
		tok = strings.TrimSpace(os.Getenv("CRON_SECRET"))
	}
	return internalJobsConfig{token: tok}
}

// requireInternalJobAuth verifies the bearer token in constant time.
// Returns (true) when authorised; writes the 401/503 response and
// returns (false) otherwise.
func requireInternalJobAuth(cfg internalJobsConfig, w http.ResponseWriter, r *http.Request) bool {
	if cfg.token == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "internal jobs disabled"})
		return false
	}
	got := strings.TrimSpace(r.Header.Get("Authorization"))
	const prefix = "Bearer "
	if !strings.HasPrefix(got, prefix) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return false
	}
	got = strings.TrimSpace(strings.TrimPrefix(got, prefix))
	if subtle.ConstantTimeCompare([]byte(got), []byte(cfg.token)) != 1 {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return false
	}
	return true
}

// processLabelsResult is the JSON payload returned by the cron
// endpoint. Tally numbers help operators read Vercel Cron logs at a
// glance.
type processLabelsResult struct {
	Scanned   int      `json:"scanned"`
	Succeeded int      `json:"succeeded"`
	Failed    int      `json:"failed"`
	Skipped   int      `json:"skipped"`
	OrderIDs  []string `json:"orderIds"`
	Errors    []string `json:"errors,omitempty"`
}

// handleProcessLabelsJob walks the queue of paid orders without a
// tracking_code and tries to generate a label for each, with backoff
// between attempts. It is safe to call concurrently — runLabelJob is
// idempotent and the DB serialises the writes.
func handleProcessLabelsJob(cfg internalJobsConfig, orders *orderStore, ship *shippingClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		if !requireInternalJobAuth(cfg, w, r) {
			return
		}
		if ship == nil || ship.cfg.AccessToken == "" {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "shipping unavailable"})
			return
		}

		limit := 5
		if v := strings.TrimSpace(r.URL.Query().Get("limit")); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 50 {
				limit = n
			}
		}
		perOrder := labelTimeoutFromEnv()

		// Pick the smallest backoff window so freshly-attempted rows
		// stay on cooldown. Older rows naturally pass the cutoff and
		// get retried.
		cutoff := time.Now().UTC().Add(-labelBackoff(1))

		ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
		defer cancel()

		pending, err := orders.listPendingLabels(ctx, cutoff, maxLabelAttempts, limit)
		if err != nil {
			log.Printf("internal_jobs.process-labels list: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "list failed"})
			return
		}

		out := processLabelsResult{Scanned: len(pending)}
		for _, o := range pending {
			delay := labelBackoff(o.TrackingAttempts)
			if !o.TrackingAttemptedAt.IsZero() && time.Since(o.TrackingAttemptedAt) < delay {
				out.Skipped++
				continue
			}
			oneCtx, oneCancel := context.WithTimeout(ctx, perOrder)
			err := runLabelJob(oneCtx, orders, ship, o.ID)
			oneCancel()
			if err != nil {
				out.Failed++
				out.Errors = append(out.Errors, o.ID+": "+err.Error())
				continue
			}
			out.Succeeded++
			out.OrderIDs = append(out.OrderIDs, o.ID)
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// labelTimeoutFromEnv reads LABEL_PROCESSING_TIMEOUT (Go duration,
// e.g. "25s", "60s") and falls back to a Stripe-budget-friendly
// default. Exposed as a function so main.go and the cron path use
// the same value.
func labelTimeoutFromEnv() time.Duration {
	const def = 25 * time.Second
	raw := strings.TrimSpace(os.Getenv("LABEL_PROCESSING_TIMEOUT"))
	if raw == "" {
		return def
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		log.Printf("internal_jobs: invalid LABEL_PROCESSING_TIMEOUT=%q (%v) — using default %s", raw, err, def)
		return def
	}
	return d
}

// errMissingShipping is exported to admin handlers via the shared
// SuperFrete client; surfaced here so admin_http.go can reuse it
// without importing test internals.
var errMissingShipping = errors.New("shipping unavailable")
