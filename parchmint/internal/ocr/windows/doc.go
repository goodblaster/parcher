// Package windows is OCR through Windows.Media.Ocr, the engine Windows 10
// and 11 ship with — so `parch ocr` there needs nothing installed, as Apple
// Vision needs nothing on macOS.
//
// # How it works
//
// The API is WinRT, reachable from Go only through cgo and C++/WinRT or raw
// COM. Neither is used. Windows PowerShell 5.1 — also part of every Windows
// 10/11 install — can load WinRT types directly, so the engine starts ONE
// powershell.exe for a whole batch, passes it the script (script.go) as an
// encoded command, and reads one JSON object per image back from a file. That keeps the
// Windows build pure Go (it cross-compiles from anywhere) and costs one
// process start per `parch ocr`, not per image.
//
// It has to be powershell.exe specifically: PowerShell 7 (pwsh) dropped
// WinRT type loading.
//
// # What it cannot do
//
//   - Report confidence: the API has none, so Line.Confidence stays zero
//     rather than being invented.
//   - Read a language whose OCR pack is not installed. Windows installs OCR
//     data per display language; the script uses the page's language when
//     the pack is there, else the user's own languages, else any pack
//     present, and fails with what to install when there is none (Windows
//     Server often has none).
//   - Read images larger than OcrEngine.MaxImageDimension on a side.
//
// Everything outside this file and script.go is `//go:build windows`; in any
// other build the package is inert and "windows" is not a registered
// engine.
package windows

// Name is the engine's name on the command line.
const Name = "windows"
