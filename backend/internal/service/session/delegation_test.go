package session

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/sudo-adduser-jordan/open-agents/backend/internal/domain"
	"github.com/sudo-adduser-jordan/open-agents/backend/internal/ports"
)

func TestDelegateTaskSpawnsPlanningWorkerThenHandsItToNewestActiveManager(t *testing.T) {
	tests := []struct {
		name      string
		agent     domain.AgentHarness
		model     string
		mode      domain.SessionMode
		approval  domain.PermissionMode
		wantAgent string
	}{
		{name: "project default"},
		{
			name:      "requested agent model mode and approvals",
			agent:     domain.HarnessOpenCode,
			model:     "  sonnet-custom  ",
			mode:      domain.SessionModeChat,
			approval:  domain.PermissionModeBypassPermissions,
			wantAgent: "opencode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := newFakeStore()
			st.projects["open-agents"] = domain.ProjectRecord{ID: "open-agents"}
			now := time.Now().UTC()
			st.sessions["orch-old"] = domain.SessionRecord{ID: "orch-old", ProjectID: "open-agents", Kind: domain.KindManager, CreatedAt: now.Add(-time.Minute)}
			st.sessions["orch-new"] = domain.SessionRecord{ID: "orch-new", ProjectID: "open-agents", Kind: domain.KindManager, CreatedAt: now}
			st.sessions["orch-exited"] = domain.SessionRecord{ID: "orch-exited", ProjectID: "open-agents", Kind: domain.KindManager, Activity: domain.Activity{State: domain.ActivityExited}, CreatedAt: now.Add(time.Minute)}
			st.sessions["orch-dead"] = domain.SessionRecord{ID: "orch-dead", ProjectID: "open-agents", Kind: domain.KindManager, IsTerminated: true, CreatedAt: now.Add(2 * time.Minute)}
			st.sessions["worker"] = domain.SessionRecord{ID: "worker", ProjectID: "open-agents", Kind: domain.KindWorker, CreatedAt: now.Add(3 * time.Minute)}
			cmd := &fakeCommander{spawnRecord: domain.SessionRecord{ID: "mer-7", ProjectID: "open-agents", Kind: domain.KindWorker}}
			svc := &Service{store: st, manager: cmd, runBackground: runInline}

			brief := "  Fix the renderer\nwithout changing the API.  "
			out, err := svc.DelegateTask(context.Background(), DelegateTaskInput{
				ProjectID: "open-agents", Brief: brief, RequestedAgent: tt.agent,
				Model: tt.model, RequestedMode: tt.mode, ApprovalMode: tt.approval,
			})
			if err != nil {
				t.Fatalf("DelegateTask: %v", err)
			}
			if out.WorkerID != "mer-7" {
				t.Fatalf("out = %#v, want worker mer-7", out)
			}
			// Task creation is the daemon's job: the worker spawn is synchronous.
			if !cmd.spawned || cmd.spawnCalls != 1 {
				t.Fatalf("worker spawns = %d, want exactly one", cmd.spawnCalls)
			}
			cfg := cmd.spawnedCfg
			if cfg.ProjectID != "open-agents" || cfg.Kind != domain.KindWorker {
				t.Fatalf("spawn identity = %#v, want open-agents worker", cfg)
			}
			if cfg.RequestedWorkflowMode != domain.WorkflowModePlanning {
				t.Fatalf("spawn workflow mode = %q, want planning", cfg.RequestedWorkflowMode)
			}
			if cfg.Harness != tt.agent || cfg.Prompt != brief {
				t.Fatalf("spawn fields = %#v, want harness %q with the brief", cfg, tt.agent)
			}
			if cfg.DisplayName != "Fix the renderer wit" {
				t.Fatalf("spawn display name = %q, want a 20-char provisional title", cfg.DisplayName)
			}
			if got := strings.TrimSpace(tt.model); cfg.AgentConfig.Model != got {
				t.Fatalf("spawn model = %q, want %q", cfg.AgentConfig.Model, got)
			}
			if cfg.AgentConfig.Permissions != tt.approval {
				t.Fatalf("spawn permissions = %q, want %q", cfg.AgentConfig.Permissions, tt.approval)
			}
			if cfg.RequestedMode != tt.mode {
				t.Fatalf("spawn mode = %q, want %q", cfg.RequestedMode, tt.mode)
			}
			// The manager owns review-and-advance, not creation.
			if len(cmd.ready) != 1 || cmd.ready[0] != "orch-new" {
				t.Fatalf("readiness waits = %#v; want orch-new", cmd.ready)
			}
			if len(cmd.sent) != 1 || cmd.sent[0] != "orch-new" {
				t.Fatalf("sent = %#v; want orch-new", cmd.sent)
			}
			for _, want := range []string{
				"Open Agents NEW TASK",
				"Worker session id: mer-7",
				"open-agents build mer-7",
				"Project: open-agents",
				brief,
			} {
				if !strings.Contains(cmd.sentMessages[0], want) {
					t.Fatalf("task handoff missing %q:\n%s", want, cmd.sentMessages[0])
				}
			}
			if !strings.Contains(cmd.sentMessages[0], "spawn another worker") {
				t.Fatalf("task handoff must forbid a second worker spawn:\n%s", cmd.sentMessages[0])
			}
			if tt.wantAgent != "" && !strings.Contains(cmd.sentMessages[0], "Requested agent: "+tt.wantAgent) {
				t.Fatalf("task handoff missing requested agent:\n%s", cmd.sentMessages[0])
			}
			if tt.model != "" && !strings.Contains(cmd.sentMessages[0], "Requested model: sonnet-custom") {
				t.Fatalf("task handoff missing requested model:\n%s", cmd.sentMessages[0])
			}
			if tt.mode != "" && !strings.Contains(cmd.sentMessages[0], "Requested interface mode: "+string(tt.mode)) {
				t.Fatalf("task handoff missing requested mode:\n%s", cmd.sentMessages[0])
			}
			if tt.approval != "" && !strings.Contains(cmd.sentMessages[0], "Requested approval mode: "+string(tt.approval)) {
				t.Fatalf("task handoff missing requested approval mode:\n%s", cmd.sentMessages[0])
			}
		})
	}
}

