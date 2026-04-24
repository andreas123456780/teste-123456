package main

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"strings"
	"testing"
)

// These tests cover only the local validation branches of the upload
// handler (missing method, missing token, wrong content-type, over-size).
// The actual PUT to Vercel Blob is exercised in production after deploy
// — there's no upstream mock for vercel-storage.com baked into the test
// suite on purpose (we'd just be testing the HTTP client stub).

func newImageFormRequest(t *testing.T, fieldName, filename, contentType string, body []byte) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="`+fieldName+`"; filename="`+filename+`"`)
	if contentType != "" {
		h.Set("Content-Type", contentType)
	}
	fw, err := w.CreatePart(h)
	if err != nil {
		t.Fatalf("CreatePart: %v", err)
	}
	if _, err := fw.Write(body); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/admin/upload", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req
}

func TestHandleAdminUpload_WrongMethod(t *testing.T) {
	t.Setenv("BLOB_READ_WRITE_TOKEN", "fake")
	h := handleAdminUpload()
	rr := httptest.NewRecorder()
	h(rr, httptest.NewRequest(http.MethodGet, "/api/admin/upload", nil))
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET → 405, got %d", rr.Code)
	}
}

func TestHandleAdminUpload_NoToken(t *testing.T) {
	os.Unsetenv("BLOB_READ_WRITE_TOKEN")
	h := handleAdminUpload()
	rr := httptest.NewRecorder()
	req := newImageFormRequest(t, "file", "x.png", "image/png", []byte{0x89, 0x50, 0x4e, 0x47})
	h(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("no token → 503, got %d (body=%s)", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "BLOB_READ_WRITE_TOKEN") {
		t.Fatalf("error body should mention missing env var, got %s", rr.Body.String())
	}
}

func TestHandleAdminUpload_UnsupportedType(t *testing.T) {
	t.Setenv("BLOB_READ_WRITE_TOKEN", "fake")
	h := handleAdminUpload()
	rr := httptest.NewRecorder()
	req := newImageFormRequest(t, "file", "x.svg", "image/svg+xml", []byte("<svg/>"))
	h(rr, req)
	if rr.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("svg → 415, got %d (body=%s)", rr.Code, rr.Body.String())
	}
}

func TestHandleAdminUpload_MissingFile(t *testing.T) {
	t.Setenv("BLOB_READ_WRITE_TOKEN", "fake")
	h := handleAdminUpload()
	// multipart with no `file` field
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("other", "value")
	_ = w.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/upload", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rr := httptest.NewRecorder()
	h(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("missing file → 400, got %d", rr.Code)
	}
}

func TestSanitizeBlobBase(t *testing.T) {
	cases := map[string]string{
		"":                           "image",
		"CAMISA BLACK AND WHITE.jpg": "camisa-black-and-white",
		"../../etc/passwd":           "passwd",
		"foto (1).png":               "foto-1",
		"wéird—name.jpg":             "wirdname",
		strings.Repeat("a", 100):     strings.Repeat("a", 40),
	}
	for in, want := range cases {
		if got := sanitizeBlobBase(in); got != want {
			t.Errorf("sanitizeBlobBase(%q) = %q, want %q", in, got, want)
		}
	}
}
