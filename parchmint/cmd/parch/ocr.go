package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	flag "github.com/spf13/pflag"

	"github.com/goodblaster/parcher/parchmint/internal/ocr"
	"github.com/goodblaster/parcher/parchmint/internal/ocr/engines"
	"github.com/goodblaster/parcher/parchmint/internal/ocr/std"
	"github.com/goodblaster/parcher/parchmint/textlayer"
)

// runOCRCommand implements `parch ocr <archive.html>`: OCR the
// archive's images and merge the recognized text into the embedded text
// layer as source:"ocr" blocks, so text inside images (charts, frozen
// canvases and iframes, photos of signs) becomes searchable and
// highlightable later — including by external indexers that only see the
// layer. OCR is resource-hungry, so it never runs at capture time; this
// command is the explicit opt-in.
func runOCRCommand(args []string) {
	fs := flag.NewFlagSet("ocr", flag.ExitOnError)
	engineName := fs.String("engine", engines.Default(), "OCR engine: "+strings.Join(ocr.Names(), ", "))
	timeout := fs.Int("timeout", 0, "give up after this many seconds (0 = no limit)")
	langFlag := fs.String("lang", "", "comma-separated OCR language hints (default: the page's declared language, else en-US)")
	minSize := fs.Int("min-size", 32, "skip images displayed smaller than this (px, either dimension)")
	fs.IntVar(minSize, "min", 32, "deprecated spelling of --min-size")
	_ = fs.MarkDeprecated("min", "use --min-size")
	output := fs.StringP("output", "o", "", "write result here instead of updating the archive in place")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s ocr [--engine %s] [--lang en-US] [-o out] <archive.html|.mht>\n\n", os.Args[0], strings.Join(ocr.Names(), "|"))
		fmt.Fprintln(os.Stderr, "Updates the archive in place unless -o names another file.")
		fmt.Fprintln(os.Stderr, "\nOptions:")
		fs.PrintDefaults()
	}
	_ = fs.Parse(args)
	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(2)
	}
	path := fs.Arg(0)

	die := fail

	if *timeout < 0 {
		usageErr(fmt.Errorf("--timeout must not be negative, got %d", *timeout))
	}
	if !slices.Contains(ocr.Names(), *engineName) {
		usageErr(fmt.Errorf("unknown OCR engine %q (want one of: %s)", *engineName, strings.Join(ocr.Names(), ", ")))
	}
	// An engine may refuse to start — a remote one with no credentials, say.
	engine, err := ocr.NewRecognizer(*engineName)
	if err != nil {
		die(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		die(err)
	}
	layer, err := textlayer.FromBytes(data, path)
	if err != nil {
		die(err)
	}
	if len(layer.Images) == 0 {
		die(fmt.Errorf("%s records no images in its text layer (the page has none, or the archive predates image recording — recapture it)", path))
	}

	// The document markup carrying the data-URI images: the file itself
	// for HTML, the decoded text/html part for MHT.
	doc := data
	if textlayer.IsMHT(data) {
		doc, err = textlayer.MHTDocument(data)
		if err != nil {
			die(err)
		}
	}

	langs := parseLangs(*langFlag, doc)

	// Pair the layer's image entries with the archive's data-URI bytes by
	// re-hashing every img src the same way the extractor did.
	byHash := imageBytesByHash(doc)

	stats := struct{ eligible, unmatched, vector, failed, empty, blocks, words int }{}

	// Re-indexing replaces earlier OCR results.
	kept := layer.Blocks[:0]
	for _, b := range layer.Blocks {
		if b.Source != "ocr" {
			kept = append(kept, b)
		}
	}
	layer.Blocks = kept
	nextID := 0
	for _, b := range layer.Blocks {
		if b.ID >= nextID {
			nextID = b.ID + 1
		}
	}
	// Hidden blocks share the id sequence; an OCR block must not reuse one.
	for _, b := range layer.Hidden {
		if b.ID >= nextID {
			nextID = b.ID + 1
		}
	}

	// Gather every image worth reading, each distinct image once (the same
	// one may appear at several positions), and hand the whole set to the
	// engine in a single call: a remote engine uploads and waits for a
	// batch, it does not want a round trip per image.
	var batch []ocr.Image
	queued := map[string]bool{}
	var eligible []textlayer.Image
	for _, img := range layer.Images {
		if img.Box[2] < *minSize || img.Box[3] < *minSize {
			continue
		}
		stats.eligible++
		ai, ok := byHash[img.Hash]
		if !ok {
			stats.unmatched++
			continue
		}
		if strings.Contains(ai.mime, "svg") {
			// Vector sources have no pixels to OCR.
			stats.vector++
			continue
		}
		eligible = append(eligible, img)
		if !queued[img.Hash] {
			queued[img.Hash] = true
			batch = append(batch, ocr.Image{ID: img.Hash, MIME: "image/" + ai.mime, Data: ai.data})
		}
	}

	ctx := context.Background()
	if *timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(*timeout)*time.Second)
		defer cancel()
	}
	results, err := engine.Recognize(ctx, batch, langs)
	if err != nil {
		die(fmt.Errorf("ocr engine %s: %w", engine.Name(), err))
	}
	byID := make(map[string]ocr.Result, len(results))
	for _, r := range results {
		byID[r.ID] = r
	}
	reported := map[string]bool{}

	for _, img := range eligible {
		res, ok := byID[img.Hash]
		if !ok {
			res.Err = fmt.Errorf("the engine returned no result")
		}
		if res.Err != nil {
			stats.failed++
			if !reported[img.Hash] {
				reported[img.Hash] = true
				fmt.Fprintf(os.Stderr, "parch: ocr failed for image %s: %v\n", img.Hash, res.Err)
			}
			continue
		}

		block, ok := ocrBlock(nextID, img, res.Lines)
		if !ok {
			stats.empty++
			continue
		}
		layer.Blocks = append(layer.Blocks, block)
		stats.blocks++
		stats.words += len(block.Words)
		nextID++
	}

	out, err := layer.Marshal()
	if err != nil {
		die(err)
	}
	updated, err := textlayer.EmbedInArchive(data, out)
	if err != nil {
		die(err)
	}

	dest := *output
	if dest == "" {
		dest = path
	}
	if err := writeAtomic(dest, updated); err != nil {
		die(err)
	}

	fmt.Fprintf(os.Stderr, "%s: %d image(s) eligible, %d indexed (%d words) [engine=%s langs=%s]",
		dest, stats.eligible, stats.blocks, stats.words, engine.Name(), strings.Join(langs, ","))
	if stats.unmatched+stats.vector+stats.failed+stats.empty > 0 {
		fmt.Fprintf(os.Stderr, "; skipped: %d unmatched, %d vector, %d failed, %d without text",
			stats.unmatched, stats.vector, stats.failed, stats.empty)
	}
	fmt.Fprintln(os.Stderr)
}

