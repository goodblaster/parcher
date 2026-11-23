package capture

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/goodblaster/parcher/parchmint/textlayer"
	"github.com/goodblaster/parcher/pscription/act"
	"github.com/goodblaster/parcher/pscription/actions"
	"github.com/goodblaster/parcher/pscription/pipeline"
	"github.com/goodblaster/parcher/pscription/runner"
)

const accessFixture = `<!doctype html><html><head><title>Entry decision</title>
<style>body{font-family:Arial}section{position:fixed;inset:0;background:white;padding:4em}</style></head><body>
<main>Protected content is unavailable.</main>
<section role="dialog" aria-modal="true"><h1>Confirm eligibility to enter</h1>
<p>%s</p><form action="/consent" method="post">
<label><input name="age" type="checkbox" required>I am at least 21 years of age.</label>
<label><input name="research" type="checkbox" required>I am a qualified researcher purchasing for laboratory research only, not for human or veterinary use.</label>
<p>By entering I accept the <a href="/terms">Terms of Sale</a> and <a href="/privacy">Privacy Policy</a>.</p>
<p role="status">Select both statements.</p><button disabled>Enter site</button><a href="/exit">Not eligible? Exit.</a></form></section>
<script>
const form=document.querySelector('form');
form.addEventListener('change',()=>{form.querySelector('[role=status]').textContent='Selection updated';form.querySelector('button').disabled=!Array.from(form.querySelectorAll('input')).every(e=>e.checked)});
document.addEventListener('keydown',e=>{if(e.key==='Escape')location.href='/exit'});
// Catch regressions in the old pre-navigation age assertions.
if (document.cookie.includes('age_verified') || document.cookie.includes('age_gate=') || localStorage.getItem('ageVerified')) location.href='/exit';
</script></body></html>`

func captureGateFixture(t *testing.T, url string, policy actions.GatePolicy, recipe pipeline.Recipe, backend Backend) *Snapshot {
	t.Helper()
	cfg := runner.DefaultConfig()
	cfg.InitialWait = 50 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	snap, err := CaptureWithOptions(ctx, url, cfg, recipe, backend, Options{Gates: policy, TextLayer: true})
	if err != nil {
		t.Fatal(err)
	}
	return snap
}

func TestProbeAccessGates(t *testing.T) {
	needBrowser(t)
	var submissions atomic.Int32
	var changed atomic.Bool
	var second atomic.Bool
	var refuse atomic.Bool
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		switch r.URL.Path {
		case "/consent":
			_ = r.ParseForm()
			if r.Method != "POST" || r.FormValue("age") != "on" || r.FormValue("research") != "on" {
				t.Error("approval did not submit the actual form")
			}
			submissions.Add(1)
			http.Redirect(w, r, "/content", http.StatusSeeOther)
		case "/content":
			if second.Load() {
				fmt.Fprintf(w, accessFixture, "Additional certification is required.")
				return
			}
			fmt.Fprint(w, "<!doctype html><title>Welcome</title><main>Cleared content marker</main>")
		case "/exit":
			fmt.Fprint(w, "<!doctype html><title>Wrong exit</title><main>Generic page marker</main>")
		default:
			statement := "Please confirm these statements."
			if changed.Load() {
				statement = "Please confirm these statements. Updated terms apply."
			}
			fmt.Fprintf(w, accessFixture, statement)
			if refuse.Load() {
				fmt.Fprint(w, `<script>document.querySelector("form").onsubmit=e=>e.preventDefault()</script>`)
			}
		}
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	snap := captureGateFixture(t, server.URL, actions.GatePolicy{}, pipeline.Staticize(), HTML{})
	if snap.Access.Status != "decision_required" || len(snap.Access.Pending) != 1 {
		t.Fatalf("expected decision: %+v", snap.Access)
	}
	gate := snap.Access.Pending[0]
	if !gate.Approvable || len(gate.Controls) != 2 || len(gate.Links) != 3 {
		t.Fatalf("missing gate evidence: %+v", gate)
	}
	if snap.Title != "Entry decision" || !strings.Contains(string(snap.Bytes), "application/x-parch-capture+json") || submissions.Load() != 0 {
		t.Fatal("default capture damaged or submitted the gate")
	}
	t.Run("preserve-mht", func(t *testing.T) {
		snap := captureGateFixture(t, server.URL, actions.GatePolicy{Preserve: true}, pipeline.Staticize(), MHT{})
		doc, err := textlayer.MHTDocument(snap.Bytes)
		if err != nil {
			t.Fatal(err)
		}
		if snap.Access.Status != "preserved" || snap.Access.Pending[0].ID != gate.ID || !strings.Contains(string(doc), "application/x-parch-capture+json") {
			t.Fatal("preserve report or stable fingerprint lost")
		}
	})
	policy := actions.GatePolicy{Approvals: []string{gate.ID}}
	t.Run("changed-terms", func(t *testing.T) {
		changed.Store(true)
		defer changed.Store(false)
		snap := captureGateFixture(t, server.URL, policy, pipeline.Staticize(), HTML{})
		if snap.Access.Status != "decision_required" || snap.Access.Pending[0].ID == gate.ID || submissions.Load() != 0 {
			t.Fatal("stale approval accepted")
		}
	})
	t.Run("approved", func(t *testing.T) {
		snap := captureGateFixture(t, server.URL, policy, pipeline.Staticize(), HTML{})
		if snap.Access.Status != "cleared" || len(snap.Access.Cleared) != 1 || len(snap.Access.Pending) != 0 || snap.Title != "Welcome" || submissions.Load() != 1 {
			t.Fatalf("normal form approval failed: title=%s access=%+v submissions=%d", snap.Title, snap.Access, submissions.Load())
		}
		if !strings.Contains(string(snap.Bytes), "Cleared content marker") {
			t.Fatal("cleared content absent")
		}
	})
	t.Run("auto-clear-form", func(t *testing.T) {
		before := submissions.Load()
		snap := captureGateFixture(t, server.URL, actions.GatePolicy{AutoClear: true}, pipeline.Staticize(), HTML{})
		if snap.Access.Status != "cleared" || !snap.Access.AutoClear || submissions.Load() != before+1 {
			t.Fatalf("automatic form approval failed: %+v", snap.Access)
		}
	})
	t.Run("second-gate", func(t *testing.T) {
		second.Store(true)
		defer second.Store(false)
		snap := captureGateFixture(t, server.URL, policy, pipeline.Staticize(), HTML{})
		if snap.Access.Status != "decision_required" || len(snap.Access.Cleared) != 1 || len(snap.Access.Pending) != 1 || snap.Access.Pending[0].ID == gate.ID {
			t.Fatalf("new gate was not stopped: %+v", snap.Access)
		}
	})
	t.Run("refused-submission", func(t *testing.T) {
		refuse.Store(true)
		defer refuse.Store(false)
		before := submissions.Load()
		snap := captureGateFixture(t, server.URL, policy, pipeline.Staticize(), HTML{})
		if snap.Access.Status != "decision_required" || snap.Access.Pending[0].Reason != "acceptance_not_confirmed" || submissions.Load() != before {
			t.Fatalf("failed approval was not reported: %+v", snap.Access)
		}
	})
}

