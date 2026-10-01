import { render as rtlRender, screen, fireEvent, waitFor } from "@testing-library/react";
import type { ReactElement } from "react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ChatComposer } from "../../../../../src/renderer/components/chat/ChatComposer";
import { sentHistoryForSnapshot } from "../../../../../src/renderer/components/chat/ChatWorkspace";
import { TooltipProvider } from "../../../../../src/renderer/components/ui/tooltip";
import type { ChatSkill, ConversationSnapshot } from "../../../../../src/renderer/types/conversation";
import {
	lexicalEditorText,
	typeInLexicalEditor,
} from "../../../../../src/renderer/test/lexical";

// Same TooltipProvider wrapper as the composer suite: every send control relies
// on the shared styled Tooltip.
function render(ui: ReactElement, options?: Parameters<typeof rtlRender>[1]) {
	return rtlRender(ui, { wrapper: TooltipProvider, ...options });
}

const SKILLS: ChatSkill[] = [
	{ name: "code-review", displayName: "code-review", description: "Review the diff", source: "user" },
	{ name: "review", displayName: "review", description: "Look it over", source: "repo" },
	{ name: "ship", displayName: "ship", description: "Open a PR", source: "user" },
];

const HISTORY = ["first prompt", "second prompt", "third prompt"];

function renderComposer(props: Partial<Parameters<typeof ChatComposer>[0]> = {}) {
	const onSend = vi.fn();
	render(<ChatComposer onSend={onSend} sentHistory={HISTORY} {...props} />);
	return { onSend, field: screen.getByLabelText("Message the agent") as HTMLElement };
}

function wireText(field: HTMLElement): string {
	return lexicalEditorText(field);
}

function snapshotWith(items: ConversationSnapshot["items"]): ConversationSnapshot {
	return {
		conversationId: "conv-history",
		sessionId: "session-history",
		harness: "codex",
		mode: "chat",
		controller: { state: "ready" },
		turns: [],
		items,
		latestSequence: items.length,
		oldestSequence: 1,
		hasMoreBefore: false,
		settings: {},
	};
}

function humanMessage(id: string, sequence: number, text: string, turnId = `turn-${id}`) {
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
		createdAt: `2026-08-26T06:00:0${sequence}Z`,
	} as const;
}

