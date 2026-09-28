package sessionmanager

import (
	"strings"
	"testing"

	"github.com/sudo-adduser-jordan/open-agents/backend/internal/domain"
	"github.com/sudo-adduser-jordan/open-agents/backend/internal/ports"
)

// `spawn --no-start` must create everything a real spawn creates and launch
// nothing at all. If it launched anything, the whole feature would be a lie: the
// operator asked for a task on the board, not a running agent.
func TestSpawnNoStartCreatesSessionWithoutLaunching(t *testing.T) {
	t.Parallel()
	m, st, rt, ws := newManager()
	repo := newManagerGitRepo(t)
	cfg := testRoleAgents()
	cfg.DefaultBranch = "main"
	st.projects["mer"] = domain.ProjectRecord{ID: "mer", Path: repo, Config: cfg}
	ws.path = repo

	rec, promptBytes, _, err := m.Spawn(ctx, ports.SpawnConfig{
		ProjectID: "mer",
		Kind:      domain.KindWorker,
		Prompt:    "stage this task",
		NoStart:   true,
	})
	if err != nil {
		t.Fatalf("spawn --no-start: %v", err)
	}

	if !rec.AgentDeferred {
		t.Errorf("AgentDeferred=false, want true")
	}
	if rt.created != 0 {
		t.Errorf("runtime Create called %d times, want 0: no agent may be launched", rt.created)
	}
	if rec.Metadata.RuntimeHandleID != "" || rec.Metadata.RuntimeLaunchID != "" {
		t.Errorf("staged session has a runtime identity (handle=%q launch=%q), want none",
			rec.Metadata.RuntimeHandleID, rec.Metadata.RuntimeLaunchID)
	}
	if rec.IsTerminated {
		t.Errorf("staged session is terminated, want live")
	}

	// The task has to be real, not a placeholder: workspace and prompt are
	// exactly what a later start needs.
	if rec.Metadata.WorkspacePath == "" {
		t.Errorf("WorkspacePath is empty, want the created worktree")
	}
	if rec.Metadata.Branch == "" {
		t.Errorf("Branch is empty, want the created branch")
	}
	if rec.Metadata.Prompt != "stage this task" {
		t.Errorf("Prompt = %q, want %q", rec.Metadata.Prompt, "stage this task")
	}
	if promptBytes == 0 {
		t.Errorf("promptBytes = 0, want the resolved prompt size")
	}

	// And it has to be durable, not just returned.
	stored, ok := st.sessions[rec.ID]
	if !ok {
		t.Fatalf("staged session %s was not persisted", rec.ID)
	}
	if !stored.AgentDeferred {
		t.Errorf("stored AgentDeferred=false, want true")
	}
}

// Staging must not silently reset the settings the user chose. The permission
// mode is resolved onto the seed row before the staging write and read back by
// the later launch, so a task started hours later still runs the way it was
// specified.
func TestStagedSessionKeepsItsResolvedPermissionMode(t *testing.T) {
	t.Parallel()
	m, st, _, ws := newManager()
	repo := newManagerGitRepo(t)
	cfg := testRoleAgents()
	cfg.DefaultBranch = "main"
	st.projects["mer"] = domain.ProjectRecord{ID: "mer", Path: repo, Config: cfg}
	ws.path = repo

	rec, _, _, err := m.Spawn(ctx, ports.SpawnConfig{
		ProjectID:   "mer",
		Kind:        domain.KindWorker,
		Prompt:      "stage this",
		NoStart:     true,
		AgentConfig: ports.AgentConfig{Permissions: domain.PermissionModeAcceptEdits},
	})
	if err != nil {
		t.Fatalf("spawn --no-start: %v", err)
	}
	if rec.Metadata.Permissions != domain.PermissionModeAcceptEdits {
		t.Fatalf("staged Permissions = %q, want %q", rec.Metadata.Permissions, domain.PermissionModeAcceptEdits)
	}

	if _, err := m.ResumeAgentWithMode(ctx, rec.ID); err != nil {
		t.Fatalf("resume staged session: %v", err)
	}
	if got := st.sessions[rec.ID].Metadata.Permissions; got != domain.PermissionModeAcceptEdits {
		t.Errorf("Permissions after start = %q, want %q", got, domain.PermissionModeAcceptEdits)
	}
}

// A staged session has never exited an agent, so the preconditions that stop
// resume-agent from creating a duplicate controller must not apply to it.
func TestResumeAgentStartsDeferredSession(t *testing.T) {
	t.Parallel()
	m, st, rt, ws := newManager()
	repo := newManagerGitRepo(t)
	cfg := testRoleAgents()
	cfg.DefaultBranch = "main"
	st.projects["mer"] = domain.ProjectRecord{ID: "mer", Path: repo, Config: cfg}
	ws.path = repo

	staged, _, _, err := m.Spawn(ctx, ports.SpawnConfig{
		ProjectID: "mer",
		Kind:      domain.KindWorker,
		Prompt:    "the original brief",
		NoStart:   true,
	})
	if err != nil {
		t.Fatalf("spawn --no-start: %v", err)
	}
	createdAfterSpawn := rt.created

	result, err := m.ResumeAgentWithMode(ctx, staged.ID)
	if err != nil {
		t.Fatalf("resume staged session: %v", err)
	}

	if rt.created != createdAfterSpawn+1 {
		t.Errorf("runtime Create called %d times after resume, want exactly one more than the %d at spawn",
			rt.created, createdAfterSpawn)
	}
	if result.Session.AgentDeferred {
		t.Errorf("AgentDeferred=true after starting, want false")
	}
	if result.Session.Metadata.RuntimeHandleID == "" {
		t.Errorf("RuntimeHandleID is empty after starting, want a live terminal")
	}
	// The stored prompt must survive the start: it is the task.
	if result.Session.Metadata.Prompt != "the original brief" {
		t.Errorf("Prompt = %q after starting, want %q", result.Session.Metadata.Prompt, "the original brief")
	}
	if stored := st.sessions[staged.ID]; stored.AgentDeferred {
		t.Errorf("stored AgentDeferred=true after starting, want false")
	}
}

