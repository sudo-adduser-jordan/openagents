import { type QueryClient, useMutation, useMutationState, useQueryClient } from "@tanstack/react-query";
import { apiClient, apiErrorCode, apiErrorMessage } from "../lib/api-client";
import { usesPreviewWorkspaceData } from "../lib/preview-mode";
import type { WorkspaceSession } from "../types/workspace";
import { workspaceQueryKey } from "./useWorkspaceQuery";

export const discardReadySessionMutationKey = ["discard-ready-session"] as const;

export type DiscardReadySessionResult = {
	/** Kill reported freed=false: the session terminated but its worktree was preserved. */
	workspacePreserved: boolean;
};

/**
 * Discard a finished Ready-lane card: terminate the session if it is still
 * alive, then retire its record.
 *
 * No new endpoint: this chains the existing kill + retire pair. Kill never
 * force-deletes a dirty worktree (it terminates and reports freed=false with
 * the worktree left for inspection), and retire never touches the disk at
 * all -- it deletes the row, its change log, and the PR facts and
 * conversation turns that cascade from it, retiring the session number so it
 * is never reused. Branch refs are untouched; only merge-local removes a
 * branch. A kill failure stops before retire and surfaces on the card.
 */
export function useDiscardReadySession() {
	const queryClient = useQueryClient();
	return useMutation({
		mutationKey: discardReadySessionMutationKey,
		mutationFn: async (session: WorkspaceSession): Promise<DiscardReadySessionResult> => {
			if (usesPreviewWorkspaceData) return { workspacePreserved: false };
			let workspacePreserved = false;
			if (session.isTerminated !== true && session.status !== "terminated") {
				const { data, error, response } = await apiClient.POST("/api/v1/sessions/{sessionId}/kill", {
					params: { path: { sessionId: session.id } },
				});
				if (error) {
					const fallback = response
						? `Failed to terminate ${session.title} (${response.status})`
						: `Failed to terminate ${session.title}`;
					throw new Error(apiErrorMessage(error, fallback));
				}
				workspacePreserved = (data as { freed?: boolean } | undefined)?.freed === false;
			}
			const { error: retireError } = await apiClient.DELETE("/api/v1/sessions/{sessionId}", {
				params: { path: { sessionId: session.id } },
			});
			if (retireError) {
				if (apiErrorCode(retireError) === "SESSION_NOT_FOUND") return { workspacePreserved };
				if (apiErrorCode(retireError) === "SESSION_NOT_TERMINATED") {
					throw new Error(`${session.id} is still running. Terminate it before removing it.`);
				}
				throw new Error(apiErrorMessage(retireError, `Failed to remove ${session.title}`));
			}
			return { workspacePreserved };
		},
		onSuccess: () => {
			void queryClient.invalidateQueries({ queryKey: workspaceQueryKey });
		},
	});
}

type DiscardReadySessionMutationState = {
	error: unknown;
	session?: WorkspaceSession;
	status: "error" | "idle" | "pending" | "success";
	submittedAt: number;
};

/** Pending/error state for one ready card's Delete action. */
export function useDiscardReadySessionState(sessionId: string) {
	const mutations = useMutationState<DiscardReadySessionMutationState>({
		filters: { mutationKey: discardReadySessionMutationKey },
		select: (mutation) => ({
			error: mutation.state.error,
			session: mutation.state.variables as WorkspaceSession | undefined,
			status: mutation.state.status,
			submittedAt: mutation.state.submittedAt,
		}),
	});
	const summary = summarizeBySession(mutations).find(({ session }) => session.id === sessionId);

	return {
		error:
			!summary?.isPending && summary?.latest.status === "error" && summary.latest.error instanceof Error
				? summary.latest.error.message
				: null,
		isPending: summary?.isPending ?? false,
	};
}

export function clearDiscardReadySessionState(queryClient: QueryClient, sessionId: string) {
	const mutationCache = queryClient.getMutationCache();
	for (const mutation of mutationCache.findAll({ mutationKey: discardReadySessionMutationKey })) {
		const target = mutation.state.variables as WorkspaceSession | undefined;
		if (target?.id === sessionId && mutation.state.status !== "pending") {
			mutationCache.remove(mutation);
		}
	}
}

function summarizeBySession(mutations: DiscardReadySessionMutationState[]) {
	const summaries = new Map<
		string,
		{ isPending: boolean; latest: DiscardReadySessionMutationState; session: WorkspaceSession }
	>();
	for (const mutation of mutations) {
		if (!mutation.session) continue;
		const current = summaries.get(mutation.session.id);
		if (!current) {
			summaries.set(mutation.session.id, {
				isPending: mutation.status === "pending",
				latest: mutation,
				session: mutation.session,
			});
			continue;
		}
		current.isPending ||= mutation.status === "pending";
		if (mutation.submittedAt >= current.latest.submittedAt) current.latest = mutation;
	}
	return [...summaries.values()];
}
