import { type QueryClient, useMutation, useMutationState, useQueryClient } from "@tanstack/react-query";
import { apiClient, apiErrorMessage } from "../lib/api-client";
import { usesPreviewWorkspaceData } from "../lib/preview-mode";
import { primaryPR } from "../types/workspace";
import type { WorkspaceSession } from "../types/workspace";
import { workspaceQueryKey } from "./useWorkspaceQuery";

export const createSessionPRMutationKey = ["create-session-pr"] as const;

export type CreateSessionPRResult = {
	prUrl: string;
	prNumber?: number;
	created?: boolean;
};

/**
 * Push a Ready session's branch to origin and open exactly one pull request
 * against dev, returning its URL. The daemon de-duplicates (durable facts,
 * then the provider listing, then the creation race), so a double-click can
 * never create two PRs — but the button still disables while pending. The
 * session stays alive for review; only the local-merge button ends it.
 */
export function useCreateSessionPR() {
	const queryClient = useQueryClient();
	return useMutation({
		mutationKey: createSessionPRMutationKey,
		mutationFn: async (session: WorkspaceSession): Promise<CreateSessionPRResult> => {
			if (usesPreviewWorkspaceData) {
				return { prUrl: primaryPR(session)?.url ?? "", created: false };
			}
			const { data, error, response } = await apiClient.POST("/api/v1/sessions/{sessionId}/pr", {
				params: { path: { sessionId: session.id } },
			});
			if (error || !data) {
				const fallback = response
					? `Failed to open a pull request for ${session.title} (${response.status})`
					: `Failed to open a pull request for ${session.title}`;
				throw new Error(apiErrorMessage(error, fallback));
			}
			return data;
		},
		onSuccess: () => {
			void queryClient.invalidateQueries({ queryKey: workspaceQueryKey });
		},
	});
}

type CreateSessionPRMutationState = {
	error: unknown;
	session?: WorkspaceSession;
	status: "error" | "idle" | "pending" | "success";
	submittedAt: number;
};

function summarizeBySession(mutations: CreateSessionPRMutationState[]) {
	const summaries = new Map<
		string,
		{ isPending: boolean; latest: CreateSessionPRMutationState; session: WorkspaceSession }
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

/** Pending/error state for one card's Open-PR action. */
export function useCreateSessionPRState(sessionId: string) {
	const summary = summarizeBySession(
		useMutationState<CreateSessionPRMutationState>({
			filters: { mutationKey: createSessionPRMutationKey },
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

export function clearCreateSessionPRState(queryClient: QueryClient, sessionId: string) {
	const mutationCache = queryClient.getMutationCache();
	for (const mutation of mutationCache.findAll({ mutationKey: createSessionPRMutationKey })) {
		const target = mutation.state.variables as WorkspaceSession | undefined;
		if (target?.id === sessionId && mutation.state.status !== "pending") {
			mutationCache.remove(mutation);
		}
	}
}
