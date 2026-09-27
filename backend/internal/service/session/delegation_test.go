package session

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sudo-adduser-jordan/open-agents/backend/internal/domain"
	"github.com/sudo-adduser-jordan/open-agents/backend/internal/ports"
)

func TestDelegateTaskHandsBriefToNewestActiveManager(t *testing.T) {
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
			cmd := &fakeCommander{}
			svc := &Service{store: st, manager: cmd, runBackground: runInline}

			brief := "  Fix the renderer\nwithout changing the API.  "
			out, err := svc.DelegateTask(context.Background(), DelegateTaskInput{
				ProjectID: "open-agents", Brief: brief, RequestedAgent: tt.agent,
				Model: tt.model, RequestedMode: tt.mode, ApprovalMode: tt.approval,
			})
			if err != nil {
				t.Fatalf("DelegateTask: %v", err)
			}
			if out.ManagerID != "orch-new" {
				t.Fatalf("out = %#v, want manager orch-new", out)
			}
			// The manager creates the worker, so the daemon must not spawn one.
			if cmd.spawned || cmd.spawnCalls != 0 {
				t.Fatalf("daemon spawned a worker: calls=%d cfg=%#v", cmd.spawnCalls, cmd.spawnedCfg)
			}
			if len(cmd.ready) != 1 || cmd.ready[0] != "orch-new" {
				t.Fatalf("readiness waits = %#v; want orch-new", cmd.ready)
			}
			if len(cmd.sent) != 1 || cmd.sent[0] != "orch-new" {
				t.Fatalf("sent = %#v; want orch-new", cmd.sent)
			}
			for _, want := range []string{
				"Open Agents NEW TASK",
				"open-agents build <worker-session-id>",
				"Project: open-agents",
				"Manager session id: orch-new",
				brief,
			} {
				if !strings.Contains(cmd.sentMessages[0], want) {
					t.Fatalf("task delegation missing %q:\n%s", want, cmd.sentMessages[0])
				}
			}
			if tt.wantAgent != "" && !strings.Contains(cmd.sentMessages[0], "Requested agent: "+tt.wantAgent) {
				t.Fatalf("task delegation missing requested agent:\n%s", cmd.sentMessages[0])
			}
			if tt.model != "" && !strings.Contains(cmd.sentMessages[0], "Requested model: sonnet-custom") {
				t.Fatalf("task delegation missing requested model:\n%s", cmd.sentMessages[0])
			}
			if tt.mode != "" && !strings.Contains(cmd.sentMessages[0], "Requested interface mode: "+string(tt.mode)) {
				t.Fatalf("task delegation missing requested mode:\n%s", cmd.sentMessages[0])
			}
			if tt.approval != "" && !strings.Contains(cmd.sentMessages[0], "Requested approval mode: "+string(tt.approval)) {
				t.Fatalf("task delegation missing requested approval mode:\n%s", cmd.sentMessages[0])
			}
		})
	}
}

func TestDelegateTaskOmitsDefaultApprovalModeAndAttachmentsSection(t *testing.T) {
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
	for _, unwanted := range []string{"Requested approval mode:", "Attached files"} {
		if strings.Contains(cmd.sentMessages[0], unwanted) {
			t.Fatalf("message should omit %q:\n%s", unwanted, cmd.sentMessages[0])
		}
	}
	if len(cmd.staged) != 0 {
		t.Fatalf("staged = %#v; want no staging without attachments", cmd.staged)
	}
}

func TestDelegateTaskStagesAttachmentsIntoManagerWorkspaceAsAbsolutePaths(t *testing.T) {
	st := newFakeStore()
	st.projects["open-agents"] = domain.ProjectRecord{ID: "open-agents"}
	st.sessions["orch"] = domain.SessionRecord{
		ID: "orch", ProjectID: "open-agents", Kind: domain.KindManager,
		Metadata: domain.SessionMetadata{WorkspacePath: filepath.Join(t.TempDir(), "manager-workspace")},
	}
	cmd := &fakeCommander{stagedRefs: []string{".open-agents/attachments/attachment-ab12.png"}}

	if _, err := (&Service{store: st, manager: cmd, runBackground: runInline}).DelegateTask(
		context.Background(),
		DelegateTaskInput{
			ProjectID:   "open-agents",
			Brief:       "Fix it",
			Attachments: []ports.SpawnAttachment{{Ext: ".png", Data: []byte("x")}},
		},
	); err != nil {
		t.Fatalf("DelegateTask: %v", err)
	}
	if len(cmd.staged) != 1 || cmd.staged[0] != "orch" {
		t.Fatalf("staged = %#v; want orch", cmd.staged)
	}
	abs := filepath.Join(st.sessions["orch"].Metadata.WorkspacePath, ".open-agents/attachments/attachment-ab12.png")
	if !strings.Contains(cmd.sentMessages[0], abs) {
		t.Fatalf("message missing absolute attachment path %q:\n%s", abs, cmd.sentMessages[0])
	}
}

