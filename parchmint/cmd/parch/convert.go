package main

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"os"
	path2 "path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/goodblaster/go-mht/pkg/documents"
	"github.com/goodblaster/parchmint/capture"
	"github.com/goodblaster/parchmint/internal/convert/pack"
	"github.com/goodblaster/parchmint/internal/convert/render"
)

// convertOffline moves an archive between the HTML and MHT containers with
// no browser: it reads the archive's own bytes and rewrites them.
//
// This is the --no-browser path. It is fast (no browser session) and, more
// to the point, it does not RE-LAY-OUT the page — the document is converted
// as saved, at whatever geometry it was captured at. The browser path
// re-renders, which is what makes --width/--dpr possible there and what
// makes them meaningless here.
//
// The trade is fidelity of the hard cases: Chrome resolves cid: stylesheets,
// CSSOM rules, and fonts for real, while this path rewrites references
// lexically. Prefer it for a straight container change, and the browser for
// anything that needs the page rendered.
func convertOffline(ctx context.Context, srcData []byte, path, sourceURL, format string, isMHT bool, warn func(string)) (*capture.RenderResult, error) {
	switch format {
	case "html":
		if !isMHT {
			return nil, fmt.Errorf("--no-browser -f html: %s is already HTML; there is nothing to convert", filepath.Base(path))
		}

		doc, err := documents.Parse(bytes.NewReader(srcData))
		if err != nil {
			return nil, fmt.Errorf("parse mht: %w", err)
		}
		out, err := render.Document(doc, render.Options{OnWarn: warn})
		if err != nil {
			return nil, err
		}

		return &capture.RenderResult{Bytes: out, MIME: "text/html"}, nil

	case "mht":
		// An archive is self-contained, so packing it needs no network —
		// and reaching out to one during a local format change would be a
		// surprise. Anything still pointing at the network is reported and
		// left as a plain reference.
		doc, err := pack.HTML(ctx, srcData, pack.Options{
			Base:   packBase(path, sourceURL),
			OnWarn: warn,
		})
		if err != nil {
			return nil, err
		}

		var buf bytes.Buffer
		if err := doc.Write(&buf); err != nil {
			return nil, fmt.Errorf("write mht: %w", err)
		}

		return &capture.RenderResult{Bytes: buf.Bytes(), MIME: "multipart/related"}, nil
	}

	return nil, fmt.Errorf("--no-browser converts between html and mht only, not %q — drop --no-browser to render one", format)
}

// packBase picks what relative references resolve against — which has to be
// the archive's own location on disk, not the address it was saved from.
//
// A browser-saved page keeps its resources in a sibling _files/ directory
// and refers to them relatively; only the file path finds those. Resolving
// against the original address instead sends every one of them at the
// network, and the packed archive comes out empty (measured: a 50MB page
// packed to 1.2MB of nothing). The page's own address is provenance, not a
// resolution base.
func packBase(path, sourceURL string) string {
	if path != stdinName {
		if absolute, err := filepath.Abs(path); err == nil {
			return "file://" + filepath.ToSlash(absolute)
		}
	}
	// Read from stdin there is no directory to resolve against, so the
	// recorded address is the only base on offer. Archives arriving that way
	// are self-contained anyway, with nothing relative left to resolve.
	return sourceURL
}

// rekeyFileBase re-names an MHT's parts from a file: base onto a
// network-style one.
//
// Chrome's MHTML loader will not resolve a subresource whose Content-Location
// is a file: URL. An archive converted from a page sitting on disk therefore
// comes out with every stylesheet and image present as a part and NONE of
// them binding: the page renders as unstyled markup. Verified by rendering
// the same archive twice, once re-keyed — the re-keyed copy matches the
// original page, the untouched one is bare HTML.
//
// The substitution covers the part headers and the document body together, so
// the root location, the parts, and any absolute references between them
// stay consistent with each other; relative references resolve against the
// new base unchanged.
func rekeyFileBase(archive []byte, fileBase, declaredBase string) []byte {
	if fileBase == "" || declaredBase == "" || !needsRekey(archive) {
		return archive
	}
	return bytes.ReplaceAll(archive, []byte(fileBase), []byte(declaredBase))
}

