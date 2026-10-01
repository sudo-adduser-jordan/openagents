import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { AskMenu } from "./AskMenu";

const options = [
	{ value: "native", label: "Native", description: "Use ACP directly" },
	{ value: "bridge", label: "Bridge" },
];

describe("AskMenu", () => {
	it("renders the question options as menu entries", () => {
		render(
			<AskMenu name="question_0" label="Approach" options={options} value={undefined} onSelect={vi.fn()} />,
		);

		const group = screen.getByRole("radiogroup", { name: "Approach" });
		expect(group).toBeInTheDocument();
		expect(screen.getByRole("radio", { name: /Native/ })).toBeInTheDocument();
		expect(screen.getByRole("radio", { name: "Bridge" })).toBeInTheDocument();
		expect(screen.getByText("Use ACP directly")).toBeInTheDocument();
	});

	it("marks the chosen option", () => {
		render(
			<AskMenu name="question_0" label="Approach" options={options} value="native" onSelect={vi.fn()} />,
		);

		expect(screen.getByRole("radio", { name: /Native/ })).toHaveAttribute("aria-checked", "true");
		expect(screen.getByRole("radio", { name: "Bridge" })).toHaveAttribute("aria-checked", "false");
	});

	it("picks an option on click", async () => {
		const user = userEvent.setup();
		const onSelect = vi.fn();
		render(
			<AskMenu name="question_0" label="Approach" options={options} value={undefined} onSelect={onSelect} />,
		);

		await user.click(screen.getByRole("radio", { name: /Native/ }));
		expect(onSelect).toHaveBeenCalledWith("native");
	});

	it("moves focus with arrow keys like the composer menus", () => {
		render(
			<AskMenu name="question_0" label="Approach" options={options} value={undefined} onSelect={vi.fn()} />,
		);

		const first = screen.getByRole("radio", { name: /Native/ });
		const second = screen.getByRole("radio", { name: "Bridge" });
		first.focus();
		fireEvent.keyDown(first, { key: "ArrowDown" });
		expect(document.activeElement).toBe(second);
		fireEvent.keyDown(second, { key: "ArrowUp" });
		expect(document.activeElement).toBe(first);
	});

	it("quick-picks an option with its number key", () => {
		const onSelect = vi.fn();
		render(
			<AskMenu name="question_0" label="Approach" options={options} value={undefined} onSelect={onSelect} />,
		);

		fireEvent.keyDown(screen.getByRole("radio", { name: /Native/ }), { key: "2" });
		expect(onSelect).toHaveBeenCalledWith("bridge");
	});

	it("toggles values in multi-select mode", async () => {
		const user = userEvent.setup();
		const onSelect = vi.fn();
		render(
			<AskMenu
				name="question_0"
				label="Approach"
				options={options}
				value={["native"]}
				multi
				onSelect={onSelect}
			/>,
		);

		expect(screen.getByRole("checkbox", { name: /Native/ })).toHaveAttribute("aria-checked", "true");
		await user.click(screen.getByRole("checkbox", { name: "Bridge" }));
		expect(onSelect).toHaveBeenCalledWith("bridge");
	});

	it("disables every entry while an answer is sending", () => {
		render(
			<AskMenu name="question_0" label="Approach" options={options} value={undefined} disabled onSelect={vi.fn()} />,
		);

		expect(screen.getByRole("radio", { name: /Native/ })).toBeDisabled();
		expect(screen.getByRole("radio", { name: "Bridge" })).toBeDisabled();
	});

	it("says so when the provider offered nothing to pick", () => {
		render(
			<AskMenu name="question_0" label="Approach" options={[]} value={undefined} onSelect={vi.fn()} />,
		);

		expect(screen.getByText("No options were offered for this question.")).toBeInTheDocument();
		expect(screen.queryByRole("radio")).not.toBeInTheDocument();
	});
});
