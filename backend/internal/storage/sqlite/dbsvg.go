package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
)

// This file renders the README schema SVG from the same introspected schema
// that produces schema.dbml. The SVG is a committed build artifact for
// readers; the DBML file is the ChartDB-editable source. Layout is a
// deterministic grouped grid: fixed group order, hand-ordered tables within
// each group, unknown tables appended alphabetically to an "Other" group so a
// future migration cannot break the layout.

const (
	svgTableWidth  = 360
	svgHeaderH     = 30
	svgRowH        = 21
	svgTablePadB   = 8
	svgGroupTitleH = 34
	svgMargin      = 40
	svgGapX        = 60
	svgGapY        = 36
	svgTitleH      = 78
)

// schemaGroupOrder fixes the diagram's column order. Tables were assigned by
// domain; keep parents before children where it reads better.
var schemaGroupOrder = []struct {
	title  string
	tables []string
}{
	{"Projects & sessions", []string{
		"projects", "sessions", "session_worktrees", "workspace_repos",
		"shell_terminals", "session_cleanup_facts", "manager_reengagements",
		"retired_session_nums", "app_settings", "telemetry_event",
		"agent_model_catalog", "agent_install_jobs",
	}},
	{"Conversations", []string{
		"conversations", "conversation_turns", "conversation_messages",
		"conversation_branches", "conversation_activities",
		"conversation_provider_events", "conversation_edit_deliveries",
		"conversation_queued_edit_deliveries", "conversation_steer_deliveries",
		"session_interface_transitions", "session_interface_transition_messages",
	}},
	{"PR & review", []string{
		"pr", "pr_checks", "pr_comment", "pr_reviews", "pr_review_threads",
		"pr_url_alias", "review", "review_run", "notifications",
	}},
	{"Usage", []string{
		"usage_bindings", "usage_sources", "model_usage_events",
	}},
	{"Change feed", []string{
		"change_log",
	}},
}

// svgTableBox is a laid-out table: position, size, and per-column text
// centerlines for foreign-key edge endpoints.
type svgTableBox struct {
	table  schemaTable
	x      int
	y      int
	height int
	colY   []int
	fkCols map[string]bool
}

// DumpSVG introspects a migrated database and renders the schema SVG
// committed for the README.
func DumpSVG(ctx context.Context, db *sql.DB) (string, error) {
	schema, err := introspectSchema(ctx, db)
	if err != nil {
		return "", err
	}
	return schema.toSVG(), nil
}

// escapeXML escapes text for SVG element content and attribute use.
func escapeXML(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}

// toSVG lays out grouped table boxes and draws foreign-key edges between
// them. Edges run from the child's right edge to the parent's left edge;
// self-references render as a small loop on the right edge.
func (s *dbSchema) toSVG() string {
	byName := map[string]schemaTable{}
	for _, t := range s.tables {
		byName[t.name] = t
	}
	assigned := map[string]bool{}
	type group struct {
		title  string
		tables []schemaTable
	}
	var groups []group
	for _, g := range schemaGroupOrder {
		var tables []schemaTable
		for _, name := range g.tables {
			if t, ok := byName[name]; ok {
				tables = append(tables, t)
				assigned[name] = true
			}
		}
		groups = append(groups, group{title: g.title, tables: tables})
	}
	var other []schemaTable
	for _, t := range s.tables {
		if !assigned[t.name] {
			other = append(other, t)
		}
	}
	if len(other) > 0 {
		sort.Slice(other, func(i, j int) bool { return other[i].name < other[j].name })
		groups = append(groups, group{title: "Other", tables: other})
	}

	fkCols := map[string]map[string]bool{}
	for _, fk := range s.fks {
		if fkCols[fk.fromTable] == nil {
			fkCols[fk.fromTable] = map[string]bool{}
		}
		fkCols[fk.fromTable][fk.fromCol] = true
	}

	boxes := map[string]*svgTableBox{}
	maxBottom := 0
	for gi, g := range groups {
		x := svgMargin + gi*(svgTableWidth+svgGapX)
		y := svgMargin + svgTitleH + svgGroupTitleH
		for _, t := range g.tables {
			h := svgHeaderH + len(t.columns)*svgRowH + svgTablePadB
			box := &svgTableBox{table: t, x: x, y: y, height: h, fkCols: fkCols[t.name]}
			for ci := range t.columns {
				box.colY = append(box.colY, y+svgHeaderH+ci*svgRowH+svgRowH/2)
			}
			boxes[t.name] = box
			y += h + svgGapY
		}
		if bottom := y - svgGapY; bottom > maxBottom {
			maxBottom = bottom
		}
	}
	width := svgMargin*2 + len(groups)*svgTableWidth + (len(groups)-1)*svgGapX
	height := maxBottom + svgMargin

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" font-family="ui-monospace, SFMono-Regular, Menlo, Consolas, monospace">`+"\n",
		width, height, width, height)
	b.WriteString(`<defs><marker id="fk-arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse"><path d="M 0 1 L 9 5 L 0 9 z" fill="#94a3b8"/></marker></defs>` + "\n")
	fmt.Fprintf(&b, `<rect x="0" y="0" width="%d" height="%d" fill="#f8fafc"/>`+"\n", width, height)
	b.WriteString(`<text x="` + itoa(svgMargin) + `" y="34" font-size="20" font-weight="bold" fill="#0f172a">Open Agents — SQLite schema</text>` + "\n")
	fmt.Fprintf(&b, `<text x="%d" y="56" font-size="12" fill="#64748b">%d tables · generated from migrations — do not edit · regen: task db:dbml · schema.dbml opens in ChartDB</text>`+"\n",
		svgMargin, len(s.tables))

	for gi, g := range groups {
		x := svgMargin + gi*(svgTableWidth+svgGapX)
		fmt.Fprintf(&b, `<text x="%d" y="%d" font-size="15" font-weight="bold" fill="#0f172a">%s</text>`+"\n",
			x, svgMargin+svgTitleH+20, escapeXML(g.title))
	}

	// Edges first so boxes paint over line endpoints.
	for _, fk := range s.fks {
		child, okChild := boxes[fk.fromTable]
		parent, okParent := boxes[fk.toTable]
		if !okChild || !okParent {
			continue
		}
		fromIdx := colIndex(child.table, fk.fromCol)
		toIdx := colIndex(parent.table, fk.toCol)
		if fromIdx < 0 || toIdx < 0 {
			continue
		}
		y1 := child.colY[fromIdx]
		title := escapeXML(fk.fromTable + "." + fk.fromCol + " → " + fk.toTable + "." + fk.toCol)
		if fk.fromTable == fk.toTable {
			x := child.x + svgTableWidth
			fmt.Fprintf(&b, `<path d="M %d %d C %d %d, %d %d, %d %d" fill="none" stroke="#94a3b8" stroke-width="1.5" marker-end="url(#fk-arrow)"><title>%s</title></path>`+"\n",
				x, y1, x+46, y1, x+46, y1+svgRowH, x, y1+svgRowH, title)
			continue
		}
		x1 := child.x + svgTableWidth
		x2 := parent.x
		y2 := parent.colY[toIdx]
		fmt.Fprintf(&b, `<path d="M %d %d C %d %d, %d %d, %d %d" fill="none" stroke="#94a3b8" stroke-width="1.5" marker-end="url(#fk-arrow)"><title>%s</title></path>`+"\n",
			x1, y1, x1+60, y1, x2-60, y2, x2, y2, title)
	}

	for _, g := range groups {
		for _, t := range g.tables {
			box := boxes[t.name]
			writeSVGTable(&b, box)
		}
	}

	b.WriteString(`</svg>` + "\n")
	return b.String()
}

