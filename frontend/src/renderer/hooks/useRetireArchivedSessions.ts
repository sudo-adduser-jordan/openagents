import { useMutation, useQueryClient } from "@tanstack/react-query";

import { apiClient, apiErrorCode, apiErrorMessage } from "../lib/api-client";
import { usesPreviewWorkspaceData } from "../lib/preview-mode";
import { workspaceQueryKey } from "./useWorkspaceQuery";

export type RetireArchivedSessionsResult = {
	/** Sessions the daemon confirmed removed, plus ones it reported already gone. */
	removed: string[];
	failed: { sessionId: string; message: string }[];
};

/**
 * Permanently retire every archived session, the bulk counterpart to
 * useRetireSession. The per-card button is the right affordance for one card;
 * this is the one for "clear the archive", which is otherwise a dozen
 * confirmations in a row.
 *
 * Requests run sequentially rather than in parallel. The daemon serializes
 * writes on a single SQLite handle anyway, so a fan-out buys no wall clock and
 * costs a summary that arrives in nondeterministic order. Sequential also means
 * a failure partway through leaves the rest in a predictable order for the
 * summary to report.
 *
 * One failure never aborts the run: a session that is somehow still running, or
 * one the daemon is mid-teardown on, is recorded and the sweep continues. The
 * alternative -- stopping at the first error -- would leave a bulk clear in a
 * state the user has to reason about and retry by hand.
 *
 * Already-gone counts as done. The retire path never touches the workspace, so
 * a session whose row is absent is a benign 200 with freed=false, and 404 is
 * tolerated for the same reason. Treating either as a failure would make a
 * retried clear report errors that no longer correspond to anything.
 */
export function useRetireArchivedSessions() {
	const queryClient = useQueryClient();
	return useMutation({
		mutationFn: async (sessionIds: string[]): Promise<RetireArchivedSessionsResult> => {
			if (usesPreviewWorkspaceData) return { removed: [], failed: [] };
			const removed: string[] = [];
			const failed: { sessionId: string; message: string }[] = [];
			for (const sessionId of sessionIds) {
				// A transport failure (daemon restarting, socket closed) rejects
				// the call instead of returning an error envelope. Without this
				// the throw escapes mutationFn, the sweep stops on that session,
				// and the remaining ones are never attempted.
				try {
					const { error } = await apiClient.DELETE("/api/v1/sessions/{sessionId}", {
						params: { path: { sessionId } },
					});
					if (error) {
						if (apiErrorCode(error) === "SESSION_NOT_FOUND") {
							removed.push(sessionId);
							continue;
						}
						failed.push({
							sessionId,
							message: apiErrorMessage(error, `Failed to remove ${sessionId}`),
						});
						continue;
					}
				} catch {
					failed.push({
						sessionId,
						message: "Could not reach the daemon",
					});
					continue;
				}
				// freed=false is the daemon's already-gone answer: nothing was
				// there to remove, which is the outcome the user asked for, so
				// it lands in removed alongside a normal removal.
				removed.push(sessionId);
			}
			return { removed, failed };
		},
		// A single invalidate for the whole sweep. The per-session hook
		// invalidates once per removal; doing that N times for one user action
		// is N refetches of the entire workspace list.
		onSettled: () => {
			void queryClient.invalidateQueries({ queryKey: workspaceQueryKey });
		},
	});
}
