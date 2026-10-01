/**
 * The manager empty-state brand mark: the Open Agents logo above an empty
 * manager composer, hidden as soon as anything competes for that space.
 *
 * ChatWorkspace takes props only, so these render it directly with fixtures
 * and assert what a user would notice: the logo shows for an empty manager
 * chat, never for a worker, and disappears with history, a pending message, a
 * queued turn, or a status banner. The staged-attachment path is covered
 * through the composer's staged notifier, which is what hides the mark while
 * files wait to send.
 */

import { render as rtlRender, screen, waitFor } from "@testing-library/react";
import type { ReactElement } from "react";
import { describe, expect, it, vi } from "vitest";
import { ChatWorkspace } from "../../../../../src/renderer/components/chat/ChatWorkspace";
import { ChatComposer } from "../../../../../src/renderer/components/chat/ChatComposer";
import {
	chatFixtureEmpty,
	chatFixtureSettled,
} from "../../../../../src/renderer/lib/chat-fixture";
import type { ConversationSnapshot } from "../../../../../src/renderer/types/conversation";
import { TooltipProvider } from "../../../../../src/renderer/components/ui/tooltip";

function render(ui: ReactElement) {
	return rtlRender(<TooltipProvider>{ui}</TooltipProvider>);
}

describe("ChatWorkspace manager empty brand", () => {
	it("shows the logo above an empty manager composer", () => {
		render(<ChatWorkspace snapshot={chatFixtureEmpty} sessionRole="manager" />);

		const brand = screen.getByTestId("manager-empty-brand");
		expect(brand).toBeInTheDocument();
		expect(brand.querySelector("img")).toHaveAttribute("src", expect.stringContaining("open-agents-logo"));
	});

	it("keeps the logo out of worker chat, even when empty", () => {
		render(<ChatWorkspace snapshot={chatFixtureEmpty} sessionRole="worker" />);

		expect(screen.queryByTestId("manager-empty-brand")).toBeNull();
	});

	it("hides the logo once the manager has history", () => {
		render(<ChatWorkspace snapshot={chatFixtureSettled} sessionRole="manager" />);

		expect(screen.queryByTestId("manager-empty-brand")).toBeNull();
	});

	it("hides the logo while a sent message is still pending", () => {
		render(
			<ChatWorkspace
				snapshot={chatFixtureEmpty}
				sessionRole="manager"
				localEchos={[
					{
						clientMessageId: "local-send",
						text: "Still on its way",
						createdAt: "2026-09-09T00:00:00Z",
					},
				]}
			/>,
		);

		expect(screen.queryByTestId("manager-empty-brand")).toBeNull();
	});

	it("hides the logo while a turn waits in the queue", () => {
		const queued: ConversationSnapshot = {
			...chatFixtureEmpty,
			turns: [{ id: "turn-q1", state: "queued", requestedAt: "2026-09-09T00:00:00Z" }],
			items: [
				{
					kind: "message",
					id: "queued-message-1",
					turnId: "turn-q1",
					sequence: 1,
					revision: 0,
					role: "user",
					origin: "human",
					text: "Runs after the current turn",
					streaming: false,
					createdAt: "2026-09-09T00:00:00Z",
				},
			],
		};
		render(<ChatWorkspace snapshot={queued} sessionRole="manager" />);

		expect(screen.queryByTestId("manager-empty-brand")).toBeNull();
	});

	it("hides the logo while a status banner owns the centered space", () => {
		const bannered: ConversationSnapshot = {
			...chatFixtureEmpty,
			threadState: { status: "system_error" },
		};
		render(<ChatWorkspace snapshot={bannered} sessionRole="manager" />);

		expect(screen.queryByTestId("manager-empty-brand")).toBeNull();
	});
});

describe("ChatComposer staged notifier", () => {
	it("reports staged attachments so the empty brand can hide", async () => {
		const onStagedChange = vi.fn();
		render(
			<ChatComposer
				onSend={vi.fn()}
				draftSeed={{
					id: "seed-1",
					text: "",
					stagedAttachments: [
						{
							id: "attachment-1",
							path: ".open-agents/attachments/attachment-1.png",
							name: "attachment-1.png",
							mimeType: "image/png",
							bytes: 12,
						},
					],
				}}
				onStagedChange={onStagedChange}
			/>,
		);

		await waitFor(() => expect(onStagedChange).toHaveBeenCalledWith(true));
	});
});
