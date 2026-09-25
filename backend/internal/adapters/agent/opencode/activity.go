package opencode

import "github.com/sudo-adduser-jordan/open-agents/backend/internal/domain"

// DeriveActivityState maps an opencode plugin hook event onto an Open Agents activity
// state. The opencode plugin (assets/open-agents-activity.ts) normalizes opencode's
// native events to "session-start" / "user-prompt-submit" / "stop" before
// invoking `open-agents hooks opencode <event>`. The bool is false when the event
// carries no activity signal.
func DeriveActivityState(event string, _ []byte) (domain.ActivityState, bool) {
	switch event {
	case "session-start":
		return domain.ActivityActive, true
	case "user-prompt-submit":
		return domain.ActivityActive, true
	case "active":
		return domain.ActivityActive, true
	case "stop":
		return domain.ActivityIdle, true
	case "permission-blocked":
		return domain.ActivityBlocked, true
	default:
		return "", false
	}
}
