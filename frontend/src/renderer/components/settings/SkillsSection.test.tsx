import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { SkillsSection } from "./SkillsSection";

const { apiErrorCodeMock, apiErrorMessageMock, skillsHook } = vi.hoisted(() => ({
	apiErrorCodeMock: vi.fn(),
	apiErrorMessageMock: vi.fn(),
	skillsHook: {
		/** What the router reports; `undefined` is the "settings opened with no session" case. */
		params: { sessionId: "sess-1" as string | undefined },
		result: { skills: [] as unknown[], isLoading: false, error: null as unknown },
		calls: [] as { sessionId: string | undefined; enabled: boolean }[],
	},
}));

vi.mock("@tanstack/react-router", async (importOriginal) => {
	const actual = await importOriginal<typeof import("@tanstack/react-router")>();
	return {
		...actual,
		// The panel reads the active session from route state, and is rendered here
		// without a router.
		useParams: () => ({ ...skillsHook.params }),
	};
});

vi.mock("../../lib/api-client", () => ({
	apiErrorCode: (error: unknown) => apiErrorCodeMock(error),
	apiErrorMessage: (error: unknown, fallback?: string) => apiErrorMessageMock(error, fallback),
}));

vi.mock("../../hooks/useConversation", () => ({
	useConversationSkills: (sessionId: string | undefined, enabled: boolean) => {
		skillsHook.calls.push({ sessionId, enabled });
		return skillsHook.result;
	},
}));

const skill = (overrides: Record<string, unknown> = {}) => ({
	name: "review",
	displayName: "Review",
	description: "Look at the diff",
	inputHint: "<branch>",
	source: "repo",
	...overrides,
});

function setSkills(skills: unknown[], extra: { isLoading?: boolean; error?: unknown } = {}) {
	skillsHook.result = { skills, isLoading: false, error: null, ...extra };
}

function renderSection() {
	const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
	return render(
		<QueryClientProvider client={client}>
			<SkillsSection />
		</QueryClientProvider>,
	);
}

describe("SkillsSection", () => {
	beforeEach(() => {
		skillsHook.calls = [];
		skillsHook.params = { sessionId: "sess-1" };
		setSkills([]);
		apiErrorCodeMock.mockReset().mockReturnValue(undefined);
		apiErrorMessageMock.mockReset().mockImplementation((_error: unknown, fallback?: string) => fallback ?? "");
	});

	it("lists the open session's skills with their scope and arguments", () => {
		setSkills([
			skill(),
			// No label, description, or hint: the minimal shape a provider may send.
			skill({ name: "compact", displayName: "", description: "", inputHint: "", source: "agent" }),
		]);
		renderSection();

		expect(screen.getByTestId("settings-section")).toHaveAttribute("data-section", "skills");
		expect(screen.getByText("/Review")).toBeInTheDocument();
		expect(screen.getByText("Look at the diff")).toBeInTheDocument();
		expect(screen.getByText("<branch>")).toBeInTheDocument();
		expect(screen.getByText("repo")).toBeInTheDocument();
		// The generic agent scope says nothing, so it is dropped here exactly as the
		// composer's slash menu drops it.
		expect(screen.queryByText("agent")).not.toBeInTheDocument();
	});

	it("falls back to the invocable name when the provider sends no label", () => {
		setSkills([skill({ displayName: "" })]);
		renderSection();

		expect(screen.getByText("/review")).toBeInTheDocument();
	});

	it("says the catalog is loading instead of claiming there are none", () => {
		setSkills([], { isLoading: true });
		renderSection();

		expect(screen.getByRole("status")).toHaveTextContent("Loading skills…");
		expect(screen.queryByText("This session has no skills")).not.toBeInTheDocument();
	});

	it("reports an empty catalog as a real answer", () => {
		renderSection();

		expect(screen.getByText("This session has no skills")).toBeInTheDocument();
		expect(screen.queryByRole("alert")).not.toBeInTheDocument();
	});

	// The panel is reachable from the sidebar with nothing selected, and the
	// catalog is per-session. It has to ask for a session rather than report an
	// absence no provider confirmed, and it must not run the shared query keyed
	// on no session at all.
	it("asks for a session instead of listing one when nothing is selected", () => {
		skillsHook.params = { sessionId: undefined };
		// Even a stale in-flight read must not be shown as this session's catalog.
		setSkills([], { isLoading: true });
		renderSection();

		expect(screen.getByText("No session is open")).toBeInTheDocument();
		expect(screen.queryByText("This session has no skills")).not.toBeInTheDocument();
		expect(screen.queryByRole("status")).not.toBeInTheDocument();
		expect(screen.queryByRole("alert")).not.toBeInTheDocument();
		expect(skillsHook.calls).toEqual([{ sessionId: undefined, enabled: false }]);
	});

	// Distinct from "no session": the session exists but no live controller owns
	// it, so the daemon answers 409. That is a missing answer.
	it("distinguishes a session whose agent has not started from an empty catalog", () => {
		apiErrorCodeMock.mockReturnValue("CHAT_CONTROLLER_NOT_READY");
		setSkills([], { error: { code: "CHAT_CONTROLLER_NOT_READY" } });
		renderSection();

		expect(screen.getByText("This session's agent isn't running yet")).toBeInTheDocument();
		expect(screen.queryByText("This session has no skills")).not.toBeInTheDocument();
	});

	it("surfaces a failed read instead of an empty list", () => {
		apiErrorMessageMock.mockReturnValue("daemon refused the request");
		setSkills([], { error: { message: "boom" } });
		renderSection();

		expect(screen.getByRole("alert")).toHaveTextContent("daemon refused the request");
		expect(screen.queryByText("This session has no skills")).not.toBeInTheDocument();
	});

	// A poll that fails after a good read must not blank a list the user is
	// already reading.
	it("keeps the last catalog and notes the failed refresh", () => {
		apiErrorMessageMock.mockReturnValue("refresh failed");
		setSkills([skill()], { error: { message: "boom" } });
		renderSection();

		expect(screen.getByText("/Review")).toBeInTheDocument();
		expect(screen.getByRole("alert")).toHaveTextContent("refresh failed");
	});
});
