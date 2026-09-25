import type { QueryClient } from "@tanstack/react-query";
import { workspaceQueryKey } from "../hooks/useWorkspaceQuery";
import type { SessionMode } from "../types/conversation";
import { ManagerSpawnError, spawnManager } from "./spawn-manager";
import type { ManagerReplacementFailure } from "../stores/ui-store";

type NavigateToSession = (options: {
	to: "/projects/$projectId/sessions/$sessionId";
	params: { projectId: string; sessionId: string };
}) => unknown;

type RestartProjectManagerOptions = {
	projectId: string;
	queryClient: QueryClient;
	navigate: NavigateToSession;
	setProjectRestarting: (projectId: string, restarting: boolean) => void;
	setManagerReplacementError: (projectId: string, failure: ManagerReplacementFailure | null) => void;
	onError?: (error: unknown) => void;
	mode?: SessionMode;
};

async function refreshWorkspaceState(queryClient: QueryClient) {
	try {
		await queryClient.invalidateQueries({ queryKey: workspaceQueryKey });
	} catch {
		// The restart outcome is more important than cache refresh bookkeeping:
		// callers still need navigation/error state even if refetching fails.
	}
}

export async function restartProjectManager({
	projectId,
	queryClient,
	navigate,
	setProjectRestarting,
	setManagerReplacementError,
	onError,
	mode,
}: RestartProjectManagerOptions) {
	// Keep the initiating control focused while the restart is pending so
	// keyboard users retain a focus target for the duration of the operation;
	// blur it only once navigation to the replacement session is about to
	// happen. On failure the control stays focused and the error dialog takes
	// focus normally.
	const activeElement = document.activeElement;
	setProjectRestarting(projectId, true);
	// Keep any replacement-error dialog mounted so Retry retains focus while pending.
	try {
		const sessionId = await spawnManager(projectId, "restart", true, mode);
		await refreshWorkspaceState(queryClient);
		setManagerReplacementError(projectId, null);
		if (activeElement instanceof HTMLElement) activeElement.blur();
		void navigate({
			to: "/projects/$projectId/sessions/$sessionId",
			params: { projectId, sessionId },
		});
	} catch (error) {
		await refreshWorkspaceState(queryClient);
		setManagerReplacementError(projectId, {
			message: error instanceof Error ? error.message : "Could not replace manager",
			...(error instanceof ManagerSpawnError
				? { code: error.code, requestId: error.requestId }
				: {}),
		});
		onError?.(error);
	} finally {
		setProjectRestarting(projectId, false);
	}
}