func TestDelegateTaskPassesAttachmentsToWorkerSpawn(t *testing.T) {
	st := newFakeStore()
	st.projects["open-agents"] = domain.ProjectRecord{ID: "open-agents"}
	st.sessions["orch"] = domain.SessionRecord{ID: "orch", ProjectID: "open-agents", Kind: domain.KindManager}
	cmd := &fakeCommander{spawnRecord: domain.SessionRecord{ID: "mer-7", ProjectID: "open-agents", Kind: domain.KindWorker}}

	out, err := (&Service{store: st, manager: cmd, runBackground: runInline}).DelegateTask(
		context.Background(),
		DelegateTaskInput{
			ProjectID:   "open-agents",
			Brief:       "Use the attached image.",
			Attachments: []ports.SpawnAttachment{{Ext: ".png", Data: []byte("x")}},
		},
	)
	if err != nil {
		t.Fatalf("DelegateTask: %v", err)
	}
	if out.WorkerID != "mer-7" {
		t.Fatalf("out = %#v, want worker mer-7", out)
	}
	if len(cmd.spawnedCfg.Attachments) != 1 {
		t.Fatalf("spawn attachments = %#v, want one", cmd.spawnedCfg.Attachments)
	}
	if got := cmd.spawnedCfg.Attachments[0]; got.Ext != ".png" || string(got.Data) != "x" {
		t.Fatalf("spawn attachment = %#v, want the png bytes", got)
	}
	// Attachments ride the worker spawn, which lands in the worker worktree.
	if len(cmd.sent) != 1 || cmd.sent[0] != "orch" {
		t.Fatalf("sent = %#v; want the handoff to orch", cmd.sent)
	}
}

func TestDelegateTaskOmitsDefaultApprovalMode(t *testing.T) {
	st := newFakeStore()
	st.projects["open-agents"] = domain.ProjectRecord{ID: "open-agents"}
	st.sessions["orch"] = domain.SessionRecord{ID: "orch", ProjectID: "open-agents", Kind: domain.KindManager}
	cmd := &fakeCommander{}

	if _, err := (&Service{store: st, manager: cmd, runBackground: runInline}).DelegateTask(
		context.Background(),
		DelegateTaskInput{ProjectID: "open-agents", Brief: "Fix it", ApprovalMode: domain.PermissionModeDefault},
	); err != nil {
		t.Fatalf("DelegateTask: %v", err)
	}
	if len(cmd.sentMessages) != 1 {
		t.Fatalf("sent = %#v; want one message", cmd.sent)
	}
	if strings.Contains(cmd.sentMessages[0], "Requested approval mode:") {
		t.Fatalf("message should omit the default approval mode:\n%s", cmd.sentMessages[0])
	}
	if cfg := cmd.spawnedCfg; cfg.AgentConfig.Permissions != domain.PermissionModeDefault {
		t.Fatalf("spawn permissions = %q, want the default", cfg.AgentConfig.Permissions)
	}
}

