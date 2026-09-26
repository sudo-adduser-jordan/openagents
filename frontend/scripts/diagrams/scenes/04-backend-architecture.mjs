/**
 * 04 — Backend.
 *
 * The Go daemon, drawn as the write path it actually is: a request comes in at
 * `httpd`, is turned into a command by `service/`, and only `session_manager`
 * and `lifecycle` are allowed to move a durable fact. Everything on the right
 * is reached through `ports/`, so the arrow direction is the dependency rule
 * made visible.
 */

import { Scene, TONE } from "../lib.mjs";

const scene = new Scene({
	name: "04-backend-architecture",
	title: "Backend — request path, single write path, CDC",
	subtitle:
		"Reads and commands enter at httpd. Only session_manager and lifecycle mutate durable\n" +
		"facts. Adapters are reachable only through the interfaces in ports/.",
	source: "https://github.com/sudo-adduser-jordan/open-agents",
});

const top = scene.heading();

// --- Inbound -------------------------------------------------------------------

scene.frame({ x: 40, y: top - 30, w: 560, h: 150, name: "Inbound" }, () => {
	scene.box({
		x: 80,
		y: top,
		w: 480,
		text: "httpd",
		sublabel: "REST controllers · SSE · terminal mux",
		tone: TONE.daemon,
	});
});

scene.frame({ x: 40, y: top + 150, w: 560, h: 300, name: "service/ — the read and command surface" }, () => {
	scene.caption({
		x: 80,
		y: top + 165,
		text: "Display status and kanban column are derived here, on read. Never stored.",
		tone: TONE.derived,
	});
	["session", "project", "pr", "review"].forEach((name, index) => {
		scene.box({ x: 80 + index * 120, y: top + 210, w: 110, text: name, tone: TONE.daemon });
	});
	scene.box({
		x: 80,
		y: top + 290,
		w: 480,
		text: "chat",
		sublabel: "native controllers, detached ACP host, durable projection",
		tone: TONE.daemon,
	});
});

scene.frame({ x: 40, y: top + 480, w: 560, h: 300, name: "The write path" }, () => {
	scene.caption({
		x: 80,
		y: top + 495,
		text: "The only code allowed to move a durable fact.",
		tone: TONE.hazard,
	});
	scene.box({
		x: 80,
		y: top + 540,
		w: 480,
		text: "session_manager",
		sublabel: "spawn · interface handoff · kill · inject",
		tone: TONE.daemon,
	});
	scene.box({
		x: 80,
		y: top + 630,
		w: 480,
		text: "lifecycle",
		sublabel: "single-writer reducer: activity_state, is_terminated",
		tone: TONE.daemon,
	});
});

// --- Observation ---------------------------------------------------------------

scene.frame({ x: 660, y: top + 150, w: 400, h: 220, name: "observe/ — external polling" }, () => {
	scene.box({ x: 690, y: top + 190, w: 340, text: "scm observer", sublabel: "PR, check and review facts", tone: TONE.daemon });
	scene.box({ x: 690, y: top + 280, w: 340, text: "runtime reaper", sublabel: "liveness probes", tone: TONE.daemon });
});

scene.frame({ x: 660, y: top + 400, w: 400, h: 380, name: "Load-bearing rules" }, () => {
	scene.note({
		x: 690,
		y: top + 430,
		w: 340,
		text: [
			"One writer per fact.",
			"Only lifecycle/ sets activity_state",
			"and is_terminated.",
		].join("\n"),
		tone: TONE.hazard,
		size: 13,
	});
	scene.note({
		x: 690,
		y: top + 540,
		w: 340,
		text: [
			"A failed probe is not proof",
			"of death. A session terminates only",
			"when runtime and process are both",
			"clearly dead and recent activity",
			"does not contradict it.",
		].join("\n"),
		tone: TONE.derived,
		size: 13,
	});
	scene.note({
		x: 690,
		y: top + 670,
		w: 340,
		text: [
			"Never force-delete a dirty",
			"registered worktree.",
		].join("\n"),
		tone: TONE.hazard,
		size: 13,
	});
});

