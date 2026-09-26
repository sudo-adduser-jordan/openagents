package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestStandaloneProjectColumnsAreNullable(t *testing.T) {
	dataDir := t.TempDir()
	store, err := Open(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dataDir, "open-agents.db")+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	for _, table := range []string{"sessions", "change_log", "notifications", "conversations"} {
		func() {
			rows, err := db.QueryContext(context.Background(), "PRAGMA table_info("+table+")")
			if err != nil {
				t.Fatalf("%s table info: %v", table, err)
			}
			defer rows.Close()
			found := false
			for rows.Next() {
				var cid, notNull, primaryKey int
				var name, columnType string
				var defaultValue any
				if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
					t.Fatal(err)
				}
				if name == "project_id" {
					found = true
					if notNull != 0 {
						t.Fatalf("%s.project_id remains NOT NULL", table)
					}
				}
			}
			if err := rows.Err(); err != nil {
				t.Fatal(err)
			}
			if !found {
				t.Fatalf("%s.project_id not found", table)
			}
		}()
	}
}
