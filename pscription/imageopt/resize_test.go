package imageopt

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"testing"
)

func TestResizePreservesAlphaAndBounds(t *testing.T) {
	im := image.NewNRGBA(image.Rect(0, 0, 800, 400))
	for y := 0; y < 400; y++ {
		for x := 0; x < 800; x++ {
			im.SetNRGBA(x, y, color.NRGBA{uint8(x), uint8(y), 60, 128})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, im); err != nil {
		t.Fatal(err)
	}
	src := "data:image/png;base64," + base64.StdEncoding.EncodeToString(b.Bytes())
	uri, w, h := Resize(src, 100, 100)
	if uri == "" || w != 200 || h != 100 {
		t.Fatalf("cover target got %dx%d, uri len %d", w, h, len(uri))
	}
	raw, _ := base64.StdEncoding.DecodeString(uri[len("data:image/png;base64,"):])
	decoded, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, alpha := decoded.At(50, 50).RGBA()
	if alpha < 32000 || alpha > 34000 {
		t.Fatalf("lost alpha: %d", alpha)
	}
	for _, size := range [][2]int{{0, 20}, {800, 400}, {1600, 800}, {-1, 2}} {
		if uri, _, _ := Resize(src, size[0], size[1]); uri != "" {
			t.Fatalf("changed image for size %v", size)
		}
	}
	if again, _, _ := Resize(uri, w, h); again != "" {
		t.Fatal("repeat pass changed image")
	}
}

func TestResizeKeepsUnsupportedAndAnimated(t *testing.T) {
	im := image.NewPaletted(image.Rect(0, 0, 80, 80), color.Palette{color.Black, color.White})
	var b bytes.Buffer
	if err := gif.EncodeAll(&b, &gif.GIF{Image: []*image.Paletted{im, im}, Delay: []int{10, 10}}); err != nil {
		t.Fatal(err)
	}
	for _, src := range []string{"data:image/gif;base64," + base64.StdEncoding.EncodeToString(b.Bytes()), "data:image/svg+xml;base64,PHN2Zy8+", "data:image/png;base64,not-an-image"} {
		if out, _, _ := Resize(src, 10, 10); out != "" {
			t.Fatal("changed unsupported or animated image")
		}
	}
}

func TestResizePreservesPNGColorMetadataAndAnimation(t *testing.T) {
	im := image.NewRGBA(image.Rect(0, 0, 800, 400))
	for y := 0; y < 400; y++ {
		for x := 0; x < 800; x++ {
			im.SetRGBA(x, y, color.RGBA{uint8(x), uint8(y), 80, 255})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, im); err != nil {
		t.Fatal(err)
	}
	chunk := func(name string, payload []byte) []byte {
		out := make([]byte, len(payload)+12)
		binary.BigEndian.PutUint32(out, uint32(len(payload)))
		copy(out[4:], name)
		copy(out[8:], payload)
		binary.BigEndian.PutUint32(out[len(out)-4:], crc32.ChecksumIEEE(out[4:len(out)-4]))
		return out
	}
	for _, name := range []string{"gAMA", "acTL"} {
		meta := chunk(name, []byte{0, 0, 0xb1, 0x8f})
		raw := append(append(append([]byte{}, b.Bytes()[:33]...), meta...), b.Bytes()[33:]...)
		src := "data:image/png;base64," + base64.StdEncoding.EncodeToString(raw)
		resized, _, _ := Resize(src, 100, 50)
		if name == "acTL" {
			if resized != "" {
				t.Fatal("flattened an animated PNG")
			}
			continue
		}
		if resized == "" {
			t.Fatal("ordinary colour metadata prevented resizing")
		}
		decoded, _ := base64.StdEncoding.DecodeString(resized[len("data:image/png;base64,"):])
		if !bytes.Contains(decoded, meta) {
			t.Fatal("lost colour metadata")
		}
	}
}
