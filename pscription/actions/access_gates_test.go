package actions

import (
	"strings"
	"testing"
)

func TestGateApprovalScope(t *testing.T) {
	base := GateEvidence{URL: "https://example.com/", Kind: "age", Text: "I am at least 21 years of age.",
		Controls: []GateControl{{Type: "checkbox", Label: "Age declaration", Required: true}},
		Links:    []GateLink{{Text: "Terms", URL: "https://example.com/terms"}},
		Action:   "https://example.com/consent", Method: "POST", Button: "Enter", Approvable: true}
	id := gateID(base)
	if err := (GatePolicy{Approvals: []string{id}}).Validate(); err != nil {
		t.Fatal(err)
	}
	changes := []func(*GateEvidence){
		func(e *GateEvidence) { e.URL = "https://other.example/" },
		func(e *GateEvidence) { e.Text += " Additional declaration." },
		func(e *GateEvidence) { e.Action = "https://example.com/new-consent" },
		func(e *GateEvidence) { e.Method = "GET" },
		func(e *GateEvidence) { e.Button = "Accept different terms" },
		func(e *GateEvidence) {
			e.Controls = []GateControl{{Type: "checkbox", Label: "Other declaration", Required: true}}
		},
		func(e *GateEvidence) { e.Links = []GateLink{{Text: "Terms", URL: "https://example.com/terms-v2"}} },
	}
	for i, change := range changes {
		changed := base
		change(&changed)
		if gateID(changed) == id {
			t.Errorf("change %d did not invalidate approval", i)
		}
	}
	for _, bad := range []GatePolicy{
		{Preserve: true, AutoClear: true},
		{Preserve: true, Approvals: []string{id}},
		{Approvals: []string{"all"}}, {Approvals: []string{"gate-v1-abcd"}},
		{Approvals: []string{strings.Repeat("a", 64)}},
	} {
		if bad.Validate() == nil {
			t.Errorf("accepted invalid policy: %+v", bad)
		}
	}
}
