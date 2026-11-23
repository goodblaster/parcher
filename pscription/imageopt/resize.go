// Package imageopt reduces embedded raster images without enlarging them or
// replacing an animation with a still. Unsupported images stay byte-identical.
package imageopt

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"image"
	"image/jpeg"
	"image/png"
	"math"
	"strings"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

// Resize returns a smaller data URI and its pixel dimensions. An empty result
// means keep the original. Width/height describe the required painted pixels;
// the source aspect ratio is preserved, including for object-fit:cover.
func Resize(src string, width, height int) (uri string, w, h int) {
	if width <= 0 || height <= 0 || len(src) > 128<<20 || !strings.HasPrefix(src, "data:image/") {
		return "", 0, 0
	}
	comma := strings.IndexByte(src, ',')
	if comma < 0 || !strings.HasSuffix(src[:comma], ";base64") {
		return "", 0, 0
	}
	b, err := base64.StdEncoding.DecodeString(src[comma+1:])
	if err != nil {
		return "", 0, 0
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > 64_000_000 {
		return "", 0, 0
	}
	switch format {
	case "jpeg":
		// EXIF rotation and colour profiles must not be silently discarded.
		if bytes.Contains(b, []byte("Exif\x00\x00")) || bytes.Contains(b, []byte("ICC_PROFILE\x00")) {
			return "", 0, 0
		}
	case "png":
		if pngHasChunk(b, "acTL") || pngHasChunk(b, "iCCP") || pngHasChunk(b, "eXIf") {
			return "", 0, 0
		}
	case "webp":
		if bytes.Contains(b, []byte("ANIM")) || bytes.Contains(b, []byte("ICCP")) || bytes.Contains(b, []byte("EXIF")) {
			return "", 0, 0
		}
	default:
		return "", 0, 0
	}
	scale := math.Max(float64(width)/float64(cfg.Width), float64(height)/float64(cfg.Height))
	if scale >= 1 {
		return "", 0, 0
	}
	w = max(1, int(math.Ceil(float64(cfg.Width)*scale)))
	h = max(1, int(math.Ceil(float64(cfg.Height)*scale)))
	im, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return "", 0, 0
	}
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.CatmullRom.Scale(out, out.Bounds(), im, im.Bounds(), draw.Src, nil)
	var encoded bytes.Buffer
	mime := "image/png"
	if format == "jpeg" || (format == "webp" && out.Opaque()) {
		mime = "image/jpeg"
		err = jpeg.Encode(&encoded, out, &jpeg.Options{Quality: 90})
	} else {
		err = png.Encode(&encoded, out)
	}
	if err != nil {
		return "", 0, 0
	}
	output := encoded.Bytes()
	if format == "png" {
		// The decoder works in the stored sample space. Keep the colour
		// interpretation of those samples; dropping gAMA/cHRM changes how
		// a browser displays the re-encoded pixels even without an ICC profile.
		metadata := pngColorChunks(b)
		if len(metadata) > 0 {
			combined := make([]byte, 0, len(output)+len(metadata))
			combined = append(combined, output[:33]...) // signature + IHDR
			combined = append(combined, metadata...)
			output = append(combined, output[33:]...)
		}
	}
	if len(output) >= len(b) {
		return "", 0, 0
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(output), w, h
}

func pngColorChunks(b []byte) []byte {
	var out []byte
	for offset := 8; offset+12 <= len(b); {
		n := uint64(binary.BigEndian.Uint32(b[offset:]))
		if n+12 > uint64(len(b)-offset) {
			break
		}
		end := offset + int(n) + 12
		switch string(b[offset+4 : offset+8]) {
		case "gAMA", "cHRM", "sRGB":
			out = append(out, b[offset:end]...)
		}
		offset = end
	}
	return out
}

func pngHasChunk(b []byte, name string) bool {
	for offset := 8; offset+12 <= len(b); {
		n := uint64(binary.BigEndian.Uint32(b[offset:]))
		if n+12 > uint64(len(b)-offset) {
			return true
		}
		if string(b[offset+4:offset+8]) == name {
			return true
		}
		offset += int(n) + 12
	}
	return false
}
