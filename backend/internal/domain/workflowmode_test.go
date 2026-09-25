package domain

import "testing"

func TestWorkflowModeValid(t *testing.T) {
	for _, tc := range []struct {
		mode WorkflowMode
		want bool
	}{
		{WorkflowModePlanning, true},
		{WorkflowModeManager, true},
		{WorkflowModeBuilding, true},
		{"", false},
		{"review", false},
		{"Planning", false},
	} {
		if got := tc.mode.Valid(); got != tc.want {
			t.Errorf("WorkflowMode(%q).Valid() = %v, want %v", tc.mode, got, tc.want)
		}
	}
}

func TestWorkflowModeValidForRole(t *testing.T) {
	for _, tc := range []struct {
		kind SessionKind
		mode WorkflowMode
		want bool
	}{
		{kind: KindWorker, mode: WorkflowModePlanning, want: true},
		{kind: KindWorker, mode: WorkflowModeBuilding, want: true},
		{kind: KindWorker, mode: WorkflowModeManager, want: false},
		{kind: KindManager, mode: WorkflowModePlanning, want: true},
		{kind: KindManager, mode: WorkflowModeManager, want: true},
		{kind: KindManager, mode: WorkflowModeBuilding, want: false},
		{kind: SessionKind("other"), mode: WorkflowModePlanning, want: false},
	} {
		if got := tc.mode.ValidForKind(tc.kind); got != tc.want {
			t.Errorf("WorkflowMode(%q).ValidForKind(%q) = %v, want %v", tc.mode, tc.kind, got, tc.want)
		}
	}
}

// Reads of durable state must always land on a role-compatible mode: rows
// written before this feature have no workflow mode, and a row written by a
// newer build may carry a mode this build does not know.
func TestNormalizeWorkflowModeFallsBackToPlanning(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   WorkflowMode
		want WorkflowMode
	}{
		{"empty legacy row", "", WorkflowModePlanning},
		{"unknown future mode", "review", WorkflowModePlanning},
		{"planning preserved", WorkflowModePlanning, WorkflowModePlanning},
		{"manager preserved", WorkflowModeManager, WorkflowModeManager},
		{"building preserved", WorkflowModeBuilding, WorkflowModeBuilding},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := NormalizeWorkflowMode(tc.in); got != tc.want {
				t.Errorf("NormalizeWorkflowMode(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// Requested workflow modes are parsed strictly so a toggle request naming a
// stage Open Agents cannot place fails loudly instead of silently landing in planning.
func TestParseWorkflowModeRejectsUnknownInsteadOfFallingBack(t *testing.T) {
	if _, err := ParseWorkflowMode("review"); err == nil {
		t.Fatal("ParseWorkflowMode(\"review\") = nil error, want a rejection")
	}

	// Absent is not invalid: callers apply their own precedence to the zero value.
	mode, err := ParseWorkflowMode("")
	if err != nil {
		t.Fatalf("ParseWorkflowMode(\"\") returned error %v, want none", err)
	}
	if mode != "" {
		t.Errorf("ParseWorkflowMode(\"\") = %q, want the zero value", mode)
	}

	for _, want := range []WorkflowMode{WorkflowModePlanning, WorkflowModeManager, WorkflowModeBuilding} {
		got, err := ParseWorkflowMode(string(want))
		if err != nil {
			t.Errorf("ParseWorkflowMode(%q) returned error %v", want, err)
			continue
		}
		if got != want {
			t.Errorf("ParseWorkflowMode(%q) = %q", want, got)
		}
	}
}

func TestDefaultWorkflowModeDependsOnRole(t *testing.T) {
	if got := DefaultWorkflowModeForKind(KindManager); got != WorkflowModeManager {
		t.Fatalf("manager default = %q, want %q", got, WorkflowModeManager)
	}
	if got := DefaultWorkflowModeForKind(KindWorker); got != WorkflowModePlanning {
		t.Fatalf("worker default = %q, want %q", got, WorkflowModePlanning)
	}
	if got := NormalizeWorkflowModeForKind(KindManager, ""); got != WorkflowModeManager {
		t.Fatalf("malformed manager row = %q, want manager default", got)
	}
	if got := NormalizeWorkflowModeForKind(KindWorker, WorkflowModeManager); got != WorkflowModePlanning {
		t.Fatalf("manager mode on worker row = %q, want planning fallback", got)
	}
	if got := NormalizeWorkflowModeForKind(KindManager, WorkflowModeBuilding); got != WorkflowModeManager {
		t.Fatalf("building mode on manager row = %q, want manager fallback", got)
	}
}
