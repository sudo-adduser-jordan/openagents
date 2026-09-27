package session

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/sudo-adduser-jordan/open-agents/backend/internal/domain"
	"github.com/sudo-adduser-jordan/open-agents/backend/internal/httpd/apierr"
	"github.com/sudo-adduser-jordan/open-agents/backend/internal/ports"
	sessionmanager "github.com/sudo-adduser-jordan/open-agents/backend/internal/session_manager"
)

const (
	delegatedTaskTitleLimit     = 20
	delegatedTaskUntitledName   = "Untitled task"
	delegatedTaskHandoffTimeout = time.Minute
)

// DelegateTaskInput describes a task Open Agents should spawn as a worker session. Brief
// may be empty to open an idle worker that the user can instruct later. Empty
// RequestedAgent means the spawn uses the project's worker-agent default.
type DelegateTaskInput struct {
	ProjectID      domain.ProjectID
	Brief          string
	RequestedAgent domain.AgentHarness
	Model          string
	ApprovalMode   domain.PermissionMode
	RequestedMode  domain.SessionMode
	Attachments    []ports.SpawnAttachment
}

// DelegateTaskOutcome identifies the spawned worker. ManagerID names the manager
// that received the follow-up review-and-advance handoff when that best-effort
// delivery resolved before the response was built; it is empty when the handoff
// is still running in the background.
type DelegateTaskOutcome struct {
	ManagerID domain.SessionID
	WorkerID  domain.SessionID
}

// DelegateTask spawns the worker directly, matching `open-agents spawn`, with a
// provisional display name derived from the task brief. The worker spawn is the
// commit point: task creation never depends on the manager agent complying with
// a message. Open Agents then best-effort hands the worker to the project
// manager in the background so it can scope the work, review the plan, and
// advance the build — resuming or creating the manager when necessary.
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
	prompt := in.Brief
	if strings.TrimSpace(prompt) == "" {
		prompt = ""
	}

	worker, _, _, err := s.manager.Spawn(ctx, ports.SpawnConfig{
		ProjectID:             in.ProjectID,
		Kind:                  domain.KindWorker,
		RequestedWorkflowMode: domain.WorkflowModePlanning,
		Harness:               in.RequestedAgent,
		Prompt:                prompt,
		DisplayName:           delegatedTaskDisplayName(in.Brief),
		AgentConfig: ports.AgentConfig{
			Model:       strings.TrimSpace(in.Model),
			Permissions: in.ApprovalMode,
		},
		RequestedMode: in.RequestedMode,
		Attachments:   in.Attachments,
	})
	if err != nil {
		return DelegateTaskOutcome{}, toSpawnAPIError(err)
	}

	// The worker spawn is the commit point. Manager resolution and the handoff
	// message must never hold the new-task response open: a promptless worker
	// stays idle with its provisional title until the manager or the user
	// supplies instructions.
	s.handWorkerToManagerInBackground(worker.ID, in)
	return DelegateTaskOutcome{WorkerID: worker.ID}, nil
}

func (s *Service) handWorkerToManagerInBackground(workerID domain.SessionID, in DelegateTaskInput) {
	work := func() {
		base := s.backgroundContext
		if base == nil {
			base = context.Background()
		}
		ctx, cancel := context.WithTimeout(base, delegatedTaskHandoffTimeout)
		defer cancel()

		if err := s.handWorkerToManager(ctx, workerID, in); err != nil && s.logger != nil {
			s.logger.Warn("delegated task manager handoff failed",
				"projectID", in.ProjectID,
				"workerID", workerID,
				"error", err,
			)
		}
	}
	if s.runBackground != nil {
		s.runBackground(work)
		return
	}
	go work()
}

func (s *Service) handWorkerToManager(ctx context.Context, workerID domain.SessionID, in DelegateTaskInput) error {
	managerID, err := s.taskManager(ctx, in.ProjectID)
	if err != nil {
		return err
	}
	if err := s.manager.WaitForMessageDeliveryReady(ctx, managerID); err != nil {
		return fmt.Errorf("wait for task manager %s: %w", managerID, err)
	}
	if err := s.manager.Send(ctx, managerID, workerHandoffMessage(workerID, in), nil); err != nil {
		return fmt.Errorf("send task handoff to %s: %w", managerID, err)
	}
	return nil
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

func delegatedTaskDisplayName(brief string) string {
	title := strings.Join(strings.Fields(brief), " ")
	if title == "" {
		return delegatedTaskUntitledName
	}
	if utf8.RuneCountInString(title) <= delegatedTaskTitleLimit {
		return title
	}
	return strings.TrimSpace(string([]rune(title)[:delegatedTaskTitleLimit]))
}

// workerHandoffMessage tells the project manager about a worker the daemon just
// spawned. The worker starts in planning mode; the manager scopes the work,
// reviews the plan, and advances the build. It must not spawn a second worker
// for this task and must not implement the task in the manager session.
func workerHandoffMessage(workerID domain.SessionID, in DelegateTaskInput) string {
	var b strings.Builder
	b.WriteString("Open Agents NEW TASK\n")
	b.WriteString("A worker was already spawned with the human's task below and starts in planning mode. You own it from here: scope it, review its plan, and advance it. Do not spawn another worker for this task and do not implement it in this manager session.\n\n")
	b.WriteString("1. Scope: inspect current state. If the brief needs narrowing, steer the worker with `open-agents send --session ")
	b.WriteString(string(workerID))
	b.WriteString(" --message \"...\"`.\n")
	b.WriteString("2. Review the plan: read it with `open-agents session get ")
	b.WriteString(string(workerID))
	b.WriteString("`. If the plan is wrong or incomplete, send corrections and leave the worker in planning.\n")
	b.WriteString("3. Build: once the plan is right, advance the worker with `open-agents build ")
	b.WriteString(string(workerID))
	b.WriteString("`. This is the only way it starts implementing.\n\n")

	b.WriteString("Project: ")
	b.WriteString(string(in.ProjectID))
	b.WriteString("\nWorker session id: ")
	b.WriteString(string(workerID))
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

	b.WriteString("\nTask brief:\n")
	b.WriteString(in.Brief)
	return b.String()
}
