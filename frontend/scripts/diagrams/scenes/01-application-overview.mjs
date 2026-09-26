/**
 * 01 — Application overview.
 *
 * The one-glance picture: which processes exist, who is allowed to talk to
 * whom, and where state actually lives. Every other diagram here is a zoom-in
 * on one of the frames drawn below.
 */

import { Scene, TONE } from "../lib.mjs";

const scene = new Scene({
	name: "01-application-overview",
	title: "Open Agents — application overview",
	subtitle:
		"Two processes and one loopback socket. The CLI is a third client of that socket,\n" +
		"not a second path into the data. The desktop app supervises the daemon; it never\n" +
		"reads the database or spawns a runtime itself.",
	source: "https://github.com/sudo-adduser-jordan/open-agents",
});

const top = scene.heading();

/*
 * Column layout. Gutters are sized for the edge labels that live in them, so
 * a label never lands on top of a frame border:
 *
 *   [desktop 40..400] [gutter A 240] [daemon 640..1060] [gutter B 140]
 *   [adapters 1200..1520] [gutter C 80] [external 1600..1900]
 */
const GUTTER_A = 500; // centre of the desktop -> daemon gutter, for labels
const GUTTER_B = 1130; // centre of the daemon -> adapters gutter
const ROW = 100; // vertical pitch inside a column

// --- Desktop supervisor ---------------------------------------------------------

scene.frame({ x: 40, y: top - 30, w: 360, h: 470, name: "Desktop app (Electron)" }, () => {
	scene.box({ x: 70, y: top, w: 300, text: "Main process", sublabel: "daemon supervisor, tray, updater", tone: TONE.ui });
	scene.box({ x: 70, y: top + ROW, w: 300, text: "Preload bridge", sublabel: "app, terminal, daemon, browser, ...", tone: TONE.ui });
	scene.box({ x: 70, y: top + 2 * ROW, w: 300, text: "Renderer", sublabel: "React routes, stores, hooks", tone: TONE.ui });
	scene.box({ x: 70, y: top + 3 * ROW, w: 300, text: "Transports", sublabel: "REST · SSE · terminal WebSocket", tone: TONE.plane });
});

scene.frame({ x: 40, y: top + 480, w: 360, h: 150, name: "CLI" }, () => {
	scene.box({
		x: 70,
		y: top + 510,
		w: 300,
		text: "open-agents",
		sublabel: "thin Cobra client, same HTTP API",
		tone: TONE.ui,
	});
});

// --- The daemon ----------------------------------------------------------------

scene.frame({ x: 640, y: top - 30, w: 420, h: 610, name: "Loopback daemon — 127.0.0.1:3001" }, () => {
	scene.box({ x: 670, y: top, w: 360, text: "httpd", sublabel: "REST controllers, SSE, terminal mux", tone: TONE.daemon });
	scene.box({ x: 670, y: top + ROW, w: 360, text: "service/", sublabel: "session · project · pr · review", tone: TONE.daemon });
	scene.box({ x: 670, y: top + 2 * ROW, w: 360, text: "session_manager", sublabel: "the internal command engine", tone: TONE.daemon });
	scene.box({ x: 670, y: top + 3 * ROW, w: 360, text: "lifecycle", sublabel: "single writer for durable facts", tone: TONE.daemon });
	scene.box({ x: 670, y: top + 4 * ROW, w: 360, text: "observe/", sublabel: "scm observer · runtime reaper", tone: TONE.daemon });
	scene.box({ x: 670, y: top + 5 * ROW, w: 360, text: "cdc", sublabel: "change_log poller + broadcaster", tone: TONE.store });
});

// --- Adapters ------------------------------------------------------------------

scene.frame({ x: 1200, y: top - 30, w: 320, h: 610, name: "Adapters (implement ports)" }, () => {
	scene.box({ x: 1230, y: top, w: 260, text: "agent", sublabel: "23+ coding harnesses", tone: TONE.plane });
	scene.box({ x: 1230, y: top + ROW, w: 260, text: "runtime", sublabel: "tmux · conpty · PTY host", tone: TONE.plane });
	scene.box({ x: 1230, y: top + 2 * ROW, w: 260, text: "chatdriver", sublabel: "ACP host · opencode", tone: TONE.plane });
	scene.box({ x: 1230, y: top + 3 * ROW, w: 260, text: "workspace", sublabel: "git worktree · directory", tone: TONE.plane });
	scene.box({ x: 1230, y: top + 4 * ROW, w: 260, text: "scm / tracker", sublabel: "GitHub", tone: TONE.plane });
	scene.box({ x: 1230, y: top + 5 * ROW, w: 260, text: "browser", sublabel: "CDP-linked view", tone: TONE.plane });
});

