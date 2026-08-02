package fetch

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goodblaster/errors"
)

func load(t *testing.T, loader *Loader, raw string) (*Resource, error) {
	t.Helper()

	target, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse %q: %v", raw, err)
	}

	return loader.Load(t.Context(), target)
}

func TestLoadDataURI(t *testing.T) {
	tests := []struct {
		name            string
		uri             string
		wantData        string
		wantContentType string
	}{
		{
			name:            "base64",
			uri:             "data:text/plain;base64,aGVsbG8=",
			wantData:        "hello",
			wantContentType: "text/plain",
		},
		{
			name:            "unpadded base64",
			uri:             "data:text/plain;base64,aGVsbG8",
			wantData:        "hello",
			wantContentType: "text/plain",
		},
		{
			name:            "percent encoded",
			uri:             "data:text/plain,hello%20world",
			wantData:        "hello world",
			wantContentType: "text/plain",
		},
		{
			name:            "no media type",
			uri:             "data:,hi",
			wantData:        "hi",
			wantContentType: "text/plain;charset=US-ASCII",
		},
		{
			name:            "svg with parameters",
			uri:             "data:image/svg+xml;charset=utf-8,%3Csvg%2F%3E",
			wantData:        "<svg/>",
			wantContentType: "image/svg+xml;charset=utf-8",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resource, err := load(t, &Loader{}, test.uri)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if string(resource.Data) != test.wantData {
				t.Errorf("Data = %q, want %q", resource.Data, test.wantData)
			}
			if resource.ContentType != test.wantContentType {
				t.Errorf("ContentType = %q, want %q", resource.ContentType, test.wantContentType)
			}
		})
	}
}

func TestLoadMalformedDataURI(t *testing.T) {
	if _, err := load(t, &Loader{}, "data:text/plain;base64"); err == nil {
		t.Error("expected an error for a data URI with no comma")
	}
}

func TestLoadFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.css")
	if err := os.WriteFile(path, []byte("body{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	resource, err := load(t, &Loader{}, (&url.URL{Scheme: "file", Path: path}).String())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if string(resource.Data) != "body{}" {
		t.Errorf("Data = %q", resource.Data)
	}
	if !strings.HasPrefix(resource.ContentType, "text/css") {
		t.Errorf("ContentType = %q, want text/css", resource.ContentType)
	}
}

// A path with characters that have to be escaped in a URL must still resolve
// back to the file on disk.
func TestLoadFileWithEscapedPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a file (1).txt")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	resource, err := load(t, &Loader{}, (&url.URL{Scheme: "file", Path: path}).String())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if string(resource.Data) != "x" {
		t.Errorf("Data = %q", resource.Data)
	}
}

func TestLoadMissingFile(t *testing.T) {
	missing := (&url.URL{Scheme: "file", Path: filepath.Join(t.TempDir(), "nope")}).String()
	if _, err := load(t, &Loader{}, missing); err == nil {
		t.Error("expected an error for a missing file")
	}
}

func TestHTTPDisabledByDefault(t *testing.T) {
	_, err := load(t, &Loader{}, "https://example.com/x.png")
	if !errors.Is(err, ErrOffline) {
		t.Errorf("err = %v, want ErrOffline", err)
	}
}

func TestLoadHTTP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != "test-agent" {
			t.Errorf("User-Agent = %q", r.Header.Get("User-Agent"))
		}
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte("PNGDATA"))
	}))
	defer server.Close()

	loader := &Loader{Client: server.Client(), UserAgent: "test-agent"}
	resource, err := load(t, loader, server.URL+"/logo.png")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if string(resource.Data) != "PNGDATA" {
		t.Errorf("Data = %q", resource.Data)
	}
	if resource.ContentType != "image/png" {
		t.Errorf("ContentType = %q", resource.ContentType)
	}
}

func TestHTTPErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusForbidden)
	}))
	defer server.Close()

	_, err := load(t, &Loader{Client: server.Client()}, server.URL+"/x")
	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Errorf("err = %v, want a 403", err)
	}
}

// A response without a Content-Type is sniffed rather than left blank.
func TestHTTPContentTypeSniffed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header()["Content-Type"] = nil
		w.Write([]byte("<html><body>hi</body></html>"))
	}))
	defer server.Close()

	resource, err := load(t, &Loader{Client: server.Client()}, server.URL+"/page")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !strings.HasPrefix(resource.ContentType, "text/html") {
		t.Errorf("ContentType = %q, want text/html", resource.ContentType)
	}
}

func TestMaxSize(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(bytes.Repeat([]byte("x"), 100))
	}))
	defer server.Close()

	loader := &Loader{Client: server.Client(), MaxSize: 50}
	if _, err := load(t, loader, server.URL+"/big"); err == nil {
		t.Error("expected an error for an oversized resource")
	}

	// Exactly at the limit is fine.
	loader.MaxSize = 100
	if _, err := load(t, loader, server.URL+"/big"); err != nil {
		t.Errorf("resource at the limit was rejected: %v", err)
	}

	// A negative limit means no limit at all.
	loader.MaxSize = -1
	if _, err := load(t, loader, server.URL+"/big"); err != nil {
		t.Errorf("unlimited loader rejected a resource: %v", err)
	}
}

func TestUnsupportedScheme(t *testing.T) {
	if _, err := load(t, &Loader{}, "ftp://example.com/x"); err == nil {
		t.Error("expected an error for an unsupported scheme")
	}
}
