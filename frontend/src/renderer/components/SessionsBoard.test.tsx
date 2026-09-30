import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { WorkspaceSession, WorkspaceSummary } from "../types/workspace";
import { toKanbanColumn } from "@openagents/product-ui";

// Instant motion updates so height tweens do not leave tests waiting on timers.
vi.mock("motion/react", async (importOriginal) => {
	const actual = await importOriginal<typeof import("motion/react")>();
	return {
		...actual,
		AnimatePresence: ({ children }: { children: React.ReactNode }) => children,
	};
});

const {
	navigateMock,
	notificationShowMock,
	openExternalMock,
	deleteMock,
	getMock,
	patchMock,
	postMock,
	workspaceQueryMock,
	usageQueryMock,
	boardActionsInPanelMock,
} = vi.hoisted(() => ({
	navigateMock: vi.fn(),
	notificationShowMock: vi.fn(),
	openExternalMock: vi.fn(),
	deleteMock: vi.fn(),
	getMock: vi.fn(),
	patchMock: vi.fn(),
	postMock: vi.fn(),
	workspaceQueryMock: vi.fn(),
	usageQueryMock: vi.fn(),
	boardActionsInPanelMock: vi.fn(() => false),
}));

vi.mock("@tanstack/react-router", () => ({
	useNavigate: () => navigateMock,
}));

vi.mock("../hooks/useWorkspaceQuery", () => ({
	workspaceQueryKey: ["workspaces"],
	cloudSessionsQueryKey: ["cloud-sessions"],
	useWorkspaceQuery: workspaceQueryMock,
	useWorkspaceScope: (projectId?: string) => {
		const query = workspaceQueryMock();
		return {
			...query,
			data: { project: query.data?.find((workspace: WorkspaceSummary) => workspace.id === projectId) },
		};
	},
}));

vi.mock("../hooks/useSessionUsageSummaries", () => ({
	useSessionUsageSummaries: usageQueryMock,
}));

vi.mock("../lib/api-client", () => ({
	apiClient: {
		GET: (...args: unknown[]) => getMock(...args),
		PATCH: (...args: unknown[]) => patchMock(...args),
		POST: (...args: unknown[]) => postMock(...args),
		DELETE: (...args: unknown[]) => deleteMock(...args),
	},
	apiErrorCode: (error: unknown) => (error as { code?: string } | undefined)?.code,
	apiErrorMessage: (_error: unknown, fallback: string) => fallback,
}));

vi.mock("../lib/bridge", () => ({
	openAgentsBridge: {
		app: {
			openExternal: (...args: unknown[]) => openExternalMock(...args),
		},
		clipboard: {
			writeText: vi.fn(),
		},
		notifications: {
			show: (...args: unknown[]) => notificationShowMock(...args),
		},
	},
}));

vi.mock("../lib/platform", async (importOriginal) => {
	const actual = await importOriginal<typeof import("../lib/platform")>();
	return {
		...actual,
		usesBoardActionsInPanel: () => boardActionsInPanelMock(),
		isLinuxPlatform: () => false,
	};
});

import { archiveToggleHeightClassName, archiveToggleOffsetClassName } from "@openagents/product-ui";
import { SessionsBoard } from "./SessionsBoard";
import { toBoardSessionPresentation } from "./SessionsBoardAdapters";
import { TooltipProvider } from "./ui/tooltip";

function renderBoard(projectId?: string) {
	const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
	renderBoardWithClient(queryClient, projectId);
	return queryClient;
}

function renderBoardWithClient(queryClient: QueryClient, projectId?: string) {
	return render(
		<QueryClientProvider client={queryClient}>
			<TooltipProvider>
				<SessionsBoard projectId={projectId} />
			</TooltipProvider>
		</QueryClientProvider>,
	);
}

/** Archive cards mount on the next frame via startTransition — wait for the list. */
async function expandArchive() {
	await userEvent.click(screen.getByRole("button", { name: /archive/i }));
	return screen.findByRole("list", { name: "Archived sessions" });
}

/**
 * The archive header's button and the confirm dialog's button share a name, so
 * the confirm is always clicked from inside the dialog.
 */
async function confirmClearArchive() {
	await userEvent.click(await screen.findByRole("button", { name: "Clear archive" }));
	const dialog = await screen.findByRole("dialog");
	await userEvent.click(within(dialog).getByRole("button", { name: "Clear archive" }));
}

beforeEach(() => {
	navigateMock.mockReset();
	notificationShowMock.mockReset().mockResolvedValue(undefined);
	openExternalMock.mockReset().mockResolvedValue(undefined);
	getMock.mockReset().mockResolvedValue({ data: {} });
	patchMock.mockReset().mockResolvedValue({ data: {} });
	postMock.mockReset().mockResolvedValue({ data: {} });
	deleteMock.mockReset().mockResolvedValue({ data: { ok: true, freed: true } });
	workspaceQueryMock.mockReset().mockReturnValue({ data: [], isError: false });
	usageQueryMock.mockReset().mockReturnValue({ data: new Map() });
	window.localStorage.removeItem("open-agents.board.archive.layout");
	boardActionsInPanelMock.mockReset().mockReturnValue(false);
});