// needsRekey reports whether the archive names any SUBRESOURCE by file: URL.
//
// A root part alone under a file: location is harmless: a self-contained
// archive resolves everything through cid: parts and data: URIs, so nothing
// has to be looked up by location at all. Converting one of those is the
// common case, and it comes out unchanged.
func needsRekey(archive []byte) bool {
	// The leading CRLF keeps this off the Snapshot-Content-Location header,
	// which names the document rather than a part.
	return bytes.Count(archive, []byte("\r\nContent-Location: file:")) > 1
}

// archiveBases works out that pair for a source file: the file: URL prefix
// its resources currently sit under, and the network-style prefix they should
// be named by.
//
// The declared base is the page's own address when the archive records one,
// so a converted page keeps the identity it was saved from. Without that
// there is nothing true to use, and a reserved .invalid host at least says so
// plainly rather than borrowing a real origin.
func archiveBases(path, sourceURL string) (fileBase, declaredBase string) {
	if path == stdinName {
		return "", ""
	}

	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", ""
	}
	directory := filepath.ToSlash(filepath.Dir(absolute)) + "/"
	fileBase = (&url.URL{Scheme: "file", Path: directory}).String()

	if sourceURL != "" {
		if parsed, err := url.Parse(sourceURL); err == nil && parsed.IsAbs() {
			// The page's directory: everything beside it was saved beside it.
			parsed.RawQuery, parsed.Fragment = "", ""
			directory := path2.Dir(parsed.Path)
			if !strings.HasSuffix(directory, "/") {
				directory += "/"
			}
			parsed.Path = directory
			return fileBase, parsed.String()
		}
	}

	return fileBase, "https://archive.invalid/"
}

// savedFromRe matches the provenance comment browsers write at the top of a
// saved page: `<!-- saved from url=(0024)https://www.test-site-3/ -->`. The
// parenthesised number is the URL's length in characters, which is what
// makes the URL recoverable even when it contains something comment-like.
var savedFromRe = regexp.MustCompile(`(?i)<!--\s*saved from url=\((\d+)\)`)

// savedFromURL recovers the address a browser-saved page came from, or ""
// when the file carries no such marker. It is the only record of the
// original URL in a page saved by Chrome or IE, and without it a freshly
// indexed layer would have to claim the archive came from a local file.
func savedFromURL(data []byte) string {
	// The marker sits in the first few lines; searching a multi-megabyte
	// archive for it would be wasted work.
	head := data
	if len(head) > 4096 {
		head = head[:4096]
	}

	match := savedFromRe.FindSubmatchIndex(head)
	if match == nil {
		return ""
	}

	length, err := strconv.Atoi(string(head[match[2]:match[3]]))
	if err != nil || length <= 0 {
		return ""
	}

	start := match[1]
	if start+length > len(head) {
		return ""
	}
	url := strings.TrimSpace(string(head[start : start+length]))

	// Anything that is not an absolute http(s) URL is not what the marker
	// promised, so treat it as absent rather than inventing provenance.
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return ""
	}

	return url
}

// convertWarner reports references the offline conversion could not resolve,
// collapsing repeats so a page that reuses one missing font does not print
// the same line fifty times.
func convertWarner(quiet bool) func(string) {
	if quiet {
		return nil
	}

	seen := map[string]bool{}
	return func(message string) {
		if seen[message] {
			return
		}
		seen[message] = true
		if len(seen) > convertWarnLimit {
			if len(seen) == convertWarnLimit+1 {
				fmt.Fprintln(os.Stderr, "parch: (further conversion warnings suppressed)")
			}
			return
		}
		fmt.Fprintln(os.Stderr, "parch: "+message)
	}
}

// convertWarnLimit caps the distinct warnings printed; a partial archive can
// reference hundreds of resources it never saved.
const convertWarnLimit = 20