// --- External systems ----------------------------------------------------------

scene.frame({ x: 1600, y: top - 30, w: 300, h: 610, name: "External systems" }, () => {
	scene.box({ x: 1630, y: top, w: 240, text: "Agent CLIs", tone: TONE.external });
	scene.box({ x: 1630, y: top + ROW, w: 240, text: "git", sublabel: "worktree operations", tone: TONE.external });
	scene.box({ x: 1630, y: top + 2 * ROW, w: 240, text: "GitHub", sublabel: "PRs, checks, reviews", tone: TONE.external });
	scene.box({ x: 1630, y: top + 3 * ROW, w: 240, text: "Chromium", sublabel: "agent-driven browsing", tone: TONE.external });
	scene.box({ x: 1630, y: top + 4 * ROW, w: 240, text: "OS notifications", tone: TONE.external });
	scene.box({ x: 1630, y: top + 5 * ROW, w: 240, text: "Release feed", sublabel: "electron-updater", tone: TONE.external });
});

// --- Durable state -------------------------------------------------------------

scene.frame({
	x: 640,
	y: top + 640,
	w: 880,
	h: 250,
	name: "~/.open-agents  (overridable via OPEN_AGENTS_DATA_DIR)",
}, () => {
	scene.box({ x: 670, y: top + 680, w: 360, text: "SQLite", sublabel: "sessions, PRs, change_log, transitions", tone: TONE.store });
	scene.box({ x: 1230, y: top + 680, w: 260, text: "Worktrees", sublabel: "one per project session", tone: TONE.store });
	scene.box({ x: 700, y: top + 760, w: 300, text: "running.json", sublabel: "daemon address + pid", tone: TONE.store });
});

// --- Edges ---------------------------------------------------------------------

// Both clients reach the same listener. The two lanes keep the labels apart.
scene.edge({
	points: [
		[370, top + 233],
		[GUTTER_A - 40, top + 233],
		[GUTTER_A - 40, top + 33],
		[670, top + 33],
	],
	label: "REST + SSE + WS",
	tone: TONE.ui,
});
scene.edge({
	points: [
		[370, top + 543],
		[GUTTER_A + 60, top + 543],
		[GUTTER_A + 60, top + 33],
		[670, top + 33],
	],
	label: "REST only",
	tone: TONE.ui,
});

// Daemon to adapters, drawn frame to frame so the labels live in the gutter.
for (const [row, label, tone] of [
	[0, "commands", TONE.daemon],
	[4, "observations", TONE.daemon],
	[1, "terminal I/O", TONE.plane],
]) {
	scene.edge({
		points: [
			[1060, top + row * ROW + 33],
			[1200, top + row * ROW + 33],
		],
		label,
		tone,
	});
}

// Adapters to the world.
for (const row of [0, 1, 2, 4]) {
	scene.edge({
		points: [
			[1520, top + row * ROW + 33],
			[1600, top + row * ROW + 33],
		],
		tone: TONE.plane,
	});
}

// Daemon to durable state. Both arrows stop on the frame border rather than
// crossing into it, so the group stays visually closed.
scene.edge({
	points: [
		[760, top + 580],
		[760, top + 640],
	],
	label: "durable facts",
	tone: TONE.store,
});
scene.edge({
	points: [
		[1300, top + 580],
		[1300, top + 640],
	],
	label: "worktrees",
	tone: TONE.store,
});

// --- Invariants ----------------------------------------------------------------

scene.note({
	x: 40,
	y: top + 700,
	w: 360,
	text: [
		"The primary listener stays bound",
		"to 127.0.0.1 and unauthenticated.",
		"There is no second listener and no",
		"auth exemption anywhere in the tree.",
	].join("\n"),
	tone: TONE.hazard,
});
scene.note({
	x: 1600,
	y: top + 700,
	w: 300,
	text: [
		"Core never imports an adapter.",
		"Dependencies point inward only,",
		"through the interfaces that",
		"ports/ declares.",
	].join("\n"),
	tone: TONE.plane,
});

export default scene;
