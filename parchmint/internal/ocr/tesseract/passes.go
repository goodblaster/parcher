package tesseract

import (
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"os"
	"strconv"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"

	"github.com/goodblaster/parcher/parchmint/internal/ocr/std"
)

// The derived copies of an image that the second and third passes read, and
// how their words are merged with the first pass's.

// defaultPasses: see passes.
const defaultPasses = 3

// passes is how many times tesseract reads each image: 1 is the image as
// given, 2 adds the darkest-channel copy, 3 adds that copy at a third of the
// size. PARCH_TESSERACT_PASSES overrides, for tuning or for speed.
func passes() int {
	if v, err := strconv.Atoi(os.Getenv("PARCH_TESSERACT_PASSES")); err == nil && v >= 1 && v <= 3 {
		return v
	}
	return defaultPasses
}

// passImages writes the derived copies of an image that the extra passes
// read, and returns their paths plus a func that removes them.
//
// Why a derived copy at all: tesseract reduces a colour image to black and
// white before reading it, and saturated text on a light ground can vanish
// in that step. Measured on a banner with pure red digits on pale blue — in
// the red channel the digits are BRIGHTER than the background, and
// tesseract's binarized working image had the photograph in it and nothing
// where the digits were. No layout mode recovers text that is no longer in
// the picture. Taking each pixel's darkest channel turns any saturated
// colour dark while leaving a light background light, so the text survives.
//
// Why a reduced copy: the recognizer is trained on body-size type and reads
// display type (letters ~100px tall) badly or not at all; the same digits
// were read at a third of the size and not at full size.
//
// Measured against Apple Vision on the same two archives as above, recaptured
// (found / correct):
//
//	1 pass                    0.39 / 0.85    0.12 / 0.50
//	2 passes                  0.44 / 0.82    0.18 / 0.44
//	3 passes (the default)    0.44 / 0.81    0.18 / 0.42
//
// The third pass barely moves the totals; it is there for large display
// type, which is exactly what a hero banner's title is, and it is what reads
// the red digits. Each pass is one more tesseract run per image.
func passImages(filename string) (files []string, cleanup func()) {
	cleanup = func() {
		for _, f := range files {
			os.Remove(f)
		}
	}
	n := passes()
	if n < 2 {
		return nil, cleanup
	}
	f, err := os.Open(filename)
	if err != nil {
		return nil, cleanup
	}
	src, _, err := image.Decode(f)
	f.Close()
	if err != nil {
		return nil, cleanup
	}

	dark := darkestChannel(src)
	write := func(img image.Image) {
		tmp, err := os.CreateTemp("", "tesseract-pass-*.png")
		if err != nil {
			return
		}
		err = png.Encode(tmp, img)
		tmp.Close()
		if err != nil {
			os.Remove(tmp.Name())
			return
		}
		files = append(files, tmp.Name())
	}
	write(dark)

	// Shrinking a small image only destroys it; display type lives in big ones.
	b := dark.Bounds()
	if n >= 3 && b.Dx() >= 3*minReducedSide && b.Dy() >= 3*minReducedSide {
		small := image.NewGray(image.Rect(0, 0, b.Dx()/3, b.Dy()/3))
		draw.CatmullRom.Scale(small, small.Bounds(), dark, b, draw.Src, nil)
		write(small)
	}
	return files, cleanup
}

// minReducedSide is the smallest side a reduced copy may have.
const minReducedSide = 100

// darkestChannel returns a grayscale image holding, per pixel, the minimum
// of R, G and B after compositing over white (transparent areas of a logo
// are page background, not black).
func darkestChannel(src image.Image) *image.Gray {
	b := src.Bounds()
	out := image.NewGray(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, a := src.At(x, y).RGBA() // alpha-premultiplied, 0..0xffff
			m := min(r, g, bl) + (0xffff - a)  // over white
			out.Pix[(y-b.Min.Y)*out.Stride+(x-b.Min.X)] = uint8(min(m, 0xffff) >> 8)
		}
	}
	return out
}

// mergeLines adds to base the words of more that sit where base read
// nothing. A word that overlaps one already present is the same piece of the
// image read twice, and the earlier pass wins.
func mergeLines(base, more []std.Line) []std.Line {
	var have []std.Word
	for _, l := range base {
		have = append(have, l.Words...)
	}
	for _, l := range more {
		var keep []std.Word
		for _, w := range l.Words {
			dup := false
			for _, h := range have {
				if overlaps(w, h) {
					dup = true
					break
				}
			}
			if !dup {
				keep = append(keep, w)
			}
		}
		if len(keep) == 0 {
			continue
		}
		l.Words = keep
		l.SetBounds()
		base = append(base, l)
		have = append(have, keep...)
	}
	return base
}

// overlaps reports whether two word boxes share more than a third of the
// smaller one's area.
func overlaps(a, b std.Word) bool {
	w := min(a.Left+a.Width, b.Left+b.Width) - max(a.Left, b.Left)
	h := min(a.Top+a.Height, b.Top+b.Height) - max(a.Top, b.Top)
	if w <= 0 || h <= 0 {
		return false
	}
	return w*h > min(a.Width*a.Height, b.Width*b.Height)/3
}

// imageSize reads the pixel dimensions of an image file (png/jpeg/gif/webp).
func imageSize(filename string) (int, int, error) {
	f, err := os.Open(filename)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return 0, 0, err
	}
	return cfg.Width, cfg.Height, nil
}
