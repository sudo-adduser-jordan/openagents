/**
 * 05 — Session interface handoff (the TUI <-> Chat saga).
 *
 * The one subsystem whose correctness rests on invariants that no single file
 * states: the session row is the commit point, everything before it is
 * reversible, and the messages that arrive during the window where no
 * controller is live have to survive whoever ends up owning the session.
 *
 * `session_manager/interface_transition.go` is roughly 1,700 lines. This is the
 * shape it implements.
 */

import { Scene, TONE } from "../lib.mjs";

const scene = new Scene({
	name: "05-interface-handoff-saga",
	title: "Session interface handoff — TUI ↔ Chat",
	subtitle:
		"A controller replacement inside one session: same id, workspace, lifecycle facts and\n" +
		"PR ownership. Only the mode-owned controller changes, and the session row is the\n" +
		"single commit point.",
	source: "https://github.com/sudo-adduser-jordan/open-agents",
});

const top = scene.heading();
const PITCH = 90;

// --- Forward path --------------------------------------------------------------

scene.frame({ x: 40, y: top - 30, w: 620, h: 1020, name: "Forward path — coordinator in session_manager" }, () => {
	const x = 80;
	const w = 540;

	scene.box({
		x,
		y: top,
		w,
		text: "requested",
		sublabel: "client posts target mode + policy",
		tone: TONE.ui,
	});
	scene.box({
		x,
		y: top + PITCH,
		w,
		text: "claim",
		sublabel: "exactly one active transition per session",
		tone: TONE.ui,
	});
	scene.box({
		x,
		y: top + 2 * PITCH,
		w,
		text: "gate the source",
		sublabel: "arm the handoff and close intake, or gate terminal input",
		tone: TONE.daemon,
	});
	scene.box({
		x,
		y: top + 3 * PITCH,
		w,
		text: "preflighting the target",
		sublabel: "binary, auth and protocol checked before anything is given up",
		tone: TONE.daemon,
	});
	scene.box({
		x,
		y: top + 4 * PITCH + 10,
		w,
		text: "policy",
		sublabel: "drain is loss-minimizing · interrupt closes intake immediately",
		tone: TONE.plane,
	});

	scene.box({
		x: 80,
		y: top + 5 * PITCH + 50,
		w: 250,
		text: "drain",
		sublabel: "finish accepted work",
		tone: TONE.derived,
	});
	scene.box({
		x: 370,
		y: top + 5 * PITCH + 50,
		w: 250,
		text: "interrupt",
		sublabel: "cancel queued and active turns",
		tone: TONE.derived,
	});

	scene.box({
		x,
		y: top + 6 * PITCH + 70,
		w,
		text: "source_stopping → source_stopped",
		sublabel: "short transcript flush, then wait for shutdown",
		tone: TONE.daemon,
	});
	scene.box({
		x,
		y: top + 7 * PITCH + 70,
		w,
		text: "COMMIT — CommitControllerEpoch",
		sublabel: "CAS mode, clear the old generation and handles, set the idle fact",
		tone: TONE.store,
	});
	scene.box({
		x,
		y: top + 8 * PITCH + 80,
		w,
		text: "target_starting → activating",
		sublabel: "native resume of the same conversation id, then persist the handle",
		tone: TONE.daemon,
	});
	scene.box({
		x,
		y: top + 9 * PITCH + 90,
		w,
		text: "completed",
		sublabel: "session_updated reaches clients through the CDC fan-out",
		tone: TONE.store,
	});

	// The spine. Hand-routed so the two policy branches leave and rejoin
	// cleanly instead of relying on the automatic dominant-axis guess.
	const cx = 350;
	scene.edge({ points: [[cx, top + 60], [cx, top + PITCH]], tone: TONE.daemon });
	scene.edge({ points: [[cx, top + PITCH + 60], [cx, top + 2 * PITCH]], tone: TONE.daemon });
	scene.edge({ points: [[cx, top + 2 * PITCH + 60], [cx, top + 3 * PITCH]], tone: TONE.daemon });
	scene.edge({ points: [[cx, top + 3 * PITCH + 60], [cx, top + 4 * PITCH + 10]], tone: TONE.daemon });

	const forkY = top + 4 * PITCH + 100;
	scene.edge({ points: [[cx, top + 4 * PITCH + 70], [cx, forkY], [205, forkY], [205, top + 5 * PITCH + 50]], tone: TONE.derived });
	scene.edge({ points: [[cx, top + 4 * PITCH + 70], [cx, forkY], [495, forkY], [495, top + 5 * PITCH + 50]], tone: TONE.derived });

	const joinY = top + 5 * PITCH + 90;
	scene.edge({ points: [[205, top + 5 * PITCH + 110], [205, joinY], [cx, joinY], [cx, top + 6 * PITCH + 70]], tone: TONE.derived });
	scene.edge({ points: [[495, top + 5 * PITCH + 110], [495, joinY], [cx, joinY], [cx, top + 6 * PITCH + 70]], tone: TONE.derived });

	scene.edge({ points: [[cx, top + 6 * PITCH + 130], [cx, top + 7 * PITCH + 70]], tone: TONE.store });
	scene.edge({ points: [[cx, top + 7 * PITCH + 140], [cx, top + 8 * PITCH + 80]], tone: TONE.store });
	scene.edge({ points: [[cx, top + 8 * PITCH + 150], [cx, top + 9 * PITCH + 90]], tone: TONE.store });
});