// colIndex returns the column's position in the table or -1.
func colIndex(t schemaTable, col string) int {
	for i, c := range t.columns {
		if c.name == col {
			return i
		}
	}
	return -1
}

// itoa formats an int without importing strconv for a single call site.
func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}

// writeSVGTable renders one table box: header, column rows with PK/FK pills,
// and right-aligned types.
func writeSVGTable(b *strings.Builder, box *svgTableBox) {
	x, y := box.x, box.y
	fmt.Fprintf(b, `<g id="tbl-%s">`+"\n", escapeXML(box.table.name))
	fmt.Fprintf(b, `<rect x="%d" y="%d" width="%d" height="%d" rx="6" fill="#ffffff" stroke="#cbd5e1"/>`+"\n",
		x, y, svgTableWidth, box.height)
	fmt.Fprintf(b, `<path d="M %d %d h %d a 6 6 0 0 1 6 6 v %d h -%d z" fill="#243044"/>`+"\n",
		x, y, svgTableWidth, svgHeaderH-6, svgTableWidth)
	fmt.Fprintf(b, `<text x="%d" y="%d" font-size="13" font-weight="bold" fill="#ffffff">%s</text>`+"\n",
		x+12, y+20, escapeXML(box.table.name))
	for ci, c := range box.table.columns {
		cy := box.colY[ci]
		weight := "normal"
		if c.pk {
			weight = "bold"
		}
		fmt.Fprintf(b, `<text x="%d" y="%d" font-size="12" font-weight="%s" fill="#0f172a">%s</text>`+"\n",
			x+12, cy+4, weight, escapeXML(c.name))
		nx := x + 12 + len(c.name)*7 + 8
		if c.pk {
			fmt.Fprintf(b, `<rect x="%d" y="%d" width="24" height="15" rx="3" fill="#f59e0b"/><text x="%d" y="%d" font-size="9" font-weight="bold" fill="#ffffff">PK</text>`+"\n",
				nx, cy-8, nx+5, cy+4)
			nx += 30
		}
		if box.fkCols[c.name] {
			fmt.Fprintf(b, `<rect x="%d" y="%d" width="22" height="15" rx="3" fill="#0ea5e9"/><text x="%d" y="%d" font-size="9" font-weight="bold" fill="#ffffff">FK</text>`+"\n",
				nx, cy-8, nx+5, cy+4)
		}
		fmt.Fprintf(b, `<text x="%d" y="%d" font-size="11" fill="#64748b" text-anchor="end">%s</text>`+"\n",
			x+svgTableWidth-10, cy+4, escapeXML(strings.ToLower(c.colType)))
	}
	b.WriteString(`</g>` + "\n")
}