describe("SessionsBoard", () => {
	it("uses the last human message time rather than generic session updatedAt", () => {
		const presentation = toBoardSessionPresentation(
			boardSession({
				id: "timestamp-session",
				lastUserMessageAt: "2026-01-01T09:00:00Z",
				status: "idle",
				title: "timestamp task",
				updatedAt: "2026-01-01T10:00:00Z",
			}),
		);

		expect(presentation.lastUserMessageAt).toBe("2026-01-01T09:00:00Z");
	});

	it("renders dynamic card actions and pull request lifecycle labels", async () => {
		workspaceQueryMock.mockReturnValue({
			data: [
				workspaceWithSessions([
					boardSession({
						id: "s-localized",
						title: "localized worker",
						status: "pr_open",
						prs: [
							{
								url: "https://github.com/acme/repo/pull/42",
								number: 42,
								state: "open",
								ci: "passing",
								review: "approved",
								mergeability: "mergeable",
								reviewComments: false,
								updatedAt: "2026-01-01T00:00:00Z",
							},
						],
					}),
				]),
			],
			isError: false,
			isSuccess: true,
		});

		renderBoard("p1");
		expect(screen.getByRole("button", { name: "Terminate localized worker" })).toBeInTheDocument();
		expect(screen.getByRole("link", { name: "PR #42 open" })).toHaveAttribute(
			"href",
			"https://github.com/acme/repo/pull/42",
		);
	});

	it("does not show an agent setup warning on the board", () => {
		renderBoard();

		expect(screen.queryByText(/reload agents/i)).not.toBeInTheDocument();
	});

	it("shows the Board identity and compact actions in the in-panel board chrome", () => {
		boardActionsInPanelMock.mockReturnValue(true);
		workspaceQueryMock.mockReturnValue({
			data: [
				{
					id: "p1",
					name: "solkit-ui",
					path: "/tmp/solkit-ui",
					sessions: [
						{
							id: "s1",
							workspaceId: "p1",
							workspaceName: "solkit-ui",
							title: "test",
							provider: "opencode",
							branch: "open-agents/dev/solkit-ui-5/root",
							status: "running",
							activity: { state: "working", lastActivityAt: "2026-01-01T00:00:00Z" },
							updatedAt: "2026-01-01T00:00:00Z",
							prs: [],
						},
					],
				},
			],
			isError: false,
			isSuccess: true,
		});

		renderBoard("p1");

		expect(screen.getByTestId("board-topbar-label").textContent).toContain("Board");
		expect(screen.queryByText("solkit-ui")).toBeNull();
		expect(screen.getByRole("button", { name: "New task" }).closest(".center-panel-titlebar")).toHaveClass(
			"workspace-topbar-container",
		);
		expect(
			within(screen.getByRole("button", { name: "New task" })).getByText("Task").hasAttribute("data-compact-label"),
		).toBe(true);
	});

	it.each([
		["active", "Working", "bg-status-working", true],
		["idle", "Idle", "bg-status-idle", false],
	] as const)("shows %s manager activity in the in-panel board toolbar", (state, label, tone, pulses) => {
		boardActionsInPanelMock.mockReturnValue(true);
		workspaceQueryMock.mockReturnValue({
			data: [
				{
					id: "p1",
					name: "solkit-ui",
					path: "/tmp/solkit-ui",
					sessions: [
						{
							id: "mgr-1",
							workspaceId: "p1",
							workspaceName: "solkit-ui",
							title: "manager",
							provider: "opencode",
							kind: "manager",
							branch: "main",
							status: "working",
							activity: { state, lastActivityAt: "2026-01-01T00:00:00Z" },
							updatedAt: "2026-01-01T00:00:00Z",
							prs: [],
						},
					],
				},
			],
			isError: false,
			isSuccess: true,
		});

		renderBoard("p1");

		const button = screen.getByRole("button", { name: `Manager, ${label}` });
		const indicator = button.querySelector("span.size-dot-sm") as HTMLElement;
		expect(within(button).getByText("Manager").hasAttribute("data-compact-label")).toBe(true);
		expect(indicator).toHaveAttribute("aria-hidden", "true");
		expect(indicator).toHaveClass(tone);
		expect(indicator).toHaveClass(pulses ? "animate-status-pulse" : "size-dot-sm");
		if (!pulses) expect(indicator).not.toHaveClass("animate-status-pulse");
	});

	it("shows the Board crumb on the root board when actions live in the panel", () => {
		boardActionsInPanelMock.mockReturnValue(true);
		workspaceQueryMock.mockReturnValue({
			data: [
				{
					id: "p1",
					name: "solkit-ui",
					path: "/tmp/solkit-ui",
					sessions: [],
				},
			],
			isError: false,
			isSuccess: true,
		});

		renderBoard();

		expect(screen.getByText("Board")).toBeInTheDocument();
	});

	it("labels an idle session as Idle, not Working", () => {
		workspaceQueryMock.mockReturnValue({
			data: [
				{
					id: "p1",
					name: "radic",
					path: "/tmp/radic",
					sessions: [
						{
							id: "s1",
							workspaceId: "p1",
							workspaceName: "radic",
							title: "brand-font-pipeline",
							provider: "opencode",
							branch: "open-agents/radic-5",
							status: "idle",
							activity: { state: "idle", lastActivityAt: "2026-01-01T00:00:00Z" },
							updatedAt: "2026-01-01T00:00:00Z",
							prs: [],
						},
					],
				},
			],
			isError: false,
		});

		renderBoard("p1");

		const idleCard = screen
			.getByText("brand-font-pipeline")
			.closest('[data-testid="board-session-card"]') as HTMLElement;
		expect(within(idleCard).getByText("Idle")).toBeInTheDocument();
		const terminateButton = within(idleCard).getByRole("button", { name: "Terminate brand-font-pipeline" });
		expect(terminateButton).toHaveClass("opacity-0", "group-hover:opacity-100", "group-focus-within:opacity-100");
		expect(terminateButton.querySelector("svg")).toHaveClass("lucide-trash-2");
		expect(within(idleCard).getByText("Idle").parentElement?.parentElement).toHaveClass("flex");
		expect(within(idleCard).getByText("brand-font-pipeline")).toHaveClass("font-semibold", "line-clamp-2");
	});

	it("shows token counts on active and archived cards", async () => {
		workspaceQueryMock.mockReturnValue({
			data: [
				workspaceWithSessions([
					boardSession({ id: "s-active", title: "active worker", status: "idle" }),
					boardSession({ id: "s-empty", title: "empty worker", status: "idle" }),
					boardSession({ id: "s-tokens", title: "tokens worker", status: "idle" }),
					terminatedSession(),
				]),
			],
			isError: false,
			isSuccess: true,
		});
		usageQueryMock.mockReturnValue({
			data: new Map([
				[
					"s-active",
					{
						sessionId: "s-active",
						processedTokens: 12_300,
						totalTokens: 12_400,
						incomplete: false,
					},
				],
				[
					"s-empty",
					{
						sessionId: "s-empty",
						processedTokens: 0,
						totalTokens: 0,
						incomplete: false,
					},
				],
				[
					"s-tokens",
					{
						sessionId: "s-tokens",
						processedTokens: 800,
						totalTokens: 800,
						incomplete: false,
					},
				],
				[
					"s-dead",
					{
						sessionId: "s-dead",
						processedTokens: 1_900,
						totalTokens: 2_000,
						incomplete: true,
					},
				],
			]),
		});

		renderBoard("p1");

		// The card shows the token count; the full count remains in the
		// hover tooltip and accessible label.
		const activeUsage = screen.getByText("12.3K", { selector: "span" });
		expect(activeUsage).toHaveAttribute("aria-hidden", "true");
		expect(screen.getByText("12,300 tokens")).toHaveClass("sr-only");
		expect(screen.queryByText(/processed/i)).not.toBeInTheDocument();
		// Sessions with token usage stay visible; sessions with no tokens
		// still show nothing.
		const emptyCard = screen.getByText("empty worker").closest('[data-testid="board-session-card"]') as HTMLElement;
		expect(within(emptyCard).queryByText("0 tokens")).not.toBeInTheDocument();
		const tokensOnlyCard = screen.getByText("tokens worker").closest('[data-testid="board-session-card"]') as HTMLElement;
		expect(within(tokensOnlyCard).getByText("800", { selector: "span" })).toHaveAttribute("aria-hidden", "true");
		expect(within(tokensOnlyCard).getByText("800 tokens")).toHaveClass("sr-only");
		expect(usageQueryMock).toHaveBeenCalledWith("p1");

		const archive = await expandArchive();
		expect(within(archive).getByText("1.9K")).toHaveAttribute("aria-hidden", "true");
		expect(within(archive).getByText("1,900 tokens")).toHaveClass("sr-only");
	});

	it("shows tokens by default and full count on hover without a tab stop", async () => {
		workspaceQueryMock.mockReturnValue({
			data: [
				workspaceWithSessions([
					boardSession({ id: "s-keyboard", title: "keyboard worker", status: "idle" }),
				]),
			],
			isError: false,
			isSuccess: true,
		});
		usageQueryMock.mockReturnValue({
			data: new Map([
				[
					"s-keyboard",
					{
						incomplete: false,
						sessionId: "s-keyboard",
						processedTokens: 12_400,
						totalTokens: 12_400,
					},
				],
			]),
		});

		renderBoard("p1");

		const card = screen.getByText("keyboard worker").closest('[data-testid="board-session-card"]') as HTMLElement;
		const usage = within(card).getByText("12.4K", { selector: "span" });
		expect(usage.tagName).toBe("SPAN");
		// The compact text is decorative; the full label is real off-screen text
		// rather than an aria-label on a generic span, which is not reliably
		// exposed. The hover trigger is not a tab stop.
		expect(usage).toHaveAttribute("aria-hidden", "true");
		expect(within(card).getByText("12,400 tokens")).toHaveClass("sr-only");

		within(card).getByRole("button", { name: "keyboard worker" }).focus();
		await userEvent.tab();
		expect(within(card).getByRole("button", { name: "Terminate keyboard worker" })).toHaveFocus();

		await userEvent.hover(usage);
		expect(await screen.findByRole("tooltip")).toHaveTextContent("12,400 tokens");
	});

	it("shows token usage on cards", async () => {
		workspaceQueryMock.mockReturnValue({
			data: [
				workspaceWithSessions([
					boardSession({ id: "s-tokens", title: "tokens worker", status: "idle" }),
				]),
			],
			isError: false,
			isSuccess: true,
		});
		usageQueryMock.mockReturnValue({
			data: new Map([
				[
					"s-tokens",
					{
						incomplete: false,
						sessionId: "s-tokens",
						processedTokens: 12_400,
						totalTokens: 12_400,
					},
				],
			]),
		});

		renderBoard("p1");

		const card = screen.getByText("tokens worker").closest('[data-testid="board-session-card"]') as HTMLElement;
		const usage = within(card).getByText("12.4K", { selector: "span" });
		expect(usage).toHaveAttribute("aria-hidden", "true");
		expect(within(card).getByText("12,400 tokens")).toHaveClass("sr-only");

		await userEvent.hover(usage);
		expect(await screen.findByRole("tooltip")).toHaveTextContent("12,400 tokens");
	});

	it("styles a working card from its building lane without inferring from runtime activity", () => {
		workspaceQueryMock.mockReturnValue({
			data: [
				workspaceWithSessions([
					boardSession({
						id: "s-active",
						title: "active-card-task",
						status: "working",
						activity: { state: "active", lastActivityAt: "2026-01-01T00:00:00Z" },
					}),
				]),
			],
			isError: false,
			isSuccess: true,
		});

		renderBoard("p1");
		const card = screen.getByText("active-card-task").closest('[data-testid="board-session-card"]') as HTMLElement;
		const working = within(card).getByText("Working").parentElement as HTMLElement;
		expect(working).toHaveAttribute("data-kanban-column", "building");
		expect(working).toHaveClass("text-status-working");
		expect(working.style.getPropertyValue("--session-status-tone")).toBe("");
		expect(working.querySelector('[aria-hidden="true"]')).toHaveClass("animate-spin");
	});

	// A multi-PR session aggregates `status` from its worst open PR while
	// `displayStatus` comes from its best one, so a settled "Mergeable" card can
	// carry status `review_pending`. The loader must follow the label the card
	// actually shows, not the hidden aggregate, or an idle session spins forever.
	it("does not spin a settled Mergeable card whose worst PR aggregates to review_pending", () => {
		workspaceQueryMock.mockReturnValue({
			data: [
				workspaceWithSessions([
					boardSession({
						id: "s-mergeable",
						title: "mergeable-card-task",
						status: "review_pending",
						displayStatus: "Mergeable",
						kanbanColumn: "ready",
						activity: { state: "idle", lastActivityAt: "2026-01-01T00:00:00Z" },
					}),
				]),
			],
			isError: false,
			isSuccess: true,
		});

		renderBoard("p1");
		const card = screen.getByText("mergeable-card-task").closest('[data-testid="board-session-card"]') as HTMLElement;
		const status = within(card).getByTestId("session-status");
		expect(status).toHaveTextContent("Mergeable");
		expect(status.querySelector(".animate-spin")).toBeNull();
	});

	it("shows review actions beside the pending label without a loader", () => {
		workspaceQueryMock.mockReturnValue({
			data: [
				workspaceWithSessions([
					boardSession({
						id: "s-review-pending",
						title: "review-pending-card-task",
						status: "review_pending",
						displayStatus: "Review pending",
						kanbanColumn: "needs_review",
						activity: { state: "idle", lastActivityAt: "2026-01-01T00:00:00Z" },
					}),
				]),
			],
			isError: false,
			isSuccess: true,
		});

		renderBoard("p1");
		const card = screen.getByText("review-pending-card-task").closest('[data-testid="board-session-card"]') as HTMLElement;
		const status = within(card).getByTestId("session-status");
		expect(status).toHaveTextContent("Review pending");
		// Delivery action rows carry buttons, not the progress spinner: the
		// daemon phrase stays, the loader does not.
		expect(status.querySelector(".animate-spin")).toBeNull();
		expect(within(status).getByRole("button", { name: "Commit" })).toBeInTheDocument();
	});

	it("shows delivery actions beside the mergeable label while its agent is working", () => {
		workspaceQueryMock.mockReturnValue({
			data: [
				workspaceWithSessions([
					boardSession({
						id: "s-mergeable-active",
						title: "mergeable-active-task",
						status: "working",
						displayStatus: "Mergeable",
						kanbanColumn: "ready",
						activity: { state: "active", lastActivityAt: "2026-01-01T00:00:00Z" },
					}),
				]),
			],
			isError: false,
			isSuccess: true,
		});

		renderBoard("p1");
		const card = screen.getByText("mergeable-active-task").closest('[data-testid="board-session-card"]') as HTMLElement;
		const status = within(card).getByTestId("session-status");
		// A Ready card shows delivery actions beside the daemon phrase; the
		// working spinner does not follow it into the action row.
		expect(status).toHaveTextContent("Mergeable");
		expect(status.querySelector(".animate-spin")).toBeNull();
		expect(within(status).getByRole("button", { name: "Merge local" })).toBeInTheDocument();
	});

	it("paints Closed without merge red while keeping merged status purple", () => {
		workspaceQueryMock.mockReturnValue({
			data: [
				workspaceWithSessions([
					boardSession({
						id: "s-closed-without-merge",
						title: "closed-without-merge-task",
						status: "idle",
						displayStatus: "Closed without merge",
						kanbanColumn: "ready",
					}),
				]),
			],
			isError: false,
			isSuccess: true,
		});

		renderBoard("p1");
		const card = screen.getByText("closed-without-merge-task").closest('[data-testid="board-session-card"]') as HTMLElement;
		const status = within(card).getByTestId("session-status");
		expect(status).toHaveTextContent("Closed without merge");
		// The phrase moved into the delivery action row with the buttons; the
		// red tone moves with it.
		const label = within(status).getByText("Closed without merge");
		expect(label).toHaveClass("text-status-exited");
		expect(label).not.toHaveClass("text-status-ready", "text-status-merged");
	});

	it("keeps a spawning card labeled Working when raw activity has not become active", () => {
		workspaceQueryMock.mockReturnValue({
			data: [
				workspaceWithSessions([
					boardSession({
						id: "s-spawning",
						title: "spawning-card-task",
						status: "working",
						activity: { state: "exited", lastActivityAt: "2026-01-01T00:00:00Z" },
					}),
				]),
			],
			isError: false,
			isSuccess: true,
		});

		renderBoard("p1");
		const card = screen.getByText("spawning-card-task").closest('[data-testid="board-session-card"]') as HTMLElement;
		expect(within(card).getByText("Working")).toBeInTheDocument();
		expect(within(card).queryByText("Exited")).not.toBeInTheDocument();
	});

	it("styles legacy statuses from their status-implied board lanes", () => {
		workspaceQueryMock.mockReturnValue({
			data: [
				{
					id: "p1",
					name: "radic",
					path: "/tmp/radic",
					sessions: [
						{
							id: "s0",
							workspaceId: "p1",
							workspaceName: "radic",
							title: "idle-card-task",
							provider: "opencode",
							branch: "open-agents/radic-5",
							status: "idle",
							activity: { state: "idle", lastActivityAt: "2026-01-01T00:00:00Z" },
							updatedAt: "2026-01-01T00:00:00Z",
							prs: [],
						},
						{
							id: "s1",
							workspaceId: "p1",
							workspaceName: "radic",
							title: "no-signal-card-task",
							provider: "opencode",
							branch: "open-agents/radic-6",
							status: "no_signal",
							activity: { state: "idle", lastActivityAt: "2026-01-01T00:00:00Z" },
							updatedAt: "2026-01-01T00:00:00Z",
							prs: [],
						},
						{
							id: "s2",
							workspaceId: "p1",
							workspaceName: "radic",
							title: "draft-card-task",
							provider: "opencode",
							branch: "open-agents/radic-7",
							status: "draft",
							activity: { state: "idle", lastActivityAt: "2026-01-01T00:00:00Z" },
							updatedAt: "2026-01-01T00:00:00Z",
							prs: [],
						},
					],
				},
			],
			isError: false,
		});

		renderBoard("p1");
		const idleCard = screen.getByText("idle-card-task").closest('[data-testid="board-session-card"]') as HTMLElement;
		const noSignalCard = screen.getByText("no-signal-card-task").closest('[data-testid="board-session-card"]') as HTMLElement;
		const draftCard = screen.getByText("draft-card-task").closest('[data-testid="board-session-card"]') as HTMLElement;

		expect(within(idleCard).getByText("Idle").parentElement).toHaveAttribute(
			"data-kanban-column",
			"building",
		);
		// Review-lane legacy cards render their status phrase inside the
		// delivery action row, so the lane marker sits on the phrase itself.
		expect(within(noSignalCard).getByText("No signal")).toHaveAttribute(
			"data-kanban-column",
			"review",
		);
		expect(within(draftCard).getByText("Draft PR")).toHaveAttribute(
			"data-kanban-column",
			"review",
		);
	});

	it("keeps a PR-less exited session in the building lane with an Exited badge", () => {
		workspaceQueryMock.mockReturnValue({
			data: [
				workspaceWithSessions([
					{
						id: "s-exited",
						workspaceId: "p1",
						workspaceName: "radic",
						title: "agent-exited-task",
						provider: "opencode",
						branch: "open-agents/exited",
						status: "exited",
						// What the daemon derives for a worker with no PR, whatever its
						// runtime status. Set explicitly so this covers the daemon path,
						// not the older-daemon fallback.
						kanbanColumn: "building",
						activity: { state: "exited", lastActivityAt: "2026-01-01T00:00:00Z" },
						updatedAt: "2026-01-01T00:00:00Z",
						prs: [],
					},
				]),
			],
			isError: false,
			isSuccess: true,
		});

		renderBoard("p1");

		// Lanes follow the daemon's column: a worker with no PR is still building,
		// whatever its runtime status. The card keeps its Exited badge.
		const buildingColumn = screen.getByLabelText("Building sessions");
		expect(within(buildingColumn).getByText("agent-exited-task")).toBeInTheDocument();
		expect(within(buildingColumn).getByText("Exited").parentElement).toHaveAttribute(
			"data-kanban-column",
			"building",
		);
	});

	it("swaps the building lane's cards when navigating between project boards", () => {
		const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
		workspaceQueryMock.mockReturnValue({
			data: [
				{
					id: "p1",
					name: "radic",
					path: "/tmp/radic",
					sessions: [
						{
							id: "p1-active",
							workspaceId: "p1",
							workspaceName: "radic",
							title: "p1 active",
							provider: "opencode",
							branch: "open-agents/radic-active",
							status: "working",
							activity: { state: "active", lastActivityAt: "2026-01-01T00:00:00Z" },
							updatedAt: "2026-01-01T00:00:00Z",
							prs: [],
						},
						{
							id: "p1-idle",
							workspaceId: "p1",
							workspaceName: "radic",
							title: "p1 idle",
							provider: "opencode",
							branch: "open-agents/radic-idle",
							status: "idle",
							activity: { state: "idle", lastActivityAt: "2026-01-01T00:00:00Z" },
							updatedAt: "2026-01-01T00:00:00Z",
							prs: [],
						},
					],
				},
				{
					id: "p2",
					name: "other",
					path: "/tmp/other",
					sessions: [
						{
							id: "p2-active",
							workspaceId: "p2",
							workspaceName: "other",
							title: "p2 active",
							provider: "opencode",
							branch: "open-agents/other-active",
							status: "working",
							activity: { state: "active", lastActivityAt: "2026-01-01T00:00:00Z" },
							updatedAt: "2026-01-01T00:00:00Z",
							prs: [],
						},
						{
							id: "p2-idle",
							workspaceId: "p2",
							workspaceName: "other",
							title: "p2 idle",
							provider: "opencode",
							branch: "open-agents/other-idle",
							status: "idle",
							activity: { state: "idle", lastActivityAt: "2026-01-01T00:00:00Z" },
							updatedAt: "2026-01-01T00:00:00Z",
							prs: [],
						},
					],
				},
			],
			isError: false,
		});
		const view = renderBoardWithClient(queryClient, "p1");

		const p1Lane = screen.getByRole("region", { name: "Building sessions" });
		expect(p1Lane).toHaveTextContent("p1 idle");
		expect(p1Lane).toHaveTextContent("p1 active");

		view.rerender(
			<QueryClientProvider client={queryClient}>
				<TooltipProvider>
					<SessionsBoard projectId="p2" />
				</TooltipProvider>
			</QueryClientProvider>,
		);

		const p2Lane = screen.getByRole("region", { name: "Building sessions" });
		expect(screen.queryByText("p1 idle")).not.toBeInTheDocument();
		expect(p2Lane).toHaveTextContent("p2 idle");
		expect(p2Lane).toHaveTextContent("p2 active");
	});

	it("shows a static archive card with a persistent restore action", async () => {
		const archivedSession = terminatedSession();
		const mergedPr = archivedSession.prs[0];
		if (!mergedPr) throw new Error("Archived-session fixture requires a pull request");
		archivedSession.prs = [
			{
				...mergedPr,
				number: 41,
				state: "open",
				url: "https://github.com/example/radic/pull/41",
			},
			mergedPr,
		];
		workspaceQueryMock.mockReturnValue({
			data: [workspaceWithSessions([archivedSession])],
			isError: false,
			isSuccess: true,
		});

		renderBoard("p1");

		const archiveButton = screen.getByRole("button", { name: /archive/i });
		expect(archiveButton).toHaveClass(archiveToggleHeightClassName, "w-full", "py-0");
		const archiveLabel = within(archiveButton).getByText("Archive");
		expect(archiveLabel).not.toHaveClass("font-mono", "uppercase");
		expect(archiveLabel).toHaveClass("text-2xs", "font-medium");
		// Expanded archive overlays the board instead of shrinking lanes (which would
		// force a persistent Needs You column scrollbar gutter).
		expect(archiveButton.parentElement).toHaveClass("absolute", "inset-x-0", "bottom-0", "bg-background");
		expect(screen.getByTestId("board")).toHaveClass("relative");
		expect(screen.getByTestId("board").querySelector(":scope > .min-h-0.flex-1")).toHaveClass(
			archiveToggleOffsetClassName,
		);
		const archive = await expandArchive();
		expect(archive).toHaveClass("scrollbar-none", "overflow-y-auto", "max-h-[28vh]");
		const terminatedCard = within(archive).getByText("dead worker").closest<HTMLElement>("[role='listitem']");
		expect(terminatedCard).not.toBeNull();
		expect(terminatedCard).toHaveAttribute("data-testid", "board-session-card");
		expect(terminatedCard).not.toHaveClass("min-h-28");
		expect(within(terminatedCard!).queryByRole("button", { name: "Open dead worker" })).not.toBeInTheDocument();
		expect(within(terminatedCard!).getByText("Terminated")).toBeInTheDocument();
		expect(within(terminatedCard!).getByTestId("session-pr-progress")).toHaveTextContent(
			"1 of 2 PRs merged · 1 open",
		);
		// Agent shown as its brand logo with an accessible name (not a text label).
		expect(within(terminatedCard!).getByRole("img", { name: "opencode" })).toBeInTheDocument();
		expect(screen.getByText("open-agents/dead-worker")).toBeInTheDocument();
		expect(within(terminatedCard!).queryByText("github:INT-17")).not.toBeInTheDocument();
		expect(within(terminatedCard!).getByRole("link", { name: "PR #42 merged" })).toHaveAttribute(
			"href",
			"https://github.com/example/radic/pull/42",
		);
		expect(within(terminatedCard!).getByRole("link", { name: "PR #41 open" })).toHaveAttribute(
			"href",
			"https://github.com/example/radic/pull/41",
		);
		expect(within(terminatedCard!).getByRole("button", { name: "Copy branch open-agents/dead-worker" })).toBeInTheDocument();
		const divider = terminatedCard!.querySelector("div.border-t.border-border");
		expect(divider).not.toBeNull();
		const mergedPrLink = within(terminatedCard!).getByRole("link", { name: "PR #42 merged" });
		expect(divider!.compareDocumentPosition(mergedPrLink) & Node.DOCUMENT_POSITION_PRECEDING).not.toBe(0);
		expect(
			screen.getByText("open-agents/dead-worker").compareDocumentPosition(divider!) & Node.DOCUMENT_POSITION_FOLLOWING,
		).not.toBe(0);
		expect(screen.getByRole("button", { name: "Restore dead worker" })).toBeInTheDocument();

		expect(screen.queryByRole("group", { name: "Archive layout" })).not.toBeInTheDocument();
	});

	it("hides PR progress for a live merged session that has not actually terminated", () => {
		const liveMergedSession = terminatedSession({
			id: "s-live-merged",
			title: "live merged worker",
			status: "merged",
			isTerminated: false,
			kanbanColumn: "ready",
		});
		workspaceQueryMock.mockReturnValue({
			data: [workspaceWithSessions([liveMergedSession])],
			isError: false,
			isSuccess: true,
		});

		renderBoard("p1");

		const card = screen.getByText("live merged worker").closest<HTMLElement>("[data-testid='board-session-card']");
		expect(card).not.toBeNull();
		expect(within(card!).queryByTestId("session-pr-progress")).not.toBeInTheDocument();
	});

	it("keeps archive cards mounted after collapse so reopen does not remount them", async () => {
		workspaceQueryMock.mockReturnValue({
			data: [workspaceWithSessions([terminatedSession()])],
			isError: false,
			isSuccess: true,
		});
		renderBoard("p1");

		const archiveButton = screen.getByRole("button", { name: /archive/i });
		const archive = await expandArchive();
		const card = within(archive).getByText("dead worker");

		await userEvent.click(archiveButton);
		expect(archiveButton).toHaveAttribute("aria-expanded", "false");
		expect(archive).toBeInTheDocument();
		expect(archive).toHaveAttribute("aria-hidden", "true");
		expect(archive).toHaveAttribute("inert");
		expect(archive).toHaveClass("pointer-events-none");
		expect(screen.queryByRole("list", { name: "Archived sessions" })).not.toBeInTheDocument();

		await userEvent.click(archiveButton);
		expect(archiveButton).toHaveAttribute("aria-expanded", "true");
		const reopened = screen.getByRole("list", { name: "Archived sessions" });
		expect(reopened).toBe(archive);
		expect(within(reopened).getByText("dead worker")).toBe(card);
		expect(reopened).not.toHaveAttribute("inert");
		expect(reopened).not.toHaveClass("pointer-events-none");
	});

	it("renders archived sessions as a grid even when rows were previously saved", async () => {
		window.localStorage.setItem("open-agents.board.archive.layout", "rows");
		workspaceQueryMock.mockReturnValue({
			data: [workspaceWithSessions([terminatedSession()])],
			isError: false,
			isSuccess: true,
		});
		renderBoard("p1");

		await expandArchive();
		expect(screen.queryByRole("group", { name: "Archive layout" })).not.toBeInTheDocument();
		const archive = screen.getByRole("list", { name: "Archived sessions" });
		expect(archive).toHaveClass("grid");
		const restore = screen.getByRole("button", { name: "Restore dead worker" });
		expect(restore.closest("[role='listitem']")).toContainElement(screen.getByText("Terminated"));
		expect(screen.queryByRole("button", { name: "Open dead worker" })).not.toBeInTheDocument();
	});

	it("restores a terminated session, refreshes workspace data, and opens the restored terminal", async () => {
		workspaceQueryMock.mockReturnValue({
			data: [workspaceWithSessions([terminatedSession()])],
			isError: false,
			isSuccess: true,
		});
		const queryClient = renderBoard("p1");
		const invalidate = vi.spyOn(queryClient, "invalidateQueries").mockResolvedValue(undefined);

		await expandArchive();
		await userEvent.click(screen.getByRole("button", { name: "Restore dead worker" }));

		await waitFor(() =>
			expect(postMock).toHaveBeenCalledWith("/api/v1/sessions/{sessionId}/restore", {
				params: { path: { sessionId: "s-dead" } },
			}),
		);
		expect(invalidate).toHaveBeenCalledWith({ queryKey: ["workspaces"] });
		expect(navigateMock).toHaveBeenCalledWith({
			to: "/projects/$projectId/sessions/$sessionId",
			params: { projectId: "p1", sessionId: "s-dead" },
		});
	});

	it("shows a toast when restore falls back to a saved-prompt conversation", async () => {
		postMock.mockResolvedValueOnce({ data: { restoreMode: "saved_prompt" } });
		workspaceQueryMock.mockReturnValue({
			data: [workspaceWithSessions([terminatedSession()])],
			isError: false,
			isSuccess: true,
		});
		renderBoard("p1");

		await expandArchive();
		await userEvent.click(screen.getByRole("button", { name: "Restore dead worker" }));

		await waitFor(() =>
			expect(notificationShowMock).toHaveBeenCalledWith(
				expect.objectContaining({
					title: "Started from saved prompt",
					body: expect.stringContaining("started a new conversation from the saved prompt"),
				}),
			),
		);
	});

	it("does not show a fallback toast when restore uses native resume", async () => {
		postMock.mockResolvedValueOnce({ data: { restoreMode: "native" } });
		workspaceQueryMock.mockReturnValue({
			data: [workspaceWithSessions([terminatedSession()])],
			isError: false,
			isSuccess: true,
		});
		renderBoard("p1");

		await expandArchive();
		await userEvent.click(screen.getByRole("button", { name: "Restore dead worker" }));

		await waitFor(() => expect(postMock).toHaveBeenCalled());
		expect(notificationShowMock).not.toHaveBeenCalled();
	});

	it("keeps restore actions visible and disables siblings while one session is restoring", async () => {
		let finishRestore: ((value: { data: Record<string, never> }) => void) | undefined;
		postMock.mockReturnValueOnce(
			new Promise((resolve) => {
				finishRestore = resolve;
			}),
		);
		workspaceQueryMock.mockReturnValue({
			data: [workspaceWithSessions([terminatedSession(), terminatedSession({ id: "s-other", title: "other worker" })])],
			isError: false,
			isSuccess: true,
		});

		renderBoard("p1");

		await expandArchive();
		await userEvent.click(screen.getByRole("button", { name: "Restore dead worker" }));

		const restoringButton = screen.getByRole("button", { name: "Restore dead worker" });
		const otherButton = screen.getByRole("button", { name: "Restore other worker" });
		expect(restoringButton.querySelector("svg")).toHaveClass("animate-spin");
		expect(otherButton).toBeDisabled();
		expect(otherButton).not.toHaveClass("opacity-0");

		await act(async () => {
			finishRestore?.({ data: {} });
		});
	});

	it("opens the restore-unavailable dialog when a session is not resumable", async () => {
		postMock.mockResolvedValueOnce({ error: { code: "SESSION_NOT_RESUMABLE" } });
		workspaceQueryMock.mockReturnValue({
			data: [workspaceWithSessions([terminatedSession()])],
			isError: false,
			isSuccess: true,
		});

		renderBoard("p1");

		await expandArchive();
		await userEvent.click(screen.getByRole("button", { name: "Restore dead worker" }));

		expect(await screen.findByText("Session can no longer be restored")).toBeInTheDocument();
	});

	it("shows an archive row error when restore fails", async () => {
		postMock.mockResolvedValueOnce({ error: { code: "RESTORE_FAILED", message: "boom" } });
		workspaceQueryMock.mockReturnValue({
			data: [workspaceWithSessions([terminatedSession()])],
			isError: false,
			isSuccess: true,
		});

		renderBoard("p1");

		await expandArchive();
		await userEvent.click(screen.getByRole("button", { name: "Restore dead worker" }));

		expect(await screen.findByText("Unable to restore session")).toBeInTheDocument();
		expect(navigateMock).not.toHaveBeenCalled();
	});

	it("cancels the clear-archive confirm without retiring anything", async () => {
		workspaceQueryMock.mockReturnValue({
			data: [workspaceWithSessions([terminatedSession()])],
			isError: false,
			isSuccess: true,
		});

		renderBoard("p1");
		await expandArchive();

		await userEvent.click(screen.getByRole("button", { name: "Clear archive" }));
		await userEvent.click(await screen.findByRole("button", { name: "Cancel" }));

		await waitFor(() =>
			expect(screen.queryByText("Clear the archive?")).not.toBeInTheDocument(),
		);
		expect(deleteMock).not.toHaveBeenCalled();
	});

	it("retires every archived session once the clear is confirmed", async () => {
		workspaceQueryMock.mockReturnValue({
			data: [
				workspaceWithSessions([
					terminatedSession(),
					terminatedSession({ id: "s-old", title: "old task" }),
				]),
			],
			isError: false,
			isSuccess: true,
		});

		renderBoard("p1");
		await expandArchive();

		await confirmClearArchive();

		await waitFor(() => expect(deleteMock).toHaveBeenCalledTimes(2));
		expect(deleteMock).toHaveBeenCalledWith("/api/v1/sessions/{sessionId}", {
			params: { path: { sessionId: "s-dead" } },
		});
		expect(deleteMock).toHaveBeenCalledWith("/api/v1/sessions/{sessionId}", {
			params: { path: { sessionId: "s-old" } },
		});
		expect(await screen.findByRole("status")).toHaveTextContent("Removed 2 archived sessions");
	});

	it("keeps clearing after a failure and reports the shortfall", async () => {
		// The middle session fails; the one after it must still be attempted.
		deleteMock
			.mockResolvedValueOnce({ data: { ok: true, freed: true } })
			.mockResolvedValueOnce({ error: { code: "SESSION_NOT_TERMINATED", message: "still running" } })
			.mockResolvedValueOnce({ data: { ok: true, freed: true } });
		workspaceQueryMock.mockReturnValue({
			data: [
				workspaceWithSessions([
					terminatedSession(),
					terminatedSession({ id: "s-live", title: "live one" }),
					terminatedSession({ id: "s-old", title: "old task" }),
				]),
			],
			isError: false,
			isSuccess: true,
		});

		renderBoard("p1");
		await expandArchive();

		await confirmClearArchive();

		await waitFor(() => expect(deleteMock).toHaveBeenCalledTimes(3));
		expect(deleteMock).toHaveBeenCalledWith("/api/v1/sessions/{sessionId}", {
			params: { path: { sessionId: "s-old" } },
		});
		expect(await screen.findByRole("status")).toHaveTextContent("Removed 2 · 1 failed: s-live");
	});

	it("keeps clearing after a transport throw, not just an error envelope", async () => {
		// A rejected request is a different failure shape than an envelope: it
		// escapes the mutation unless it is caught per session.
		deleteMock
			.mockResolvedValueOnce({ data: { ok: true, freed: true } })
			.mockRejectedValueOnce(new TypeError("fetch failed"))
			.mockResolvedValueOnce({ data: { ok: true, freed: true } });
		workspaceQueryMock.mockReturnValue({
			data: [
				workspaceWithSessions([
					terminatedSession(),
					terminatedSession({ id: "s-live", title: "live one" }),
					terminatedSession({ id: "s-old", title: "old task" }),
				]),
			],
			isError: false,
			isSuccess: true,
		});

		renderBoard("p1");
		await expandArchive();

		await confirmClearArchive();

		await waitFor(() => expect(deleteMock).toHaveBeenCalledTimes(3));
		expect(await screen.findByRole("status")).toHaveTextContent("Removed 2 · 1 failed: s-live");
	});

	it("treats an already-gone session as removed rather than failed", async () => {
		// The retire path never touches the workspace, so an absent row is a
		// benign 200 with freed=false. Counting it as a failure would make a
		// retried clear report errors for sessions that are simply gone.
		deleteMock.mockResolvedValueOnce({ data: { ok: true, sessionId: "s-dead", freed: false } });
		workspaceQueryMock.mockReturnValue({
			data: [workspaceWithSessions([terminatedSession()])],
			isError: false,
			isSuccess: true,
		});

		renderBoard("p1");
		await expandArchive();

		await confirmClearArchive();

		expect(await screen.findByRole("status")).toHaveTextContent("Removed 1 archived session");
		expect(screen.getByRole("status")).not.toHaveTextContent("failed");
	});

	it("does not navigate when the static archive card is clicked", async () => {
		workspaceQueryMock.mockReturnValue({
			data: [workspaceWithSessions([terminatedSession()])],
			isError: false,
			isSuccess: true,
		});

		renderBoard("p1");

		await expandArchive();
		await userEvent.click(screen.getByText("dead worker"));

		expect(postMock).not.toHaveBeenCalled();
		expect(navigateMock).not.toHaveBeenCalled();
	});

	it("ignores restore completion after navigating to another project board", async () => {
		let finishRestore: ((value: { data: Record<string, never> }) => void) | undefined;
		postMock.mockReturnValueOnce(
			new Promise((resolve) => {
				finishRestore = resolve;
			}),
		);
		workspaceQueryMock.mockReturnValue({
			data: [
				workspaceWithSessions([terminatedSession()]),
				{
					id: "p2",
					name: "other",
					path: "/tmp/other",
					sessions: [],
				},
			],
			isError: false,
			isSuccess: true,
		});
		const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
		const view = renderBoardWithClient(queryClient, "p1");

		await expandArchive();
		await userEvent.click(screen.getByRole("button", { name: "Restore dead worker" }));

		view.rerender(
			<QueryClientProvider client={queryClient}>
				<TooltipProvider>
					<SessionsBoard projectId="p2" />
				</TooltipProvider>
			</QueryClientProvider>,
		);
		await act(async () => {
			finishRestore?.({ data: {} });
		});

		expect(navigateMock).not.toHaveBeenCalled();
		expect(screen.queryByText("Session can no longer be restored")).not.toBeInTheDocument();
	});

	it("ignores restore-unavailable completion after navigating to another project board", async () => {
		let finishRestore: ((value: { error: { code: string } }) => void) | undefined;
		postMock.mockReturnValueOnce(
			new Promise((resolve) => {
				finishRestore = resolve;
			}),
		);
		workspaceQueryMock.mockReturnValue({
			data: [
				workspaceWithSessions([terminatedSession()]),
				{
					id: "p2",
					name: "other",
					path: "/tmp/other",
					sessions: [],
				},
			],
			isError: false,
			isSuccess: true,
		});
		const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
		const view = renderBoardWithClient(queryClient, "p1");

		await expandArchive();
		await userEvent.click(screen.getByRole("button", { name: "Restore dead worker" }));

		view.rerender(
			<QueryClientProvider client={queryClient}>
				<TooltipProvider>
					<SessionsBoard projectId="p2" />
				</TooltipProvider>
			</QueryClientProvider>,
		);
		await act(async () => {
			finishRestore?.({ error: { code: "SESSION_NOT_RESUMABLE" } });
		});

		expect(navigateMock).not.toHaveBeenCalled();
		expect(screen.queryByText("Session can no longer be restored")).not.toBeInTheDocument();
	});

	it("keeps a live merged session in the ready lane and opens its card without restore", async () => {
		workspaceQueryMock.mockReturnValue({
			data: [workspaceWithSessions([boardSession({ id: "s-merged", title: "merged worker", status: "merged" })])],
			isError: false,
			isSuccess: true,
		});

		renderBoard("p1");

		const readyLane = screen.getByRole("region", { name: "Ready sessions" });
		expect(within(readyLane).getByText("Ready")).toHaveClass("text-status-ready");
		expect(within(readyLane).getByText("merged worker")).toBeInTheDocument();
		expect(screen.queryByRole("button", { name: /archive/i })).not.toBeInTheDocument();
		expect(screen.queryByRole("button", { name: "Restore merged worker" })).not.toBeInTheDocument();

		await userEvent.click(screen.getByText("merged worker"));

		expect(postMock).not.toHaveBeenCalled();
		expect(navigateMock).toHaveBeenCalledWith({
			to: "/projects/$projectId/sessions/$sessionId",
			params: { projectId: "p1", sessionId: "s-merged" },
		});
	});

	it("groups lanes by the daemon's Kanban column, not by display status", () => {
		workspaceQueryMock.mockReturnValue({
			data: [
				workspaceWithSessions([
					// Both are "working" on the card; only the column decides the lane.
					boardSession({
						id: "s-validating",
						title: "validating worker",
						status: "working",
						kanbanColumn: "validating",
					}),
					boardSession({
						id: "s-building",
						title: "building worker",
						status: "working",
						kanbanColumn: "building",
					}),
					// Mergeable on the card, but no Open Agents loop is turning it, so the
					// review-feedback loop is on a person's turn. It joins the same
					// Review lane as the validating worker.
					boardSession({
						id: "s-needs-review",
						title: "in review worker",
						status: "mergeable",
						kanbanColumn: "needs_review",
					}),
				]),
			],
			isError: false,
			isSuccess: true,
		});

		renderBoard("p1");

		const lane = (label: string) => screen.getByLabelText(label);
		expect(within(lane("Building sessions")).getByText("building worker")).toBeInTheDocument();
		const review = lane("Review sessions");
		expect(within(review).getByText("validating worker")).toBeInTheDocument();
		expect(within(review).getByText("in review worker")).toBeInTheDocument();
		expect(within(lane("Ready sessions")).queryByText("in review worker")).toBeNull();
	});

	it("renders the daemon's display status on the card", () => {
		workspaceQueryMock.mockReturnValue({
			data: [
				workspaceWithSessions([
					boardSession({
						id: "s-ci",
						title: "ci worker",
						status: "ci_failed",
						kanbanColumn: "validating",
						displayStatus: "Fixing CI failures",
					}),
				]),
			],
			isError: false,
			isSuccess: true,
		});

		renderBoard("p1");

		expect(screen.getByText("Fixing CI failures")).toBeInTheDocument();
		expect(screen.queryByText("CI failed")).not.toBeInTheDocument();
	});

	function awaitingPrSession(overrides: Partial<WorkspaceSession> = {}): WorkspaceSession {
		return boardSession({
			id: "s-stage",
			title: "stage worker",
			status: "idle",
			kanbanColumn: "building",
			displayStatus: "Awaiting PR",
			workflowMode: "planning",
			...overrides,
		});
	}

	it("places an Awaiting PR card in the review lane once building is confirmed", () => {
		workspaceQueryMock.mockReturnValue({
			data: [
				workspaceWithSessions([
					awaitingPrSession({ id: "s-build", title: "built worker", workflowMode: "building" }),
					awaitingPrSession({ id: "s-plan", title: "planned worker", workflowMode: "planning" }),
				]),
			],
			isError: false,
			isSuccess: true,
		});

		renderBoard("p1");

		expect(
			within(screen.getByLabelText("Review sessions")).getByText("built worker"),
		).toBeInTheDocument();
		expect(
			within(screen.getByLabelText("Planning sessions")).getByText("planned worker"),
		).toBeInTheDocument();
	});

	it("confirms building from an Awaiting PR card", async () => {
		workspaceQueryMock.mockReturnValue({
			data: [workspaceWithSessions([awaitingPrSession()])],
			isError: false,
			isSuccess: true,
		});

		renderBoard("p1");
		expect(screen.queryByText("Awaiting PR")).not.toBeInTheDocument();
		const build = screen.getByRole("button", { name: "Build" });
		// The one-word label drops the "approve the plan" nuance, so the long
		// form has to survive as a tooltip.
		expect(build).toHaveAttribute("title", "Approve this plan and let the worker start building");
		await userEvent.click(build);

		await waitFor(() =>
			expect(patchMock).toHaveBeenCalledWith("/api/v1/sessions/{sessionId}/workflow-mode", {
				params: { path: { sessionId: "s-stage" } },
				body: { workflowMode: "building" },
			}),
		);
	});

	it("reviews to commit from an Awaiting PR card by approving the pending edit", async () => {
		workspaceQueryMock.mockReturnValue({
			data: [workspaceWithSessions([awaitingPrSession({ workflowMode: "building" })])],
			isError: false,
			isSuccess: true,
		});
		getMock.mockResolvedValue({
			data: {
				activities: [
					{
						activityKind: "approval",
						status: "pending",
						requestId: "approval-1",
						detail: { decisions: [{ id: "accept", label: "Approve" }] },
					},
				],
			},
		});

		renderBoard("p1");
		await userEvent.click(screen.getByRole("button", { name: "Commit" }));

		await waitFor(() =>
			expect(postMock).toHaveBeenCalledWith(
				"/api/v1/sessions/{sessionId}/conversation/approvals/{requestId}/resolve",
				{
					params: { path: { sessionId: "s-stage", requestId: "approval-1" } },
					body: { decisionId: "accept" },
				},
			),
		);
	});

	it("commits a finished Review card with nothing to approve by ensuring a pull request", async () => {
		// The 19 exhibit: work committed, worktree clean, no PR, daemon idles
		// on "Awaiting PR". Commit must push + open the PR (POST /pr) instead
		// of opening the session and leaving the card where it was.
		workspaceQueryMock.mockReturnValue({
			data: [workspaceWithSessions([awaitingPrSession({ workflowMode: "building" })])],
			isError: false,
			isSuccess: true,
		});
		getMock.mockResolvedValue({ data: { activities: [] } });
		postMock.mockResolvedValue({
			data: { ok: true, prUrl: "https://github.com/example/radic/pull/145", prNumber: 145, created: true },
		});

		renderBoard("p1");
		await userEvent.click(screen.getByRole("button", { name: "Commit" }));

		await waitFor(() =>
			expect(postMock).toHaveBeenCalledWith("/api/v1/sessions/{sessionId}/pr", {
				params: { path: { sessionId: "s-stage" } },
			}),
		);
		await waitFor(() =>
			expect(openExternalMock).toHaveBeenCalledWith("https://github.com/example/radic/pull/145"),
		);
		expect(navigateMock).not.toHaveBeenCalled();
	});

	it("commits an in-review card with no pending edit by pushing to its pull request", async () => {
		workspaceQueryMock.mockReturnValue({
			data: [workspaceWithSessions([reviewLaneSession()])],
			isError: false,
			isSuccess: true,
		});
		getMock.mockResolvedValue({ data: { activities: [] } });
		postMock.mockResolvedValue({
			data: { ok: true, prUrl: "https://github.com/example/radic/pull/146", prNumber: 146, created: false },
		});

		renderBoard("p1");
		await userEvent.click(screen.getByRole("button", { name: "Commit" }));

		await waitFor(() =>
			expect(postMock).toHaveBeenCalledWith("/api/v1/sessions/{sessionId}/pr", {
				params: { path: { sessionId: "s-review" } },
			}),
		);
		await waitFor(() =>
			expect(openExternalMock).toHaveBeenCalledWith("https://github.com/example/radic/pull/146"),
		);
		expect(navigateMock).not.toHaveBeenCalled();
	});

	it("reports a Commit pull-request failure on the review card without advancing it", async () => {
		workspaceQueryMock.mockReturnValue({
			data: [workspaceWithSessions([reviewLaneSession()])],
			isError: false,
			isSuccess: true,
		});
		getMock.mockResolvedValue({ data: { activities: [] } });
		postMock.mockResolvedValue({
			error: { code: "GH_AUTH_MISSING", message: "gh is not authenticated" },
			response: { status: 403 },
		});

		renderBoard("p1");
		await userEvent.click(screen.getByRole("button", { name: "Commit" }));

		const card = screen.getByText("review worker").closest('[data-testid="board-session-card"]') as HTMLElement;
		// The suite mocks apiErrorMessage to its fallback, so the card shows
		// the status-qualified fallback; production surfaces the daemon text.
		expect(await within(card).findByRole("alert")).toHaveTextContent(
			"Failed to open a pull request for review worker (403)",
		);
		expect(navigateMock).not.toHaveBeenCalled();
		expect(openExternalMock).not.toHaveBeenCalled();
		// The card stays live: a failed Commit never terminates or advances.
		expect(within(card).getByRole("button", { name: "Commit" })).toBeEnabled();
	});

	it("fires a single pull request creation on Commit double-click", async () => {
		workspaceQueryMock.mockReturnValue({
			data: [workspaceWithSessions([reviewLaneSession()])],
			isError: false,
			isSuccess: true,
		});
		getMock.mockResolvedValue({ data: { activities: [] } });
		let resolvePost!: (value: { data: Record<string, unknown> }) => void;
		postMock.mockReturnValueOnce(
			new Promise((resolve) => {
				resolvePost = resolve;
			}),
		);

		renderBoard("p1");
		const commit = screen.getByRole("button", { name: "Commit" });
		await userEvent.click(commit);
		// The pending request disables the button, so the second click lands on
		// a disabled control and fires nothing; a race that still reaches the
		// daemon resolves to the same PR via durable/remote/race protection.
		await waitFor(() => expect(commit).toBeDisabled());
		await userEvent.click(commit);
		expect(postMock).toHaveBeenCalledTimes(1);

		await act(async () => {
			resolvePost({ data: { ok: true, prUrl: "https://github.com/example/radic/pull/146", created: true } });
		});
		await waitFor(() =>
			expect(openExternalMock).toHaveBeenCalledWith("https://github.com/example/radic/pull/146"),
		);
	});

	it("never shows Commit on building or archived cards", () => {
		workspaceQueryMock.mockReturnValue({
			data: [
				workspaceWithSessions([
					boardSession({ id: "s-building", title: "building worker", status: "working", kanbanColumn: "building" }),
					terminatedSession({ id: "s-dead", title: "dead worker" }),
				]),
			],
			isError: false,
			isSuccess: true,
		});

		renderBoard("p1");

		const buildingCard = screen.getByText("building worker").closest('[data-testid="board-session-card"]') as HTMLElement;
		expect(within(buildingCard).queryByRole("button", { name: "Commit" })).not.toBeInTheDocument();
		expect(screen.queryByRole("button", { name: "Commit" })).not.toBeInTheDocument();
	});

	function readySession(overrides: Partial<WorkspaceSession> = {}): WorkspaceSession {
		return boardSession({
			id: "s-ready",
			title: "ready worker",
			status: "approved",
			kanbanColumn: "ready",
			displayStatus: "Approved",
			prs: [
				{
					url: "https://github.com/example/radic/pull/144",
					number: 144,
					state: "open",
					ci: "passing",
					review: "approved",
					mergeability: "mergeable",
					reviewComments: false,
					updatedAt: "2026-01-01T00:00:00Z",
				},
			],
			...overrides,
		});
	}

	function reviewLaneSession(overrides: Partial<WorkspaceSession> = {}): WorkspaceSession {
		return boardSession({
			id: "s-review",
			title: "review worker",
			status: "review_pending",
			kanbanColumn: "needs_review",
			displayStatus: "Needs human review",
			...overrides,
		});
	}

	it("shows merge-local and open-PR actions on a ready card and nowhere else", () => {
		workspaceQueryMock.mockReturnValue({
			data: [
				workspaceWithSessions([
					readySession(),
					boardSession({ id: "s-building", title: "building worker", status: "working", kanbanColumn: "building" }),
					reviewLaneSession({ id: "s-review", title: "review worker" }),
					awaitingPrSession({ id: "s-plan", title: "planned worker", workflowMode: "planning" }),
					terminatedSession({ id: "s-dead", title: "dead worker" }),
				]),
			],
			isError: false,
			isSuccess: true,
		});

		renderBoard("p1");

		const readyCard = screen.getByText("ready worker").closest('[data-testid="board-session-card"]') as HTMLElement;
		expect(within(readyCard).getByRole("button", { name: "Merge local" })).toBeInTheDocument();
		expect(within(readyCard).getByRole("button", { name: "Open PR" })).toBeInTheDocument();

		for (const title of ["building worker", "review worker", "planned worker"]) {
			const card = screen.getByText(title).closest('[data-testid="board-session-card"]') as HTMLElement;
			expect(within(card).queryByRole("button", { name: "Merge local" })).not.toBeInTheDocument();
			expect(within(card).queryByRole("button", { name: "Open PR" })).not.toBeInTheDocument();
		}
		// Ready is the only lane with delivery buttons: exactly one card owns them.
		expect(screen.getAllByRole("button", { name: "Merge local" })).toHaveLength(1);
		expect(screen.getAllByRole("button", { name: "Open PR" })).toHaveLength(1);
	});

	it("shows exactly one Commit button on review-lane cards and keeps Build on planning cards", () => {
		workspaceQueryMock.mockReturnValue({
			data: [
				workspaceWithSessions([
					reviewLaneSession(),
					awaitingPrSession({ id: "s-plan", title: "planned worker", workflowMode: "planning" }),
					awaitingPrSession({ id: "s-build", title: "built worker", workflowMode: "building" }),
				]),
			],
			isError: false,
			isSuccess: true,
		});

		renderBoard("p1");

		// Awaiting-PR in building mode sits in the review lane: Commit, no Build.
		const builtCard = screen.getByText("built worker").closest('[data-testid="board-session-card"]') as HTMLElement;
		expect(within(builtCard).getByRole("button", { name: "Commit" })).toBeInTheDocument();
		expect(within(builtCard).queryByRole("button", { name: "Build" })).not.toBeInTheDocument();
		// In-review feedback loop: exactly one Commit, nothing else.
		const reviewCard = screen.getByText("review worker").closest('[data-testid="board-session-card"]') as HTMLElement;
		expect(within(reviewCard).getByRole("button", { name: "Commit" })).toBeInTheDocument();
		expect(within(reviewCard).queryByRole("button", { name: "Build" })).not.toBeInTheDocument();
		// Planning lane keeps Build and gains no Commit.
		const plannedCard = screen.getByText("planned worker").closest('[data-testid="board-session-card"]') as HTMLElement;
		expect(within(plannedCard).getByRole("button", { name: "Build" })).toBeInTheDocument();
		expect(within(plannedCard).queryByRole("button", { name: "Commit" })).not.toBeInTheDocument();
		expect(screen.getAllByRole("button", { name: "Commit" })).toHaveLength(2);
	});

	it("reviews to commit from an in-review card by approving the pending edit", async () => {
		workspaceQueryMock.mockReturnValue({
			data: [workspaceWithSessions([reviewLaneSession()])],
			isError: false,
			isSuccess: true,
		});
		getMock.mockResolvedValue({
			data: {
				activities: [
					{
						activityKind: "approval",
						status: "pending",
						requestId: "approval-1",
						detail: { decisions: [{ id: "accept", label: "Approve" }] },
					},
				],
			},
		});

		renderBoard("p1");
		await userEvent.click(screen.getByRole("button", { name: "Commit" }));

		await waitFor(() =>
			expect(postMock).toHaveBeenCalledWith(
				"/api/v1/sessions/{sessionId}/conversation/approvals/{requestId}/resolve",
				{
					params: { path: { sessionId: "s-review", requestId: "approval-1" } },
					body: { decisionId: "accept" },
				},
			),
		);
	});

	it("asks for confirmation before merging a ready branch into local dev", async () => {
		workspaceQueryMock.mockReturnValue({
			data: [workspaceWithSessions([readySession()])],
			isError: false,
			isSuccess: true,
		});
		postMock.mockResolvedValue({ data: { ok: true } });

		renderBoard("p1");
		await userEvent.click(screen.getByRole("button", { name: "Merge local" }));

		// No request until the destructive-ish chain is confirmed.
		expect(postMock).not.toHaveBeenCalled();
		const dialog = screen.getByRole("dialog", { name: "Merge ready worker into dev?" });
		await userEvent.click(within(dialog).getByRole("button", { name: "Yes, merge into dev" }));

		await waitFor(() =>
			expect(postMock).toHaveBeenCalledWith("/api/v1/sessions/{sessionId}/merge-local", {
				params: { path: { sessionId: "s-ready" } },
			}),
		);
	});

	it("reports a dirty-checkout merge refusal on the ready card without terminating", async () => {
		workspaceQueryMock.mockReturnValue({
			data: [workspaceWithSessions([readySession()])],
			isError: false,
			isSuccess: true,
		});
		postMock.mockResolvedValue({ error: { code: "WORKSPACE_DIRTY", message: "Project checkout has uncommitted changes" }, response: { status: 409 } });

		renderBoard("p1");
		await userEvent.click(screen.getByRole("button", { name: "Merge local" }));
		const dialog = screen.getByRole("dialog", { name: "Merge ready worker into dev?" });
		await userEvent.click(within(dialog).getByRole("button", { name: "Yes, merge into dev" }));

		const card = screen.getByText("ready worker").closest('[data-testid="board-session-card"]') as HTMLElement;
		// The suite mocks apiErrorMessage to its fallback, so the card shows
		// the status-qualified fallback; production surfaces the daemon text.
		expect(await within(card).findByRole("alert")).toHaveTextContent("Failed to merge ready worker into dev (409)");
		// The card stays live: a failed merge never terminates.
		expect(within(card).getByRole("button", { name: "Merge local" })).toBeEnabled();
	});

	it("opens the pull request url after ensuring exactly one PR", async () => {
		workspaceQueryMock.mockReturnValue({
			data: [workspaceWithSessions([readySession()])],
			isError: false,
			isSuccess: true,
		});
		postMock.mockResolvedValue({
			data: { ok: true, prUrl: "https://github.com/example/radic/pull/144", prNumber: 144, created: false },
		});

		renderBoard("p1");
		await userEvent.click(screen.getByRole("button", { name: "Open PR" }));

		await waitFor(() =>
			expect(postMock).toHaveBeenCalledWith("/api/v1/sessions/{sessionId}/pr", {
				params: { path: { sessionId: "s-ready" } },
			}),
		);
		await waitFor(() =>
			expect(openExternalMock).toHaveBeenCalledWith("https://github.com/example/radic/pull/144"),
		);
	});

	it("fires a single pull request creation on double-click", async () => {
		workspaceQueryMock.mockReturnValue({
			data: [workspaceWithSessions([readySession()])],
			isError: false,
			isSuccess: true,
		});
		let resolvePost!: (value: { data: Record<string, unknown> }) => void;
		postMock.mockReturnValueOnce(
			new Promise((resolve) => {
				resolvePost = resolve;
			}),
		);

		renderBoard("p1");
		const openPR = screen.getByRole("button", { name: "Open PR" });
		await userEvent.click(openPR);
		// The pending request disables the button, so the second click lands on
		// a disabled control and fires nothing.
		expect(openPR).toBeDisabled();
		await userEvent.click(openPR);
		expect(postMock).toHaveBeenCalledTimes(1);

		await act(async () => {
			resolvePost({ data: { ok: true, prUrl: "https://github.com/example/radic/pull/144", created: true } });
		});
		await waitFor(() =>
			expect(openExternalMock).toHaveBeenCalledWith("https://github.com/example/radic/pull/144"),
		);
	});

	it("highlights every user-attention status while leaving ordinary cards neutral", () => {
		const attentionStatuses = [
			"ci_failed",
			"changes_requested",
		] as const;
		workspaceQueryMock.mockReturnValue({
			data: [
				workspaceWithSessions([
					...attentionStatuses.map((status) =>
						boardSession({
							id: `s-${status}`,
							kanbanColumn: "building",
							status,
							title: `${status} worker`,
						}),
					),
					boardSession({ id: "s-idle", title: "idle worker", status: "idle" }),
				]),
			],
			isError: false,
			isSuccess: true,
		});

		renderBoard("p1");

		for (const status of attentionStatuses) {
			const card = screen
				.getByText(`${status} worker`)
				.closest('[data-testid="board-session-card"]');
			expect(card).toHaveClass(
				"animate-attention-card-pulse",
				"border-status-needs-you",
				"bg-[color-mix(in_srgb,var(--color-status-needs-you)_8%,var(--color-surface))]",
			);
		}

		const ordinaryCard = screen
			.getByText("idle worker")
			.closest('[data-testid="board-session-card"]');
		expect(ordinaryCard).toHaveClass("border-border", "bg-surface");
		expect(ordinaryCard).not.toHaveClass("animate-attention-card-pulse");
	});

	// Mixed-version upgrade: an older daemon sends no kanbanColumn at all. Cards
	// must stay in the lanes their status already put them in, not pile into the
	// leftmost one.
	it("keeps an older daemon's sessions in their status-implied lanes", () => {
		const legacy = (id: string, title: string, status: WorkspaceSession["status"]): WorkspaceSession => {
			const session = boardSession({ id, title, status });
			delete session.kanbanColumn;
			return session;
		};
		workspaceQueryMock.mockReturnValue({
			data: [
				workspaceWithSessions([
					legacy("s-ready", "legacy ready worker", "mergeable"),
					legacy("s-action", "legacy action worker", "changes_requested"),
					legacy("s-review", "legacy review worker", "review_pending"),
					legacy("s-working", "legacy working worker", "working"),
				]),
			],
			isError: false,
			isSuccess: true,
		});

		renderBoard("p1");

		const lane = (label: string) => screen.getByLabelText(label);
		expect(within(lane("Building sessions")).getByText("legacy working worker")).toBeInTheDocument();
		const review = lane("Review sessions");
		expect(within(review).getByText("legacy review worker")).toBeInTheDocument();
		expect(within(review).getByText("legacy action worker")).toBeInTheDocument();
		expect(within(lane("Ready sessions")).getByText("legacy ready worker")).toBeInTheDocument();
	});

	it("orders the lanes planning, building, review, then ready", () => {
		workspaceQueryMock.mockReturnValue({
			data: [workspaceWithSessions([boardSession({ id: "s-one", title: "worker one", status: "idle" })])],
			isError: false,
			isSuccess: true,
		});

		renderBoard("p1");

		expect(screen.getAllByTestId("board-column").map((column) => column.dataset.column)).toEqual([
			"planning",
			"building",
			"review",
			"ready",
		]);
	});

	it("uses the shared minimal scrollbar styling for every Kanban lane", () => {
		workspaceQueryMock.mockReturnValue({
			data: [
				workspaceWithSessions([
					boardSession({ id: "s-idle", title: "idle worker", status: "idle" }),
					boardSession({ id: "s-working", title: "working worker", status: "working" }),
					boardSession({ id: "s-action", title: "action worker", status: "needs_input" }),
					boardSession({ id: "s-review", title: "review worker", status: "review_pending" }),
					boardSession({ id: "s-ready", title: "ready worker", status: "mergeable" }),
					boardSession({ id: "s-merged", title: "merged worker", status: "merged" }),
				]),
			],
			isError: false,
			isSuccess: true,
		});

		renderBoard("p1");

		const laneScrollers = screen
			.getAllByTestId("board-column")
			.flatMap((column) => Array.from(column.querySelectorAll<HTMLElement>(".overflow-y-auto")));
		expect(laneScrollers).toHaveLength(4);
		for (const scroller of laneScrollers) {
			expect(scroller).toHaveClass("board-scrollbar", "overflow-y-auto");
		}
	});

	it("archives a terminated merged runtime without duplicating it in the ready lane", async () => {
		workspaceQueryMock.mockReturnValue({
			data: [
				workspaceWithSessions([
					boardSession({ id: "s-live-merged", title: "live merged worker", status: "merged" }),
					terminatedSession({ id: "s-archived-merged", title: "archived merged worker", status: "merged" }),
				]),
			],
			isError: false,
			isSuccess: true,
		});

		renderBoard("p1");

		const readyLane = screen.getByRole("region", { name: "Ready sessions" });
		expect(within(readyLane).getByText("live merged worker")).toBeInTheDocument();
		expect(within(readyLane).queryByText("archived merged worker")).not.toBeInTheDocument();

		await expandArchive();
		const archive = screen.getByRole("list", { name: "Archived sessions" });
		const archivedMergedCard = within(archive)
			.getByText("archived merged worker")
			.closest<HTMLElement>("[role='listitem']");
		expect(archivedMergedCard).not.toBeNull();
		expect(
			within(archivedMergedCard!).queryByRole("button", { name: "Open archived merged worker" }),
		).not.toBeInTheDocument();
		expect(
			within(archivedMergedCard!).queryByRole("button", { name: "Terminate archived merged worker" }),
		).not.toBeInTheDocument();
		expect(within(archivedMergedCard!).getByText("Merged").parentElement).toHaveAttribute(
			"data-kanban-column",
			"archive",
		);
		expect(within(archive).getByRole("button", { name: "Restore archived merged worker" })).toBeInTheDocument();
	});

	it("asks for confirmation when terminating an ordinary live session from its card", async () => {
		workspaceQueryMock.mockReturnValue({
			data: [workspaceWithSessions([boardSession({ id: "s-idle", title: "idle worker", status: "idle" })])],
			isError: false,
			isSuccess: true,
		});
		renderBoard("p1");

		await userEvent.click(screen.getByRole("button", { name: "Terminate idle worker" }));

		expect(navigateMock).not.toHaveBeenCalled();
		expect(screen.getByRole("dialog", { name: "Terminate idle worker?" })).toBeInTheDocument();
	});

	it("terminates a live merged session from its card without opening the session", async () => {
		workspaceQueryMock.mockReturnValue({
			data: [workspaceWithSessions([boardSession({ id: "s-merged", title: "merged worker", status: "merged" })])],
			isError: false,
			isSuccess: true,
		});
		renderBoard("p1");

		const terminateButton = screen.getByRole("button", { name: "Terminate merged worker" });
		expect(terminateButton).toHaveClass("opacity-100");
		expect(terminateButton).not.toHaveClass("opacity-0");
		await userEvent.click(terminateButton);
		expect(navigateMock).not.toHaveBeenCalled();
		const dialog = screen.getByRole("dialog", { name: "Terminate merged worker?" });
		await userEvent.click(within(dialog).getByRole("button", { name: "Yes, terminate session" }));

		await waitFor(() =>
			expect(postMock).toHaveBeenCalledWith("/api/v1/sessions/{sessionId}/kill", {
				params: { path: { sessionId: "s-merged" } },
			}),
		);
		expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
		expect(navigateMock).not.toHaveBeenCalled();
	});

	it("keeps only the targeted card disabled while its termination is pending", async () => {
		let finishKill!: (value: { data: { ok: boolean; sessionId: string }; error: undefined }) => void;
		postMock.mockReturnValueOnce(
			new Promise((resolve) => {
				finishKill = resolve;
			}),
		);
		workspaceQueryMock.mockReturnValue({
			data: [
				workspaceWithSessions([
					boardSession({ id: "s-one", title: "worker one", status: "working" }),
					boardSession({ id: "s-two", title: "worker two", status: "merged" }),
				]),
			],
			isError: false,
			isSuccess: true,
		});
		renderBoard("p1");

		await userEvent.click(screen.getByRole("button", { name: "Terminate worker one" }));
		await userEvent.click(
			within(screen.getByRole("dialog")).getByRole("button", { name: "Yes, terminate session" }),
		);

		expect(screen.getByRole("button", { name: "Killing worker one" })).toBeDisabled();
		expect(screen.getByRole("button", { name: "Killing worker one" })).toHaveClass("opacity-100");
		expect(screen.getByRole("button", { name: "Terminate worker two" })).toBeEnabled();
		expect(postMock).toHaveBeenCalledTimes(1);

		finishKill({ data: { ok: true, sessionId: "s-one" }, error: undefined });
		await waitFor(() => expect(screen.getByRole("button", { name: "Terminate worker one" })).toBeEnabled());
	});

	it("keeps the merged-card confirmation dismissed and surfaces termination failures", async () => {
		postMock.mockResolvedValueOnce({ error: { message: "runtime failed" }, response: { status: 500 } });
		workspaceQueryMock.mockReturnValue({
			data: [workspaceWithSessions([boardSession({ id: "s-merged", title: "merged worker", status: "merged" })])],
			isError: false,
			isSuccess: true,
		});
		renderBoard("p1");

		await userEvent.click(screen.getByRole("button", { name: "Terminate merged worker" }));
		await userEvent.click(
			within(screen.getByRole("dialog")).getByRole("button", { name: "Yes, terminate session" }),
		);

		await waitFor(() => expect(postMock).toHaveBeenCalledTimes(1));
		expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
		expect(await screen.findByRole("alert")).toHaveTextContent("Failed to terminate session (500)");
		expect(screen.getByRole("button", { name: "Terminate merged worker" })).toBeEnabled();
	});

	it("shows a folder-missing banner when the project root no longer exists on disk", () => {
		workspaceQueryMock.mockReturnValue({
			data: [{ ...workspaceWithSessions([]), folderMissing: true }],
		});
		renderBoard("p1");
		expect(screen.getByText("Folder missing")).toBeInTheDocument();
	});

	it("does not show the folder-missing banner when the project folder exists", () => {
		workspaceQueryMock.mockReturnValue({
			data: [{ ...workspaceWithSessions([]), folderMissing: false }],
		});
		renderBoard("p1");
		expect(screen.queryByText("Folder missing")).not.toBeInTheDocument();
	});
});

