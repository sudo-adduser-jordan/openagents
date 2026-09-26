/**
 * 02 — Session state machines.
 *
 * Two machines that are constantly mistaken for one. The left is durable: five
 * `activity_state` values plus the `is_terminated` flag, written by exactly one
 * reducer and nothing else. The right is derived: the display status a client
 * sees, recomputed on every read and never stored anywhere.
 *
 * The dashed derivation on the right is the point of the diagram. If a status
 * looks like it is persisting, this is the picture that is wrong.
 */

import { Scene, TONE } from "../lib.mjs";

const scene = new Scene({
	name: "02-session-state-machines",
	title: "Session state machines — durable facts vs. derived status",
	subtitle:
		"The durable machine has five states and one flag, with a single writer. Display status\n" +
		"is a pure function of those facts plus PR observations, evaluated fresh on each read.",
	source: "https://github.com/sudo-adduser-jordan/open-agents",
});

const top = scene.heading();

// --- Left: the durable machine -------------------------------------------------
//
// Laid out tall and narrow on purpose. The state graph is much taller than it
// is wide, and a wide layout makes the orthogonal router pick horizontal-first
// for edges that are really vertical, which then run straight through the
// neighbouring states.

scene.frame({ x: 40, y: top - 30, w: 640, h: 870, name: "Durable — written only by lifecycle/" }, () => {
	const active = scene.box({ x: 260, y: top + 60, w: 200, text: "active", tone: TONE.daemon, shape: "ellipse", initial: true });
	const idle = scene.box({ x: 80, y: top + 340, w: 160, text: "idle", tone: TONE.daemon, shape: "ellipse" });
	const waiting = scene.box({ x: 270, y: top + 340, w: 180, text: "waiting_input", tone: TONE.derived, shape: "ellipse" });
	const blocked = scene.box({ x: 480, y: top + 340, w: 160, text: "blocked", tone: TONE.hazard, shape: "ellipse" });
	const exited = scene.box({ x: 80, y: top + 620, w: 180, text: "exited", tone: TONE.hazard, shape: "ellipse" });

	// Leaving `active` is where the interesting guards live; the returns are
	// unlabelled because the state names already say what resumed it.
	scene.link(active, idle, { label: "no output", tone: TONE.derived, dashed: true });
	scene.link(active, waiting, { label: "empty prompt", tone: TONE.derived, dashed: true, at: "end" });
	scene.link(active, blocked, { label: "pending approval", tone: TONE.hazard, dashed: true });
	scene.link(idle, active, { tone: TONE.derived, dashed: true });
	scene.link(waiting, active, { tone: TONE.derived, dashed: true });
	scene.link(blocked, active, { tone: TONE.derived, dashed: true });

	// Any live state can turn into `exited`; the three edges converge on purpose.
	scene.link(idle, exited, { tone: TONE.hazard });
	scene.link(waiting, exited, { label: "process exits", tone: TONE.hazard, bend: 30 });
	scene.link(blocked, exited, { tone: TONE.hazard, bend: 46 });

	// `is_terminated` is a flag, not a state: it is orthogonal to the machine,
	// so it sits beside `exited` rather than in the graph.
	scene.box({
		x: 460,
		y: top + 620,
		w: 200,
		text: "is_terminated",
		sublabel: "separate boolean",
		tone: TONE.store,
	});
	scene.edge({
		points: [
			[260, top + 710],
			[460, top + 710],
		],
		label: "Kill()",
		tone: TONE.hazard,
	});
});

scene.note({
	x: 40,
	y: top + 880,
	w: 640,
	text: [
		"Automation may inject input into waiting_input — that is an agent sitting",
		"at an empty prompt. It must never inject into blocked, which is an agent",
		"stopped on a permission decision only a human can make.",
	].join("\n"),
	tone: TONE.hazard,
});

// --- Right: the derived ladder -------------------------------------------------

scene.frame({
	x: 740,
	y: top - 30,
	w: 1040,
	h: 780,
	name: "Derived — recomputed on every read, never written",
}, () => {
	scene.caption({
		x: 780,
		y: top - 8,
		text: "Guards, highest precedence first. The first match wins.",
		tone: TONE.derived,
	});

	const rows = [
		["is_terminated  &&  PR merged", "merged", TONE.store],
		["is_terminated", "terminated", TONE.hazard],
		["activity_state ∈ {waiting_input, blocked}", "needs_input", TONE.derived],
		["PR facts present", "ci_failed · draft · changes_requested\nmerge_conflict · mergeable · approved\nreview_pending · pr_open", TONE.plane],
		["activity_state == active", "working", TONE.daemon],
		["signal capable  &&  no signal", "no_signal", TONE.derived],
		["otherwise", "idle", TONE.external],
	];

	rows.forEach(([guard, result, tone], index) => {
		const y = top + 50 + index * 96;
		scene.text({ x: 780, y: y + 14, text: guard, fontSize: 13, tone: TONE.external });
		scene.box({ x: 1210, y, w: 520, text: result, tone });
		scene.edge({ points: [[1190, y + 32], [1210, y + 32]], tone: TONE.derived });
	});

	scene.caption({
		x: 780,
		y: top + 700,
		text: [
			"The kanban column comes out of the same facts:",
			"building · validating · needs_review · ready · archive.",
		].join("\n"),
		tone: TONE.plane,
	});
});

export default scene;
