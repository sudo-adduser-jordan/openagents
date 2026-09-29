package chat

import (
	"context"
	"errors"
	"fmt"

	"github.com/sudo-adduser-jordan/open-agents/backend/internal/domain"
	"github.com/sudo-adduser-jordan/open-agents/backend/internal/ports"
)

// OpenCode provider session modes bound to the worker delivery stage. These
// are the provider's own values on the `mode` config option, not Open Agents
// vocabulary.
const (
	openCodeModePlan  = "plan"
	openCodeModeBuild = "build"
)

// WorkflowModeToOpenCodeMode maps a worker delivery stage onto the opencode
// provider session mode driven through the existing `mode` config option:
// planning runs plan mode, building runs build mode. Empty means no binding:
// manager sessions never leave the manager stage, and unknown stages carry no
// provider posture.
func WorkflowModeToOpenCodeMode(kind domain.SessionKind, mode domain.WorkflowMode) string {
	if kind != domain.KindWorker {
		return ""
	}
	switch mode {
	case domain.WorkflowModePlanning:
		return openCodeModePlan
	case domain.WorkflowModeBuilding:
		return openCodeModeBuild
	default:
		return ""
	}
}

// SyncProviderMode drives a worker's opencode provider session into the mode
// matching its delivery stage, through the existing `mode` config option.
//
// The daemon calls this on every worker SetWorkflowMode; the persisted stage
// remains the authority. Conflict behavior: the stage-derived value wins at
// stage boundaries and at controller (re)starts (which restore the persisted
// choice). An agent self-change through the same control stands mid-turn —
// this never interrupts an in-flight turn; the provider applies the mode to
// subsequent turns — until the next stage change or restart re-applies the
// stage value. Manager and non-opencode sessions are untouched.
func (s *Service) SyncProviderMode(ctx context.Context, id domain.SessionID, mode domain.WorkflowMode) error {
	if s.sessions == nil {
		return errors.New("chat session reader is not configured")
	}
	rec, found, err := s.sessions.GetSession(ctx, id)
	if err != nil {
		return fmt.Errorf("read session %s for provider mode sync: %w", id, err)
	}
	if !found {
		return nil
	}
	if rec.Harness != domain.HarnessOpenCode {
		return nil
	}
	desired := WorkflowModeToOpenCodeMode(rec.Kind, mode)
	if desired == "" {
		return nil
	}
	if _, err := s.Controller(id); err == nil {
		// Live controller: drive the provider through the mode control. This
		// also persists the choice into the conversation settings, so a
		// restart restores exactly what the stage asked for.
		_, err := s.SetConfigOption(ctx, id, "mode", ports.ChatConfigOptionValue{Select: desired})
		return err
	}
	// No live controller: persist the stage-derived choice so the next start
	// restores it. A session with no conversation row yet needs nothing: a
	// fresh start derives the initial mode from the stage directly.
	conversation, err := s.store.ConversationForSession(ctx, id)
	if errors.Is(err, domain.ErrNoConversation) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("conversation for %s for provider mode sync: %w", id, err)
	}
	if conversation.Settings.OpenCodeMode == desired {
		return nil
	}
	settings := conversation.Settings
	settings.OpenCodeMode = desired
	if err := s.store.SetConversationSettings(ctx, conversation.ID, settings, s.now()); err != nil {
		return fmt.Errorf("record stage-derived provider mode for %s: %w", id, err)
	}
	return nil
}

// applyFreshOpenCodeMode sets the stage-derived provider mode on a freshly
// started opencode worker controller that has no stored choice yet.
//
// Best-effort by design: a fresh spawn must not fail because the provider did
// not advertise the mode control. When the control is absent the provider
// default stands (the TUI planning tool-policy overlay still restricts a
// planning worker), and the next stage change or restart repairs the mode.
// A failed apply is logged, never returned.
func (s *Service) applyFreshOpenCodeMode(ctx context.Context, conv ports.ChatConversation, conversation *domain.ConversationRecord, mode domain.WorkflowMode) {
	desired := WorkflowModeToOpenCodeMode(domain.KindWorker, mode)
	if desired == "" || conversation.Settings.OpenCodeMode != "" {
		return
	}
	configurer, ok := conv.(ports.ChatConfigOptionController)
	if !ok {
		return
	}
	options, err := configurer.ListConfigOptions(ctx)
	if err != nil {
		s.log.Debug("fresh provider mode: mode catalog unavailable; leaving provider default", "mode", desired, "error", err)
		return
	}
	advertised := false
	ready := false
	for _, option := range options {
		if option.ID != "mode" {
			continue
		}
		advertised = true
		if option.Current.Select == desired {
			ready = true
			break
		}
		confirmed, err := configurer.SetConfigOption(ctx, "mode", ports.ChatConfigOptionValue{Select: desired})
		if err != nil {
			s.log.Debug("fresh provider mode: provider refused stage mode; leaving provider default", "mode", desired, "error", err)
			return
		}
		for _, opt := range confirmed {
			if opt.ID == "mode" && opt.Current.Select == desired {
				ready = true
				break
			}
		}
		if !ready {
			s.log.Debug("fresh provider mode: provider did not confirm stage mode; leaving provider default", "mode", desired)
			return
		}
		break
	}
	if !advertised || !ready {
		return
	}
	settings := conversation.Settings
	settings.OpenCodeMode = desired
	if err := s.store.SetConversationSettings(ctx, conversation.ID, settings, s.now()); err != nil {
		s.log.Debug("fresh provider mode: settings write failed", "mode", desired, "error", err)
		return
	}
	conversation.Settings = settings
}