function workspaceWithSessions(sessions: WorkspaceSession[]): WorkspaceSummary {
	return {
		id: "p1",
		name: "radic",
		path: "/tmp/radic",
		sessions,
	};
}

function boardSession(
	overrides: Pick<WorkspaceSession, "id" | "title" | "status"> & Partial<WorkspaceSession>,
): WorkspaceSession {
	return {
		workspaceId: "p1",
		workspaceName: "radic",
		provider: "opencode",
		branch: `open-agents/${overrides.id}`,
		kanbanColumn: toKanbanColumn(undefined, overrides.status),
		updatedAt: "2026-01-01T00:00:00Z",
		prs: [],
		...overrides,
	};
}

function terminatedSession(overrides: Partial<WorkspaceSession> = {}): WorkspaceSession {
	return {
		id: "s-dead",
		workspaceId: "p1",
		workspaceName: "radic",
		title: "dead worker",
		issueId: "github:INT-17",
		provider: "opencode",
		kind: "worker",
		branch: "open-agents/dead-worker",
		status: "terminated",
		kanbanColumn: "archive",
		isTerminated: true,
		updatedAt: "2026-01-01T00:00:00Z",
		prs: [
			{
				url: "https://github.com/example/radic/pull/42",
				number: 42,
				state: "merged",
				ci: "passing",
				review: "approved",
				mergeability: "mergeable",
				reviewComments: false,
				updatedAt: "2026-01-01T00:00:00Z",
			},
		],
		...overrides,
	};
}
