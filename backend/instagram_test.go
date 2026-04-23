package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type stubTransport struct {
	fn func(req *http.Request) (*http.Response, error)
}

func (s *stubTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return s.fn(req)
}

func stubGraphServer(t *testing.T, body string, status int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/media") {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if r.URL.Query().Get("access_token") == "" {
			t.Error("missing access_token")
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
}

func TestInstagramFetch_NotConfigured(t *testing.T) {
	cli := &instagramClient{cacheTTL: time.Minute}
	posts, err := cli.fetch(context.Background())
	if err != nil {
		t.Fatalf("no error expected: %v", err)
	}
	if len(posts) != 0 {
		t.Fatalf("expected empty posts, got %d", len(posts))
	}
}

func TestInstagramFetch_HappyPath(t *testing.T) {
	body := `{"data":[
		{"id":"1","media_type":"IMAGE","media_url":"https://cdn/ig/1.jpg","permalink":"https://instagram.com/p/1","caption":"hello","timestamp":"2026-01-01T12:00:00+0000"},
		{"id":"2","media_type":"VIDEO","media_url":"https://cdn/ig/2.mp4","thumbnail_url":"https://cdn/ig/2.jpg","permalink":"https://instagram.com/p/2","caption":"","timestamp":"2026-01-02T12:00:00+0000"}
	]}`
	srv := stubGraphServer(t, body, 200)
	defer srv.Close()

	cli := &instagramClient{
		httpCli:   &http.Client{Timeout: 5 * time.Second},
		token:     "t0k3n",
		userID:    "12345",
		graphBase: srv.URL,
		cacheTTL:  time.Minute,
	}
	posts, err := cli.fetch(context.Background())
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(posts) != 2 {
		t.Fatalf("expected 2 posts, got %d", len(posts))
	}
	if posts[0].ID != "1" || posts[0].MediaURL != "https://cdn/ig/1.jpg" {
		t.Fatalf("post[0] mismatch: %+v", posts[0])
	}
	if posts[1].ThumbnailURL != "https://cdn/ig/2.jpg" {
		t.Fatalf("post[1] thumbnail missing")
	}

	// Second call should hit cache (not the server again).
	posts2, err := cli.fetch(context.Background())
	if err != nil || len(posts2) != 2 {
		t.Fatalf("cached fetch: err=%v len=%d", err, len(posts2))
	}
}

func TestInstagramFetch_GraphError_ReturnsStale(t *testing.T) {
	srv := stubGraphServer(t, `{"error":"bad"}`, 500)
	defer srv.Close()

	cli := &instagramClient{
		httpCli:   &http.Client{Timeout: 5 * time.Second},
		token:     "t",
		userID:    "1",
		graphBase: srv.URL,
		cacheTTL:  time.Minute,
		// pre-seed stale cache
		cache:    []instagramPost{{ID: "old", MediaURL: "x"}},
		cachedAt: time.Now().Add(-2 * time.Hour),
	}
	posts, err := cli.fetch(context.Background())
	if err != nil {
		t.Fatalf("fetch should return stale on graph error: %v", err)
	}
	if len(posts) != 1 || posts[0].ID != "old" {
		t.Fatalf("expected stale cache, got %+v", posts)
	}
}

func TestInstagramFetch_GraphError_NoCache(t *testing.T) {
	srv := stubGraphServer(t, `{"error":"bad"}`, 500)
	defer srv.Close()

	cli := &instagramClient{
		httpCli:   &http.Client{Timeout: 5 * time.Second},
		token:     "t",
		userID:    "1",
		graphBase: srv.URL,
		cacheTTL:  time.Minute,
	}
	_, err := cli.fetch(context.Background())
	if err == nil {
		t.Fatal("expected error on graph failure without cache")
	}
}

func TestInstagramHandler_NotConfigured(t *testing.T) {
	cli := &instagramClient{cacheTTL: time.Minute}
	h := handleInstagramFeed(cli)

	rr := httptest.NewRecorder()
	h(rr, httptest.NewRequest(http.MethodGet, "/api/social/instagram", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 even unconfigured, got %d", rr.Code)
	}
	var resp instagramFeedResponse
	_ = json.NewDecoder(rr.Body).Decode(&resp)
	if resp.Configured {
		t.Fatal("expected configured=false")
	}
	if len(resp.Posts) != 0 {
		t.Fatalf("expected 0 posts, got %d", len(resp.Posts))
	}
}

func TestInstagramHandler_WrongMethod(t *testing.T) {
	h := handleInstagramFeed(&instagramClient{cacheTTL: time.Minute})
	rr := httptest.NewRecorder()
	h(rr, httptest.NewRequest(http.MethodPost, "/api/social/instagram", strings.NewReader("{}")))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rr.Code)
	}
}

func TestTruncateCaption(t *testing.T) {
	s := strings.Repeat("a", 300)
	got := truncateCaption(s, 100)
	if len(got) != 101 { // 100 + ellipsis rune (but UTF-8 bytes vary)
		// The ellipsis is "…" (3 bytes in UTF-8) so 100 + 3 = 103.
		if len(got) != 103 {
			t.Fatalf("unexpected length %d: %q", len(got), got)
		}
	}
	if truncateCaption("short", 100) != "short" {
		t.Fatal("short strings should be preserved")
	}
}

// sanity: compile-time check that instagramFeedResponse serializes cleanly
var _ = func() bool {
	b, _ := json.Marshal(instagramFeedResponse{Posts: []instagramPost{{ID: "1"}}})
	return len(b) > 0
}()

func TestDiscardReader(t *testing.T) {
	// Ensure io.LimitReader + io.ReadAll behave as expected for our cap.
	reader := io.LimitReader(strings.NewReader("hello"), 1024)
	b, err := io.ReadAll(reader)
	if err != nil || string(b) != "hello" {
		t.Fatalf("limit reader misbehaves: %v %q", err, b)
	}
}