func TestDelegateTaskAcceptsEmptyBrief(t *testing.T) {
	st := newFakeStore()
	st.projects["open-agents"] = domain.ProjectRecord{ID: "open-agents"}
	st.sessions["orch"] = domain.SessionRecord{ID: "orch", ProjectID: "open-agents", Kind: domain.KindManager}
	cmd := &fakeCommander{spawnRecord: domain.SessionRecord{ID: "mer-7", ProjectID: "open-agents", Kind: domain.KindWorker}}

	out, err := (&Service{store: st, manager: cmd, runBackground: runInline}).DelegateTask(
		context.Background(),
		DelegateTaskInput{ProjectID: "open-agents", Brief: " \n\t "},
	)
	if err != nil {
		t.Fatalf("DelegateTask: %v", err)
	}
	if out.WorkerID != "mer-7" {
		t.Fatalf("out = %#v, want worker mer-7", out)
	}
	if cmd.spawnedCfg.Prompt != "" {
		t.Fatalf("spawn prompt = %q, want a promptless worker", cmd.spawnedCfg.Prompt)
	}
	if cmd.spawnedCfg.DisplayName != delegatedTaskUntitledName {
		t.Fatalf("spawn display name = %q, want %q", cmd.spawnedCfg.DisplayName, delegatedTaskUntitledName)
	}
	if len(cmd.sent) != 1 {
		t.Fatalf("sent = %#v; want the handoff even for an empty brief", cmd.sent)
	}
}

func TestDelegateTaskRejectsUnknownAgentAndModeBeforeSpawning(t *testing.T) {
	for _, tt := range []struct {
		name string
		in   DelegateTaskInput
	}{
		{name: "unknown agent", in: DelegateTaskInput{ProjectID: "open-agents", Brief: "x", RequestedAgent: "nope"}},
		{name: "invalid mode", in: DelegateTaskInput{ProjectID: "open-agents", Brief: "x", RequestedMode: "nope"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			st := newFakeStore()
			st.projects["open-agents"] = domain.ProjectRecord{ID: "open-agents"}
			cmd := &fakeCommander{}

			if _, err := (&Service{store: st, manager: cmd, runBackground: runInline}).DelegateTask(
				context.Background(), tt.in,
			); err == nil {
				t.Fatal("DelegateTask accepted an invalid request")
			}
			if cmd.spawned || len(cmd.ready) != 0 || len(cmd.sent) != 0 {
				t.Fatalf("invalid request spawned or contacted the manager: spawned=%v ready=%#v sent=%#v", cmd.spawned, cmd.ready, cmd.sent)
			}
		})
	}
}

func TestDelegateTaskMapsWorkerSpawnFailure(t *testing.T) {
	st := newFakeStore()
	st.projects["open-agents"] = domain.ProjectRecord{ID: "open-agents"}
	st.sessions["orch"] = domain.SessionRecord{ID: "orch", ProjectID: "open-agents", Kind: domain.KindManager}
	cmd := &fakeCommander{spawnErr: errors.New("spawn: boom")}

	if _, err := (&Service{store: st, manager: cmd, runBackground: runInline}).DelegateTask(
		context.Background(), DelegateTaskInput{ProjectID: "open-agents", Brief: "Fix it"},
	); err == nil {
		t.Fatal("DelegateTask accepted a failed spawn")
	}
	// A worker that was never created must not be handed to the manager.
	if len(cmd.sent) != 0 {
		t.Fatalf("sent = %#v; want no handoff after spawn failure", cmd.sent)
	}
}

func TestDelegateTaskKeepsWorkerWhenManagerHandoffFails(t *testing.T) {
	for _, tt := range []struct {
		name string
		cmd  *fakeCommander
	}{
		{name: "readiness never resolves", cmd: &fakeCommander{readyErr: errors.New("readiness timed out")}},
		{name: "delivery fails", cmd: &fakeCommander{sendErr: errors.New("manager exited")}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			st := newFakeStore()
			st.projects["open-agents"] = domain.ProjectRecord{ID: "open-agents"}
			st.sessions["orch"] = domain.SessionRecord{ID: "orch", ProjectID: "open-agents", Kind: domain.KindManager}
			tt.cmd.spawnRecord = domain.SessionRecord{ID: "mer-7", ProjectID: "open-agents", Kind: domain.KindWorker}

			// The handoff is best-effort: the worker spawn already committed,
			// so a manager failure must not fail task creation.
			out, err := (&Service{store: st, manager: tt.cmd, runBackground: runInline}).DelegateTask(
				context.Background(), DelegateTaskInput{ProjectID: "open-agents", Brief: "Fix it"},
			)
			if err != nil {
				t.Fatalf("DelegateTask: %v", err)
			}
			if out.WorkerID != "mer-7" {
				t.Fatalf("out = %#v, want worker mer-7", out)
			}
		})
	}
}

