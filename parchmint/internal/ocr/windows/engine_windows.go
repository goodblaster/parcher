//go:build windows

package windows

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/draw"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode/utf16"

	_ "golang.org/x/image/webp"

	"github.com/goodblaster/parcher/parchmint/internal/ocr"
)

func init() {
	ocr.Register(Name, func() (ocr.Recognizer, error) { return engine{}, nil })
}

type engine struct{}

func (engine) Name() string { return Name }

// Recognize writes the batch to a temporary directory, runs the script once
// over all of it, and reads the results back.
func (engine) Recognize(ctx context.Context, images []ocr.Image, langs []string) ([]ocr.Result, error) {
	if len(images) == 0 {
		return nil, nil
	}
	dir, err := os.MkdirTemp("", "parch-ocr-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	results := make(map[string]ocr.Result, len(images))
	var list strings.Builder
	for i, img := range images {
		data, ext, err := prepare(img)
		if err != nil {
			results[img.ID] = ocr.Result{ID: img.ID, Err: err}
			continue
		}
		path := filepath.Join(dir, fmt.Sprintf("%d%s", i, ext))
		if err := os.WriteFile(path, data, 0o600); err != nil {
			return nil, err
		}
		fmt.Fprintf(&list, "%s\t%s\r\n", img.ID, path)
	}
	listPath, outPath := filepath.Join(dir, "list.txt"), filepath.Join(dir, "out.jsonl")
	if err := os.WriteFile(listPath, []byte(list.String()), 0o600); err != nil {
		return nil, err
	}

	lang := ""
	if len(langs) > 0 {
		lang = langs[0]
	}
	// The script travels on the command line (-EncodedCommand: base64 of
	// UTF-16LE), so no .ps1 file exists for the execution policy to refuse
	// and none of standard input's line-at-a-time parsing applies.
	cmd := exec.CommandContext(ctx, powershell(), "-NoProfile", "-NonInteractive", "-NoLogo", "-EncodedCommand", encodedScript())
	cmd.Env = append(os.Environ(), "PARCH_OCR_LIST="+listPath, "PARCH_OCR_OUT="+outPath, "PARCH_OCR_LANG="+lang)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	runErr := cmd.Run()

	out, _ := os.ReadFile(outPath)
	parsed, err := parseOutput(out)
	if err != nil {
		return nil, err
	}
	if ctx.Err() != nil {
		runErr = ctx.Err()
	}
	for _, r := range parsed {
		results[r.ID] = r
	}
	if runErr != nil && len(parsed) == 0 {
		detail := strings.TrimSpace(stderr.String())
		if len(detail) > 400 {
			detail = detail[:400]
		}
		return nil, fmt.Errorf("windows ocr: PowerShell failed (%v): %s", runErr, detail)
	}

	ordered := make([]ocr.Result, 0, len(images))
	for _, img := range images {
		r, ok := results[img.ID]
		if !ok {
			// The script stopped before reaching this image.
			r = ocr.Result{ID: img.ID, Err: errors.New("windows ocr: no result")}
			if runErr != nil {
				r.Err = fmt.Errorf("windows ocr: no result (%v)", runErr)
			}
		}
		ordered = append(ordered, r)
	}
	return ordered, nil
}

// encodedScript is the script as -EncodedCommand wants it.
func encodedScript() string {
	units := utf16.Encode([]rune(script))
	raw := make([]byte, 2*len(units))
	for i, u := range units {
		binary.LittleEndian.PutUint16(raw[2*i:], u)
	}
	return base64.StdEncoding.EncodeToString(raw)
}

// powershell locates Windows PowerShell 5.1. Not pwsh: PowerShell 7 cannot
// load WinRT types.
func powershell() string {
	if root := os.Getenv("SystemRoot"); root != "" {
		p := filepath.Join(root, "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "powershell.exe"
}

// prepare returns the bytes to hand Windows and the file extension for
// them. JPEG goes through untouched. Everything else is decoded, flattened
// onto WHITE and re-encoded as PNG: Windows may have no WebP codec, and a
// transparent logo decoded straight to pixels comes out as dark text on
// black, which no OCR reads.
func prepare(img ocr.Image) ([]byte, string, error) {
	if img.MIME == "image/jpeg" || img.MIME == "image/jpg" {
		return img.Data, ".jpg", nil
	}
	src, _, err := image.Decode(bytes.NewReader(img.Data))
	if err != nil {
		return nil, "", fmt.Errorf("windows ocr: cannot decode %s image: %w", img.MIME, err)
	}
	flat := image.NewRGBA(src.Bounds())
	draw.Draw(flat, flat.Bounds(), image.White, image.Point{}, draw.Src)
	draw.Draw(flat, flat.Bounds(), src, src.Bounds().Min, draw.Over)
	var buf bytes.Buffer
	if err := png.Encode(&buf, flat); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), ".png", nil
}
