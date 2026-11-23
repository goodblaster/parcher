// Package imagearchive maintains image sizing across archive serialization and
// performs the optional, browser-free reduction after OCR.
package imagearchive

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/goodblaster/parcher/parchmint/textlayer"
	"github.com/goodblaster/parcher/pscription/imageopt"
	"golang.org/x/net/html"
)

const densityAttr = "data-parch-image-density"

func attr(t html.Token, key string) string {
	for _, a := range t.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func set(t *html.Token, key, value string) {
	for i, a := range t.Attr {
		if a.Key == key {
			t.Attr[i].Val = value
			return
		}
	}
	t.Attr = append(t.Attr, html.Attribute{Key: key, Val: value})
}

func remove(t *html.Token, keys ...string) {
	kept := t.Attr[:0]
	for _, a := range t.Attr {
		drop := false
		for _, key := range keys {
			if a.Key == key {
				drop = true
			}
		}
		if !drop {
			kept = append(kept, a)
		}
	}
	t.Attr = kept
}

// editTags preserves every byte except the start tags actually changed. A DOM
// parse/render would normalize unrelated markup, including shadow templates.
func editTags(doc []byte, edit func(*html.Token) bool) []byte {
	z := html.NewTokenizer(bytes.NewReader(doc))
	var out bytes.Buffer
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			break
		}
		raw := append([]byte(nil), z.Raw()...)
		if tt == html.StartTagToken || tt == html.SelfClosingTagToken {
			t := z.Token()
			if edit(&t) {
				out.WriteString(t.String())
				continue
			}
		}
		out.Write(raw)
	}
	if z.Err() != io.EOF {
		return doc
	}
	return out.Bytes()
}

// RestoreDensity repairs Chrome's removal of srcset in snapshots. The source
// appears only once, in srcset, so preserving density does not double its bytes.
func RestoreDensity(doc []byte) []byte {
	return editTags(doc, func(t *html.Token) bool {
		if t.Data != "img" {
			return false
		}
		density, err := strconv.ParseFloat(attr(*t, densityAttr), 64)
		src := attr(*t, "src")
		if err != nil || density <= 0 || math.IsNaN(density) || math.IsInf(density, 0) || !strings.HasPrefix(src, "data:image/") {
			return false
		}
		set(t, "srcset", src+" "+strconv.FormatFloat(density, 'g', -1, 64)+"x")
		set(t, "src", "") // retain [src] CSS selectors, without a competing 1x candidate
		remove(t, "sizes")
		return true
	})
}

func imageSource(t html.Token) string {
	if srcset := attr(t, "srcset"); srcset != "" {
		fields := strings.Fields(srcset)
		if len(fields) == 2 && strings.HasSuffix(fields[1], "x") && strings.HasPrefix(fields[0], "data:image/") {
			return fields[0]
		}
		return ""
	}
	return attr(t, "src")
}

type target struct {
	width, height int
	natural       int
	blocked       bool
}

var backgroundImage = regexp.MustCompile(`(?i)(?:^|;)\s*background-image\s*:\s*url\(\s*["']?(data:image/[a-z0-9.+-]+;base64,[a-z0-9+/=]+)["']?\s*\)`)

// Downsize runs after recognition, preserving CSS natural size and page-space
// OCR coordinates while updating all image hash links to the new source bytes.
// Older archives fall back to their recorded image boxes; newer ones include
// measured targets (including object-fit and device pixel ratio) in markup.
func Downsize(data []byte, layer *textlayer.Layer) ([]byte, int, error) {
	fallback := map[string]target{}
	dpr := layer.Viewport.DPR
	if dpr <= 0 {
		dpr = 1
	}
	for _, im := range layer.Images {
		t := fallback[im.Hash]
		t.width = max(t.width, int(math.Ceil(float64(im.Box[2])*dpr)))
		t.height = max(t.height, int(math.Ceil(float64(im.Box[3])*dpr)))
		t.natural = im.Natural[0]
		fallback[im.Hash] = t
	}
	remap := map[string]string{}
	count := 0
	targets := map[string]target{}
	get := func(t html.Token) (string, target) {
		var src, size string
		if t.Data == "img" {
			src, size = imageSource(t), attr(t, "data-parch-image-size")
		} else if size = attr(t, "data-parch-background-size"); size != "" {
			// Only the single URL measured by prep, never other CSS URLs.
			style := attr(t, "style")
			if match := backgroundImage.FindStringSubmatch(style); match != nil {
				src = match[1]
			}
		}
		if !strings.HasPrefix(src, "data:image/") {
			return "", target{}
		}
		v := fallback[textlayer.FNV1a(src)]
		if size != "" {
			var w, h, n int
			if _, e := fmt.Sscanf(size, "%d,%d,%d", &w, &h, &n); e == nil {
				v = target{width: w, height: h, natural: n}
			}
		}
		v.blocked = v.width <= 0 || v.height <= 0 || (t.Data == "img" && v.natural <= 0)
		return src, v
	}
	// One target per source across ALL frame documents keeps shared hashes
	// consistent, and preserves enough pixels for the largest recorded use.
	_, err := MapDocuments(data, func(doc []byte) []byte {
		editTags(doc, func(t *html.Token) bool {
			src, v := get(*t)
			if src != "" {
				old := targets[src]
				v.width = max(v.width, old.width)
				v.height = max(v.height, old.height)
				v.blocked = v.blocked || old.blocked
				targets[src] = v
			}
			return false
		})
		return doc
	})
	if err != nil {
		return nil, 0, err
	}
	type replacement struct {
		src   string
		width int
	}
	replacements := map[string]replacement{}
	for src, v := range targets {
		if v.blocked {
			continue
		}
		uri, w, _ := imageopt.Resize(src, v.width, v.height)
		if uri != "" {
			replacements[src] = replacement{uri, w}
		}
	}
	out, err := MapDocuments(data, func(doc []byte) []byte {
		return editTags(doc, func(t *html.Token) bool {
			src, v := get(*t)
			r, ok := replacements[src]
			if !ok {
				return false
			}
			if t.Data == "img" {
				if v.natural <= 0 {
					return false
				}
				density := strconv.FormatFloat(float64(r.width)/float64(v.natural), 'g', -1, 64)
				set(t, densityAttr, density)
				set(t, "srcset", r.src+" "+density+"x")
				set(t, "src", "")
				remove(t, "sizes")
			} else {
				style := attr(*t, "style")
				indices := backgroundImage.FindStringSubmatchIndex(style)
				if indices == nil {
					return false
				}
				set(t, "style", style[:indices[2]]+r.src+style[indices[3]:])
			}
			remap[textlayer.FNV1a(src)] = textlayer.FNV1a(r.src)
			count++
			return true
		})
	})
	if err != nil {
		return nil, 0, err
	}
	for i := range layer.Images {
		if hash := remap[layer.Images[i].Hash]; hash != "" {
			layer.Images[i].Hash = hash
		}
	}
	for _, blocks := range [][]textlayer.Block{layer.Blocks, layer.Hidden} {
		for i := range blocks {
			if hash := remap[blocks[i].Image]; hash != "" {
				blocks[i].Image = hash
			}
		}
	}
	return out, count, nil
}
