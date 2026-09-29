package sqlite

// schema.dbml and the README schema SVG are generated from the embedded goose
// migrations — do not edit them by hand.
//
//go:generate go run ../../../cmd/gendbml -dbml schema.dbml -svg ../../../../assets/diagrams/06-database-schema.svg

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
)

// Migrate runs the embedded goose migrations on db. Production startup goes
// through Open; this export exists for schema tools (cmd/gendbml) that need a
// migrated database without a data directory.
func Migrate(db *sql.DB) error { return migrate(db) }

// schemaColumn is one introspected SQLite column.
type schemaColumn struct {
	name         string
	colType      string
	notNull      bool
	defaultValue sql.NullString
	pk           bool
	unique       bool
}

// schemaFK is one FOREIGN KEY clause: fromTable.fromCol references
// toTable.toCol.
type schemaFK struct {
	fromTable string
	fromCol   string
	toTable   string
	toCol     string
}

// schemaTable is one introspected table with its columns and constraints.
type schemaTable struct {
	name             string
	columns          []schemaColumn
	compositePK      []string
	compositeUniques [][]string
}

// dbSchema is the migrated database's table surface: tables plus foreign
// keys. Views (usage_session_integrity) and the goose_db_version ledger are
// excluded; triggers are CDC plumbing documented in docs/architecture.md.
type dbSchema struct {
	tables []schemaTable
	fks    []schemaFK
}

