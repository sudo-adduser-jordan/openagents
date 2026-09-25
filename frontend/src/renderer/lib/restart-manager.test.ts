import { QueryClient } from "@tanstack/react-query";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { workspaceQueryKey } from "../hooks/useWorkspaceQuery";

const { spawnMock } = vi.hoisted(() => ({
	spawnMock: vi.fn(),
}));

vi.mock("./spawn-manager", () => ({
	ManagerSpawnError: class ManagerSpawnError extends Error {
		constructor(
			message: string,
			readonly code?: string,
			readonly requestId?: string,
			readonly status?: number,
		) {
			super(message);
		}
	},
	spawnManager: spawnMock,
}));

import { ManagerSpawnError } from "./spawn-manager";
import { restartProjectManager } from "./restart-manager";

describe("restartProjectManager", () => {
	beforeEach(() => {
		spawnMock.mockReset();
	});

	it("invalidates workspace state and records an error when clean restart fails", async () => {
		const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
		const invalidateSpy = vi.spyOn(queryClient, "invalidateQueries").mockResolvedValue();
		const navigate = vi.fn();
		const setProjectRestarting = vi.fn();
		const setManagerReplacementError = vi.fn();
		const onError = vi.fn();
		const failure = new Error("missing goose binary");
		spawnMock.mockRejectedValue(failure);

		await restartProjectManager({
			projectId: "proj-1",
			queryClient,
			navigate,
			setProjectRestarting,
			setManagerReplacementError,
			onError,
		});

		expect(spawnMock).toHaveBeenCalledWith("proj-1", "restart", true, undefined);
		expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: workspaceQueryKey });
		expect(setManagerReplacementError).toHaveBeenNthCalledWith(1, "proj-1", {
			message: "missing goose binary",
		});
		expect(setProjectRestarting).toHaveBeenNthCalledWith(1, "proj-1", true);
		expect(setProjectRestarting).toHaveBeenLastCalledWith("proj-1", false);
		expect(onError).toHaveBeenCalledWith(failure);
		expect(navigate).not.toHaveBeenCalled();
	});

	it("blurs the restart control so the replacement terminal can reclaim focus", async () => {
		const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
		vi.spyOn(queryClient, "invalidateQueries").mockResolvedValue();
		const restartButton = document.createElement("button");
		document.body.appendChild(restartButton);
		restartButton.focus();
		spawnMock.mockResolvedValue("session-2");

		await restartProjectManager({
			projectId: "proj-1",
			queryClient,
			navigate: vi.fn(),
			setProjectRestarting: vi.fn(),
			setManagerReplacementError: vi.fn(),
		});

		expect(restartButton).not.toHaveFocus();
		restartButton.remove();
	});

	it("keeps the initiating control focused until replacement and refresh finish", async () => {
		let completeSpawn!: (id: string) => void;
		let completeRefresh!: () => void;
		spawnMock.mockImplementation(() => new Promise<string>((resolve) => { completeSpawn = resolve; }));
		const queryClient = new QueryClient();
		vi.spyOn(queryClient, "invalidateQueries").mockImplementation(() => new Promise<void>((resolve) => { completeRefresh = resolve; }));
		const button = document.createElement("button");
		document.body.appendChild(button);
		button.focus();
		const navigate = vi.fn();
		const setError = vi.fn();
		try {
			const restart = restartProjectManager({ projectId: "proj-1", queryClient, navigate,
				setProjectRestarting: vi.fn(), setManagerReplacementError: setError });
			expect(button).toHaveFocus();
			expect(setError).not.toHaveBeenCalled();
			completeSpawn("replacement");
			await Promise.resolve();
			expect(button).toHaveFocus();
			expect(navigate).not.toHaveBeenCalled();
			completeRefresh();
			await restart;
			expect(button).not.toHaveFocus();
			expect(setError).toHaveBeenCalledWith("proj-1", null);
			expect(navigate).toHaveBeenCalledOnce();
		} finally {
			button.remove();
		}
	});

	it("still records the replacement error when workspace invalidation fails", async () => {
		const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
		vi.spyOn(queryClient, "invalidateQueries").mockRejectedValue(new Error("refetch failed"));
		const navigate = vi.fn();
		const setProjectRestarting = vi.fn();
		const setManagerReplacementError = vi.fn();
		const onError = vi.fn();
		const failure = new Error("missing goose binary");
		spawnMock.mockRejectedValue(failure);

		await restartProjectManager({
			projectId: "proj-1",
			queryClient,
			navigate,
			setProjectRestarting,
			setManagerReplacementError,
			onError,
		});

		expect(setManagerReplacementError).toHaveBeenLastCalledWith("proj-1", {
			message: "missing goose binary",
		});
		expect(setProjectRestarting).toHaveBeenLastCalledWith("proj-1", false);
		expect(onError).toHaveBeenCalledWith(failure);
		expect(navigate).not.toHaveBeenCalled();
	});

	it("preserves typed preflight details and retries with an explicit TUI mode", async () => {
		const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
		vi.spyOn(queryClient, "invalidateQueries").mockResolvedValue();
		const setManagerReplacementError = vi.fn();
		spawnMock.mockRejectedValue(
			new ManagerSpawnError(
				"Codex is unavailable",
				"CHAT_DRIVER_UNAVAILABLE",
				"request-42",
				400,
			),
		);

		await restartProjectManager({
			projectId: "proj-1",
			queryClient,
			navigate: vi.fn(),
			setProjectRestarting: vi.fn(),
			setManagerReplacementError,
			mode: "tui",
		});

		expect(spawnMock).toHaveBeenCalledWith("proj-1", "restart", true, "tui");
		expect(setManagerReplacementError).toHaveBeenLastCalledWith("proj-1", {
			message: "Codex is unavailable",
			code: "CHAT_DRIVER_UNAVAILABLE",
			requestId: "request-42",
		});
	});
});
