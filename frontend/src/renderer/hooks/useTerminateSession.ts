import { type QueryClient, useMutation, useMutationState, useQueryClient } from "@tanstack/react-query";
import { toKanbanColumn, type WorkspaceSession, type WorkspaceSummary } from "../types/workspace";
import { workspaceQueryKey } from "./useWorkspaceQuery";
import { apiClient, apiErrorMessage } from "../lib/api-client";

type TerminateSessionOptions = {
	onSuccess?: (session: WorkspaceSession) => void;
};

export const terminateSessionMutationKey = ["terminate-session"] as const;

async function terminateSession(session: WorkspaceSession): Promise<void> {
	const { error, response } = await apiClient.POST("/api/v1/sessions/{sessionId}/kill", {
		params: { path: { sessionId: session.id } },
	});
	if (error) {
		const fallback = response ? `Failed to terminate session (${response.status})` : "Failed to terminate session";
		throw new Error(apiErrorMessage(error, fallback));
	}
}

// A killed session keeps its row and flips to terminated, which is exactly what
// the next workspace fetch would report. Applying it locally lets the board
// settle on the click rather than on the refetch.
function markTerminated(sessionId: string) {
	return (session: WorkspaceSession): WorkspaceSession =>
		session.id === sessionId
			? {
				...session,
				isTerminated: true,
				status: "terminated",
				kanbanColumn: toKanbanColumn(undefined, "terminated"),
			}
			: session;
}

// Snapshot the workspace cache entry so onError can roll back.
type WorkspaceSnapshot = [readonly unknown[], WorkspaceSummary[] | undefined];

type TerminateMutationContext = {
	workspace: WorkspaceSnapshot | undefined;
};

type TerminateSessionMutationState = {
	error: unknown;
	session?: WorkspaceSession;
	status: "error" | "idle" | "pending" | "success";
	submittedAt: number;
};

function useTerminateSessionMutations() {
	return useMutationState<TerminateSessionMutationState>({
		filters: { mutationKey: terminateSessionMutationKey },
		select: (mutation) => ({
			error: mutation.state.error,
			session: mutation.state.variables as WorkspaceSession | undefined,
			status: mutation.state.status,
			submittedAt: mutation.state.submittedAt,
		}),
	});
}

function summarizeBySession(mutations: TerminateSessionMutationState[]) {
	const summaries = new Map<
		string,
		{ isPending: boolean; latest: TerminateSessionMutationState; session: WorkspaceSession }
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

export function useTerminateSession(options: TerminateSessionOptions = {}) {
	const queryClient = useQueryClient();
	return useMutation({
		mutationKey: terminateSessionMutationKey,
		mutationFn: async (session: WorkspaceSession) => {
			await terminateSession(session);
		},
// Archive the card on the click, not on the round trip: a card that does
		// not move reads as "the click did nothing". Roll the optimistic write
		// back in onError.
		onMutate: async (session): Promise<TerminateMutationContext> => {
			await queryClient.cancelQueries({ queryKey: workspaceQueryKey });
			const workspace: WorkspaceSnapshot = [
				workspaceQueryKey,
				queryClient.getQueryData<WorkspaceSummary[]>(workspaceQueryKey),
			];
			queryClient.setQueryData<WorkspaceSummary[]>(workspaceQueryKey, (workspaces) =>
				workspaces?.map((ws) =>
					ws.id === session.workspaceId
						? { ...ws, sessions: ws.sessions.map(markTerminated(session.id)) }
						: ws,
				),
			);
			return { workspace };
		},
		onSuccess: (_data, session) => {
			// The optimistic write already settled the board; refresh in the
			// background to reconcile with the daemon's real state.
			void queryClient.invalidateQueries({ queryKey: workspaceQueryKey });
			options.onSuccess?.(session);
		},
		onError: (_error, _session, context) => {
			// Restore the pre-mutation snapshot so a failed kill un-archives the card
			// rather than leaving it wrongly terminated.
			const ctx = context as TerminateMutationContext | undefined;
			if (ctx?.workspace) queryClient.setQueryData(ctx.workspace[0], ctx.workspace[1]);
		},
	});
}

export function useTerminateSessionState(sessionId: string) {
	const summary = summarizeBySession(useTerminateSessionMutations()).find(({ session }) => session.id === sessionId);

	return {
		error:
			!summary?.isPending && summary?.latest.status === "error" && summary.latest.error instanceof Error
				? summary.latest.error.message
				: null,
		isPending: summary?.isPending ?? false,
	};
}

export function useProjectTerminateSessionStates(workspaceId: string | undefined) {
	return summarizeBySession(useTerminateSessionMutations())
		.filter(({ isPending, latest, session }) => {
			return session.workspaceId === workspaceId && (isPending || latest.status === "error");
		})
		.sort((a, b) => b.latest.submittedAt - a.latest.submittedAt)
		.map(({ isPending, latest, session }) => ({
			error: !isPending && latest.error instanceof Error ? latest.error.message : null,
			isPending,
			session,
		}));
}

export function clearTerminateSessionState(queryClient: QueryClient, sessionId: string) {
	const mutationCache = queryClient.getMutationCache();
	for (const mutation of mutationCache.findAll({ mutationKey: terminateSessionMutationKey })) {
		const target = mutation.state.variables as WorkspaceSession | undefined;
		if (target?.id === sessionId && mutation.state.status !== "pending") {
			mutationCache.remove(mutation);
		}
	}
}
