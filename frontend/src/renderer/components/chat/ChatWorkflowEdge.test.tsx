import { fireEvent, render as rtlRender, screen } from "@testing-library/react";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { type ReactElement } from "react";
import { describe, expect, it, vi } from "vitest";
import { ChatWorkspace } from "./ChatWorkspace";
import type {
	ConversationItem,
	ConversationSnapshot,
	ConversationTurn,
} from "../../types/conversation";
import type { WorkspaceSession } from "../../types/workspace";
import { setApiBaseUrl } from "../../lib/api-client";
import { useUiStore } from "../../stores/ui-store";
import { TooltipProvider } from "../ui/tooltip";

vi.mock("../../lib/platform", () => ({
	isMacPlatform: () => true,
	isLinuxPlatform: () => false,
	isWindowsPlatform: () => false,
}));

function render(ui: ReactElement) {
	const result = rtlRender(<TooltipProvider>{ui}</TooltipProvider>);
	return {
		...result,
		rerender: (nextUi: ReactElement) => result.rerender(<TooltipProvider>{nextUi}</TooltipProvider>),
	};
}

function turn(id: string, workflowMode: ConversationTurn["workflowMode"], requestedAt: string): ConversationTurn {
	return {
		id,
		state: "completed",
		requestedAt,
		startedAt: requestedAt,
		completedAt: requestedAt,
		workflowMode,
	};
}

function humanMessage(id: string, turnId: string, sequence: number, text: string): ConversationItem {
	return {
		kind: "message",
		id,
		turnId,
		sequence,
		revision: 0,
		role: "user",
		origin: "human",
		text,
		streaming: false,
		createdAt: "2026-08-08T00:00:00Z",
	};
}

const SESSION_ID = "open-agents-workflow-edge";

function snapshotWithModes(): ConversationSnapshot {
	const turns: ConversationTurn[] = [
		turn("turn-planning", "planning", "2026-08-08T00:00:00Z"),
		turn("turn-building", "building", "2026-08-08T00:01:00Z"),
		// Pre-change history carries no recorded mode.
		{ id: "turn-legacy", state: "completed", requestedAt: "2026-08-08T00:02:00Z" },
	];
	return {
		conversationId: "conv-workflow-edge",
		sessionId: SESSION_ID,
		harness: "opencode",
		mode: "chat",
		controller: { state: "ready" },
		turns,
		items: [
			humanMessage("m-planning", "turn-planning", 1, "plan-first question"),
			humanMessage("m-building", "turn-building", 2, "build-it question"),
			humanMessage("m-legacy", "turn-legacy", 3, "legacy question"),
		],
		latestSequence: 3,
		oldestSequence: 1,
		hasMoreBefore: false,
		settings: {},
	};
}

function stubGeometry(node: HTMLElement, values: { scrollHeight: number; clientHeight: number; scrollTop: number }) {
	Object.defineProperty(node, "scrollHeight", { configurable: true, value: values.scrollHeight });
	Object.defineProperty(node, "clientHeight", { configurable: true, value: values.clientHeight });
	Object.defineProperty(node, "scrollTop", { configurable: true, writable: true, value: values.scrollTop });
}

