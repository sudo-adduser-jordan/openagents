package contract_test

import (
	"testing"
	"time"

	"github.com/sudo-adduser-jordan/open-agents/backend/pkg/contract"
)

// A session staged by `spawn --no-start` has no agent, so it has no hooks and
// no PR. The dangerous case is the silent-past-grace rule: without an explicit
// pending reading, a staged session would be reported as no_signal, which the
// contract defines as a broken hook pipeline -- the opposite of the truth.
func TestDeriveStatusPendingForStagedSession(t *testing.T) {
	const grace = 90 * time.Second
	longSilent := statusNow.Add(-2 * grace)

	tests := []struct {
		name           string
		facts          contract.SessionFacts
		prs            []contract.PRFacts
		signalExpected bool
		now            time.Time
		want           contract.SessionStatus
	}{
		{
			// The core case: quiet for far longer than the grace period, with
			// signal expected because the harness would normally report. Without
			// the pending branch this is no_signal.
			name:           "silent past grace stays pending",
			facts:          contract.SessionFacts{Activity: contract.ActivityIdle, LastActivityAt: longSilent, AgentDeferred: true},
			signalExpected: true,
			now:            statusNow,
			want:           contract.StatusPending,
		},
		{
			name:  "fresh staged session is pending not idle",
			facts: contract.SessionFacts{Activity: contract.ActivityIdle, LastActivityAt: statusNow, AgentDeferred: true},
			now:   statusNow,
			want:  contract.StatusPending,
		},
		{
			// Terminated still wins: a staged session a user deleted is archived,
			// not left looking like a task waiting to be picked up.
			name:  "terminated deferred reports terminated",
			facts: contract.SessionFacts{IsTerminated: true, AgentDeferred: true, LastActivityAt: longSilent},
			now:   statusNow,
			want:  contract.StatusTerminated,
		},
		{
			// A session that was staged and then started is no longer deferred,
			// so the normal ladder applies unchanged.
			name:           "started session falls through to normal rules",
			facts:          contract.SessionFacts{Activity: contract.ActivityIdle, LastActivityAt: statusNow},
			signalExpected: true,
			now:            statusNow,
			want:           contract.StatusIdle,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			facts := tt.facts
			facts.SignalExpected = tt.signalExpected
			got := contract.DeriveStatus(facts, tt.prs, tt.now, grace)
			if got != tt.want {
				t.Fatalf("DeriveStatus() = %q, want %q", got, tt.want)
			}
		})
	}
}

// The board card for a staged session has to read as not-yet-started, and must
// not claim work is in progress or that a hook pipeline broke.
func TestKanbanPresentationNotStartedForStagedSession(t *testing.T) {
	const grace = 90 * time.Second

	staged := contract.KanbanSessionFacts{
		SessionFacts: contract.SessionFacts{
			Activity:       contract.ActivityIdle,
			LastActivityAt: statusNow.Add(-2 * grace),
			// A signal-capable TUI session would normally be expected to report
			// by now; that is exactly what makes the staged reading a real
			// decision rather than a no-op.
			SignalExpected: true,
			AgentDeferred:  true,
		},
	}

	got := contract.DeriveKanbanPresentation(staged, nil, statusNow, grace)
	if got.Column != contract.KanbanBuilding {
		t.Errorf("Column = %q, want %q", got.Column, contract.KanbanBuilding)
	}
	if got.DisplayStatus != contract.DisplayNotStarted {
		t.Errorf("DisplayStatus = %q, want %q", got.DisplayStatus, contract.DisplayNotStarted)
	}

	// Starting the session must restore the ordinary phrases.
	started := staged
	started.AgentDeferred = false
	gotStarted := contract.DeriveKanbanPresentation(started, nil, statusNow, grace)
	if gotStarted.DisplayStatus == contract.DisplayNotStarted {
		t.Errorf("DisplayStatus = %q after starting, want an ordinary phrase", gotStarted.DisplayStatus)
	}
	if gotStarted.DisplayStatus != contract.DisplayNoSignal {
		t.Errorf("DisplayStatus = %q, want %q for a silent started session", gotStarted.DisplayStatus, contract.DisplayNoSignal)
	}
}