func TestProbeGateClassificationAndLateArrival(t *testing.T) {
	needBrowser(t)
	cases := []struct {
		name, body, status string
		approvable         bool
	}{
		{"cookie", `<section role="dialog"><form>Cookie preferences<label><input required type="checkbox">Allow cookies</label><button>Accept all</button></form></section>`, "no_gate_detected", false},
		{"named-overlay", `<div class="modal" style="position:fixed;inset:0">You must be 21 or older.<button>Yes, enter</button></div>`, "decision_required", true},
		{"offscreen-dialog", `<section role="dialog" style="position:absolute;left:-10000px">Are you over 21?<button>Yes, enter</button></section>`, "no_gate_detected", false},
		{"custom-widget", `<section role="dialog">Are you over 21?<div role="checkbox" aria-required="true">I confirm my age</div><button>Continue</button></section>`, "decision_required", false},
		{"footer", `<main><h1>Ordinary article</h1></main><footer>Research use only. Must be 21 years of age.</footer>`, "no_gate_detected", false},
		{"checkout", `<section role="dialog"><form><label><input required type="checkbox">I am at least 21 years of age.</label><button>Place order</button></form></section>`, "no_gate_detected", false},
		{"login", `<section role="dialog"><form>Sign in to continue<input type="password"><button>Continue</button></form></section>`, "decision_required", false},
		{"birthdate", `<section role="dialog"><form>Verify your age<input type="date" required><button>Continue</button></form></section>`, "decision_required", false},
		{"button", `<section role="dialog"><p>Are you over 21?</p><button onclick="this.parentElement.remove()">Yes, enter</button><button>No, leave</button></section>`, "decision_required", true},
		{"shadow", `<div id="host"></div><script>host.attachShadow({mode:'open'}).innerHTML='<section role="dialog">Are you over 21?<button>Yes, enter</button></section>'</script>`, "decision_required", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, "<!doctype html><title>Fixture</title>"+tc.body)
			}))
			defer server.Close()
			snap := captureGateFixture(t, server.URL, actions.GatePolicy{}, pipeline.Recipe{}, HTML{})
			if snap.Access.Status != tc.status {
				t.Fatalf("status %s, want %s", snap.Access.Status, tc.status)
			}
			if len(snap.Access.Pending) > 0 && snap.Access.Pending[0].Approvable != tc.approvable {
				t.Fatalf("approvable: %+v", snap.Access.Pending)
			}
		})
	}
	t.Run("late-gate-stops-cleanup", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, "<!doctype html><title>Late gate</title><body>Initial content</body>")
		}))
		defer server.Close()
		ran := false
		recipe := pipeline.Recipe{Steps: []pipeline.Step{
			pipeline.NamedStep("arrives", act.Evaluate(`document.body.innerHTML='<section role="dialog">Verify your age: are you over 21?<button>Yes, enter</button></section>'`, nil)),
			pipeline.NamedStep("destructive", func(context.Context) error { ran = true; return nil }),
		}}
		snap := captureGateFixture(t, server.URL, actions.GatePolicy{}, recipe, HTML{})
		if ran || snap.Access.Status != "decision_required" {
			t.Fatal("late gate was not protected")
		}
	})
}

