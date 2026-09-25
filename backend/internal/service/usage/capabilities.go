package usage

import "github.com/sudo-adduser-jordan/open-agents/backend/internal/domain"

// SupportedHarness reports whether the harness has a certified usage pipeline.
func SupportedHarness(h domain.AgentHarness) bool {
	return h == domain.HarnessOpenCode
}
