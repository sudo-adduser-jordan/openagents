package domain

import "fmt"

// WorkflowMode is the user-controlled delivery posture of a session. Unlike
// SessionMode, which selects the conversation controller, WorkflowMode controls
// whether work is being scoped, coordinated by a manager, or executed.
//
//   - WorkflowModePlanning scopes or designs work before execution. A manager in
//     this mode may not delegate.
//   - WorkflowModeManager lets a human-facing manager coordinate work and
//     delegate it to workers. Every newly created manager starts here.
//   - WorkflowModeBuilding executes work directly and advances toward review.
type WorkflowMode string

// The workflow modes.
const (
	WorkflowModePlanning WorkflowMode = "planning"
	WorkflowModeManager  WorkflowMode = "manager"
	WorkflowModeBuilding WorkflowMode = "building"
)

// DefaultWorkflowModeForKind returns the durable starting mode for a new
// session. Workers always begin in planning; managers begin ready to
// coordinate and delegate.
func DefaultWorkflowModeForKind(kind SessionKind) WorkflowMode {
	if kind == KindManager {
		return WorkflowModeManager
	}
	return WorkflowModePlanning
}

// ValidForKind reports whether mode is valid for the session role. Manager mode
// is exclusive to manager sessions; building is exclusive to workers; planning
// is shared by both roles.
func (m WorkflowMode) ValidForKind(kind SessionKind) bool {
	if !m.Valid() {
		return false
	}
	switch m {
	case WorkflowModePlanning:
		return kind == KindManager || kind == KindWorker
	case WorkflowModeManager:
		return kind == KindManager
	case WorkflowModeBuilding:
		return kind == KindWorker
	default:
		return false
	}
}

// NormalizeWorkflowModeForKind collapses an empty, unrecognized, or
// role-incompatible mode to the safe default for that session role.
func NormalizeWorkflowModeForKind(kind SessionKind, mode WorkflowMode) WorkflowMode {
	if mode.ValidForKind(kind) {
		return mode
	}
	return DefaultWorkflowModeForKind(kind)
}

// Valid reports whether mode is one Open Agents knows how to apply to some
// session role.
func (m WorkflowMode) Valid() bool {
	switch m {
	case WorkflowModePlanning, WorkflowModeManager, WorkflowModeBuilding:
		return true
	default:
		return false
	}
}

// NormalizeWorkflowMode collapses an empty or unrecognized mode to Planning for
// callers that do not have a session kind. Use NormalizeWorkflowModeForKind when
// the kind is available so a malformed manager row still receives the safe
// manager default.
func NormalizeWorkflowMode(mode WorkflowMode) WorkflowMode {
	if mode.Valid() {
		return mode
	}
	return WorkflowModePlanning
}

// ParseWorkflowMode converts caller-supplied input into a mode, strictly. An
// empty string means "no mode requested" and yields the zero value with no
// error, so callers can distinguish absent from invalid and apply their own
// precedence. Anything else unrecognized is an error: a request that named a
// mode Open Agents cannot apply must fail loudly rather than downgrade.
func ParseWorkflowMode(raw string) (WorkflowMode, error) {
	if raw == "" {
		return "", nil
	}
	mode := WorkflowMode(raw)
	if !mode.Valid() {
		return "", fmt.Errorf("unknown workflow mode %q: want %q, %q, or %q", raw, WorkflowModePlanning, WorkflowModeManager, WorkflowModeBuilding)
	}
	return mode, nil
}