// --- Ports and adapters --------------------------------------------------------

scene.frame({ x: 1120, y: top - 30, w: 400, h: 300, name: "ports/ — the inward dependency" }, () => {
	["workspace", "runtime", "agent", "chatdriver", "scm", "tracker"].forEach((name, index) => {
		scene.box({
			x: 1140 + (index % 2) * 190,
			y: top + 10 + Math.floor(index / 2) * 80,
			w: 170,
			text: name,
			tone: TONE.plane,
		});
	});
});

scene.frame({ x: 1120, y: top + 300, w: 400, h: 560, name: "adapters/ — implementations" }, () => {
	const rows = [
		["agent", "23+ coding harnesses"],
		["runtime", "tmux · conpty · macOS PTY host"],
		["chatdriver", "ACP host · opencode controller"],
		["workspace", "git worktree · plain directory"],
		["scm", "GitHub"],
		["tracker", "GitHub tracker"],
	];
	rows.forEach(([text, sublabel], index) => {
		scene.box({ x: 1150, y: top + 330 + index * 85, w: 340, text, sublabel, tone: TONE.plane });
	});
});

// --- Durable state and CDC -----------------------------------------------------

scene.frame({ x: 40, y: top + 820, w: 1480, h: 240, name: "storage/sqlite + cdc" }, () => {
	scene.caption({
		x: 80,
		y: top + 835,
		text: [
			"One write path for durable change: database triggers append to change_log.",
			"Store methods never emit events by hand.",
		].join("\n"),
		tone: TONE.store,
	});

	const y = top + 890;
	const sqlite = scene.box({ x: 80, y, w: 340, text: "SQLite", sublabel: "migrations, queries, stores", tone: TONE.store });
	const log = scene.box({ x: 480, y, w: 260, text: "change_log", sublabel: "trigger-appended", tone: TONE.store });
	const poller = scene.box({ x: 800, y, w: 260, text: "cdc poller", sublabel: "tailed by seq watermark", tone: TONE.store });
	const fanout = scene.box({
		x: 1120,
		y,
		w: 260,
		text: "broadcaster",
		sublabel: "terminal · SSE · cache",
		tone: TONE.store,
	});

	scene.link(sqlite, log, { label: "triggers", tone: TONE.store, at: "end" });
	scene.link(log, poller, { label: "polls", tone: TONE.store, at: "end" });
	scene.link(poller, fanout, { label: "fan-out", tone: TONE.store, at: "end" });

	scene.edge({
		points: [
			[1250, top + 950],
			[1250, top + 1000],
			[930, top + 1000],
			[930, top + 950],
		],
		label: "resume position",
		tone: TONE.store,
		dashed: true,
		head: "arrow",
	});

	scene.caption({
		x: 1400,
		y: top + 900,
		text: ["SSE writers feed", "connected clients", "through httpd."].join("\n"),
		tone: TONE.derived,
	});
});

// --- Edges between the columns -------------------------------------------------

scene.edge({
	points: [
		[320, top + 70],
		[320, top + 150],
	],
	tone: TONE.daemon,
});
scene.edge({
	points: [
		[320, top + 357],
		[320, top + 480],
	],
	label: "command",
	tone: TONE.daemon,
});
scene.edge({
	points: [
		[320, top + 607],
		[320, top + 630],
	],
	tone: TONE.daemon,
});
scene.edge({
	points: [
		[560, top + 695],
		[630, top + 695],
		[630, top + 320],
		[690, top + 320],
	],
	label: "reduce",
	tone: TONE.daemon,
});
scene.edge({
	points: [
		[1060, top + 225],
		[1120, top + 225],
	],
	label: "consumes",
	tone: TONE.plane,
});
scene.edge({
	points: [
		[1520, top + 60],
		[1560, top + 60],
		[1560, top + 360],
		[1520, top + 360],
	],
	label: "implements",
	tone: TONE.plane,
});

export default scene;