describe("workflow-to-token mapping", () => {
	const styles = readFileSync(join(process.cwd(), "src/renderer/styles.css"), "utf8");

	it("maps each workflow to the composer's exact tokens in one place", () => {
		// The mapping lives once, in the --workflow-tone assignment consumed by
		// both the message edge and the ticks. Assert the mapping, not the hex:
		// planning resolves to the planning token, manager to the review token,
		// and building to the working blue — exactly the tones the composer frame
		// uses today.
		expect(styles).toMatch(
			/\.cursor-chat-human-message\[data-workflow="planning"\][\s\S]*?--workflow-tone:\s*var\(--color-status-planning\)/,
		);
		expect(styles).toMatch(
			/\.cursor-chat-human-message\[data-workflow="manager"\][\s\S]*?--workflow-tone:\s*var\(--color-status-review\)/,
		);
		expect(styles).toMatch(
			/\.cursor-chat-human-message\[data-workflow="building"\][\s\S]*?--workflow-tone:\s*var\(--color-status-working\)/,
		);
		// The composer frame agrees per workflow: this feature follows it.
		expect(styles).toMatch(
			/\.cursor-chat-composer\[data-workflow="planning"\][\s\S]*?--composer-border-hover:\s*var\(--color-status-planning\)/,
		);
		expect(styles).toMatch(
			/\.cursor-chat-composer\[data-workflow="building"\][\s\S]*?--composer-border-hover:\s*var\(--color-status-working\)/,
		);
	});

	it("paints edges and ticks from the shared variable, not separate colors", () => {
		expect(styles).toMatch(
			/\.cursor-chat-human-message\[data-workflow\][\s\S]*?border-left-color:\s*var\(--workflow-tone\)/,
		);
		expect(styles).toMatch(
			/\.chat-scroll-marker\[data-workflow\][\s\S]*?background:\s*var\(--workflow-tone\)/,
		);
	});

	it("adds, removes, and revalues no status tokens", () => {
		const defined = Array.from(
			styles.matchAll(/^\s*(--color-status-[a-z-]+):/gm),
			(match) => match[1],
		);
		expect(new Set(defined)).toEqual(
			new Set([
				"--color-status-planning",
				"--color-status-working",
				"--color-status-needs-you",
				"--color-status-validating",
				"--color-status-in-review",
				"--color-status-review",
				"--color-status-ready",
				"--color-status-merged",
				"--color-status-idle",
				"--color-status-exited",
				"--color-status-terminated",
				"--color-status-terminated-foreground",
				"--color-status-unknown",
			]),
		);
	});
});

describe("human message workflow edge", () => {
	const chatSession = {
		id: SESSION_ID,
		workspaceId: "project-1",
		workspaceName: "open-agents",
		title: "Workflow edge chat",
		provider: "opencode",
		kind: "manager",
		mode: "chat",
		status: "working",
		updatedAt: "2026-08-15T00:00:00Z",
		activity: { state: "active", lastActivityAt: "2026-08-15T00:00:00Z" },
		prs: [],
	} satisfies WorkspaceSession;

	it("colors each human message by its recorded send mode, not the current mode", () => {
		setApiBaseUrl("http://127.0.0.1:3001");
		useUiStore.setState({ inspectorSessions: { [SESSION_ID]: { isOpen: false, view: "summary" } } });
		try {
			const view = render(<ChatWorkspace snapshot={snapshotWithModes()} session={chatSession} />);
			const bubbles = Array.from(
				document.querySelectorAll<HTMLElement>(".cursor-chat-human-message"),
			);
			expect(bubbles).toHaveLength(3);
			expect(bubbles[0]).toHaveAttribute("data-workflow", "planning");
			expect(bubbles[1]).toHaveAttribute("data-workflow", "building");
			// Pre-change history renders the pinned fallback: no edge, no errors.
			expect(bubbles[2]).not.toHaveAttribute("data-workflow");

			// Switching the session's mode afterwards recolors the composer,
			// never the recorded prompts.
			view.rerender(
				<ChatWorkspace
					snapshot={snapshotWithModes()}
					session={{ ...chatSession, workflowMode: "planning" }}
				/>,
			);
			const after = Array.from(
				document.querySelectorAll<HTMLElement>(".cursor-chat-human-message"),
			);
			expect(after.map((bubble) => bubble.getAttribute("data-workflow"))).toEqual([
				"planning",
				"building",
				null,
			]);
		} finally {
			setApiBaseUrl(null);
		}
	});

	it("carries the same workflow onto each message's scrollbar tick", () => {
		setApiBaseUrl("http://127.0.0.1:3001");
		useUiStore.setState({ inspectorSessions: { [SESSION_ID]: { isOpen: false, view: "summary" } } });
		try {
			render(<ChatWorkspace snapshot={snapshotWithModes()} />);
			const log = screen.getByRole("log");
			const scrollbar = screen.getByRole("scrollbar", { name: "Conversation scrollbar" });
			stubGeometry(log, { scrollHeight: 4000, clientHeight: 800, scrollTop: 1000 });
			stubGeometry(scrollbar, { scrollHeight: 800, clientHeight: 800, scrollTop: 0 });
			fireEvent.scroll(log);
			const ticks = Array.from(
				scrollbar.querySelectorAll<HTMLElement>(".chat-scroll-marker"),
			);
			expect(ticks).toHaveLength(3);
			expect(ticks.map((tick) => tick.getAttribute("data-workflow"))).toEqual([
				"planning",
				"building",
				null,
			]);
		} finally {
			setApiBaseUrl(null);
		}
	});
});
