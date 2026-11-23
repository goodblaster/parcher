package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/goodblaster/parcher/parchmint/capture"
	flag "github.com/spf13/pflag"
)

// Run the actual main path in a child so os.Exit, stdout and stderr are tested.
func TestGateCLIHelper(t *testing.T) {
	raw := os.Getenv("PARCH_TEST_GATE_ARGS")
	if raw == "" {
		return
	}
	var args []string
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		t.Fatal(err)
	}
	os.Args = append([]string{"parch"}, args...)
	flag.CommandLine = flag.NewFlagSet("parch", flag.ExitOnError)
	main()
	os.Exit(0)
}

func TestProbeGateCLI(t *testing.T) {
	if os.Getenv("PARCH_BROWSER_TESTS") == "" {
		t.Skip("requires Chrome; run make probe")
	}
	var revision atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if r.URL.Path == "/plain" {
			fmt.Fprint(w, "<!doctype html><title>Plain</title><main>Public content</main>")
			return
		}
		if r.URL.Path == "/manual" {
			fmt.Fprint(w, `<section role="dialog"><form>Verify your age<input type="date"><button>Continue</button></form></section>`)
			return
		}
		fmt.Fprintf(w, `<!doctype html><title>Access fixture</title><body><main>Content marker</main><section role="dialog"><p>Are you over 21?</p><p>Terms revision %d</p><button onclick="this.parentElement.remove()">Yes, enter</button><button>No, leave</button></section></body>`, revision.Load())
	}))
	defer server.Close()
	dir := t.TempDir()
	report := filepath.Join(dir, "report.json")
	target := server.URL
	baseOutput := filepath.Join(dir, "page.html")
	run := func(t *testing.T, want int, extra ...string) []byte {
		t.Helper()
		args := []string{"--timeout", "30", "--no-favicon", "--report", report, "-o", baseOutput}
		if target != "" {
			args = append(args, target)
		}
		args = append(args, extra...)
		return runGateCLI(t, args, want)
	}
	readReport := func(t *testing.T, wantOutput string) capture.Report {
		t.Helper()
		raw, err := os.ReadFile(report)
		if err != nil {
			t.Fatal(err)
		}
		var result capture.Report
		if err := json.Unmarshal(raw, &result); err != nil {
			t.Fatal(err)
		}
		if result.Schema != "parch-capture/1" || result.Output != wantOutput {
			t.Fatalf("invalid report: %+v", result)
		}
		return result
	}
	output := run(t, 3)
	initial := readReport(t, gatedFilename(baseOutput))
	if initial.Access.Status != "decision_required" || len(initial.Access.Pending) != 1 {
		t.Fatalf("wrong decision report: %+v", initial)
	}
	gated, err := os.ReadFile(gatedFilename(baseOutput))
	if err != nil || !bytes.Contains(gated, []byte("Are you over 21?")) || len(output) > 0 {
		t.Fatal("gated archive was not saved")
	}
	sidecar := metadataFilename(gatedFilename(baseOutput))
	saved, err := readClearMetadata(sidecar)
	if err != nil || saved.ClearOutput != "page.html" {
		t.Fatalf("automatic gate file invalid: %+v %v", saved, err)
	}
	if _, err := os.Stat(baseOutput); !os.IsNotExist(err) {
		t.Fatal("unresolved capture used the regular name")
	}
	t.Run("clear-from-file", func(t *testing.T) {
		target = ""
		defer func() { target = server.URL }()
		// No explicit URL or output: the sidecar supplies both.
		runGateCLI(t, []string{"--timeout", "30", "--no-favicon", "--report", report, "--clear", sidecar}, 0)
		if readReport(t, baseOutput).Access.Status != "cleared" {
			t.Fatal("clear report lost")
		}
		if _, err := os.Stat(baseOutput); err != nil {
			t.Fatal("regular cleared archive missing")
		}
		after, err := os.ReadFile(gatedFilename(baseOutput))
		if err != nil || !bytes.Equal(after, gated) {
			t.Fatal("clear modified the gated archive")
		}
		if _, err := os.Stat(sidecar); err != nil {
			t.Fatal("clear removed the reusable gate file")
		}
	})
	t.Run("clear-changed-gate", func(t *testing.T) {
		revision.Add(1)
		defer revision.Store(0)
		original, err := os.ReadFile(baseOutput)
		if err != nil {
			t.Fatal(err)
		}
		run(t, 3, "--clear", sidecar)
		changed, err := readClearMetadata(sidecar)
		if err != nil || changed.Access.Pending[0].ID == initial.Access.Pending[0].ID {
			t.Fatal("changed gate file was not updated")
		}
		current, err := os.ReadFile(baseOutput)
		if err != nil || !bytes.Equal(current, original) {
			t.Fatal("failed clearance overwrote the regular archive")
		}
	})
	run(t, 0, "--gates=preserve")
	if readReport(t, gatedFilename(baseOutput)).Access.Status != "preserved" {
		t.Fatal("preserve did not succeed")
	}
	run(t, 0, "--approve-gate", initial.Access.Pending[0].ID)
	if readReport(t, baseOutput).Access.Status != "cleared" {
		t.Fatal("approval not cleared")
	}
	run(t, 3, "--approve-gate", "gate-v1-"+strings.Repeat("0", 64))
	run(t, 2, "--gates=invalid")
	run(t, 2, "--approve-gate", "all")
	run(t, 2, "--gates=preserve", "--approve-gate", initial.Access.Pending[0].ID)
	run(t, 2, "--auto-clear", "--gates=preserve")
	t.Run("auto-clear-file", func(t *testing.T) {
		base := filepath.Join(dir, "page.html")
		run(t, 0, "--auto-clear", "-o", base)
		result := readReport(t, base)
		if result.Access.Status != "cleared" || !result.Access.AutoClear {
			t.Fatalf("auto-clear not reported: %+v", result.Access)
		}
		cleared, err := os.ReadFile(result.Output)
		if err != nil || !bytes.Contains(cleared, []byte("Content marker")) {
			t.Fatal("cleared archive absent")
		}
	})
	t.Run("auto-clear-no-gate", func(t *testing.T) {
		target = server.URL + "/plain"
		defer func() { target = server.URL }()
		path := filepath.Join(dir, "plain.html")
		run(t, 0, "--auto-clear", "-o", path)
		if readReport(t, path).Access.Status != "no_gate_detected" {
			t.Fatal("no-gate outcome lost")
		}
	})
	t.Run("auto-clear-manual", func(t *testing.T) {
		target = server.URL + "/manual"
		defer func() { target = server.URL }()
		path := filepath.Join(dir, "manual.html")
		run(t, 3, "--auto-clear", "-o", path)
		if readReport(t, gatedFilename(path)).Access.Pending[0].Reason != "manual_interaction_required" {
			t.Fatal("unsupported gate auto-approved")
		}
	})
	run(t, 1, "-o", dir) // Archive write failure takes precedence over decision exit 3.
}

