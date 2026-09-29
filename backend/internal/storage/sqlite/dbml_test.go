package sqlite

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The DBML document and schema SVG are generated from the embedded
// migrations. These tests fail when the committed artifacts drift from the
// schema so a migration without `task db:dbml` is caught in CI.
func TestSchemaDBMLMatchesCommitted(t *testing.T) {
	db := openMigratedTestDB(t)

	got, err := DumpDBML(context.Background(), db)
	if err != nil {
		t.Fatalf("dump DBML: %v", err)
	}
	want, err := os.ReadFile("schema.dbml")
	if err != nil {
		t.Fatalf("read committed schema.dbml: %v", err)
	}
	if got != string(want) {
		t.Fatal("schema.dbml is stale; regenerate with `task db:dbml` " +
			"(backend: go generate ./internal/storage/sqlite/) and commit both outputs")
	}
}

func TestSchemaSVGMatchesCommitted(t *testing.T) {
	db := openMigratedTestDB(t)

	got, err := DumpSVG(context.Background(), db)
	if err != nil {
		t.Fatalf("dump SVG: %v", err)
	}
	want, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "assets", "diagrams", "06-database-schema.svg"))
	if err != nil {
		t.Fatalf("read committed schema SVG: %v", err)
	}
	if got != string(want) {
		t.Fatal("06-database-schema.svg is stale; regenerate with `task db:dbml` " +
			"(backend: go generate ./internal/storage/sqlite/) and commit both outputs")
	}
}

// TestSchemaDBMLCoversCoreTables pins the tables the architecture docs reason
// about so a generator regression that silently drops a table family fails
// loudly even before the byte comparison above runs.
func TestSchemaDBMLCoversCoreTables(t *testing.T) {
	db := openMigratedTestDB(t)

	got, err := DumpDBML(context.Background(), db)
	if err != nil {
		t.Fatalf("dump DBML: %v", err)
	}
	for _, table := range []string{
		"projects", "sessions", "conversations", "conversation_turns",
		"conversation_messages", "pr", "pr_checks", "review", "review_run",
		"session_interface_transitions", "change_log",
		"usage_bindings", "usage_sources", "model_usage_events",
	} {
		if !strings.Contains(got, "Table "+table+" {") {
			t.Errorf("DBML is missing Table %s", table)
		}
	}
	if !strings.Contains(got, "Ref: sessions.project_id > projects.id") {
		t.Error("DBML is missing the sessions -> projects ref")
	}
}
