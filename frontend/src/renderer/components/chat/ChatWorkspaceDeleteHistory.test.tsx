/**
 * The per-turn manager history-deletion control and the honesty of its confirmation.
 *
 * Mirrors the rollback suite: ChatWorkspace takes props only, so these render it
 * directly with a fixture and a spy. What is asserted is behaviour a user would
 * notice: the control appears only for a manager with an idle agent, the
 * confirmation says the transcript is permanently removed while the agent keeps
 * its own context, and the turn id that reaches the daemon is the one clicked.
 */

import { render as rtlRender, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactElement } from "react";
import { describe, expect, it, vi } from "vitest";
import { ChatWorkspace } from "./ChatWorkspace";
import { chatFixture } from "../../lib/chat-fixture";
import type { ConversationSnapshot } from "../../types/conversation";
import { TooltipProvider } from "../ui/tooltip";

function render(ui: ReactElement) {
	return rtlRender(<TooltipProvider>{ui}</TooltipProvider>);
}

/** A conversation with nothing in flight, which is when a trim is offered. */
function idleSnapshot(): ConversationSnapshot {
	return {
		...chatFixture,
		controller: { state: "ready" },
		turns: chatFixture.turns.map((turn) =>
			turn.state === "running"
				? { ...turn, state: "completed" as const, completedAt: turn.requestedAt }
				: turn,
		),
	};
}

describe("ChatWorkspace delete history before", () => {
	it("offers a trim on each settled turn and reports the clicked turn", async () => {
		const onDeleteBefore = vi.fn();
		render(
			<ChatWorkspace
				snapshot={idleSnapshot()}
				sessionRole="manager"
				onDeleteBefore={onDeleteBefore}
			/>,
		);

		const controls = screen.getAllByRole("button", { name: "Delete history before here" });
		expect(controls.length).toBeGreaterThan(0);

		await userEvent.click(controls[0]!);
		const dialog = screen.getByRole("dialog");
		await userEvent.click(within(dialog).getByRole("button", { name: "Delete history" }));

		// The first settled turn in the fixture is turn-1; the daemon must be given
		// Open Agents's own turn id, which is what the snapshot exposes.
		expect(onDeleteBefore).toHaveBeenCalledWith("turn-1");
	});

	it("says the transcript is permanently removed while the agent keeps its context", async () => {
		render(
			<ChatWorkspace
				snapshot={idleSnapshot()}
				sessionRole="manager"
				onDeleteBefore={vi.fn()}
			/>,
		);
		await userEvent.click(
			screen.getAllByRole("button", { name: "Delete history before here" })[0]!,
		);

		const dialog = screen.getByRole("dialog");
		expect(dialog.textContent).toContain("permanently deleted");
		// The agent keeps whatever context it holds: only Open Agents's own record goes.
		expect(dialog.textContent).toContain("keeps whatever");
		expect(dialog.textContent).toContain("left exactly as they are");
		expect(dialog.textContent).toContain("cannot be recovered");
	});

	it("does not commit anything when the confirmation is cancelled", async () => {
		const onDeleteBefore = vi.fn();
		render(
			<ChatWorkspace
				snapshot={idleSnapshot()}
				sessionRole="manager"
				onDeleteBefore={onDeleteBefore}
			/>,
		);

		await userEvent.click(
			screen.getAllByRole("button", { name: "Delete history before here" })[0]!,
		);
		const dialog = screen.getByRole("dialog");
		await userEvent.click(within(dialog).getByRole("button", { name: "Cancel" }));

		expect(onDeleteBefore).not.toHaveBeenCalled();
	});

	// The daemon refuses a trim mid-turn. A control that exists only to be
	// refused is worse than one that waits for the agent to finish.
	it("withholds the control while a turn is in flight", () => {
		render(
			<ChatWorkspace
				snapshot={chatFixture}
				sessionRole="manager"
				onDeleteBefore={vi.fn()}
			/>,
		);
		expect(screen.queryByRole("button", { name: "Delete history before here" })).toBeNull();
	});

	it("draws no control when the callback is absent", () => {
		render(<ChatWorkspace snapshot={idleSnapshot()} sessionRole="manager" />);
		expect(screen.queryByRole("button", { name: "Delete history before here" })).toBeNull();
	});

	// A worker conversation is scoped to its own task: nothing worth trimming,
	// so the affordance is not drawn at all rather than shown and refused.
	it("draws no control for a worker session", () => {
		render(
			<ChatWorkspace
				snapshot={idleSnapshot()}
				sessionRole="worker"
				onDeleteBefore={vi.fn()}
			/>,
		);
		expect(screen.queryByRole("button", { name: "Delete history before here" })).toBeNull();
	});
});
