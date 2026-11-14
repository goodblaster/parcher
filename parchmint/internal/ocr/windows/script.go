package windows

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"strings"

	"github.com/goodblaster/parcher/parchmint/internal/ocr"
	"github.com/goodblaster/parcher/parchmint/internal/ocr/std"
)

// script is the PowerShell program run on Windows. It lives in an untagged
// file, with the parser for its output, so both can be tested anywhere.
//
// In:  $env:PARCH_OCR_LIST — a UTF-8 file, one "id<TAB>path" per line
//
//	$env:PARCH_OCR_LANG — BCP 47 tag to prefer (may be empty)
//
// Out: $env:PARCH_OCR_OUT  — UTF-8, one JSON object per line:
//
//	{"id":…,"width":W,"height":H,"lines":[{"words":[{"text":…,"x":…,"y":…,"w":…,"h":…}]}]}
//	{"id":…,"error":"…"}        one image failed
//	{"fatal":"…"}               nothing can be read (no OCR language)
//
// Boxes are pixels; the Go side divides by width and height.
const script = `
$ErrorActionPreference = 'Stop'
$utf8 = New-Object System.Text.UTF8Encoding $false
$out = $env:PARCH_OCR_OUT
function Emit($obj) {
  [System.IO.File]::AppendAllText($out, (($obj | ConvertTo-Json -Compress -Depth 8) + "` + "`n" + `"), $utf8)
}
try {
  Add-Type -AssemblyName System.Runtime.WindowsRuntime
  $null = [Windows.Media.Ocr.OcrEngine, Windows.Foundation, ContentType = WindowsRuntime]
  $null = [Windows.Graphics.Imaging.BitmapDecoder, Windows.Foundation, ContentType = WindowsRuntime]
  $null = [Windows.Storage.StorageFile, Windows.Storage, ContentType = WindowsRuntime]
  $null = [Windows.Storage.Streams.IRandomAccessStream, Windows.Storage.Streams, ContentType = WindowsRuntime]
  $null = [Windows.Globalization.Language, Windows.Globalization, ContentType = WindowsRuntime]

  # WinRT calls return IAsyncOperation<T>; PowerShell 5.1 cannot await one,
  # so each is converted to a .NET Task and waited on.
  $asTask = ([System.WindowsRuntimeSystemExtensions].GetMethods() | Where-Object {
    $_.Name -eq 'AsTask' -and $_.GetParameters().Count -eq 1 -and
    $_.GetParameters()[0].ParameterType.Name -eq 'IAsyncOperation` + "`" + `1' })[0]
  function Await($op, [Type]$type) {
    $task = $asTask.MakeGenericMethod($type).Invoke($null, @($op))
    $task.Wait(-1) | Out-Null
    $task.Result
  }

  $engine = $null
  if ($env:PARCH_OCR_LANG) {
    try {
      $lang = New-Object Windows.Globalization.Language $env:PARCH_OCR_LANG
      if ([Windows.Media.Ocr.OcrEngine]::IsLanguageSupported($lang)) {
        $engine = [Windows.Media.Ocr.OcrEngine]::TryCreateFromLanguage($lang)
      }
    } catch { }
  }
  if (-not $engine) { $engine = [Windows.Media.Ocr.OcrEngine]::TryCreateFromUserProfileLanguages() }
  if (-not $engine) {
    foreach ($lang in [Windows.Media.Ocr.OcrEngine]::AvailableRecognizerLanguages) {
      $engine = [Windows.Media.Ocr.OcrEngine]::TryCreateFromLanguage($lang)
      if ($engine) { break }
    }
  }
  if (-not $engine) {
    Emit @{ fatal = 'Windows has no OCR language installed. Add one in Settings > Time & language > Language & region (a language with the "Optical character recognition" feature), or run: Add-WindowsCapability -Online -Name "Language.OCR~~~en-US~0.0.1.0"' }
    exit 3
  }
  $maxDim = [Windows.Media.Ocr.OcrEngine]::MaxImageDimension
} catch {
  Emit @{ fatal = ('Windows OCR is not available: ' + $_.Exception.Message) }
  exit 3
}

foreach ($entry in [System.IO.File]::ReadAllLines($env:PARCH_OCR_LIST, $utf8)) {
  if (-not $entry) { continue }
  $id, $path = $entry -split "` + "`t" + `", 2
  try {
    $file = Await ([Windows.Storage.StorageFile]::GetFileFromPathAsync($path)) ([Windows.Storage.StorageFile])
    $stream = Await ($file.OpenAsync([Windows.Storage.FileAccessMode]::Read)) ([Windows.Storage.Streams.IRandomAccessStream])
    $decoder = Await ([Windows.Graphics.Imaging.BitmapDecoder]::CreateAsync($stream)) ([Windows.Graphics.Imaging.BitmapDecoder])
    if ($decoder.PixelWidth -gt $maxDim -or $decoder.PixelHeight -gt $maxDim) {
      throw ("image is {0}x{1}; Windows OCR reads at most {2} pixels on a side" -f $decoder.PixelWidth, $decoder.PixelHeight, $maxDim)
    }
    $bitmap = Await ($decoder.GetSoftwareBitmapAsync([Windows.Graphics.Imaging.BitmapPixelFormat]::Bgra8, [Windows.Graphics.Imaging.BitmapAlphaMode]::Premultiplied)) ([Windows.Graphics.Imaging.SoftwareBitmap])
    $result = Await ($engine.RecognizeAsync($bitmap)) ([Windows.Media.Ocr.OcrResult])
    $lines = @()
    foreach ($line in $result.Lines) {
      $words = @()
      foreach ($word in $line.Words) {
        $r = $word.BoundingRect
        $words += ,@{ text = $word.Text; x = $r.X; y = $r.Y; w = $r.Width; h = $r.Height }
      }
      $lines += ,@{ words = $words }
    }
    Emit @{ id = $id; width = $decoder.PixelWidth; height = $decoder.PixelHeight; lines = $lines }
    $stream.Dispose()
  } catch {
    Emit @{ id = $id; error = $_.Exception.Message }
  }
}
`