// introspectSchema reads tables, columns, and foreign keys from a migrated
// database via PRAGMA. Table order is alphabetical so generated output diffs
// cleanly.
func introspectSchema(ctx context.Context, db *sql.DB) (*dbSchema, error) {
	rows, err := db.QueryContext(ctx, `SELECT name FROM sqlite_master
		WHERE type = 'table' AND name NOT LIKE 'sqlite_%' AND name <> 'goose_db_version'
		ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list tables: %w", err)
	}
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan table name: %w", err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("iterate tables: %w", err)
	}
	_ = rows.Close()

	schema := &dbSchema{}
	for _, name := range names {
		table, err := introspectTable(ctx, db, name)
		if err != nil {
			return nil, err
		}
		schema.tables = append(schema.tables, *table)
		for _, fk := range tableFKs(ctx, db, name) {
			schema.fks = append(schema.fks, fk)
		}
	}
	sort.Slice(schema.fks, func(i, j int) bool {
		a, b := schema.fks[i], schema.fks[j]
		if a.fromTable != b.fromTable {
			return a.fromTable < b.fromTable
		}
		if a.fromCol != b.fromCol {
			return a.fromCol < b.fromCol
		}
		if a.toTable != b.toTable {
			return a.toTable < b.toTable
		}
		return a.toCol < b.toCol
	})
	return schema, nil
}

// quoteIdent quotes an SQLite identifier from sqlite_master for PRAGMA use.
// Names come from the database itself, but quote defensively anyway.
func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func introspectTable(ctx context.Context, db *sql.DB, name string) (*schemaTable, error) {
	table := &schemaTable{name: name}

	rows, err := db.QueryContext(ctx, `PRAGMA table_info(`+quoteIdent(name)+`)`)
	if err != nil {
		return nil, fmt.Errorf("read %s columns: %w", name, err)
	}
	type pkCol struct {
		order int
		index int
	}
	var pkCols []pkCol
	for rows.Next() {
		var cid, notNull, pk int
		var colName, colType string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &colName, &colType, &notNull, &dflt, &pk); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan %s columns: %w", name, err)
		}
		if colType == "" {
			colType = "TEXT"
		}
		table.columns = append(table.columns, schemaColumn{
			name:         colName,
			colType:      colType,
			notNull:      notNull == 1,
			defaultValue: dflt,
		})
		if pk > 0 {
			pkCols = append(pkCols, pkCol{order: pk, index: len(table.columns) - 1})
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("iterate %s columns: %w", name, err)
	}
	_ = rows.Close()

	sort.Slice(pkCols, func(i, j int) bool { return pkCols[i].order < pkCols[j].order })
	if len(pkCols) == 1 {
		table.columns[pkCols[0].index].pk = true
	} else {
		for _, pk := range pkCols {
			table.compositePK = append(table.compositePK, table.columns[pk.index].name)
		}
	}

	if err := markUniques(ctx, db, table); err != nil {
		return nil, err
	}
	return table, nil
}

// tableFKs lists one entry per FOREIGN KEY clause on the table.
func tableFKs(ctx context.Context, db *sql.DB, name string) []schemaFK {
	rows, err := db.QueryContext(ctx, `PRAGMA foreign_key_list(`+quoteIdent(name)+`)`)
	if err != nil {
		return nil
	}
	defer func() { _ = rows.Close() }()
	seen := map[schemaFK]struct{}{}
	var fks []schemaFK
	for rows.Next() {
		var id, seq int
		var toTable, fromCol, toCol, onUpdate, onDelete, match string
		if err := rows.Scan(&id, &seq, &toTable, &fromCol, &toCol, &onUpdate, &onDelete, &match); err != nil {
			return fks
		}
		fk := schemaFK{fromTable: name, fromCol: fromCol, toTable: toTable, toCol: toCol}
		if _, dup := seen[fk]; !dup {
			seen[fk] = struct{}{}
			fks = append(fks, fk)
		}
	}
	return fks
}

// markUniques flags columns covered by non-partial UNIQUE indexes (excluding
// primary-key auto-indexes). Partial indexes (WHERE ...) cannot be expressed
// in DBML and are skipped rather than overstated.
func markUniques(ctx context.Context, db *sql.DB, table *schemaTable) error {
	rows, err := db.QueryContext(ctx, `PRAGMA index_list(`+quoteIdent(table.name)+`)`)
	if err != nil {
		return fmt.Errorf("read %s indexes: %w", table.name, err)
	}
	type indexDef struct {
		seq     int
		name    string
		unique  bool
		origin  string
		partial bool
	}
	var indexes []indexDef
	for rows.Next() {
		var idx indexDef
		var unique, partial int
		if err := rows.Scan(&idx.seq, &idx.name, &unique, &idx.origin, &partial); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan %s indexes: %w", table.name, err)
		}
		idx.unique = unique == 1
		idx.partial = partial == 1
		indexes = append(indexes, idx)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("iterate %s indexes: %w", table.name, err)
	}
	_ = rows.Close()

	nameToIndex := map[string]int{}
	for i, col := range table.columns {
		nameToIndex[col.name] = i
	}
	for _, idx := range indexes {
		if !idx.unique || idx.origin == "pk" || idx.partial {
			continue
		}
		infoRows, err := db.QueryContext(ctx, `PRAGMA index_info(`+quoteIdent(idx.name)+`)`)
		if err != nil {
			return fmt.Errorf("read index %s info: %w", idx.name, err)
		}
		type keyCol struct {
			seqno int
			name  string
		}
		var keyCols []keyCol
		for infoRows.Next() {
			var seqno, cid int
			var colName string
			if err := infoRows.Scan(&seqno, &cid, &colName); err != nil {
				_ = infoRows.Close()
				return fmt.Errorf("scan index %s info: %w", idx.name, err)
			}
			keyCols = append(keyCols, keyCol{seqno: seqno, name: colName})
		}
		_ = infoRows.Close()
		if err := infoRows.Err(); err != nil {
			return fmt.Errorf("iterate index %s info: %w", idx.name, err)
		}
		sort.Slice(keyCols, func(i, j int) bool { return keyCols[i].seqno < keyCols[j].seqno })
		var cols []string
		for _, kc := range keyCols {
			cols = append(cols, kc.name)
		}
		if len(cols) == 1 {
			if ci, ok := nameToIndex[cols[0]]; ok && !table.columns[ci].pk {
				table.columns[ci].unique = true
			}
			continue
		}
		if len(cols) > 1 {
			table.compositeUniques = append(table.compositeUniques, cols)
		}
	}
	return nil
}

// formatDefault renders a PRAGMA default value as a DBML default setting.
// Literals pass through; expressions are backtick-quoted per DBML syntax.
func formatDefault(dflt sql.NullString) string {
	if !dflt.Valid {
		return ""
	}
	v := strings.TrimSpace(dflt.String)
	if v == "" {
		return ""
	}
	if len(v) >= 2 && v[0] == '\'' && v[len(v)-1] == '\'' {
		return v
	}
	lower := strings.ToLower(v)
	switch lower {
	case "true", "false", "null":
		return lower
	}
	isNumber := true
	for i, r := range v {
		if r >= '0' && r <= '9' || r == '.' || (i == 0 && (r == '-' || r == '+')) {
			continue
		}
		isNumber = false
		break
	}
	if isNumber {
		return v
	}
	return "`" + v + "`"
}

// DumpDBML introspects a migrated database and renders the ChartDB-compatible
// DBML document committed as schema.dbml.
func DumpDBML(ctx context.Context, db *sql.DB) (string, error) {
	schema, err := introspectSchema(ctx, db)
	if err != nil {
		return "", err
	}
	return schema.toDBML(), nil
}

// toDBML renders tables and refs in dbdiagram/ChartDB DBML syntax.
func (s *dbSchema) toDBML() string {
	var b strings.Builder
	b.WriteString("-- Code-generated from backend/internal/storage/sqlite/migrations — do not edit.\n")
	b.WriteString("-- Regenerate: task db:dbml (backend: go generate ./internal/storage/sqlite/).\n")
	b.WriteString("-- Open in ChartDB: import this file at https://chartdb.io, then File > Export as > SVG.\n")
	b.WriteString("-- Excluded: goose_db_version (migration ledger), usage_session_integrity (view).\n\n")
	for _, t := range s.tables {
		fmt.Fprintf(&b, "Table %s {\n", t.name)
		for _, c := range t.columns {
			var settings []string
			if c.pk {
				settings = append(settings, "pk")
			}
			if c.notNull {
				settings = append(settings, "not null")
			}
			if c.unique {
				settings = append(settings, "unique")
			}
			if d := formatDefault(c.defaultValue); d != "" {
				settings = append(settings, "default: "+d)
			}
			line := "  " + c.name + " " + strings.ToLower(c.colType)
			if len(settings) > 0 {
				line += " [" + strings.Join(settings, ", ") + "]"
			}
			b.WriteString(line + "\n")
		}
		if len(t.compositePK) > 0 || len(t.compositeUniques) > 0 {
			b.WriteString("\n  indexes {\n")
			if len(t.compositePK) > 0 {
				fmt.Fprintf(&b, "    (%s) [pk]\n", strings.Join(t.compositePK, ", "))
			}
			for _, u := range t.compositeUniques {
				fmt.Fprintf(&b, "    (%s) [unique]\n", strings.Join(u, ", "))
			}
			b.WriteString("  }\n")
		}
		b.WriteString("}\n\n")
	}
	for _, fk := range s.fks {
		fmt.Fprintf(&b, "Ref: %s.%s > %s.%s\n", fk.fromTable, fk.fromCol, fk.toTable, fk.toCol)
	}
	return b.String()
}
