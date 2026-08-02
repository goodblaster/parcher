package main

import (
	"bytes"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSavedFromURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "chrome marker",
			in:   "<!DOCTYPE html>\n<!-- saved from url=(0024)https://www.test-site-3/ -->\n<html>",
			want: "https://www.test-site-3/",
		},
		{
			name: "case insensitive",
			in:   "<!-- Saved from url=(0019)http://example.com -->",
			want: "http://example.com",
		},
		{
			name: "url containing a comment close",
			in:   "<!-- saved from url=(0027)https://ex.com/?q=--%3E&a=1 -->",
			want: "https://ex.com/?q=--%3E&a=1",
		},
		{
			name: "no marker",
			in:   "<!DOCTYPE html><html><body>hi</body></html>",
			want: "",
		},
		{
			// IE writes this for a page saved from the local machine; it is
			// a zone marker, not an address.
			name: "non-http scheme is not provenance",
			in:   "<!-- saved from url=(0014)about:internet -->",
			want: "",
		},
		{
			name: "length runs past the end",
			in:   "<!-- saved from url=(9999)https://ex.com -->",
			want: "",
		},
		{
			name: "malformed length",
			in:   "<!-- saved from url=(abcd)https://ex.com -->",
			want: "",
		},
		{
			// Only the head of the file is searched; a marker this deep is
			// not a browser's provenance comment.
			name: "beyond the search window",
			in:   strings.Repeat(" ", 5000) + "<!-- saved from url=(0018)https://ex.com/x -->",
			want: "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := savedFromURL([]byte(test.in)); got != test.want {
				t.Errorf("savedFromURL() = %q, want %q", got, test.want)
			}
		})
	}
}

// The resolution base must be the file on disk even when the page records
// where it came from: a browser-saved page's sidecar _files/ directory is
// only reachable that way.
func TestPackBasePrefersTheFile(t *testing.T) {
	got := packBase("testdata/page.html", "https://www.test-site-3/")
	if !strings.HasPrefix(got, "file://") || !strings.HasSuffix(got, "/testdata/page.html") {
		t.Errorf("packBase() = %q, want the file's own location", got)
	}

	// With no file to resolve against, the recorded address is all there is.
	if got := packBase(stdinName, "https://ex.com/p"); got != "https://ex.com/p" {
		t.Errorf("packBase(stdin) = %q", got)
	}
}

// Chrome's MHTML loader ignores parts named by a file: URL, so a converted
// archive has to be re-keyed onto a network-style base or it opens as
// unstyled markup with every resource present and none of them bound.
func TestArchiveBases(t *testing.T) {
	fileBase, declared := archiveBases("dir/t1.html", "https://www.test-site-3/")
	if !strings.HasPrefix(fileBase, "file://") || !strings.HasSuffix(fileBase, "/dir/") {
		t.Errorf("fileBase = %q, want the file's directory", fileBase)
	}
	if declared != "https://www.test-site-3/" {
		t.Errorf("declared = %q", declared)
	}

	// A page saved from a sub-path keeps that directory, so resources saved
	// beside it keep their relative position.
	if _, declared := archiveBases("t1.html", "https://ex.com/cars/index.html"); declared != "https://ex.com/cars/" {
		t.Errorf("declared = %q, want the page's directory", declared)
	}

	// Query strings and fragments are not part of a directory.
	if _, declared := archiveBases("t1.html", "https://ex.com/a/b.html?x=1#top"); declared != "https://ex.com/a/" {
		t.Errorf("declared = %q", declared)
	}

	// With no recorded address there is nothing true to claim, so a reserved
	// host is used rather than borrowing a real one.
	if _, declared := archiveBases("t1.html", ""); declared != "https://archive.invalid/" {
		t.Errorf("declared = %q, want a reserved host", declared)
	}

	// Nothing on disk to re-key.
	if fileBase, declared := archiveBases(stdinName, "https://ex.com/"); fileBase != "" || declared != "" {
		t.Errorf("archiveBases(stdin) = %q, %q, want empty", fileBase, declared)
	}
}

