package actions

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/goodblaster/parcher/pscription/act"
	js "github.com/goodblaster/parcher/pscription/scripts"
)

// GatePolicy requires a decision by default. Preserve deliberately captures
// the unresolved view. Approvals authorize exact reported evidence; AutoClear
// explicitly authorizes automatic approval of supported gates as they appear.
type GatePolicy struct {
	Preserve  bool
	AutoClear bool
	Approvals []string
}

const maxAutoClearAttempts = 8

func (p GatePolicy) Validate() error {
	if p.Preserve && (p.AutoClear || len(p.Approvals) > 0) {
		return fmt.Errorf("--gates=preserve cannot be combined with --approve-gate or --auto-clear")
	}
	for _, id := range p.Approvals {
		hash, err := hex.DecodeString(strings.TrimPrefix(id, "gate-v1-"))
		if !strings.HasPrefix(id, "gate-v1-") || err != nil || len(hash) != sha256.Size {
			return fmt.Errorf("invalid --approve-gate fingerprint %q (use an ID from --report)", id)
		}
	}
	return nil
}

type GateControl struct {
	Type     string `json:"type"`
	Name     string `json:"name"`
	Label    string `json:"label"`
	Required bool   `json:"required"`
}

type GateLink struct {
	Text string `json:"text"`
	URL  string `json:"url"`
}

// GateEvidence deliberately omits checked state and hidden anti-CSRF tokens,
// but includes the full displayed terms, links, destination and page URL.
// Field order matches access_gates.js for its last-moment evidence comparison.
type GateEvidence struct {
	URL        string        `json:"url"`
	Kind       string        `json:"kind"`
	Text       string        `json:"text"`
	Controls   []GateControl `json:"controls"`
	Links      []GateLink    `json:"links"`
	Action     string        `json:"action"`
	Method     string        `json:"method"`
	Button     string        `json:"button"`
	Approvable bool          `json:"approvable"`
}

type AccessGate struct {
	ID string `json:"id"`
	GateEvidence
	Reason string `json:"reason,omitempty"`
}

// Validate checks evidence loaded from a gate file against its approval token.
func (g AccessGate) Validate() error {
	if g.ID != gateID(g.GateEvidence) {
		return fmt.Errorf("gate fingerprint does not match its evidence")
	}
	return nil
}

func gateID(e GateEvidence) string {
	b, _ := json.Marshal(e)
	sum := sha256.Sum256(b)
	return fmt.Sprintf("gate-v1-%x", sum)
}

// AccessReport describes only observed gates; no_gate_detected does not
// guarantee that a site supplied all its content or that detection is complete.
type AccessReport struct {
	AutoClear bool         `json:"auto_clear,omitempty"`
	Status    string       `json:"status"`
	Pending   []AccessGate `json:"pending"`
	Cleared   []AccessGate `json:"cleared"`
}

// GateController belongs to one browser session. A pending gate stops prep;
// the caller can still serialize the page and its report.
type GateController struct {
	Policy    GatePolicy
	Report    AccessReport
	attempted map[string]bool
	stopped   bool
}

func NewGateController(policy GatePolicy) *GateController {
	return &GateController{Policy: policy, Report: AccessReport{
		AutoClear: policy.AutoClear,
		Status:    "no_gate_detected", Pending: []AccessGate{}, Cleared: []AccessGate{},
	}, attempted: map[string]bool{}}
}

// Attach installs the read-only detector before navigation, also making it
// available to popup cleanup scripts so they can stop within their own loops.
func (g *GateController) Attach() act.Func {
	return func(ctx context.Context) error {
		src, err := js.AccessGates.Source()
		if err != nil {
			return err
		}
		source := "window.__pscriptionInspectAccessGates = () => (" + src + ")();" +
			"window.__pscriptionHasAccessGate = () => window.__pscriptionInspectAccessGates().length > 0;"
		_, err = chromedp.Call(ctx, page.AddScriptToEvaluateOnNewDocument, page.AddScriptToEvaluateOnNewDocumentParams{Source: source})
		return err
	}
}