// parseOutput turns the script's output file into results, one per image
// the script reported. A "fatal" record is returned as the batch error.
func parseOutput(out []byte) ([]ocr.Result, error) {
	type word struct {
		Text       string
		X, Y, W, H float64
	}
	type record struct {
		ID            string `json:"id"`
		Fatal, Error  string
		Width, Height float64
		// ConvertTo-Json writes a one-element array as a bare object in some
		// PowerShell builds, so both shapes are accepted (see oneOrMany).
		Lines oneOrMany[struct {
			Words oneOrMany[word]
		}]
	}

	var results []ocr.Result
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 0, 1<<20), 64<<20)
	for sc.Scan() {
		raw := bytes.TrimSpace(bytes.TrimPrefix(sc.Bytes(), []byte("\xef\xbb\xbf")))
		if len(raw) == 0 {
			continue
		}
		var rec record
		if err := json.Unmarshal(raw, &rec); err != nil {
			return nil, errors.New("windows ocr: unreadable output from PowerShell: " + err.Error())
		}
		switch {
		case rec.Fatal != "":
			return nil, errors.New(rec.Fatal)
		case rec.Error != "":
			results = append(results, ocr.Result{ID: rec.ID, Err: errors.New("windows ocr: " + rec.Error)})
			continue
		}
		res := ocr.Result{ID: rec.ID}
		if rec.Width > 0 && rec.Height > 0 {
			for _, l := range rec.Lines {
				var line std.Line
				for _, w := range l.Words {
					if strings.TrimSpace(w.Text) == "" {
						continue
					}
					line.Words = append(line.Words, std.Word{
						Text:   w.Text,
						Left:   w.X / rec.Width,
						Top:    w.Y / rec.Height,
						Width:  w.W / rec.Width,
						Height: w.H / rec.Height,
					})
				}
				if len(line.Words) > 0 {
					line.SetBounds()
					res.Lines = append(res.Lines, line)
				}
			}
		}
		results = append(results, res)
	}
	return results, sc.Err()
}

// oneOrMany decodes a JSON array, or a single value standing in for a
// one-element array, or null.
type oneOrMany[T any] []T

func (o *oneOrMany[T]) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	if b[0] == '[' {
		return json.Unmarshal(b, (*[]T)(o))
	}
	var one T
	if err := json.Unmarshal(b, &one); err != nil {
		return err
	}
	*o = []T{one}
	return nil
}