// The staged -> started transition has to reach the change log, or a connected
// board would keep rendering "Not started" until some unrelated update arrived.
func TestStartingDeferredSessionEmitsChangeEvent(t *testing.T) {
	t.Parallel()
	m, st, _, ws := newManager()
	repo := newManagerGitRepo(t)
	cfg := testRoleAgents()
	cfg.DefaultBranch = "main"
	st.projects["mer"] = domain.ProjectRecord{ID: "mer", Path: repo, Config: cfg}
	ws.path = repo

	staged, _, _, err := m.Spawn(ctx, ports.SpawnConfig{
		ProjectID: "mer",
		Kind:      domain.KindWorker,
		NoStart:   true,
	})
	if err != nil {
		t.Fatalf("spawn --no-start: %v", err)
	}
	if _, err := m.ResumeAgentWithMode(ctx, staged.ID); err != nil {
		t.Fatalf("resume staged session: %v", err)
	}

	// The fake store does not run SQL triggers, so assert the durable write
	// happened rather than the change-log row the trigger would have produced.
	if st.sessions[staged.ID].AgentDeferred {
		t.Error("stored AgentDeferred is still true, want the start transition committed")
	}
}

// Without the flag the session is not deferred and behaves exactly as before.
func TestSpawnWithoutNoStartIsNotDeferred(t *testing.T) {
	t.Parallel()
	m, st, rt, ws := newManager()
	repo := newManagerGitRepo(t)
	cfg := testRoleAgents()
	cfg.DefaultBranch = "main"
	st.projects["mer"] = domain.ProjectRecord{ID: "mer", Path: repo, Config: cfg}
	ws.path = repo

	rec, _, _, err := m.Spawn(ctx, ports.SpawnConfig{ProjectID: "mer", Kind: domain.KindWorker})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if rec.AgentDeferred {
		t.Errorf("AgentDeferred=true for an ordinary spawn, want false")
	}
	if rt.created == 0 {
		t.Error("runtime Create was never called for an ordinary spawn, want a launched agent")
	}
	if strings.TrimSpace(rec.Metadata.RuntimeHandleID) == "" {
		t.Error("RuntimeHandleID is empty for an ordinary spawn, want a live terminal")
	}
}

// A staged Chat session has no provider conversation to reattach to, so the
// stored prompt is the only thing that will ever ask it to do the work. It has
// to be delivered on start, or the session comes up running and silent.
func TestResumeAgentStartsDeferredChatSessionAndDeliversStoredPrompt(t *testing.T) {
	t.Parallel()
	launcher := &recordingLauncher{}
	m, st, _, ws := newChatManagerWithWorkspace(t, launcher)
	repo := newManagerGitRepo(t)
	cfg := testRoleAgents()
	cfg.DefaultBranch = "main"
	st.projects["mer"] = domain.ProjectRecord{ID: "mer", Path: repo, Config: cfg}
	ws.path = repo

	staged, _, _, err := m.Spawn(ctx, ports.SpawnConfig{
		ProjectID:     "mer",
		Kind:          domain.KindWorker,
		RequestedMode: domain.SessionModeChat,
		Prompt:        "the original brief",
		NoStart:       true,
	})
	if err != nil {
		t.Fatalf("spawn --no-start chat: %v", err)
	}
	if !staged.AgentDeferred {
		t.Fatalf("AgentDeferred=false, want true")
	}
	// A staged chat session launches no controller and delivers no turn.
	if len(launcher.started) != 0 {
		t.Errorf("chat StartChat called %d times at spawn, want 0", len(launcher.started))
	}
	if len(launcher.turns) != 0 {
		t.Errorf("chat turns = %v at spawn, want none", launcher.turns)
	}

	if _, err := m.ResumeAgentWithMode(ctx, staged.ID); err != nil {
		t.Fatalf("resume staged chat session: %v", err)
	}

	if len(launcher.started) != 1 {
		t.Errorf("chat StartChat called %d times, want exactly 1", len(launcher.started))
	}
	if len(launcher.turns) != 1 || launcher.turns[0] != "the original brief" {
		t.Errorf("chat turns = %v, want exactly the stored prompt", launcher.turns)
	}
	if st.sessions[staged.ID].AgentDeferred {
		t.Errorf("stored AgentDeferred=true after starting, want false")
	}
}
