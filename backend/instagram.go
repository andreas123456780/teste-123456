package main

// Instagram feed integration via the Meta Graph API.
//
// The widget on the homepage pulls the 6 most recent posts from the
// brand's Instagram (Business or Creator account tied to a Facebook
// Page). We cache results in memory for 10 minutes so we don't hammer
// the Graph API quota on every page view and so the frontend renders
// quickly.
//
// Configuration (both must be set — otherwise the endpoint returns an
// empty list and the frontend hides the widget):
//
//   INSTAGRAM_ACCESS_TOKEN  long-lived access token (60 days, renewable
//                           via the Graph API debug endpoint).
//   INSTAGRAM_USER_ID       numeric IG Business Account ID. Find it in
//                           the Meta Business Suite or via
//                           GET /me/accounts → .instagram_business_account.id
//
// The access token is NEVER sent to the browser — the frontend only
// hits our /api/social/instagram endpoint which proxies server-side.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// instagramPost is the slimmed-down shape we return to the frontend.
// We deliberately drop the access-token-bearing fields and the user's
// internal numeric IDs — only what's needed to render a thumbnail grid.
type instagramPost struct {
	ID           string    `json:"id"`
	MediaType    string    `json:"mediaType"` // IMAGE | VIDEO | CAROUSEL_ALBUM
	MediaURL     string    `json:"mediaUrl"`
	ThumbnailURL string    `json:"thumbnailUrl,omitempty"`
	Permalink    string    `json:"permalink"`
	Caption      string    `json:"caption,omitempty"`
	Timestamp    time.Time `json:"timestamp"`
}

type instagramFeedResponse struct {
	Posts      []instagramPost `json:"posts"`
	FetchedAt  time.Time       `json:"fetchedAt"`
	Configured bool            `json:"configured"` // false when token/user_id missing
}

type instagramClient struct {
	httpCli    *http.Client
	token      string
	userID     string
	graphBase  string
	mu         sync.Mutex
	cache      []instagramPost
	cachedAt   time.Time
	cacheTTL   time.Duration
}

func newInstagramClient() *instagramClient {
	return &instagramClient{
		httpCli:   &http.Client{Timeout: 5 * time.Second},
		token:     strings.TrimSpace(os.Getenv("INSTAGRAM_ACCESS_TOKEN")),
		userID:    strings.TrimSpace(os.Getenv("INSTAGRAM_USER_ID")),
		graphBase: graphBaseOrDefault(),
		cacheTTL:  10 * time.Minute,
	}
}

func graphBaseOrDefault() string {
	if s := strings.TrimSpace(os.Getenv("INSTAGRAM_GRAPH_BASE")); s != "" {
		return s
	}
	return "https://graph.facebook.com/v19.0"
}

func (c *instagramClient) configured() bool {
	return c.token != "" && c.userID != ""
}

// fetch returns cached posts when fresh, otherwise pulls the latest 6
// from Graph API. Errors are logged (not surfaced to the caller) and
// the previous cache is returned — that keeps the homepage widget
// rendering even if Meta has a hiccup.
func (c *instagramClient) fetch(ctx context.Context) ([]instagramPost, error) {
	if !c.configured() {
		return nil, nil
	}
	c.mu.Lock()
	if time.Since(c.cachedAt) < c.cacheTTL && len(c.cache) > 0 {
		out := append([]instagramPost(nil), c.cache...)
		c.mu.Unlock()
		return out, nil
	}
	c.mu.Unlock()

	posts, err := c.callGraph(ctx)
	if err != nil {
		c.mu.Lock()
		defer c.mu.Unlock()
		if len(c.cache) > 0 {
			log.Printf("instagram: refresh failed, returning stale cache: %v", err)
			return append([]instagramPost(nil), c.cache...), nil
		}
		return nil, err
	}

	c.mu.Lock()
	c.cache = posts
	c.cachedAt = time.Now()
	c.mu.Unlock()
	return posts, nil
}

// callGraph talks to the Meta Graph API directly. Separate from fetch
// so tests can stub the transport without re-implementing caching.
func (c *instagramClient) callGraph(ctx context.Context) ([]instagramPost, error) {
	q := url.Values{}
	q.Set("fields", "id,media_type,media_url,permalink,thumbnail_url,caption,timestamp")
	q.Set("limit", "6")
	q.Set("access_token", c.token)
	endpoint := fmt.Sprintf("%s/%s/media?%s", c.graphBase, url.PathEscape(c.userID), q.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	resp, err := c.httpCli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("graph api: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1 MiB cap
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		// Don't leak Meta's full error text to clients — log server-side.
		log.Printf("instagram: graph returned %d: %s", resp.StatusCode, truncateForLog(string(body), 400))
		return nil, fmt.Errorf("graph status %d", resp.StatusCode)
	}

	var raw struct {
		Data []struct {
			ID           string `json:"id"`
			MediaType    string `json:"media_type"`
			MediaURL     string `json:"media_url"`
			Permalink    string `json:"permalink"`
			ThumbnailURL string `json:"thumbnail_url"`
			Caption      string `json:"caption"`
			Timestamp    string `json:"timestamp"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	out := make([]instagramPost, 0, len(raw.Data))
	for _, r := range raw.Data {
		// Skip anything without a renderable URL — videos sometimes
		// return only a thumbnail, which is fine.
		if r.MediaURL == "" && r.ThumbnailURL == "" {
			continue
		}
		ts, _ := time.Parse(time.RFC3339, r.Timestamp)
		out = append(out, instagramPost{
			ID:           r.ID,
			MediaType:    r.MediaType,
			MediaURL:     r.MediaURL,
			ThumbnailURL: r.ThumbnailURL,
			Permalink:    r.Permalink,
			Caption:      truncateCaption(r.Caption, 280),
			Timestamp:    ts,
		})
	}
	return out, nil
}

func truncateCaption(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func truncateForLog(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…(truncated)"
}

// handleInstagramFeed returns the most recent 6 posts. Always 200 —
// configuration-missing and transient Graph errors collapse to an
// empty list, and the frontend knows to hide the widget. This matches
// how we treat the shipping cache: prefer a degraded UI over a 500.
func handleInstagramFeed(cli *instagramClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
		defer cancel()
		posts, err := cli.fetch(ctx)
		if err != nil {
			// Log server-side, return empty so the UI hides.
			log.Printf("instagram: fetch failed: %v", err)
			posts = nil
		}
		writeJSON(w, http.StatusOK, instagramFeedResponse{
			Posts:      posts,
			FetchedAt:  time.Now().UTC(),
			Configured: cli.configured(),
		})
	}
}