func TestProbeGateSubmissionProgress(t *testing.T) {
	needBrowser(t)
	for _, changeTerms := range []bool{false, true} {
		t.Run(fmt.Sprint("change-terms=", changeTerms), func(t *testing.T) {
			var posted atomic.Int32
			handler := `<script>
        document.querySelector('form').onsubmit=e=>{
            e.preventDefault();
            const b=document.querySelector('button'); b.disabled=true; b.textContent='Verifying…';
            setTimeout(()=>{document.querySelector('section').remove();document.querySelector('main').textContent='Completed marker'},1200);
        };
        </script>`
			if changeTerms {
				handler = `<script>
            const boxes=document.querySelectorAll('input');
            boxes[0].onchange=()=>{boxes[1].parentElement.lastChild.textContent='I accept a different declaration.'};
            boxes[1].onchange=()=>{fetch('/unexpected',{method:'POST'})};
        </script>`
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" {
					posted.Add(1)
					return
				}
				fmt.Fprintf(w, accessFixture, "Confirm these statements.")
				fmt.Fprint(w, handler)
			}))
			defer server.Close()
			initial := captureGateFixture(t, server.URL, actions.GatePolicy{}, pipeline.Recipe{}, HTML{})
			policy := actions.GatePolicy{Approvals: []string{initial.Access.Pending[0].ID}}
			result := captureGateFixture(t, server.URL, policy, pipeline.Recipe{}, HTML{})
			if changeTerms {
				if result.Access.Status != "decision_required" || result.Access.Pending[0].ID == initial.Access.Pending[0].ID || posted.Load() != 0 {
					t.Fatalf("acted on changed declarations: %+v, posts=%d", result.Access, posted.Load())
				}
			} else if result.Access.Status != "cleared" || !strings.Contains(string(result.Bytes), "Completed marker") {
				t.Fatalf("loading state mistaken for clearance: %+v", result.Access)
			}
		})
	}
}

func TestProbeGateRevealedByCleanup(t *testing.T) {
	needBrowser(t)
	var posted atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			posted.Add(1)
			return
		}
		fmt.Fprint(w, `<!doctype html><title>Layered dialogs</title>
        <div role="dialog"><button onclick="this.parentElement.remove();document.getElementById('entry').style.display='block'">Accept cookies</button></div>
        <section id="entry" role="dialog" style="display:none">Are you over 21?
        <button onclick="fetch('/unexpected',{method:'POST'});this.parentElement.remove()">Yes, enter</button></section>`)
	}))
	defer server.Close()
	snap := captureGateFixture(t, server.URL, actions.GatePolicy{}, pipeline.Staticize(), HTML{})
	if snap.Access.Status != "decision_required" || posted.Load() != 0 {
		t.Fatalf("cleanup answered a newly revealed gate: %+v posts=%d", snap.Access, posted.Load())
	}
}

func TestProbeAutoClearChain(t *testing.T) {
	needBrowser(t)
	for _, count := range []int{2, 10} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var stage int
				_, _ = fmt.Sscanf(r.URL.Query().Get("stage"), "%d", &stage)
				if stage >= count {
					fmt.Fprint(w, "<!doctype html><title>Done</title><main>All gates cleared</main>")
					return
				}
				fmt.Fprintf(w, `<!doctype html><section role="dialog">Are you over 21? Decision %d<button onclick="location.href='/?stage=%d'">Yes, enter</button></section>`, stage, stage+1)
			}))
			defer server.Close()
			snap := captureGateFixture(t, server.URL, actions.GatePolicy{AutoClear: true}, pipeline.Recipe{}, HTML{})
			if count == 2 {
				if snap.Access.Status != "cleared" || len(snap.Access.Cleared) != 2 {
					t.Fatalf("chain not cleared: %+v", snap.Access)
				}
			} else if snap.Access.Status != "decision_required" || len(snap.Access.Cleared) >= count || snap.Access.Pending[0].Reason != "auto_clear_limit_reached" {
				t.Fatalf("unbounded auto-clear: %+v", snap.Access)
			}
		})
	}
}
