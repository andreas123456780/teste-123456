package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"strings"
	"time"
)

// admin_upload.go adds a tiny image upload endpoint that relays multipart
// file uploads to Vercel Blob and returns the public URL back to the
// admin UI. It is scoped to image/* payloads, caps the size at 8 MiB,
// and generates a collision-proof pathname so uploads can't overwrite
// each other (or break arbitrary remote URLs by path collision).
//
// It intentionally does not use the @vercel/blob SDK: we talk to the
// same REST API (api.vercel.com/v2/blob/upload) with BLOB_READ_WRITE_TOKEN
// so the dependency graph stays Go-only.
//
// The endpoint skips the global 64 KiB body limiter by resetting
// r.Body with a larger MaxBytesReader before consuming it — see the
// early `r.Body = http.MaxBytesReader(...)` line below.

const (
	uploadMaxBytes     = 8 << 20 // 8 MiB hard cap
	vercelBlobEndpoint = "https://blob.vercel-storage.com"
)

// allowedImageContentTypes is the whitelist we trust for <img> rendering.
// (Browsers sniff, but letting SVG through opens XSS via <script>.)
var allowedImageContentTypes = map[string]string{
	"image/jpeg":    ".jpg",
	"image/png":     ".png",
	"image/webp":    ".webp",
	"image/gif":     ".gif",
	"image/avif":    ".avif",
	"image/heic":    ".heic",
	"image/heif":    ".heif",
}

type uploadResponse struct {
	URL      string `json:"url"`
	Pathname string `json:"pathname"`
}

func handleAdminUpload() http.HandlerFunc {
	token := strings.TrimSpace(os.Getenv("BLOB_READ_WRITE_TOKEN"))
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		if token == "" {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "upload store not configured (BLOB_READ_WRITE_TOKEN missing)"})
			return
		}

		// Bypass the global 64 KiB body cap: we need room for a full image.
		r.Body = http.MaxBytesReader(w, r.Body, uploadMaxBytes)

		if err := r.ParseMultipartForm(uploadMaxBytes); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid multipart payload: " + err.Error()})
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing 'file' field"})
			return
		}
		defer file.Close()

		if header.Size <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "empty file"})
			return
		}

		data, err := io.ReadAll(io.LimitReader(file, uploadMaxBytes+1))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read failed: " + err.Error()})
			return
		}
		if int64(len(data)) > uploadMaxBytes {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "file exceeds 8 MiB limit"})
			return
		}

		ct := header.Header.Get("Content-Type")
		if ct == "" {
			ct = http.DetectContentType(data)
		}
		ct = strings.ToLower(strings.TrimSpace(strings.Split(ct, ";")[0]))
		ext, ok := allowedImageContentTypes[ct]
		if !ok {
			writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{"error": "only JPG, PNG, WEBP, GIF, AVIF, HEIC are allowed (got " + ct + ")"})
			return
		}

		// Collision-proof pathname: products/<unix-ms>-<rand>.<ext>.
		// We also sanitize the original filename onto the blob so operators
		// can recognize it in the Vercel Blob dashboard.
		randBuf := make([]byte, 6)
		if _, err := rand.Read(randBuf); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "rand failed"})
			return
		}
		base := sanitizeBlobBase(header.Filename)
		pathname := fmt.Sprintf("products/%d-%s-%s%s",
			time.Now().UnixMilli(),
			hex.EncodeToString(randBuf),
			base,
			ext,
		)

		url, err := uploadToVercelBlob(r.Context(), token, pathname, ct, data)
		if err != nil {
			log.Printf("admin_upload: blob put failed: %v", err)
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "storage upload failed"})
			return
		}

		writeJSON(w, http.StatusOK, uploadResponse{URL: url, Pathname: pathname})
	}
}

// sanitizeBlobBase strips the extension and keeps only alnum/dash
// characters from the original upload filename, so the Blob path stays
// readable without risking URL-breaking characters.
func sanitizeBlobBase(filename string) string {
	name := path.Base(filename)
	if i := strings.LastIndex(name, "."); i > 0 {
		name = name[:i]
	}
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			b.WriteRune(r)
		case r == ' ' || r == '_' || r == '.':
			b.WriteRune('-')
		}
		if b.Len() >= 40 {
			break
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "image"
	}
	return out
}

// uploadToVercelBlob PUTs the body to the Vercel Blob public store and
// returns the resulting public URL. See:
//
//	https://vercel.com/docs/vercel-blob/using-blob-sdk
//
// for the underlying REST contract.
func uploadToVercelBlob(ctx context.Context, token, pathname, contentType string, data []byte) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, vercelBlobEndpoint+"/"+pathname, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("x-content-type", contentType)
	req.Header.Set("access", "public")
	req.Header.Set("x-api-version", "7")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("blob PUT failed: status=%d body=%s", resp.StatusCode, string(body))
	}

	var parsed struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("blob response unparseable: %w", err)
	}
	if parsed.URL == "" {
		return "", fmt.Errorf("blob response missing url: %s", string(body))
	}
	return parsed.URL, nil
}
