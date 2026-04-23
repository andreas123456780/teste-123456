package main

// Order lookup tokens.
//
// Customers receive a URL like `/pedido/<token>` after checkout so they
// can look up their order status and tracking code without logging in.
// The token is a compact HMAC construction — we avoid exposing the raw
// `ord_...` id and avoid relying on a session cookie.
//
// Structure:   base64url("<orderID>.<timestamp>") + "." + base64url(hmac)
// Signature:   HMAC_SHA256(secret, "<orderID>.<timestamp>")
//
// Tokens don't carry any user data beyond the order id; leaking one gives
// the holder read access to that order's public-safe view (status,
// tracking, last4 of amount) — nothing more.

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	// orderTokenMaxAge caps how long a token is accepted. Orders live
	// forever, so in principle we could make this permanent — but if a
	// link ever leaks we'd rather it eventually stop working than be
	// valid in perpetuity.
	orderTokenMaxAge = 180 * 24 * time.Hour
)

// makeOrderToken builds a signed lookup token for orderID using secret.
// Returns an opaque string safe for URLs.
func makeOrderToken(secret []byte, orderID string, now time.Time) string {
	payload := orderID + "." + strconv.FormatInt(now.Unix(), 10)
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(payload))
	sig := mac.Sum(nil)

	enc := base64.RawURLEncoding
	return enc.EncodeToString([]byte(payload)) + "." + enc.EncodeToString(sig)
}

// parseOrderToken reverses makeOrderToken. Returns the orderID or a
// non-nil error if the token is malformed, expired, or signature-invalid.
func parseOrderToken(secret []byte, token string, now time.Time) (string, error) {
	if len(secret) == 0 {
		return "", errors.New("token key not configured")
	}
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return "", errors.New("malformed token")
	}
	enc := base64.RawURLEncoding
	payload, err := enc.DecodeString(parts[0])
	if err != nil {
		return "", fmt.Errorf("decode payload: %w", err)
	}
	sigBytes, err := enc.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("decode signature: %w", err)
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write(payload)
	expected := mac.Sum(nil)
	if subtle.ConstantTimeCompare(sigBytes, expected) != 1 {
		return "", errors.New("signature mismatch")
	}

	inner := strings.SplitN(string(payload), ".", 2)
	if len(inner) != 2 {
		return "", errors.New("malformed payload")
	}
	orderID := strings.TrimSpace(inner[0])
	if !strings.HasPrefix(orderID, "ord_") || len(orderID) > 64 {
		return "", errors.New("invalid order id in token")
	}
	ts, err := strconv.ParseInt(inner[1], 10, 64)
	if err != nil {
		return "", fmt.Errorf("invalid timestamp: %w", err)
	}
	if d := now.Sub(time.Unix(ts, 0)); d < -time.Hour || d > orderTokenMaxAge {
		return "", errors.New("token expired")
	}
	return orderID, nil
}
