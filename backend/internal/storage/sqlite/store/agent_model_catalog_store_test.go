package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/sudo-adduser-jordan/open-agents/backend/internal/ports"
)

func TestListAgentModelCatalogsByAgentReturnsOnlyRequestedScopes(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	for _, record := range []ports.CachedAgentModelCatalog{
		{AgentID: "codex", ProjectID: "project-b", CatalogJSON: `{}`, FetchedAt: now},
		{AgentID: "codex", ProjectID: "project-a", CatalogJSON: `{}`, FetchedAt: now},
		{AgentID: "muse", ProjectID: "project-c", CatalogJSON: `{}`, FetchedAt: now},
	} {
		if err := store.UpsertAgentModelCatalog(ctx, record); err != nil {
			t.Fatal(err)
		}
	}

	records, err := store.ListAgentModelCatalogsByAgent(ctx, "codex")
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || records[0].ProjectID != "project-a" || records[1].ProjectID != "project-b" {
		t.Fatalf("records = %#v", records)
	}
}