func TestDelegateTaskResumesNewestExitedManagerForHandoff(t *testing.T) {
	st := newFakeStore()
	st.projects["open-agents"] = domain.ProjectRecord{ID: "open-agents"}
	now := time.Now().UTC()
	st.sessions["orch-old"] = domain.SessionRecord{ID: "orch-old", ProjectID: "open-agents", Kind: domain.KindManager, Activity: domain.Activity{State: domain.ActivityExited}, CreatedAt: now.Add(-time.Minute)}
	st.sessions["orch-new"] = domain.SessionRecord{ID: "orch-new", ProjectID: "open-agents", Kind: domain.KindManager, Activity: domain.Activity{State: domain.ActivityExited}, CreatedAt: now}
	cmd := &fakeCommander{spawnRecord: domain.SessionRecord{ID: "mer-7", ProjectID: "open-agents", Kind: domain.KindWorker}}

	out, err := (&Service{store: st, manager: cmd, runBackground: runInline}).DelegateTask(context.Background(), DelegateTaskInput{ProjectID: "open-agents", Brief: "Fix it"})
	if err != nil {
		t.Fatalf("DelegateTask: %v", err)
	}
	if out.WorkerID != "mer-7" {
		t.Fatalf("out = %#v, want worker mer-7", out)
	}
	if len(cmd.resumed) != 1 || cmd.resumed[0] != "orch-new" {
		t.Fatalf("resumed = %#v, want orch-new", cmd.resumed)
	}
	if len(cmd.ready) != 1 || cmd.ready[0] != "orch-new" {
		t.Fatalf("readiness waits = %#v; want orch-new", cmd.ready)
	}
	if len(cmd.sent) != 1 || cmd.sent[0] != "orch-new" {
		t.Fatalf("sent = %#v; want orch-new", cmd.sent)
	}
}

func TestDelegateTaskStartsMissingManagerForHandoff(t *testing.T) {
	st := newFakeStore()
	st.projects["open-agents"] = domain.ProjectRecord{ID: "open-agents"}
	st.sessions["orch-dead"] = domain.SessionRecord{ID: "orch-dead", ProjectID: "open-agents", Kind: domain.KindManager, IsTerminated: true}
	cmd := &fakeCommander{spawnFunc: func(cfg ports.SpawnConfig) domain.SessionRecord {
		if cfg.Kind == domain.KindManager {
			return domain.SessionRecord{ID: "orch-new", ProjectID: cfg.ProjectID, Kind: cfg.Kind}
		}
		return domain.SessionRecord{ID: "mer-7", ProjectID: cfg.ProjectID, Kind: cfg.Kind}
	}}

	out, err := (&Service{store: st, manager: cmd, runBackground: runInline}).DelegateTask(context.Background(), DelegateTaskInput{ProjectID: "open-agents", Brief: "Fix it"})
	if err != nil {
		t.Fatalf("DelegateTask: %v", err)
	}
	if out.WorkerID != "mer-7" {
		t.Fatalf("out = %#v, want worker mer-7", out)
	}
	// The worker first, then the fresh manager for the handoff.
	if cmd.spawnCalls != 2 {
		t.Fatalf("spawn calls = %d, want worker plus manager", cmd.spawnCalls)
	}
	if cmd.spawnedCfgs[0].Kind != domain.KindWorker || cmd.spawnedCfgs[1].Kind != domain.KindManager {
		t.Fatalf("spawn order = %#v, want worker then manager", cmd.spawnedCfgs)
	}
	if len(cmd.ready) != 1 || cmd.ready[0] != "orch-new" {
		t.Fatalf("readiness waits = %#v; want orch-new", cmd.ready)
	}
	if len(cmd.sent) != 1 || cmd.sent[0] != "orch-new" {
		t.Fatalf("sent = %#v; want orch-new", cmd.sent)
	}
}

func runInline(work func()) {
	work()
}
