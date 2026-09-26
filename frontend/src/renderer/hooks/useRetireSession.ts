import { useMutation, useQueryClient } from "@tanstack/react-query";

import { apiClient, apiErrorCode, apiErrorMessage } from "../lib/api-client";
import { usesPreviewWorkspaceData } from "../lib/preview-mode";
import type { WorkspaceSummary } from "../types/workspace";
import { workspaceQueryKey } from "./useWorkspaceQuery";

/**
 * Permanently remove a finished session. The counterpart to terminate: that ends
 * a running session and keeps its record, this deletes the record of one that
 * already ended.
 *
 * The write is optimistic so the archived card leaves the board on the click
 * rather than on the round trip, and is rolled back on failure. Rollback is the
 * common case worth handling well: a session that is somehow still running comes
 * back with the daemon's explanation rather than silently vanishing.
 */
export function useRetireSession() {
	const queryClient = useQueryClient();
	return useMutation({
		mutationFn: async (sessionId: string) => {
			if (usesPreviewWorkspaceData) return;
			const { error } = await apiClient.DELETE("/api/v1/sessions/{sessionId}", {
				params: { path: { sessionId } },
			});
			if (error) {
				throw new Error(retireErrorMessage(error, sessionId));
			}
		},
		onMutate: async (sessionId: string) => {
			await queryClient.cancelQueries({ queryKey: workspaceQueryKey });
			const previous = queryClient.getQueryData<WorkspaceSummary[]>(workspaceQueryKey);
			queryClient.setQueryData<WorkspaceSummary[]>(workspaceQueryKey, (current) =>
				current?.map((workspace) => ({
					...workspace,
					sessions: workspace.sessions.filter((session) => session.id !== sessionId),
				})),
			);
			return { previous };
		},
		onError: (_error, _sessionId, context) => {
			if (context?.previous) queryClient.setQueryData(workspaceQueryKey, context.previous);
		},
		onSettled: () => {
			void queryClient.invalidateQueries({ queryKey: workspaceQueryKey });
		},
	});
}

function retireErrorMessage(error: unknown, sessionId: string): string {
	if (apiErrorCode(error) === "SESSION_NOT_TERMINATED") {
		return `${sessionId} is still running. Terminate it before removing it.`;
	}
	return apiErrorMessage(error, `Failed to remove ${sessionId}`);
}