func TestSeparateReportPath(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "archive.html")
	if err := separateReportPath(a, filepath.Join(dir, ".", "archive.html")); err == nil {
		t.Fatal("same file accepted")
	}
	if err := os.WriteFile(a, []byte("archive"), 0600); err != nil {
		t.Fatal(err)
	}
	b := filepath.Join(dir, "report.json")
	if err := os.Link(a, b); err != nil {
		t.Fatal(err)
	}
	if err := separateReportPath(a, b); err == nil {
		t.Fatal("hard link alias accepted")
	}
	if err := separateReportPath(a, filepath.Join(dir, "different.json")); err != nil {
		t.Fatal(err)
	}
}

func TestGatedFilename(t *testing.T) {
	for _, tc := range []struct{ normal, gated string }{
		{"example.com.html", "example.com.gated.html"},
		{"dir/page.mht", "dir/page.gated.mht"},
		{"dir/page.pdf", "dir/page.gated.pdf"},
		{"page", "page.gated"}, {".page", ".page.gated"},
		{"dir.gated/page.html", "dir.gated/page.gated.html"},
	} {
		if got := gatedFilename(tc.normal); got != tc.gated {
			t.Errorf("%s: got %s", tc.normal, got)
		}
		if got := gatedFilename(tc.gated); got != tc.gated {
			t.Errorf("duplicated gated marker: %s", got)
		}
		if got := plainFilename(tc.gated); got != tc.normal {
			t.Errorf("wrong plain name: %s", got)
		}
	}
}

func runGateCLI(t *testing.T, args []string, want int) []byte {
	t.Helper()
	return runGateCLIAt(t, args, want, t.TempDir())
}

func runGateCLIAt(t *testing.T, args []string, want int, dir string) []byte {
	t.Helper()
	raw, _ := json.Marshal(args)
	cmd := exec.Command(os.Args[0], "-test.run=^TestGateCLIHelper$")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "PARCH_TEST_GATE_ARGS="+string(raw))
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			code = exit.ExitCode()
		} else {
			t.Fatal(err)
		}
	}
	if code != want {
		t.Fatalf("exit %d want %d: %s", code, want, stderr.String())
	}
	return stdout.Bytes()
}

func TestProbeGateArtifacts(t *testing.T) {
	if os.Getenv("PARCH_BROWSER_TESTS") == "" {
		t.Skip("requires Chrome")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<!doctype html><main>Content after clearance</main><section role="dialog">Are you over 21?<button onclick="this.parentElement.remove()">Yes, enter</button></section>`)
	}))
	defer server.Close()
	dir := t.TempDir()
	for _, format := range []string{"html", "mht"} {
		t.Run(format, func(t *testing.T) {
			normal := filepath.Join(dir, "page."+format)
			runGateCLI(t, []string{"--timeout", "30", "--no-favicon", "-f", format, "-o", normal, server.URL}, 3)
			path := metadataFilename(gatedFilename(normal))
			g, err := readClearMetadata(path)
			if err != nil || g.Format != format || g.ClearOutput != "page."+format {
				t.Fatalf("wrong sidecar: %+v %v", g, err)
			}
			if _, err := os.Stat(gatedFilename(normal)); err != nil {
				t.Fatal(err)
			}
			runGateCLI(t, []string{"--timeout", "30", "--no-favicon", "--clear", path}, 0)
			if _, err := os.Stat(normal); err != nil {
				t.Fatal("gate file did not supply output format/path")
			}
		})
	}
	t.Run("stdout", func(t *testing.T) {
		stdout := runGateCLIAt(t, []string{"--timeout", "30", "--no-favicon", "-o", "-", server.URL}, 3, dir)
		archive := filepath.Join(dir, gatedFilename(deriveFilename(server.URL)+".html"))
		disk, err := os.ReadFile(archive)
		if err != nil || !bytes.Equal(disk, stdout) {
			t.Fatal("stdout and saved gated archive differ")
		}
		if _, err := readClearMetadata(metadataFilename(archive)); err != nil {
			t.Fatal("stdout capture did not save a valid gate file")
		}
	})
}