func TestDelegateTaskFailsWhenAttachmentsCannotBeStaged(t *testing.T) {
	st := newFakeStore()
	st.projects["open-agents"] = domain.ProjectRecord{ID: "open-agents"}
	st.sessions["orch"] = domain.SessionRecord{
		ID: "orch", ProjectID: "open-agents", Kind: domain.KindManager,
		Metadata: domain.SessionMetadata{WorkspacePath: t.TempDir()},
	}
	cmd := &fakeCommander{stageErr: errors.New("disk full")}

	_, err := (&Service{store: st, manager: cmd, runBackground: runInline}).DelegateTask(
		context.Background(),
		DelegateTaskInput{
			ProjectID:   "open-agents",
			Brief:       "Fix it",
			Attachments: []ports.SpawnAttachment{{Ext: ".png", Data: []byte("x")}},
		},
	)
	if err == nil || !strings.Contains(err.Error(), "stage task attachments") {
		t.Fatalf("err = %v, want attachment staging failure", err)
	}
	// A task whose files never landed must not be reported as delivered.
	if len(cmd.sent) != 0 {
		t.Fatalf("sent = %#v; want no delivery after staging failure", cmd.sent)
	}
}

func TestDelegateTaskFailsWhenManagerHasNoWorkspaceForAttachments(t *testing.T) {
	st := newFakeStore()
	st.projects["open-agents"] = domain.ProjectRecord{ID: "open-agents"}
	st.sessions["orch"] = domain.SessionRecord{ID: "orch", ProjectID: "open-agents", Kind: domain.KindManager}
	cmd := &fakeCommander{}

	_, err := (&Service{store: st, manager: cmd, runBackground: runInline}).DelegateTask(
		context.Background(),
		DelegateTaskInput{
			ProjectID:   "open-agents",
			Brief:       "Fix it",
			Attachments: []ports.SpawnAttachment{{Ext: ".png", Data: []byte("x")}},
		},
	)
	if err == nil || !strings.Contains(err.Error(), "no workspace for attachments") {
		t.Fatalf("err = %v, want missing workspace failure", err)
	}
}

func TestDelegateTaskAcceptsEmptyBrief(t *testing.T) {
	st := newFakeStore()
	st.projects["open-agents"] = domain.ProjectRecord{ID: "open-agents"}
	st.sessions["orch"] = domain.SessionRecord{ID: "orch", ProjectID: "open-agents", Kind: domain.KindManager}
	cmd := &fakeCommander{}

	out, err := (&Service{store: st, manager: cmd, runBackground: runInline}).DelegateTask(
		context.Background(),
		DelegateTaskInput{ProjectID: "open-agents", Brief: " \n\t "},
	)
	if err != nil {
		t.Fatalf("DelegateTask: %v", err)
	}
	if out.ManagerID != "orch" {
		t.Fatalf("out = %#v, want orch", out)
	}
	if len(cmd.sent) != 1 {
		t.Fatalf("sent = %#v; want the brief handed to the manager even when empty", cmd.sent)
	}
}

func TestDelegateTaskRejectsUnknownAgentAndModeBeforeContactingManager(t *testing.T) {
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
			if len(cmd.ready) != 0 || len(cmd.sent) != 0 {
				t.Fatalf("invalid request contacted the manager: ready=%#v sent=%#v", cmd.ready, cmd.sent)
			}
		})
	}
}

