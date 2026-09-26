import type React from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { ArchiveRemoveButton } from "./ArchiveRemoveButton";
import { TooltipProvider } from "./ui/tooltip";

function renderButton(node: React.ReactNode) {
	return render(<TooltipProvider>{node}</TooltipProvider>);
}

describe("ArchiveRemoveButton", () => {
	beforeEach(() => {
		vi.restoreAllMocks();
	});

	it("asks before removing, and removing is not the first click", () => {
		const onRemove = vi.fn();
		renderButton(<ArchiveRemoveButton isRemoving={false} label="old task" onRemove={onRemove} />);

		fireEvent.click(screen.getByRole("button", { name: "old task" }));
		expect(onRemove).not.toHaveBeenCalled();
		expect(screen.getByTestId("archive-remove-confirm")).toBeInTheDocument();
	});

	it("removes on confirmation", () => {
		const onRemove = vi.fn();
		renderButton(<ArchiveRemoveButton isRemoving={false} label="old task" onRemove={onRemove} />);

		fireEvent.click(screen.getByRole("button", { name: "old task" }));
		fireEvent.click(screen.getByRole("button", { name: "Confirm removing old task" }));
		expect(onRemove).toHaveBeenCalledTimes(1);
	});

	it("keeps the session when the confirmation is declined", () => {
		const onRemove = vi.fn();
		renderButton(<ArchiveRemoveButton isRemoving={false} label="old task" onRemove={onRemove} />);

		fireEvent.click(screen.getByRole("button", { name: "old task" }));
		fireEvent.click(screen.getByRole("button", { name: "Keep session" }));
		expect(onRemove).not.toHaveBeenCalled();
		expect(screen.queryByTestId("archive-remove-confirm")).not.toBeInTheDocument();
	});

	it("does not let a click on the confirmation reach the card", () => {
		const onRemove = vi.fn();
		const onCardClick = vi.fn();
		renderButton(
			<button onClick={onCardClick} type="button">
				<ArchiveRemoveButton isRemoving={false} label="old task" onRemove={onRemove} />
			</button>,
		);

		fireEvent.click(screen.getByRole("button", { name: "old task" }));
		fireEvent.click(screen.getByRole("button", { name: "Confirm removing old task" }));
		expect(onCardClick).not.toHaveBeenCalled();
	});

	it("shows the remove state while the request is in flight", () => {
		renderButton(<ArchiveRemoveButton isRemoving label="old task" onRemove={vi.fn()} />);
		expect(screen.getByRole("button", { name: "old task" })).toBeDisabled();
	});
});
