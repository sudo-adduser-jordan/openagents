package domain

import "time"

// ManagerReengagementState is the durable state of a manager's bounded
// self-continuation loop.
type ManagerReengagementState string

const (
	// ManagerReengagementActive indicates that automatic re-engagement is eligible to run.
	ManagerReengagementActive ManagerReengagementState = "active"
	// ManagerReengagementCompleted indicates that the manager declared its work complete.
	ManagerReengagementCompleted ManagerReengagementState = "completed"
	// ManagerReengagementExhausted indicates that the automatic retry ceiling was reached.
	ManagerReengagementExhausted ManagerReengagementState = "exhausted"
)

// ManagerReengagement records retry progress independently from derived
// session status.
type ManagerReengagement struct {
	SessionID            SessionID
	AttemptCount         int
	NextAttemptAt        time.Time
	LastAttemptAt        time.Time
	ProgressSinceAttempt bool
	AttentionNotified    bool
	State                ManagerReengagementState
	CreatedAt            time.Time
	UpdatedAt            time.Time
}