// ocrBlock converts one image's OCR lines into a text-layer block: the
// text is a SINGLE paragraph (lines joined with spaces — web images give
// us no canonical structure, and a wrapped sentence must stay matchable
// across its line break), words carry page-coordinate boxes derived from
// the image's recorded box, and the block links back to the image by
// hash so a viewer can anchor highlights to the element.
func ocrBlock(id int, img textlayer.Image, lines []std.Line) (textlayer.Block, bool) {
	var text strings.Builder
	var words []textlayer.Word
	var confSum float64
	var confN int

	for _, line := range lines {
		if line.Confidence > 0 {
			confSum += line.Confidence
			confN++
		}
		for _, w := range line.Words {
			t := strings.TrimSpace(w.Text)
			if t == "" {
				continue
			}
			if text.Len() > 0 {
				text.WriteByte(' ')
			}
			start := utf16Length(text.String())
			text.WriteString(t)
			words = append(words, textlayer.Word{
				Start: start,
				End:   start + utf16Length(t),
				Box: textlayer.Box{
					img.Box[0] + int(w.Left*float64(img.Box[2])),
					img.Box[1] + int(w.Top*float64(img.Box[3])),
					int(w.Width * float64(img.Box[2])),
					int(w.Height * float64(img.Box[3])),
				},
			})
		}
	}
	if len(words) == 0 {
		return textlayer.Block{}, false
	}

	block := textlayer.Block{
		ID:     id,
		Type:   "img",
		Node:   img.Node,
		Frame:  img.Frame,
		Box:    img.Box,
		Text:   text.String(),
		Words:  words,
		Source: "ocr",
		Image:  img.Hash,
	}
	if confN > 0 {
		block.Confidence = confSum / float64(confN)
	}
	return block, true
}

// archiveImage is one decodable image found in the archive.
type archiveImage struct {
	mime string
	data []byte
}

// imageBytesByHash scans the archive for base64 data-URI images and maps
// the extractor's hash of each src string to the decoded bytes.
func imageBytesByHash(html []byte) map[string]archiveImage {
	re := regexp.MustCompile(`data:image/([a-zA-Z.+-]+);base64,[A-Za-z0-9+/=]+`)
	out := map[string]archiveImage{}
	for _, m := range re.FindAllSubmatch(html, -1) {
		src := string(m[0])
		h := textlayer.FNV1a(src)
		if _, seen := out[h]; seen {
			continue
		}
		comma := strings.IndexByte(src, ',')
		data, err := base64.StdEncoding.DecodeString(src[comma+1:])
		if err != nil {
			continue
		}
		out[h] = archiveImage{mime: string(m[1]), data: data}
	}
	return out
}

// parseLangs resolves OCR language hints: the -lang flag, else the
// archive's <html lang>, else en-US.
func parseLangs(flagVal string, html []byte) []string {
	if flagVal != "" {
		var langs []string
		for _, l := range strings.Split(flagVal, ",") {
			if l = strings.TrimSpace(l); l != "" {
				langs = append(langs, l)
			}
		}
		if len(langs) > 0 {
			return langs
		}
	}
	if m := regexp.MustCompile(`<html[^>]*\slang=["']?([A-Za-z-]+)`).FindSubmatch(html); m != nil {
		return []string{string(m[1])}
	}
	return []string{"en-US"}
}

// utf16Length is the length of s in UTF-16 code units — the offset space
// of the text layer.
func utf16Length(s string) int {
	n := 0
	for _, r := range s {
		if r > 0xFFFF {
			n += 2
		} else {
			n++
		}
	}
	return n
}

// writeAtomic replaces dest via a temp file + rename so a failed index
// never corrupts an archive.
func writeAtomic(dest string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".parch-index-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), dest)
}
