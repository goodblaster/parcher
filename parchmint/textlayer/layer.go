// Package textlayer reads and searches the parchmint-text layer embedded
// in HTML archives. See TEXTLAYER.md for the format contract. This is the
// library half of `parch text` / `parch find`, and what a future catalog
// application indexes.
package textlayer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"unicode/utf16"
)

const (
	// ScriptType is the <script type> attribute identifying the embedded
	// layer inside an archive.
	ScriptType = "application/x-parchmint-text"

	// Format is the layer format identifier this package understands.
	Format = "parchmint-text/1"
)

// Layer is the embedded text layer of one archive.
type Layer struct {
	Format     string          `json:"format"`
	URL        string          `json:"url"`
	CapturedAt string          `json:"capturedAt"`
	Generator  string          `json:"generator"`
	Viewport   Viewport        `json:"viewport"`
	Stats      json.RawMessage `json:"stats"`
	Blocks     []Block         `json:"blocks"`
	Images     []Image         `json:"images,omitempty"`
}

// Image is a visible image recorded at capture time so `parch index` can
// OCR the archive's images and place words in page coordinates without a
// browser. Hash is FNV-1a over the src string (see FNV1a), pairing the
// entry with the archive's data-URI bytes.
type Image struct {
	Node    string `json:"node"`
	Frame   string `json:"frame,omitempty"`
	Box     Box    `json:"box"`
	Natural [2]int `json:"natural"`
	Hash    string `json:"hash"`
}

type Viewport struct {
	Width int     `json:"width"`
	DPR   float64 `json:"dpr"`
}

// Block is one paragraph-level unit: the phrase boundary for matching.
// OCR blocks (added by `parch index`) carry Source "ocr", the source
// image's hash in Image, and a recognition Confidence; their words'
// boxes are page coordinates derived from the image's recorded box.
type Block struct {
	ID         int     `json:"id"`
	Type       string  `json:"type"`
	Node       string  `json:"node"`
	Frame      string  `json:"frame,omitempty"`
	Box        Box     `json:"box"`
	Text       string  `json:"text"`
	Words      []Word  `json:"words"`
	Oracle     string  `json:"oracle,omitempty"`
	Source     string  `json:"source,omitempty"`
	Image      string  `json:"image,omitempty"`
	Confidence float64 `json:"confidence,omitempty"`
}

// Box is [x, y, w, h] in page-space CSS pixels at the recorded viewport.
type Box [4]int

// Word annotates Block.Text: [Start, End) are UTF-16 code-unit offsets
// (JavaScript string semantics, as written by the extractor).
type Word struct {
	Start, End int
	Box        Box
}

// MarshalJSON emits the compact array form the format uses.
func (w Word) MarshalJSON() ([]byte, error) {
	return json.Marshal([6]int{w.Start, w.End, w.Box[0], w.Box[1], w.Box[2], w.Box[3]})
}

// UnmarshalJSON decodes the compact array form; per the format contract,
// trailing elements beyond the sixth are ignored (reserved for later
// versions, e.g. OCR source tags).
func (w *Word) UnmarshalJSON(data []byte) error {
	var arr []float64
	if err := json.Unmarshal(data, &arr); err != nil {
		return err
	}
	if len(arr) < 6 {
		return fmt.Errorf("word entry has %d elements, want >= 6", len(arr))
	}
	w.Start, w.End = int(arr[0]), int(arr[1])
	w.Box = Box{int(arr[2]), int(arr[3]), int(arr[4]), int(arr[5])}
	return nil
}

// FromFile extracts and parses the layer embedded in an archive file
// (HTML or MHT — sniffed from the bytes).
func FromFile(path string) (*Layer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return FromBytes(data, path)
}

// FromBytes extracts and parses the layer from archive bytes — HTML, MHT,
// or PDF (sniffed from the bytes). name is used in error messages only.
func FromBytes(data []byte, name string) (*Layer, error) {
	switch {
	case IsPDF(data):
		raw, err := pdfLayerBytes(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w (exported without a text layer or an older parch?)", name, err)
		}
		return parseLayer(raw, name)
	case IsMHT(data):
		raw, err := mhtLayerBytes(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w (captured with -text=false or an older parch?)", name, err)
		}
		return parseLayer(raw, name)
	default:
		return FromHTML(data, name)
	}
}