func TestDelegateTaskResumesNewestExitedManager(t *testing.T) {
	st := newFakeStore()
	st.projects["open-agents"] = domain.ProjectRecord{ID: "open-agents"}
	now := time.Now().UTC()
	st.sessions["orch-old"] = domain.SessionRecord{ID: "orch-old", ProjectID: "open-agents", Kind: domain.KindManager, Activity: domain.Activity{State: domain.ActivityExited}, CreatedAt: now.Add(-time.Minute)}
	st.sessions["orch-new"] = domain.SessionRecord{ID: "orch-new", ProjectID: "open-agents", Kind: domain.KindManager, Activity: domain.Activity{State: domain.ActivityExited}, CreatedAt: now}
	cmd := &fakeCommander{}

	out, err := (&Service{store: st, manager: cmd, runBackground: runInline}).DelegateTask(context.Background(), DelegateTaskInput{ProjectID: "open-agents", Brief: "Fix it"})
	if err != nil {
		t.Fatalf("DelegateTask: %v", err)
	}
	if out.ManagerID != "orch-new" {
		t.Fatalf("out = %#v, want orch-new", out)
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

func TestDelegateTaskStartsMissingManager(t *testing.T) {
	st := newFakeStore()
	st.projects["open-agents"] = domain.ProjectRecord{ID: "open-agents"}
	st.sessions["orch-dead"] = domain.SessionRecord{ID: "orch-dead", ProjectID: "open-agents", Kind: domain.KindManager, IsTerminated: true}
	cmd := &fakeCommander{spawnFunc: func(cfg ports.SpawnConfig) domain.SessionRecord {
		return domain.SessionRecord{ID: "orch-new", ProjectID: cfg.ProjectID, Kind: cfg.Kind}
	}}

	out, err := (&Service{store: st, manager: cmd, runBackground: runInline}).DelegateTask(context.Background(), DelegateTaskInput{ProjectID: "open-agents", Brief: "Fix it"})
	if err != nil {
		t.Fatalf("DelegateTask: %v", err)
	}
	if out.ManagerID != "orch-new" {
		t.Fatalf("out = %#v, want orch-new", out)
	}
	// Only the manager is spawned; the manager owns spawning the worker.
	if cmd.spawnCalls != 1 || cmd.spawnedCfg.Kind != domain.KindManager {
		t.Fatalf("spawn calls = %d, cfg = %#v; want a single manager spawn", cmd.spawnCalls, cmd.spawnedCfg)
	}
	if len(cmd.ready) != 1 || cmd.ready[0] != "orch-new" {
		t.Fatalf("readiness waits = %#v; want orch-new", cmd.ready)
	}
	if len(cmd.sent) != 1 || cmd.sent[0] != "orch-new" {
		t.Fatalf("sent = %#v; want orch-new", cmd.sent)
	}
}

func TestDelegateTaskReturnsErrorWhenManagerNeverBecomesReady(t *testing.T) {
	st := newFakeStore()
	st.projects["open-agents"] = domain.ProjectRecord{ID: "open-agents"}
	st.sessions["orch"] = domain.SessionRecord{ID: "orch", ProjectID: "open-agents", Kind: domain.KindManager}
	cmd := &fakeCommander{readyErr: errors.New("readiness timed out")}

	_, err := (&Service{store: st, manager: cmd, runBackground: runInline}).DelegateTask(context.Background(), DelegateTaskInput{ProjectID: "open-agents", Brief: "Fix it"})
	if err == nil || !strings.Contains(err.Error(), "readiness timed out") {
		t.Fatalf("err = %v, want readiness failure", err)
	}
	if len(cmd.sent) != 0 {
		t.Fatalf("sent = %#v; want no delivery before readiness", cmd.sent)
	}
}

func TestDelegateTaskReturnsErrorWhenDeliveryFails(t *testing.T) {
	st := newFakeStore()
	st.projects["open-agents"] = domain.ProjectRecord{ID: "open-agents"}
	st.sessions["orch"] = domain.SessionRecord{ID: "orch", ProjectID: "open-agents", Kind: domain.KindManager}
	cmd := &fakeCommander{sendErr: errors.New("manager exited")}

	_, err := (&Service{store: st, manager: cmd, runBackground: runInline}).DelegateTask(context.Background(), DelegateTaskInput{ProjectID: "open-agents", Brief: "Fix it"})
	if err == nil || !strings.Contains(err.Error(), "send task to orch") {
		t.Fatalf("err = %v, want send failure", err)
	}
}

func runInline(work func()) {
	work()
}