describe("sent history recall", () => {
	it("recalls newest-first on ArrowUp and walks forward on ArrowDown", async () => {
		const { field } = renderComposer();
		expect(wireText(field)).toBe("");

		await userEvent.keyboard("{ArrowUp}");
		expect(wireText(field)).toBe("third prompt");
		await userEvent.keyboard("{ArrowUp}");
		expect(wireText(field)).toBe("second prompt");
		await userEvent.keyboard("{ArrowUp}");
		expect(wireText(field)).toBe("first prompt");
		await userEvent.keyboard("{ArrowDown}");
		expect(wireText(field)).toBe("second prompt");
		await userEvent.keyboard("{ArrowDown}");
		expect(wireText(field)).toBe("third prompt");
		await userEvent.keyboard("{ArrowDown}");
		expect(wireText(field)).toBe("");
	});

	it("clamps at the oldest entry and ignores ArrowDown while not navigating", async () => {
		const { field } = renderComposer();
		await userEvent.keyboard("{ArrowDown}");
		expect(wireText(field)).toBe("");

		await userEvent.keyboard("{ArrowUp}{ArrowUp}{ArrowUp}{ArrowUp}{ArrowUp}");
		expect(wireText(field)).toBe("first prompt");
	});

	it("does nothing when the composer already holds text", async () => {
		const { field } = renderComposer();
		await typeInLexicalEditor(field, "a fresh draft");
		await userEvent.keyboard("{ArrowUp}");
		expect(wireText(field)).toBe("a fresh draft");
	});

	it("restores the empty draft on Escape", async () => {
		const { field } = renderComposer();
		await userEvent.keyboard("{ArrowUp}");
		expect(wireText(field)).toBe("third prompt");
		await userEvent.keyboard("{Escape}");
		expect(wireText(field)).toBe("");
		// Navigation ended: a further ArrowDown is a no-op, not a restore.
		await userEvent.keyboard("{ArrowDown}");
		expect(wireText(field)).toBe("");
	});

	it("sends a recalled entry and picks up the appended history", async () => {
		const onSend = vi.fn();
		const view = render(<ChatComposer onSend={onSend} sentHistory={HISTORY} />);
		const field = screen.getByLabelText("Message the agent") as HTMLElement;

		await userEvent.keyboard("{ArrowUp}");
		expect(wireText(field)).toBe("third prompt");
		await userEvent.keyboard("{Enter}");
		await waitFor(() => expect(onSend).toHaveBeenCalledWith("third prompt"));
		await waitFor(() => expect(wireText(field)).toBe(""));

		// The snapshot refresh carries the just-sent prompt as the newest entry.
		view.rerender(
			<ChatComposer onSend={onSend} sentHistory={[...HISTORY, "third prompt"]} />,
		);
		await userEvent.keyboard("{ArrowUp}");
		expect(wireText(field)).toBe("third prompt");
		await userEvent.keyboard("{ArrowUp}");
		expect(wireText(field)).toBe("third prompt");
	});

	it("recalls multiline messages intact", async () => {
		const { field } = renderComposer({ sentHistory: ["line one\nline two"] });
		await userEvent.keyboard("{ArrowUp}");
		expect(wireText(field)).toBe("line one\nline two");
	});

	it("keeps suggestion navigation priority while the menu is open", async () => {
		const { field } = renderComposer({ skills: SKILLS });
		await typeInLexicalEditor(field, "/");

		expect(screen.getByRole("listbox")).toBeInTheDocument();
		await userEvent.keyboard("{ArrowDown}");
		const selected = screen
			.getAllByRole("option")
			.findIndex((node) => node.getAttribute("aria-selected") === "true");
		expect(selected).toBe(1);
		expect(wireText(field)).toBe("/");
	});

	it("ignores arrow recall during IME composition", async () => {
		const { field } = renderComposer();
		fireEvent.keyDown(field, { key: "ArrowUp", isComposing: true });
		expect(wireText(field)).toBe("");
	});

	it("does not recall while editing a queued turn", async () => {
		const { field } = renderComposer({
			editingQueuedTurnId: "queued-1",
			draftSeed: { id: "queued-1", text: "queued edit" },
			onCancelQueuedEdit: vi.fn(),
		});
		await waitFor(() => expect(wireText(field)).toBe("queued edit"));
		await userEvent.keyboard("{ArrowUp}");
		expect(wireText(field)).toBe("queued edit");
	});

	it("typing after a recall leaves navigation and keeps the typed text", async () => {
		const { field } = renderComposer();
		await userEvent.keyboard("{ArrowUp}");
		expect(wireText(field)).toBe("third prompt");

		await typeInLexicalEditor(field, "!");
		expect(wireText(field)).toBe("third prompt!");
		// Navigation ended by typing: ArrowUp no longer recalls while text remains.
		await userEvent.keyboard("{ArrowUp}");
		expect(wireText(field)).toBe("third prompt!");
	});

	it("recalls while a turn is running and the composer will queue", async () => {
		const { field } = renderComposer({ willQueue: true });
		await userEvent.keyboard("{ArrowUp}");
		expect(wireText(field)).toBe("third prompt");
	});
});

describe("sentHistoryForSnapshot", () => {
	it("returns durable human prompts oldest-first", () => {
		const snapshot = snapshotWith([
			humanMessage("m1", 1, "first prompt"),
			humanMessage("m2", 2, "second prompt"),
		]);
		expect(sentHistoryForSnapshot(snapshot)).toEqual(["first prompt", "second prompt"]);
	});

	it("excludes assistant, automation, and steer activity rows", () => {
		const snapshot = snapshotWith([
			humanMessage("m1", 1, "human prompt"),
			{
				kind: "message",
				id: "a1",
				turnId: "turn-a1",
				sequence: 2,
				revision: 0,
				role: "assistant",
				origin: "provider",
				text: "agent answer",
				streaming: false,
				createdAt: "2026-08-26T06:00:02Z",
			},
			{
				kind: "activity",
				id: "s1",
				turnId: "turn-1",
				sequence: 3,
				revision: 0,
				activityKind: "system",
				status: "completed",
				summary: "steered",
				detail: { event: "steer", clientMessageId: "steer-1" },
				createdAt: "2026-08-26T06:00:03Z",
			},
		]);
		expect(sentHistoryForSnapshot(snapshot)).toEqual(["human prompt"]);
	});

	it("appends a renderer-only echo until its durable row arrives", () => {
		const snapshot = snapshotWith([humanMessage("m1", 1, "first prompt")]);
		const echo = {
			clientMessageId: "echo-1",
			text: "just sent",
			createdAt: "2026-08-26T06:00:09Z",
		};
		expect(sentHistoryForSnapshot(snapshot, [echo])).toEqual(["first prompt", "just sent"]);

		const reconciled = snapshotWith([
			humanMessage("m1", 1, "first prompt"),
			{ ...humanMessage("m2", 2, "just sent", "local:echo-1"), turnId: "local:echo-1", createdAt: "2026-08-26T06:00:09Z" },
		]);
		expect(
			sentHistoryForSnapshot(reconciled, [{ ...echo, turnId: "local:echo-1" }]),
		).toEqual(["first prompt", "just sent"]);
	});
});
