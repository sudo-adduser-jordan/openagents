import {
	attentionZone,
	defaultProductUITranslator,
	attentionZoneOrder,
	boardAttentionZoneOrder,
	boardLaneOrder,
	getAgentActivityView as getPortableAgentActivityView,
	getAttentionZoneView as getPortableAttentionZoneView,
	getAttentionZoneViewForZone as getPortableAttentionZoneViewForZone,
	getBoardLaneView as getPortableBoardLaneView,
	getSessionStatusView as getPortableSessionStatusView,
	getSessionTimelinePillView as getPortableSessionTimelinePillView,
	isAgentActivityWorking,
	isSessionIdle,
	toBoardLane,
	type AgentActivityView,
	type AttentionZone,
	type AttentionZoneView,
	type BoardLane,
	type BoardLaneView,
	type KanbanColumn,
	type SessionStatusView,
	type SessionTimelinePillStatus,
	type SessionTimelinePillView,
	type WorkflowMode,
} from "@openagents/product-ui";
import type { SessionActivity, SessionStatus } from "../types/workspace";

export function getAgentActivityView(
	activity?: SessionActivity | null
): AgentActivityView {
	return getPortableAgentActivityView(activity, defaultProductUITranslator);
}

export function getSessionStatusView(
	status: SessionStatus
): SessionStatusView {
	return getPortableSessionStatusView(status, defaultProductUITranslator);
}

export function getAttentionZoneView(
	status: SessionStatus
): AttentionZoneView {
	return getPortableAttentionZoneView(status, defaultProductUITranslator);
}

export function getAttentionZoneViewForZone(
	zone: AttentionZone
): AttentionZoneView {
	return getPortableAttentionZoneViewForZone(zone, defaultProductUITranslator);
}

export type SessionStatusDotView = {
	className: string;
	breathe: boolean;
};

// The session dot carries two independent signals. Colour comes from the board
// section represented by the SCM state, which survives a running agent —
// `status` is activity-first, so it collapses to `working` the moment an agent
// wakes and would otherwise take every pull request tone with it. Merged keeps
// its split-section tone instead of sharing Ready to merge's tone.
//
// Motion stays on raw agent activity. A no-PR idle session is the exception to
// the preserved section colour: when its agent starts working it blinks blue.
export function getSessionStatusDotView(
	session: {
		activity?: SessionActivity | null;
		displayStatus?: string;
		scmStatus?: SessionStatus;
		status: SessionStatus;
	}
): SessionStatusDotView {
	const working = isAgentActivityWorking(session.activity);
	const closedWithoutMerge = session.displayStatus === "Closed without merge";
	const sectionStatus: SessionStatus =
		closedWithoutMerge ? "exited" : (session.scmStatus ?? session.status);
	const toneStatus = sectionStatus === "idle" && working ? "working" : sectionStatus;
	const className =
		closedWithoutMerge
			? getSessionStatusView("exited").dotClassName
			: toneStatus === "idle" || toneStatus === "merged"
				? getSessionStatusView(toneStatus).dotClassName
				: getAttentionZoneView(toneStatus).dotClassName;

	return {
		className,
		breathe: working,
	};
}

export function getBoardLaneView(
	lane: BoardLane
): BoardLaneView {
	return getPortableBoardLaneView(lane, defaultProductUITranslator);
}

export function getSessionTimelinePillView(
	status: SessionTimelinePillStatus
): SessionTimelinePillView {
	return getPortableSessionTimelinePillView(status, defaultProductUITranslator);
}

/** Live labels for the current locale (getters re-resolve on each access). */
export const attentionZoneLabel: Record<AttentionZone, string> = {
	get merge() {
		return getAttentionZoneViewForZone("merge").label;
	},
	get action() {
		return getAttentionZoneViewForZone("action").label;
	},
	get pending() {
		return getAttentionZoneViewForZone("pending").label;
	},
	get working() {
		return getAttentionZoneViewForZone("working").label;
	},
	get done() {
		return getAttentionZoneViewForZone("done").label;
	},
};

export {
	attentionZone,
	attentionZoneOrder,
	boardAttentionZoneOrder,
	boardLaneOrder,
	isAgentActivityWorking,
	isSessionIdle,
	toBoardLane,
};
export type {
	AgentActivityView,
	AttentionZone,
	AttentionZoneView,
	BoardLane,
	BoardLaneView,
	KanbanColumn,
	SessionStatusView,
	SessionTimelinePillStatus,
	SessionTimelinePillView,
	WorkflowMode,
};