// FromHTML extracts and parses the layer from HTML archive bytes.
func FromHTML(html []byte, name string) (*Layer, error) {
	open := []byte(`<script type="` + ScriptType + `">`)
	start := bytes.Index(html, open)
	if start == -1 {
		return nil, fmt.Errorf("%s has no embedded text layer (captured with -text=false, an older parch, or not an archive)", name)
	}
	start += len(open)
	// The payload contains no raw `</` (escaped to `<\/` at embed time),
	// so the first closing script tag is exact; json.Unmarshal undoes the
	// escape for free.
	end := bytes.Index(html[start:], []byte("</script>"))
	if end == -1 {
		return nil, fmt.Errorf("%s: text layer is truncated", name)
	}
	return parseLayer(html[start:start+end], name)
}

func parseLayer(raw []byte, name string) (*Layer, error) {
	var layer Layer
	if err := json.Unmarshal(raw, &layer); err != nil {
		return nil, fmt.Errorf("%s: invalid text layer: %w", name, err)
	}
	if !strings.HasPrefix(layer.Format, "parchmint-text/") {
		return nil, fmt.Errorf("%s: unknown layer format %q", name, layer.Format)
	}
	return &layer, nil
}

// EmbedInArchive embeds layerJSON in archive bytes of any format,
// replacing an existing layer.
func EmbedInArchive(data, layerJSON []byte) ([]byte, error) {
	switch {
	case IsPDF(data):
		return EmbedInPDF(data, layerJSON)
	case IsMHT(data):
		return EmbedInMHT(data, layerJSON)
	default:
		return EmbedInHTML(data, layerJSON), nil
	}
}

// Marshal serializes the layer back to its JSON document form (compact
// word arrays included), for re-embedding after modification.
func (l *Layer) Marshal() ([]byte, error) {
	return json.Marshal(l)
}

// FNV1a is a 32-bit FNV-1a hash over the string's UTF-16 code units, as
// lowercase hex — kept in lockstep with fnv1a() in text_layer.js, which
// stamps Image.Hash at capture time; index-time re-hashes the archive's
// img srcs with this to pair them.
func FNV1a(s string) string {
	h := uint32(2166136261)
	var buf [2]uint16
	for _, r := range s {
		if r > 0xFFFF {
			r1, r2 := utf16.EncodeRune(r)
			buf[0], buf[1] = uint16(r1), uint16(r2)
			for _, cu := range buf {
				h ^= uint32(cu)
				h *= 16777619
			}
		} else {
			h ^= uint32(r)
			h *= 16777619
		}
	}
	return fmt.Sprintf("%08x", h)
}

// EmbedInHTML returns archive bytes carrying layerJSON as the typed
// <script> element: an existing layer is replaced in place; otherwise the
// element is spliced before </body> (appended when the tag is absent —
// SingleFile's compressHTML omits it). Every `</` in the JSON is escaped
// to `<\/` — a no-op JSON string escape — so the payload cannot terminate
// its own script element; json.Unmarshal undoes it for free.
func EmbedInHTML(html, layerJSON []byte) []byte {
	esc := bytes.ReplaceAll(layerJSON, []byte("</"), []byte(`<\/`))

	open := []byte(`<script type="` + ScriptType + `">`)
	if start := bytes.Index(html, open); start != -1 {
		payloadStart := start + len(open)
		if rel := bytes.Index(html[payloadStart:], []byte("</script>")); rel != -1 {
			out := make([]byte, 0, payloadStart+len(esc)+len(html)-payloadStart-rel)
			out = append(out, html[:payloadStart]...)
			out = append(out, esc...)
			out = append(out, html[payloadStart+rel:]...)
			return out
		}
	}

	var tag bytes.Buffer
	tag.Grow(len(esc) + 64)
	tag.Write(open)
	tag.Write(esc)
	tag.WriteString("</script>")

	idx := bytes.LastIndex(bytes.ToLower(html), []byte("</body>"))
	if idx == -1 {
		return append(append([]byte{}, html...), tag.Bytes()...)
	}
	out := make([]byte, 0, len(html)+tag.Len())
	out = append(out, html[:idx]...)
	out = append(out, tag.Bytes()...)
	out = append(out, html[idx:]...)
	return out
}

// UTF16Slice returns the substring of s covering UTF-16 code units
// [start, end) — the offset space Block.Text annotations live in.
func UTF16Slice(s string, start, end int) string {
	var b strings.Builder
	pos := 0
	for _, r := range s {
		n := utf16.RuneLen(r)
		if n < 0 {
			n = 1
		}
		if pos >= end {
			break
		}
		if pos >= start {
			b.WriteRune(r)
		}
		pos += n
	}
	return b.String()
}