// --- Failure and recovery ------------------------------------------------------

scene.frame({ x: 720, y: top - 30, w: 580, h: 900, name: "Failure and recovery" }, () => {
	const x = 750;
	const w = 520;

	scene.box({
		x,
		y: top,
		w,
		text: "target startup fails",
		sublabel: "the session row still names the source",
		tone: TONE.hazard,
	});
	scene.box({
		x,
		y: top + 90,
		w,
		text: "CAS the row back",
		sublabel: "mode, generation and handles restored to the source",
		tone: TONE.hazard,
	});
	scene.box({
		x,
		y: top + 180,
		w,
		text: "resume the source",
		sublabel: "files and completed provider context survive",
		tone: TONE.derived,
	});
	scene.box({ x, y: top + 270, w, text: "phase = failed", tone: TONE.hazard });

	scene.box({
		x,
		y: top + 400,
		w,
		text: "the daemon dies mid-handoff",
		tone: TONE.hazard,
	});
	scene.box({
		x,
		y: top + 490,
		w,
		text: "boot reconciliation",
		sublabel: "finds the interrupted transition on startup",
		tone: TONE.derived,
	});
	scene.box({
		x,
		y: top + 580,
		w,
		text: "restore the last committed session_mode",
		sublabel: "the controller that actually ran, not the one being aimed at",
		tone: TONE.derived,
	});
	scene.box({ x, y: top + 670, w, text: "phase = recovery_required", tone: TONE.hazard });

	for (const [from, to] of [
		[90, 180],
		[180, 270],
		[400, 490],
		[490, 580],
		[580, 670],
	]) {
		scene.edge({ points: [[1010, top + from + 60], [1010, top + to]], tone: TONE.hazard });
	}

	// The rollback, drawn back to the commit point it undoes. It leaves this
	// frame on the left, so it is drawn outside the callback: a frame cannot
	// claim a child that extends past its own border.
	scene.note({
		x: 750,
		y: top + 760,
		w: 520,
		text: [
			"Cancelled at acceptance leaves the source untouched: everything before",
			"the commit point is reversible, and the dispatch fence stops a queued",
			"completion from being promoted while the target is still preflighting.",
		].join("\n"),
		tone: TONE.derived,
		size: 13,
	});
});

// The rollback, drawn back to the commit point it undoes. It leaves the
// failure frame on the left, so it is emitted outside that frame's callback: a
// frame cannot claim a child that extends past its own border.
scene.edge({
	points: [
		[620, top + 7 * PITCH + 105],
		[690, top + 7 * PITCH + 105],
		[690, top + 30],
		[750, top + 30],
	],
	tone: TONE.hazard,
	dashed: true,
});

// --- The fences ----------------------------------------------------------------

scene.frame({ x: 1360, y: top - 30, w: 440, h: 560, name: "Fences and the no-controller gap" }, () => {
	scene.note({
		x: 1390,
		y: top,
		w: 380,
		text: [
			"Old Chat events are fenced by",
			"controller generation. Old TUI",
			"hooks are fenced by runtime",
			"launch id. Neither can be",
			"mistaken for the new owner.",
		].join("\n"),
		tone: TONE.plane,
		size: 13,
	});
	scene.note({
		x: 1390,
		y: top + 130,
		w: 380,
		text: [
			"Messages that arrive while no",
			"controller is live wait in a",
			"durable outbox, delivered",
			"through whichever controller ends",
			"up owning the session.",
		].join("\n"),
		tone: TONE.store,
		size: 13,
	});
	scene.note({
		x: 1390,
		y: top + 270,
		w: 380,
		text: [
			"A drain may wait indefinitely on",
			"an approval. An unverified idle",
			"surface has a bounded proof",
			"window; a visible draft fails",
			"with the source untouched.",
		].join("\n"),
		tone: TONE.derived,
		size: 13,
	});
});

export default scene;
