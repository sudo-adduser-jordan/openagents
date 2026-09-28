import { type QueryClient, useMutation, useMutationState, useQueryClient } from "@tanstack/react-query";
import { apiClient, apiErrorMessage } from "../lib/api-client";
import { usesPreviewWorkspaceData } from "../lib/preview-mode";
import type { WorkspaceSession } from "../types/workspace";
import { workspaceQueryKey } from "./useWorkspaceQuery";

export const mergeSessionLocalMutationKey = ["merge-session-local"] as const;

export type MergeSessionLocalResult = {
	prUrl?: string;
	targetBranch?: string;
	targetHeadSha?: string;
	alreadyMerged?: boolean;
	branchRemoved?: boolean;
};

/**
 * Merge a Ready session's branch into the local dev checkout. On verified
 * success the daemon removes the branch and terminates the session, so the
 * card settles into archive on the next fetch. Any failure stops before
 * termination and surfaces on the card — the mutation is never optimistic.
 */
export function useMergeSessionLocal() {
	const queryClient = useQueryClient();
	return useMutation({
		mutationKey: mergeSessionLocalMutationKey,
		mutationFn: async (session: WorkspaceSession): Promise<MergeSessionLocalResult> => {
			if (usesPreviewWorkspaceData) return {};
			const { data, error, response } = await apiClient.POST("/api/v1/sessions/{sessionId}/merge-local", {
				params: { path: { sessionId: session.id } },
			});
			if (error || !data) {
				const fallback = response
					? `Failed to merge ${session.title} into dev (${response.status})`
					: `Failed to merge ${session.title} into dev`;
				throw new Error(apiErrorMessage(error, fallback));
			}
			return data;
		},
		onSuccess: () => {
			void queryClient.invalidateQueries({ queryKey: workspaceQueryKey });
		},
	});
}

type MergeSessionLocalMutationState = {
	error: unknown;
	session?: WorkspaceSession;
	status: "error" | "idle" | "pending" | "success";
	submittedAt: number;
};

function summarizeBySession(mutations: MergeSessionLocalMutationState[]) {
	const summaries = new Map<
		string,
		{ isPending: boolean; latest: MergeSessionLocalMutationState; session: WorkspaceSession }
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

/** Pending/error state for one card's Merge-local action. */
export function useMergeSessionLocalState(sessionId: string) {
	const summary = summarizeBySession(
		useMutationState<MergeSessionLocalMutationState>({
			filters: { mutationKey: mergeSessionLocalMutationKey },
			select: (mutation) => ({
				error: mutation.state.error,
				session: mutation.state.variables as WorkspaceSession | undefined,
				status: mutation.state.status,
				submittedAt: mutation.state.submittedAt,
			}),
		}),
	).find(({ session }) => session.id === sessionId);

	return {
		error:
			!summary?.isPending && summary?.latest.status === "error" && summary.latest.error instanceof Error
				? summary.latest.error.message
				: null,
		isPending: summary?.isPending ?? false,
	};
}

export function clearMergeSessionLocalState(queryClient: QueryClient, sessionId: string) {
	const mutationCache = queryClient.getMutationCache();
	for (const mutation of mutationCache.findAll({ mutationKey: mergeSessionLocalMutationKey })) {
		const target = mutation.state.variables as WorkspaceSession | undefined;
		if (target?.id === sessionId && mutation.state.status !== "pending") {
			mutationCache.remove(mutation);
		}
	}
}
