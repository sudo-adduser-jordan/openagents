package sqlite

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMigration0154RenamesRoleAndPreservesPhysicalSchema(t *testing.T) {
	db := openTestDB(t)
	upTo(t, db, 153)

	// Simulate a real legacy profile whose physical harness CHECK contains QM.
	// Migration 0154 must edit only the role/workflow constraint text and leave
	// this variant, its row, and every session trigger intact.
	if _, err := db.Exec(`PRAGMA writable_schema = ON`); err != nil {
		t.Fatalf("enable writable_schema: %v", err)
	}
	if _, err := db.Exec(`
UPDATE sqlite_master
SET sql = replace(
    sql,
    char(39) || 'fake' || char(39),
    char(39) || 'qm' || char(39) || ', ' || char(39) || 'fake' || char(39)
)
WHERE type = 'table' AND name = 'sessions'
`); err != nil {
		t.Fatalf("seed legacy QM harness variant: %v", err)
	}
	if _, err := db.Exec(`PRAGMA writable_schema = RESET`); err != nil {
		t.Fatalf("reparse legacy QM harness variant: %v", err)
	}

	const timestamp = "2026-09-24T12:00:00Z"
	_, err := db.Exec(`
INSERT INTO projects (id, path, display_name, registered_at, config)
VALUES (
    'demo', '/tmp/demo', 'Demo', ?,
    '{"agentRules":"keep","orchestratorRules":"Coordinate through workers.","orchestrator":{"agent":"codex","agentConfig":{"model":"legacy-manager"}},"custom":{"keep":true}}'
);
INSERT INTO sessions (
    id, project_id, num, kind, harness, activity_state, activity_last_at,
    branch, workspace_path, workflow_mode, created_at, updated_at
) VALUES
    ('legacy-opaque-id', 'demo', 1, 'orchestrator', 'opencode', 'idle', ?, 'open-agents/dev/demo-orchestrator', '/old/manager/worktree', 'planning', ?, ?),
    ('demo-2', 'demo', 2, 'worker', 'opencode', 'idle', ?, 'open-agents/dev/demo-2/root', '/old/worker/worktree', 'planning', ?, ?),
    ('legacy-qm-worker', 'demo', 3, 'worker', 'qm', 'idle', ?, 'open-agents/dev/legacy-qm/root', '/old/qm/worktree', 'planning', ?, ?),
    ('legacy-building-manager', 'demo', 4, 'orchestrator', 'opencode', 'idle', ?, 'open-agents/dev/legacy-building-orchestrator', '/old/building/worktree', 'building', ?, ?);
`, timestamp, timestamp, timestamp, timestamp, timestamp, timestamp, timestamp, timestamp, timestamp, timestamp, timestamp, timestamp)
	if err != nil {
		t.Fatalf("seed pre-rename database: %v", err)
	}

	if err := migrate(db); err != nil {
		t.Fatalf("migrate to manager vocabulary: %v", err)
	}

	var kind, workflowMode, branch, workspacePath string
	if err := db.QueryRow(`
SELECT kind, workflow_mode, branch, workspace_path
FROM sessions
WHERE id = 'legacy-opaque-id'
`).Scan(&kind, &workflowMode, &branch, &workspacePath); err != nil {
		t.Fatalf("read migrated manager: %v", err)
	}
	if kind != "manager" || workflowMode != "planning" {
		t.Fatalf("migrated planning role = %q/%q, want manager/planning", kind, workflowMode)
	}
	if branch != "open-agents/dev/demo-orchestrator" || workspacePath != "/old/manager/worktree" {
		t.Fatalf("opaque branch/path changed: branch=%q path=%q", branch, workspacePath)
	}

	if err := db.QueryRow(`SELECT kind, workflow_mode FROM sessions WHERE id = 'demo-2'`).Scan(&kind, &workflowMode); err != nil {
		t.Fatalf("read migrated worker: %v", err)
	}
	if kind != "worker" || workflowMode != "planning" {
		t.Fatalf("migrated worker = %q/%q, want worker/planning", kind, workflowMode)
	}
	if err := db.QueryRow(`SELECT kind, workflow_mode FROM sessions WHERE id = 'legacy-building-manager'`).Scan(&kind, &workflowMode); err != nil {
		t.Fatalf("read migrated building manager: %v", err)
	}
	if kind != "manager" || workflowMode != "manager" {
		t.Fatalf("migrated building manager = %q/%q, want manager/manager", kind, workflowMode)
	}

	var rawConfig string
	if err := db.QueryRow(`SELECT config FROM projects WHERE id = 'demo'`).Scan(&rawConfig); err != nil {
		t.Fatalf("read migrated project config: %v", err)
	}
	var config map[string]any
	if err := json.Unmarshal([]byte(rawConfig), &config); err != nil {
		t.Fatalf("decode migrated project config: %v", err)
	}
	if _, legacy := config["orchestrator"]; legacy {
		t.Fatalf("legacy role key survived: %s", rawConfig)
	}
	if _, legacy := config["orchestratorRules"]; legacy {
		t.Fatalf("legacy rules key survived: %s", rawConfig)
	}
	if config["managerRules"] != "Coordinate through workers." {
		t.Fatalf("managerRules = %#v", config["managerRules"])
	}
	manager, ok := config["manager"].(map[string]any)
	if !ok || manager["agent"] != "codex" {
		t.Fatalf("manager override = %#v", config["manager"])
	}
	if config["agentRules"] != "keep" || config["custom"] == nil {
		t.Fatalf("unrelated config changed: %s", rawConfig)
	}

	var schema string
	if err := db.QueryRow(`SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'sessions'`).Scan(&schema); err != nil {
		t.Fatalf("read sessions schema: %v", err)
	}
	if !strings.Contains(schema, "CHECK (kind IN ('worker', 'manager'))") {
		t.Fatalf("canonical kind CHECK missing:\n%s", schema)
	}
	if !strings.Contains(schema, "kind = 'manager' AND workflow_mode IN ('planning', 'manager')") ||
		!strings.Contains(schema, "kind = 'worker' AND workflow_mode IN ('planning', 'building')") {
		t.Fatalf("role-aware workflow CHECK missing:\n%s", schema)
	}
	if !strings.Contains(schema, "'qm'") {
		t.Fatalf("legacy QM harness CHECK was replaced:\n%s", schema)
	}
	var qmWorkers int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE id = 'legacy-qm-worker' AND harness = 'qm'`).Scan(&qmWorkers); err != nil {
		t.Fatalf("read legacy QM session: %v", err)
	}
	if qmWorkers != 1 {
		t.Fatalf("legacy QM session count = %d, want 1", qmWorkers)
	}

	// The pre-existing database-owned revision and CDC triggers stay attached.
	var revisionBefore int
	if err := db.QueryRow(`SELECT revision FROM sessions WHERE id = 'legacy-opaque-id'`).Scan(&revisionBefore); err != nil {
		t.Fatalf("read revision before update: %v", err)
	}
	if _, err := db.Exec(`UPDATE sessions SET workflow_mode = 'planning' WHERE id = 'legacy-opaque-id'`); err != nil {
		t.Fatalf("update migrated manager: %v", err)
	}
	var revisionAfter int
	if err := db.QueryRow(`SELECT revision FROM sessions WHERE id = 'legacy-opaque-id'`).Scan(&revisionAfter); err != nil {
		t.Fatalf("read revision after update: %v", err)
	}
	if revisionAfter != revisionBefore+1 {
		t.Fatalf("revision = %d, want trigger-maintained %d", revisionAfter, revisionBefore+1)
	}

	if _, err := db.Exec(`
INSERT INTO sessions (id, project_id, num, kind, harness, activity_last_at, created_at, updated_at)
VALUES ('retired-kind', 'demo', 5, 'orchestrator', 'opencode', ?, ?, ?)
`, timestamp, timestamp, timestamp); err == nil {
		t.Fatal("retired role value was accepted after migration")
	}
	if _, err := db.Exec(`
INSERT INTO sessions (id, project_id, num, kind, workflow_mode, harness, activity_last_at, created_at, updated_at)
VALUES ('worker-manager-mode', 'demo', 5, 'worker', 'manager', 'opencode', ?, ?, ?)
`, timestamp, timestamp, timestamp); err == nil {
		t.Fatal("manager workflow mode was accepted for a worker")
	}
	if _, err := db.Exec(`
INSERT INTO sessions (id, project_id, num, kind, workflow_mode, harness, activity_last_at, created_at, updated_at)
VALUES ('new-planning-manager', 'demo', 5, 'manager', 'planning', 'opencode', ?, ?, ?)
`, timestamp, timestamp, timestamp); err != nil {
		t.Fatalf("planning mode was rejected for a manager: %v", err)
	}
	if _, err := db.Exec(`
INSERT INTO sessions (id, project_id, num, kind, workflow_mode, harness, activity_last_at, created_at, updated_at)
VALUES ('building-manager-mode', 'demo', 6, 'manager', 'building', 'opencode', ?, ?, ?)
`, timestamp, timestamp, timestamp); err == nil {
		t.Fatal("building workflow mode was accepted for a manager")
	}

	var violations int
	rows, err := db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatalf("foreign-key check: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		violations++
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("foreign-key rows: %v", err)
	}
	if violations != 0 {
		t.Fatalf("foreign-key check found %d violations", violations)
	}

	var triggerCount int
	if err := db.QueryRow(`
SELECT COUNT(*) FROM sqlite_master
WHERE type = 'trigger' AND name IN (
    'sessions_cdc_insert', 'sessions_cdc_update',
    'sessions_revision_update', 'conversation_branch_root_provider_update'
)
`).Scan(&triggerCount); err != nil {
		t.Fatalf("count preserved session triggers: %v", err)
	}
	if triggerCount != 4 {
		t.Fatalf("preserved session trigger count = %d, want 4", triggerCount)
	}
}
