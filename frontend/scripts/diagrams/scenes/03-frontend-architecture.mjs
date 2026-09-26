/**
 * 03 — Frontend.
 *
 * The Electron side of the app, and the boundaries that keep it a supervisor
 * rather than a second implementation. Two rules are visible here: the
 * renderer never opens the database or spawns a runtime, and every byte that
 * crosses into it goes through the preload bridge.
 */

import { Scene, TONE } from "../lib.mjs";

const scene = new Scene({
	name: "03-frontend-architecture",
	title: "Frontend — supervisor, bridge, renderer",
	subtitle:
		"Main owns processes, preload owns capability, renderer owns presentation. The\n" +
		"daemon is reached only over loopback HTTP; the renderer never touches storage\n" +
		"or spawns a runtime itself.",
	source: "https://github.com/sudo-adduser-jordan/open-agents",
});

const top = scene.heading();
const PITCH = 85;

// --- Main process --------------------------------------------------------------

scene.frame({ x: 40, y: top - 30, w: 420, h: 540, name: "Main process (Node)" }, () => {
	const rows = [
		["Window composition", "app://renderer/* custom scheme"],
		["Daemon supervisor", "spawn, own, health-check, restart"],
		["Tray + menu", "lifecycle and quick actions"],
		["Auto-updater", "electron-updater, feed channel"],
		["Browser runtime", "WebContentsView + CDP bridge"],
		["Notifications, keybindings", "OS signals, global shortcuts"],
	];
	rows.forEach(([text, sublabel], index) => {
		scene.box({ x: 70, y: top + index * PITCH, w: 360, text, sublabel, tone: TONE.ui });
	});
});

// --- Preload -------------------------------------------------------------------

scene.frame({ x: 520, y: top - 30, w: 380, h: 400, name: "Preload" }, () => {
	scene.box({
		x: 550,
		y: top,
		w: 320,
		text: "contextBridge",
		sublabel: "exposes window.openAgents",
		tone: TONE.plane,
	});
	scene.box({
		x: 550,
		y: top + 110,
		w: 320,
		text: "17 namespaces",
		sublabel: [
			"app · terminal · window · theme · menu",
			"clipboard · daemon · editorHandoff",
			"browser · downloads · browserProfiles",
			"notifications · tray · updates · ...",
		].join("\n"),
		tone: TONE.plane,
	});
	scene.box({
		x: 550,
		y: top + 290,
		w: 320,
		text: "ipcRenderer",
		sublabel: "invoke / on, and nothing else",
		tone: TONE.plane,
	});
});

// --- Renderer ------------------------------------------------------------------

scene.frame({ x: 960, y: top - 30, w: 460, h: 540, name: "Renderer (React)" }, () => {
	const rows = [
		["Routes", "TanStack Router, file-based"],
		["Stores + hooks", "ui-store, selectors"],
		["api-client", "typed from generated schema.ts"],
		["event-transport", "SSE consumer + reconnect"],
		["terminal-mux", "WebSocket, one per terminal"],
		["product-ui", "presentational only"],
	];
	rows.forEach(([text, sublabel], index) => {
		scene.box({ x: 990, y: top + index * PITCH, w: 400, text, sublabel, tone: TONE.ui });
	});
});

// --- The daemon, as seen from here ---------------------------------------------

scene.frame({ x: 1480, y: top - 30, w: 400, h: 250, name: "Peer process" }, () => {
	scene.box({
		x: 1510,
		y: top,
		w: 340,
		text: "Loopback daemon",
		sublabel: "127.0.0.1:3001  (diagram 04)",
		tone: TONE.daemon,
	});
	scene.caption({
		x: 1510,
		y: top + 90,
		text: [
			"Supervised as a child process, but a",
			"separate program: the desktop app can",
			"reconnect to a running daemon, and a",
			"replacement daemon reattaches to the",
			"detached ACP host without stopping an",
			"in-flight turn.",
		].join("\n"),
	});
});

scene.frame({ x: 1480, y: top + 280, w: 400, h: 200, name: "Static surfaces" }, () => {
	scene.box({ x: 1510, y: top + 320, w: 340, text: "Docs site", sublabel: "Next + Fumadocs", tone: TONE.external });
	scene.box({ x: 1510, y: top + 400, w: 340, text: "Landing", sublabel: "Next, site-theme tokens", tone: TONE.external });
});

// --- Transports ----------------------------------------------------------------

scene.frame({ x: 40, y: top + 570, w: 1380, h: 280, name: "Transports across the loopback socket" }, () => {
	scene.caption({
		x: 80,
		y: top + 590,
		text: "Renderer to daemon. Long-lived routes sit outside middleware.Timeout on purpose.",
		tone: TONE.daemon,
	});

	const out = [
		["REST", "sessions, projects, PRs, reviews"],
		["SSE  /events", "CDC fan-out to the UI"],
		["WS  /mux", "terminal attach, per terminal"],
		["SSE  /notifications/stream", "per-session notification state"],
		["SSE  /workspace/events", "worktree presence"],
	];
	out.forEach(([text, sublabel], index) => {
		scene.box({ x: 80 + index * 262, y: top + 630, w: 240, text, sublabel, tone: TONE.daemon });
	});

	scene.caption({
		x: 80,
		y: top + 730,
		text: "Daemon to main process — the reverse direction people rarely expect:",
		tone: TONE.hazard,
	});
	scene.box({
		x: 80,
		y: top + 770,
		w: 640,
		text: "Agent browser runtime link",
		sublabel: "newline-delimited JSON → WebContentsView, token-authenticated",
		tone: TONE.hazard,
	});
	scene.box({
		x: 760,
		y: top + 770,
		w: 640,
		text: "Notification signals",
		sublabel: "IPC when a durable event deserves the user's attention",
		tone: TONE.hazard,
	});
});

// --- Edges ---------------------------------------------------------------------

scene.edge({
	points: [
		[460, top + 30],
		[520, top + 30],
	],
	tone: TONE.plane,
});
scene.edge({
	points: [
		[900, top + 30],
		[960, top + 30],
	],
	tone: TONE.plane,
});

// The three transport boxes hang off the renderer's api layer.
for (const index of [0, 1, 2]) {
	scene.edge({
		points: [
			[1390, top + 2 * PITCH + 30],
			[1430, top + 2 * PITCH + 30],
			[1430, top + 660],
			[80 + index * 262 + 240, top + 660],
		],
		tone: TONE.daemon,
		head: "none",
	});
}

scene.note({
	x: 1480,
	y: top + 540,
	w: 400,
	text: [
		"All Open Agents state lives under",
		"~/.open-agents. Electron userData is",
		"pinned there rather than left at the OS",
		"default, and never at ~/Library.",
	].join("\n"),
	tone: TONE.plane,
});

export default scene;
