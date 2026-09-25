import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useUiStore } from "../stores/ui-store";
import type { WorkspaceSession, WorkspaceSummary } from "../types/workspace";
import { RestoreUnavailableDialog } from "./RestoreUnavailableDialog";

const { spawnMock, workspaceQueryMock } = vi.hoisted(() => ({
	spawnMock: vi.fn(),
	workspaceQueryMock: vi.fn(),
}));

vi.mock("../hooks/useWorkspaceQuery", () => ({
	useWorkspaceScope: () => workspaceQueryMock(),
}));

vi.mock("../lib/spawn-manager", () => ({
	spawnManager: spawnMock,
}));

const session: WorkspaceSession = {
	id: "mgr-old",
	workspaceId: "proj-1",
	workspaceName: "Project One",
	title: "manager",
	provider: "opencode",
	kind: "manager",
	status: "terminated",
	updatedAt: "2026-07-26T00:00:00Z",
	prs: [],
};

const workspace: WorkspaceSummary = {
	id: "proj-1",
	name: "Project One",
	path: "/repo/project-one",
	managerAgent: "opencode",
	sessions: [session],
};

beforeEach(() => {
	vi.clearAllMocks();
	useUiStore.setState({ settingsModal: null });
	workspaceQueryMock.mockReturnValue({ data: { project: workspace }, isLoading: false });
});

describe("RestoreUnavailableDialog", () => {
	it("opens project settings instead of recreating when no manager agent is configured", async () => {
		const onOpenChange = vi.fn();
		const onRecreated = vi.fn();
		workspaceQueryMock.mockReturnValue({
		data: { project: { ...workspace, managerAgent: undefined } },
			isLoading: false,
		});
		render(
			<RestoreUnavailableDialog
				open
				session={session}
				onOpenChange={onOpenChange}
				onRecreated={onRecreated}
			/>,
		);

		await userEvent.click(screen.getByRole("button", { name: "Configure manager agent" }));

		expect(onOpenChange).toHaveBeenCalledWith(false);
		expect(useUiStore.getState().settingsModal).toEqual({ scope: "project", projectId: "proj-1" });
		expect(spawnMock).not.toHaveBeenCalled();
		expect(onRecreated).not.toHaveBeenCalled();
	});

	it("preserves clean recreation when a manager agent is configured", async () => {
		const onOpenChange = vi.fn();
		const onRecreated = vi.fn();
		spawnMock.mockResolvedValue("mgr-new");
		render(
			<RestoreUnavailableDialog
				open
				session={session}
				onOpenChange={onOpenChange}
				onRecreated={onRecreated}
			/>,
		);

		await userEvent.click(screen.getByRole("button", { name: "Create new manager" }));

		await waitFor(() => expect(onRecreated).toHaveBeenCalledWith("mgr-new"));
		expect(spawnMock).toHaveBeenCalledWith("proj-1", "restore_dialog", true);
		expect(onOpenChange).toHaveBeenCalledWith(false);
	});
});
