package session

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/sudo-adduser-jordan/open-agents/backend/internal/domain"
	"github.com/sudo-adduser-jordan/open-agents/backend/internal/httpd/apierr"
	"github.com/sudo-adduser-jordan/open-agents/backend/internal/ports"
	sessionmanager "github.com/sudo-adduser-jordan/open-agents/backend/internal/session_manager"
)

// DelegateTaskInput describes a task the user submitted through the New Task
// composer. Open Agents does not create the worker itself: the project manager
// scopes the work, spawns the worker in planning mode, reviews its plan, and
// advances it into building. Brief may be empty to hand the manager a task to
// scope later. Empty RequestedAgent leaves the choice to the project default.
type DelegateTaskInput struct {
	ProjectID      domain.ProjectID
	Brief          string
	RequestedAgent domain.AgentHarness
	Model          string
	ApprovalMode   domain.PermissionMode
	RequestedMode  domain.SessionMode
	Attachments    []ports.SpawnAttachment
}

// DelegateTaskOutcome identifies the manager that now owns the new task. There
// is deliberately no worker id: the manager creates the worker, so the board
// card appears when the manager spawns it rather than when the user submits.
type DelegateTaskOutcome struct {
	ManagerID domain.SessionID
}

// DelegateTask hands the user's brief to the project manager, resuming or
// creating the manager when necessary, and asks it to scope, delegate, review,
// and advance the work. The manager owns task creation end to end; the daemon
// only guarantees the brief reaches it.
func (s *Service) DelegateTask(ctx context.Context, in DelegateTaskInput) (DelegateTaskOutcome, error) {
	if _, err := s.requireProject(ctx, in.ProjectID); err != nil {
		return DelegateTaskOutcome{}, err
	}
	if in.RequestedAgent != "" && !in.RequestedAgent.IsKnown() {
		return DelegateTaskOutcome{}, apierr.Invalid("UNKNOWN_HARNESS", "Unknown requested agent", nil)
	}
	if in.RequestedMode != "" && !in.RequestedMode.Valid() {
		return DelegateTaskOutcome{}, apierr.Invalid("INVALID_SESSION_MODE", "mode must be chat or tui", nil)
	}

	managerID, err := s.taskManager(ctx, in.ProjectID)
	if err != nil {
		return DelegateTaskOutcome{}, err
	}
	if err := s.manager.WaitForMessageDeliveryReady(ctx, managerID); err != nil {
		return DelegateTaskOutcome{}, fmt.Errorf("wait for task manager %s: %w", managerID, err)
	}

	attachmentPaths, err := s.stageDelegatedAttachments(ctx, managerID, in.Attachments)
	if err != nil {
		return DelegateTaskOutcome{}, err
	}
	if err := s.manager.Send(ctx, managerID, taskDelegationMessage(managerID, in, attachmentPaths), nil); err != nil {
		return DelegateTaskOutcome{}, fmt.Errorf("send task to %s: %w", managerID, err)
	}
	return DelegateTaskOutcome{ManagerID: managerID}, nil
}

// stageDelegatedAttachments writes the submitted files into the manager's
// workspace and returns their absolute paths. The manager passes those paths
// on to the worker it spawns, so they are resolved against the manager's
// workspace rather than a worker worktree that does not exist yet.
func (s *Service) stageDelegatedAttachments(
	ctx context.Context,
	managerID domain.SessionID,
	attachments []ports.SpawnAttachment,
) ([]string, error) {
	if len(attachments) == 0 {
		return nil, nil
	}
	record, ok, err := s.store.GetSession(ctx, managerID)
	if err != nil {
		return nil, fmt.Errorf("load task manager %s: %w", managerID, err)
	}
	if !ok {
		return nil, fmt.Errorf("task manager %s no longer exists", managerID)
	}
	workspace := strings.TrimSpace(record.Metadata.WorkspacePath)
	if workspace == "" {
		return nil, fmt.Errorf("task manager %s has no workspace for attachments", managerID)
	}
	refs, err := s.manager.StageAttachments(ctx, managerID, attachments)
	if err != nil {
		return nil, fmt.Errorf("stage task attachments: %w", err)
	}
	paths := make([]string, 0, len(refs))
	for _, ref := range refs {
		if filepath.IsAbs(ref) {
			paths = append(paths, ref)
			continue
		}
		paths = append(paths, filepath.Join(workspace, filepath.FromSlash(ref)))
	}
	return paths, nil
}

// taskManager resolves the project manager that should own a new task,
// resuming the newest exited manager or spawning a fresh one as needed.
func (s *Service) taskManager(ctx context.Context, projectID domain.ProjectID) (domain.SessionID, error) {
	unlock := s.lockManagerProject(projectID)
	managers, err := s.activeManagers(ctx, projectID)
	if err != nil {
		unlock()
		return "", fmt.Errorf("list project managers: %w", err)
	}

	running := make([]domain.Session, 0, len(managers))
	for _, manager := range managers {
		if manager.Activity.State != domain.ActivityExited {
			running = append(running, manager)
		}
	}
	if len(running) > 0 {
		managerID := newestSession(running).ID
		unlock()
		return managerID, nil
	}
	if len(managers) > 0 {
		managerID := newestSession(managers).ID
		_, resumeErr := s.manager.ResumeAgentWithMode(ctx, managerID)
		unlock()
		if resumeErr != nil && !errors.Is(resumeErr, sessionmanager.ErrAgentNotExited) {
			return "", fmt.Errorf("resume project manager %s: %w", managerID, resumeErr)
		}
		return managerID, nil
	}
	unlock()

	manager, err := s.SpawnManager(ctx, projectID, false, "")
	if err != nil {
		return "", fmt.Errorf("start project manager: %w", err)
	}
	return manager.ID, nil
}

func taskDelegationMessage(managerID domain.SessionID, in DelegateTaskInput, attachmentPaths []string) string {
	var b strings.Builder
	b.WriteString("Open Agents NEW TASK\n")
	b.WriteString("The human submitted this task from the New Task composer. You own it from here: scope it, delegate it, review the worker's plan, and advance it. Do not implement it in this manager session.\n")
	b.WriteString("Spawn the worker with a --name label of 20 characters or fewer, review the plan it produces, then advance it with `open-agents build <worker-session-id>`.\n\n")

	b.WriteString("Project: ")
	b.WriteString(string(in.ProjectID))
	b.WriteString("\nManager session id: ")
	b.WriteString(string(managerID))
	if agent := strings.TrimSpace(string(in.RequestedAgent)); agent != "" {
		b.WriteString("\nRequested agent: ")
		b.WriteString(agent)
	}
	if model := strings.TrimSpace(in.Model); model != "" {
		b.WriteString("\nRequested model: ")
		b.WriteString(model)
	}
	if in.RequestedMode != "" {
		b.WriteString("\nRequested interface mode: ")
		b.WriteString(string(in.RequestedMode))
	}
	if in.ApprovalMode != "" && in.ApprovalMode != domain.PermissionModeDefault {
		b.WriteString("\nRequested approval mode: ")
		b.WriteString(string(in.ApprovalMode))
	}
	if len(attachmentPaths) > 0 {
		b.WriteString("\n\nAttached files, staged in this manager's workspace. Pass these paths to the worker so it can read them:\n")
		for _, path := range attachmentPaths {
			b.WriteString("- ")
			b.WriteString(path)
			b.WriteString("\n")
		}
	}

	b.WriteString("\nTask brief:\n")
	b.WriteString(in.Brief)
	return b.String()
}
