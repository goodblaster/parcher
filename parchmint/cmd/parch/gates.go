package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/goodblaster/parcher/parchmint/capture"
)

const captureMetadataSchema = "parch-metadata/1"

// Capture metadata has room for information beyond access gates. Paths are
// basenames relative to the metadata file, so moving the pair preserves --clear.
type captureMetadata struct {
	capture.Report
	Format      string `json:"format"`
	ClearOutput string `json:"clear_output"`
}

func gatedFilename(path string) string {
	ext := filepath.Ext(path)
	if ext == filepath.Base(path) {
		ext = ""
	}
	stem := strings.TrimSuffix(path, ext)
	if ext == ".gated" || strings.HasSuffix(stem, ".gated") {
		return path
	}
	return stem + ".gated" + ext
}

func plainFilename(path string) string {
	ext := filepath.Ext(path)
	if ext == ".gated" {
		return strings.TrimSuffix(path, ext)
	}
	stem := strings.TrimSuffix(path, ext)
	return strings.TrimSuffix(stem, ".gated") + ext
}

func metadataFilename(archive string) string {
	normal := plainFilename(archive)
	ext := filepath.Ext(normal)
	if ext == filepath.Base(normal) {
		ext = ""
	}
	return strings.TrimSuffix(normal, ext) + ".json"
}

func newCaptureMetadata(snap *capture.Snapshot, normalOutput, format string) captureMetadata {
	normalOutput = plainFilename(normalOutput)
	r := snap.Report(filepath.Base(gatedFilename(normalOutput)))
	r.Schema = captureMetadataSchema
	return captureMetadata{Report: r, Format: format, ClearOutput: filepath.Base(normalOutput)}
}

func readClearMetadata(path string) (*captureMetadata, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read --clear metadata file: %w", err)
	}
	var g captureMetadata
	if err := json.Unmarshal(b, &g); err != nil {
		return nil, fmt.Errorf("read --clear metadata file: %w", err)
	}
	if g.Schema != captureMetadataSchema && g.Schema != "parch-gate/1" {
		return nil, fmt.Errorf("--clear requires a %s metadata file", captureMetadataSchema)
	}
	u, err := url.Parse(g.URL)
	if err != nil || u.Scheme == "" || (u.Host == "" && u.Scheme != "file") {
		return nil, fmt.Errorf("metadata file has an invalid capture URL")
	}
	if capture.Lookup(g.Format) == nil {
		return nil, fmt.Errorf("metadata file has an unknown format %q", g.Format)
	}
	if !validGateBasename(g.ClearOutput) || !validGateBasename(g.Output) || g.Output != gatedFilename(g.ClearOutput) {
		return nil, fmt.Errorf("metadata file has invalid output names")
	}
	if g.Access == nil || len(g.Access.Pending) == 0 || (g.Access.Status != "decision_required" && g.Access.Status != "preserved") {
		return nil, fmt.Errorf("metadata file contains no pending access decision")
	}
	for _, gate := range g.Access.Pending {
		if err := gate.Validate(); err != nil {
			return nil, fmt.Errorf("invalid metadata file: %w", err)
		}
	}
	return &g, nil
}

func validGateBasename(name string) bool {
	return name != "" && name != "." && name != ".." && filepath.Base(name) == name && !strings.ContainsAny(name, "/\\")
}

func (g *captureMetadata) destination(path, ext string, changeFormat bool) string {
	name := g.ClearOutput
	if changeFormat {
		oldExt := filepath.Ext(name)
		if oldExt == name {
			oldExt = ""
		}
		name = strings.TrimSuffix(name, oldExt) + ext
	}
	return filepath.Join(filepath.Dir(path), name)
}

func writeCaptureJSON(path string, value any) error {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(path, append(b, '\n'), 0600); err != nil {
		return err
	}
	return nil
}