func TestRekeyFileBase(t *testing.T) {
	// Part headers arrive preceded by the CRLF that ends the boundary line.
	archive := []byte("\r\nContent-Location: file:///tmp/d/page.html\r\n" +
		"\r\nContent-Location: file:///tmp/d/assets/x.css\r\n")

	got := string(rekeyFileBase(archive, "file:///tmp/d/", "https://ex.com/"))
	want := "\r\nContent-Location: https://ex.com/page.html\r\n" +
		"\r\nContent-Location: https://ex.com/assets/x.css\r\n"
	if got != want {
		t.Errorf("rekeyFileBase() = %q, want %q", got, want)
	}

	// Nothing to do without both bases: leave the archive untouched rather
	// than mangling it.
	if got := rekeyFileBase(archive, "", "https://ex.com/"); string(got) != string(archive) {
		t.Error("rekeyFileBase rewrote with no file base")
	}
	if got := rekeyFileBase(archive, "file:///tmp/d/", ""); string(got) != string(archive) {
		t.Error("rekeyFileBase rewrote with no declared base")
	}
}

// A self-contained archive resolves through cid: parts and data: URIs, so a
// lone file: root harms nothing and must not be rewritten; only an archive
// that names subresources that way needs re-keying.
func TestRekeySkipsSelfContainedArchives(t *testing.T) {
	selfContained := []byte("Snapshot-Content-Location: file:///tmp/d/page.html\r\n" +
		"\r\nContent-Location: file:///tmp/d/page.html\r\n" +
		"\r\nContent-Location: cid:css-1@mhtml.blink\r\n")

	if got := rekeyFileBase(selfContained, "file:///tmp/d/", "https://ex.com/"); !bytes.Equal(got, selfContained) {
		t.Errorf("self-contained archive was rewritten:\n%s", got)
	}

	withSidecars := append(selfContained, "\r\nContent-Location: file:///tmp/d/a/x.css\r\n"...)
	if got := rekeyFileBase(withSidecars, "file:///tmp/d/", "https://ex.com/"); bytes.Equal(got, withSidecars) {
		t.Error("archive with file: subresources was not re-keyed")
	}
}

// --fetch is what lets the offline path go and get the resources a saved page
// still points at over the network. Without it those stay live references, so
// a local format change never reaches for the network on its own.
func TestConvertOfflineFetch(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte("PNGDATA"))
	}))
	defer server.Close()

	dir := t.TempDir()
	page := filepath.Join(dir, "page.html")
	source := []byte(`<img src="` + server.URL + `/remote.png">`)
	if err := os.WriteFile(page, source, 0o644); err != nil {
		t.Fatal(err)
	}

	convert := func(network bool) string {
		t.Helper()
		res, err := convertOffline(t.Context(), offlineRequest{
			source:  source,
			path:    page,
			format:  "mht",
			network: network,
			warn:    func(string) {},
		})
		if err != nil {
			t.Fatalf("convertOffline(network=%v): %v", network, err)
		}
		return string(res.Bytes)
	}

	if got := convert(false); strings.Contains(got, "PNGDATA") {
		t.Error("resource was fetched without --fetch")
	}
	if requests != 0 {
		t.Errorf("made %d requests without --fetch, want 0", requests)
	}

	if got := convert(true); !strings.Contains(got, base64.StdEncoding.EncodeToString([]byte("PNGDATA"))) {
		t.Errorf("resource was not embedded with --fetch:\n%s", got)
	}
	if requests != 1 {
		t.Errorf("made %d requests with --fetch, want 1", requests)
	}
}

// The fallback base puts the page's own address where the file on disk
// stands, so a reference that escapes the saved directory can be resolved a
// second way.
func TestFallbackBase(t *testing.T) {
	got := fallbackBase("dir/page.html", "https://www.test-site-3/")
	if got != "https://www.test-site-3/page.html" {
		t.Errorf("fallbackBase() = %q", got)
	}

	// Nothing recorded to fall back to.
	if got := fallbackBase("dir/page.html", ""); got != "" {
		t.Errorf("fallbackBase() = %q, want empty without a source URL", got)
	}
	// Nothing on disk to stand in for.
	if got := fallbackBase(stdinName, "https://ex.com/"); got != "" {
		t.Errorf("fallbackBase(stdin) = %q, want empty", got)
	}
}
