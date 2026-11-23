package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/goodblaster/parcher/parchmint/capture"
	"github.com/goodblaster/parcher/pscription/actions"
)

func captureMetadataFixture() captureMetadata {
	evidence := actions.GateEvidence{URL: "https://example.com/", Kind: "age", Text: "Are you over 21?", Button: "Yes, enter", Approvable: true}
	raw, _ := json.Marshal(evidence)
	gate := actions.AccessGate{ID: fmt.Sprintf("gate-v1-%x", sha256.Sum256(raw)), GateEvidence: evidence}
	snap := capture.Snapshot{URL: evidence.URL, FinalURL: evidence.URL, MIME: "text/html", Access: &actions.AccessReport{
		Status: "decision_required", Pending: []actions.AccessGate{gate}, Cleared: []actions.AccessGate{},
	}}
	return newCaptureMetadata(&snap, "captures/page.html", "html")
}

func TestGateFileValidation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "page.json")
	valid := captureMetadataFixture()
	if err := writeCaptureJSON(path, valid); err != nil {
		t.Fatal(err)
	}
	got, err := readClearMetadata(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.destination(path, ".html", false) != filepath.Join(dir, "page.html") {
		t.Fatal("destination did not move with gate file")
	}
	if got.destination(path, ".pdf", true) != filepath.Join(dir, "page.pdf") {
		t.Fatal("format override retained old extension")
	}
	cases := map[string]func(*captureMetadata){
		"schema":           func(g *captureMetadata) { g.Schema = "parch-capture/1" },
		"missing-url":      func(g *captureMetadata) { g.URL = "" },
		"format":           func(g *captureMetadata) { g.Format = "unknown" },
		"output-traversal": func(g *captureMetadata) { g.ClearOutput = "../other.html" },
		"unrelated-name":   func(g *captureMetadata) { g.Output = "other.gated.html" },
		"no-decisions":     func(g *captureMetadata) { g.Access.Pending = nil },
		"cleared":          func(g *captureMetadata) { g.Access.Status = "cleared" },
		"changed-evidence": func(g *captureMetadata) { g.Access.Pending[0].Text = "Different declarations" },
		"changed-id":       func(g *captureMetadata) { g.Access.Pending[0].ID = "gate-v1-invalid" },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			invalid := captureMetadataFixture()
			change(&invalid)
			if err := writeCaptureJSON(path, invalid); err != nil {
				t.Fatal(err)
			}
			if _, err := readClearMetadata(path); err == nil {
				t.Fatal("invalid gate file accepted")
			}
		})
	}
}

func TestClearInvocationErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "page.json")
	if err := writeCaptureJSON(path, captureMetadataFixture()); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"--clear", path, "--auto-clear"},
		{"--clear", path, "--gates=preserve"},
		{"--clear", path, "--report", path},
		{"--clear", path, "-o", path},
		{"--clear", path, "https://different.example/"},
		{"--clear", path + ".missing"},
	} {
		runGateCLI(t, args, 2)
	}
}

func TestCaptureMetadataFilename(t *testing.T) {
	for _, tc := range []struct{ archive, want string }{
		{"example.com.gated.html", "example.com.json"},
		{"captures/page.gated.pdf", "captures/page.json"},
		{"captures/page.gated.mht", "captures/page.json"},
		{"page.gated", "page.json"}, {".page.gated", ".page.json"},
		{"dir.gated/page.html", "dir.gated/page.json"},
	} {
		if got := metadataFilename(tc.archive); got != tc.want {
			t.Errorf("%s: got %s want %s", tc.archive, got, tc.want)
		}
	}
}

func TestClearLegacyGateFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "page.gated.html.gate.json")
	legacy := captureMetadataFixture()
	legacy.Schema = "parch-gate/1"
	if err := writeCaptureJSON(path, legacy); err != nil {
		t.Fatal(err)
	}
	got, err := readClearMetadata(path)
	if err != nil || got.ClearOutput != "page.html" {
		t.Fatalf("legacy gate file not supported: %+v %v", got, err)
	}
}
