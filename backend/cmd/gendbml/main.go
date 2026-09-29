// Command gendbml regenerates the ChartDB-compatible schema.dbml and the
// README schema SVG from the embedded SQLite migrations. It is invoked via
// `go generate` (see internal/storage/sqlite/dbml.go) or `task db:dbml`;
// both outputs are committed.
package main

import (
	"context"
	"database/sql"
	"flag"
	"log"
	"os"
	"path/filepath"

	sqlitepkg "github.com/sudo-adduser-jordan/open-agents/backend/internal/storage/sqlite"

	_ "modernc.org/sqlite"
)

func main() {
	dbmlOut := flag.String("dbml", "schema.dbml", "output path for the generated DBML document")
	svgOut := flag.String("svg", "schema.svg", "output path for the generated schema SVG")
	flag.Parse()

	ctx := context.Background()
	dir, err := os.MkdirTemp("", "gendbml")
	if err != nil {
		log.Fatalf("gendbml: temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	raw, err := sql.Open("sqlite", "file:"+filepath.Join(dir, "open-agents.db"))
	if err != nil {
		log.Fatalf("gendbml: open sqlite: %v", err)
	}
	raw.SetMaxOpenConns(1)
	defer func() { _ = raw.Close() }()

	if err := sqlitepkg.Migrate(raw); err != nil {
		log.Fatalf("gendbml: migrate: %v", err)
	}

	dbml, err := sqlitepkg.DumpDBML(ctx, raw)
	if err != nil {
		log.Fatalf("gendbml: dump DBML: %v", err)
	}
	svg, err := sqlitepkg.DumpSVG(ctx, raw)
	if err != nil {
		log.Fatalf("gendbml: dump SVG: %v", err)
	}
	if err := os.WriteFile(*dbmlOut, []byte(dbml), 0o644); err != nil {
		log.Fatalf("gendbml: write %s: %v", *dbmlOut, err)
	}
	if err := os.WriteFile(*svgOut, []byte(svg), 0o644); err != nil {
		log.Fatalf("gendbml: write %s: %v", *svgOut, err)
	}
}
