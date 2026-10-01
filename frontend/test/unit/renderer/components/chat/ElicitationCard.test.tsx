import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { openAgentsBridge } from "../../../../../src/renderer/lib/bridge";
import type { ConversationActivity } from "../../../../../src/renderer/types/conversation";
import { ElicitationCard } from "../../../../../src/renderer/components/chat/ElicitationCard";

function activity(
	detail: ConversationActivity["detail"],
	status: ConversationActivity["status"] = "pending",
): ConversationActivity {
	return {
		kind: "activity",
		id: "question-1",
		sequence: 1,
		revision: 0,
		activityKind: "user_input",
		status,
		summary: "Choose a direction",
		requestId: "request-1",
		detail,
		createdAt: "2026-08-04T00:00:00Z",
	};
}

describe("ElicitationCard", () => {
	const formQuestions = {
		type: "object" as const,
		required: ["question_0", "question_1"],
		properties: {
			question_0: {
				type: "string",
				title: "Approach",
				oneOf: [
					{ const: "Native", title: "Native", description: "Use ACP directly" },
					{ const: "Bridge", title: "Bridge" },
				],
			},
			question_0_custom: { type: "string", title: "Other approach" },
			question_1: {
				type: "string",
				title: "Language",
				oneOf: [
					{ const: "Go", title: "Go" },
					{ const: "TypeScript", title: "TypeScript" },
				],
			},
			question_1_custom: { type: "string", title: "Other language" },
		},
	};

	it("shows one form question and its Other field at a time", () => {
		render(
			<ElicitationCard
				activity={activity({ inputMode: "form", schema: formQuestions })}
				onResolve={vi.fn()}
			/>,
		);

		expect(screen.getByRole("group", { name: /Approach/ })).toBeInTheDocument();
		expect(screen.getByLabelText("Other approach")).toBeInTheDocument();
		expect(screen.queryByRole("group", { name: /Language/ })).not.toBeInTheDocument();
		expect(screen.queryByLabelText("Other language")).not.toBeInTheDocument();
	});

	it("validates the active form question before moving forward", async () => {
		const user = userEvent.setup();
		render(
			<ElicitationCard
				activity={activity({ inputMode: "form", schema: formQuestions })}
				onResolve={vi.fn()}
			/>,
		);

		await user.click(screen.getByRole("button", { name: "Next" }));

		expect(screen.getByText("Choose an answer.")).toBeInTheDocument();
		expect(screen.queryByRole("group", { name: /Language/ })).not.toBeInTheDocument();
	});

	it("navigates form questions and preserves answers when going back", async () => {
		const user = userEvent.setup();
		render(
			<ElicitationCard
				activity={activity({ inputMode: "form", schema: formQuestions })}
				onResolve={vi.fn()}
			/>,
		);

		await user.click(screen.getByRole("radio", { name: /Native/ }));
		await user.type(screen.getByLabelText("Other approach"), "Hybrid");
		await user.click(screen.getByRole("button", { name: "Next" }));
		expect(screen.getByRole("group", { name: /Language/ })).toBeInTheDocument();

		await user.click(screen.getByRole("button", { name: "Back" }));
		expect(screen.getByRole("radio", { name: /Native/ })).toBeChecked();
		expect(screen.getByLabelText("Other approach")).toHaveValue("Hybrid");
	});

	it("submits all form answers together from the final question", async () => {
		const user = userEvent.setup();
		const onResolve = vi.fn().mockResolvedValue(undefined);
		render(
			<ElicitationCard
				activity={activity({
					inputMode: "form",
					message: "Which implementation should we use?",
					schema: formQuestions,
				})}
				onResolve={onResolve}
			/>,
		);

		await user.click(screen.getByRole("radio", { name: /Native/ }));
		await user.type(screen.getByLabelText("Other approach"), "Hybrid");
		await user.click(screen.getByRole("button", { name: "Next" }));
		await user.click(screen.getByRole("radio", { name: "Go" }));
		await user.type(screen.getByLabelText("Other language"), "Rust");
		await user.click(screen.getByRole("button", { name: "Continue" }));
		expect(onResolve).toHaveBeenCalledWith("request-1", "accept", {
			question_0: "Native",
			question_0_custom: "Hybrid",
			question_1: "Go",
			question_1_custom: "Rust",
		});
	});

	it("keeps generic MCP forms in the all-fields layout", () => {
		render(
			<ElicitationCard
				activity={activity({
					inputMode: "form",
					schema: {
						type: "object",
						properties: {
							name: { type: "string", title: "Name" },
							team: { type: "string", title: "Team" },
						},
					},
				})}
				onResolve={vi.fn()}
			/>,
		);

		expect(screen.getByLabelText("Name")).toBeInTheDocument();
		expect(screen.getByLabelText("Team")).toBeInTheDocument();
		expect(screen.queryByRole("button", { name: "Next" })).not.toBeInTheDocument();
		expect(screen.getByRole("button", { name: "Continue" })).toBeInTheDocument();
	});

	it("keeps required fields actionable instead of sending an invalid form", async () => {
		const user = userEvent.setup();
		const onResolve = vi.fn();
		render(
			<ElicitationCard
				activity={activity({
					inputMode: "form",
					schema: {
						type: "object",
						required: ["name"],
						properties: { name: { type: "string", title: "Name" } },
					},
				})}
				onResolve={onResolve}
			/>,
		);
		await user.click(screen.getByRole("button", { name: "Continue" }));
		expect(screen.getByText("This field is required.")).toBeInTheDocument();
		expect(onResolve).not.toHaveBeenCalled();
	});

	it("opens an external URL only after the user explicitly consents", async () => {
		const user = userEvent.setup();
		const openExternal = vi.spyOn(openAgentsBridge.app, "openExternal").mockResolvedValue(undefined);
		const onResolve = vi.fn().mockResolvedValue(undefined);
		render(
			<ElicitationCard
				activity={activity({ inputMode: "url", url: "https://example.com/oauth", message: "Sign in" })}
				onResolve={onResolve}
			/>,
		);
		expect(openExternal).not.toHaveBeenCalled();
		expect(screen.getByText("https://example.com/oauth")).toBeInTheDocument();
		await user.click(screen.getByRole("button", { name: "Open example.com" }));
		expect(openExternal).toHaveBeenCalledWith("https://example.com/oauth");
		expect(onResolve).toHaveBeenCalledWith("request-1", "accept", undefined);
	});

	it("refuses unsafe URL schemes", () => {
		render(
			<ElicitationCard
				activity={activity({ inputMode: "url", url: "file:///Users/alice/.ssh/id_rsa" })}
				onResolve={vi.fn()}
			/>,
		);
		expect(screen.getByRole("alert")).toHaveTextContent(/unsafe or invalid URL/i);
		expect(screen.getByRole("button", { name: "Open link" })).toBeDisabled();
	});
});