// DetectAccessGates is read-only. It does not assert eligibility or submit forms.
func DetectAccessGates(out *[]AccessGate) act.Func {
	return func(ctx context.Context) error {
		var evidence []GateEvidence
		if err := js.AccessGates.Action(&evidence)(ctx); err != nil {
			return err
		}
		*out = make([]AccessGate, 0, len(evidence))
		for _, e := range evidence {
			*out = append(*out, AccessGate{ID: gateID(e), GateEvidence: e})
		}
		return nil
	}
}

func (g *GateController) stop(gates []AccessGate) (bool, error) {
	g.Report.Pending = gates
	g.Report.Status = "decision_required"
	if g.Policy.Preserve {
		g.Report.Status = "preserved"
	}
	g.stopped = true
	return false, nil
}

// Check is a pipeline guard. Approval uses normal click/change/submit handlers
// and is considered cleared only after the observed gate disappears. A changed
// gate requires a fresh fingerprint (approved automatically under AutoClear);
// a failed submission is never retried blindly.
func (g *GateController) Check(ctx context.Context) (bool, error) {
	if g.stopped {
		return false, nil
	}
	for {
		var gates []AccessGate
		if err := DetectAccessGates(&gates)(ctx); err != nil {
			return false, err
		}
		if len(gates) == 0 {
			return true, nil
		}
		for i := range gates {
			gate := &gates[i]
			approved := g.Policy.AutoClear
			for _, id := range g.Policy.Approvals {
				if id == gate.ID {
					approved = true
				}
			}
			switch {
			case !gate.Approvable:
				gate.Reason = "manual_interaction_required"
			case g.attempted[gate.ID]:
				gate.Reason = "acceptance_not_confirmed"
			case g.Policy.AutoClear && len(g.attempted) >= maxAutoClearAttempts:
				gate.Reason = "auto_clear_limit_reached"
			case !approved:
				gate.Reason = "approval_required"
			}
		}
		// Never partially act on a set containing an unapproved decision.
		for _, gate := range gates {
			if g.Policy.Preserve || gate.Reason != "" {
				return g.stop(gates)
			}
		}
		gate := gates[0]
		g.attempted[gate.ID] = true
		var clicked bool
		// Navigation may invalidate the execution context after the click.
		// Verification below resolves that ambiguity against the resulting DOM.
		clickErr := js.AccessGates.Action(&clicked, gate.GateEvidence)(ctx)
		if clickErr == nil && !clicked {
			var current []AccessGate
			if err := DetectAccessGates(&current)(ctx); err != nil {
				return false, err
			}
			if len(current) == 0 {
				current = []AccessGate{gate}
			}
			for i := range current {
				current[i].Reason = "gate_changed_before_submission"
			}
			return g.stop(current)
		}
		deadline := time.Now().Add(5 * time.Second)
		absent := 0
		verified := false
		for time.Now().Before(deadline) {
			if err := act.Sleep(250 * time.Millisecond)(ctx); err != nil {
				return false, err
			}
			var current []AccessGate
			if err := DetectAccessGates(&current)(ctx); err != nil {
				absent = 0
				continue
			}
			// A disabled/renamed submit button is progress, not success. Keep
			// waiting while the original gate is visible, even if its evidence
			// has changed. Navigation drops this document-local reference.
			var present bool
			if err := act.Evaluate("Boolean(window.__pscriptionSubmittedGateVisible?.())", &present)(ctx); err != nil {
				absent = 0
				continue
			}
			for _, item := range current {
				if item.ID == gate.ID {
					present = true
				}
			}
			if present {
				absent = 0
				continue
			}
			absent++
			if absent >= 2 {
				verified = true
				break
			}
		}
		if !verified {
			gate.Reason = "acceptance_not_confirmed"
			return g.stop([]AccessGate{gate})
		}
		g.Report.Cleared = append(g.Report.Cleared, gate)
		g.Report.Status = "cleared"
		// Observe again: navigating may have revealed a different decision.
	}
}