describe("ElicitationCard ask menu", () => {
	const singleQuestion = {
		type: "object" as const,
		required: ["question_0"],
		properties: {
			question_0: {
				type: "string",
				title: "Approach",
				oneOf: [
					{ const: "Native", title: "Native", description: "Use ACP directly" },
					{ const: "Bridge", title: "Bridge" },
				],
			},
		},
	};

	it("renders options as menu entries with their descriptions", () => {
		render(
			<ElicitationCard
				activity={activity({ inputMode: "form", schema: singleQuestion })}
				onResolve={vi.fn()}
			/>,
		);

		expect(screen.getByRole("radiogroup", { name: "Approach" })).toBeInTheDocument();
		expect(screen.getByRole("radio", { name: /Native/ })).toBeInTheDocument();
		expect(screen.getByText("Use ACP directly")).toBeInTheDocument();
	});

	it("picks a menu entry with the keyboard and resolves on Continue", async () => {
		const user = userEvent.setup();
		const onResolve = vi.fn().mockResolvedValue(undefined);
		render(
			<ElicitationCard
				activity={activity({ inputMode: "form", schema: singleQuestion })}
				onResolve={onResolve}
			/>,
		);

		const native = screen.getByRole("radio", { name: /Native/ });
		native.focus();
		fireEvent.keyDown(native, { key: "ArrowDown" });
		expect(document.activeElement).toBe(screen.getByRole("radio", { name: "Bridge" }));
		fireEvent.keyDown(screen.getByRole("radio", { name: "Bridge" }), { key: "1" });
		expect(screen.getByRole("radio", { name: /Native/ })).toHaveAttribute("aria-checked", "true");

		await user.click(screen.getByRole("button", { name: "Continue" }));
		expect(onResolve).toHaveBeenCalledWith("request-1", "accept", { question_0: "Native" });
	});

	it("shows the answered state without menu entries", () => {
		render(
			<ElicitationCard
				activity={activity({ inputMode: "form", schema: singleQuestion }, "resolved")}
				onResolve={vi.fn()}
			/>,
		);

		expect(screen.getByText("Answered")).toBeInTheDocument();
		expect(screen.queryByRole("radiogroup")).not.toBeInTheDocument();
		expect(screen.queryByRole("button", { name: "Continue" })).not.toBeInTheDocument();
	});

	it("shows the expired state without menu entries", () => {
		render(
			<ElicitationCard
				activity={activity({ inputMode: "form", schema: singleQuestion }, "failed")}
				onResolve={vi.fn()}
			/>,
		);

		expect(screen.getByText("Expired")).toBeInTheDocument();
		expect(screen.queryByRole("radiogroup")).not.toBeInTheDocument();
	});

	it("says so when the request carries no questions", () => {
		render(
			<ElicitationCard
				activity={activity({ inputMode: "form", message: "Nothing to ask" })}
				onResolve={vi.fn()}
			/>,
		);

		expect(screen.getByText("No questions were offered for this request.")).toBeInTheDocument();
	});

	it("surfaces a resolve failure in place", async () => {
		const user = userEvent.setup();
		const onResolve = vi.fn().mockRejectedValue(new Error("The agent went away."));
		render(
			<ElicitationCard
				activity={activity({ inputMode: "form", schema: singleQuestion })}
				onResolve={onResolve}
			/>,
		);

		await user.click(screen.getByRole("radio", { name: /Native/ }));
		await user.click(screen.getByRole("button", { name: "Continue" }));
		expect(await screen.findByRole("alert")).toHaveTextContent("The agent went away.");
	});
});
