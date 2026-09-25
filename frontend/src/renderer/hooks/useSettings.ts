/**
 * Daemon-owned user preferences.
 *
 * Read from and written to the daemon rather than held in the renderer, because
 * `open-agents spawn`, mobile, and headless spawns resolve the same value. A preference
 * kept in one client would look correct in Settings and disagree with the others.
 */

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiClient, apiErrorMessage } from "../lib/api-client";
import type { SessionMode } from "../types/workspace";

export const settingsQueryKey = ["settings"] as const;

export interface Settings {
	/** Applies to sessions created from now on; never to an existing one. */
	defaultSessionMode: SessionMode;
	/** Agents that can run in chat mode today. Empty means chat is unavailable. */
	chatHarnesses: string[];
}

export function useSettings() {
	const query = useQuery({
		queryKey: settingsQueryKey,
		// Settings must recover from a transient startup failure. The daemon
		// can still be booting on first fetch ("Open Agents daemon is starting");
		// without a refetch the settings stay stale until a manual reload.
		// Poll like the workspace query so it self-heals once the daemon is ready.
		refetchInterval: 15_000,
		retry: 5,
		queryFn: async (): Promise<Settings> => {
			const { data, error } = await apiClient.GET("/api/v1/settings");
			if (error) throw error;
			return {
				defaultSessionMode: (data?.defaultSessionMode ?? "tui") as SessionMode,
				chatHarnesses: data?.chatHarnesses ?? [],
			};
		},
	});

	return {
		settings: query.data,
		isLoading: query.isLoading,
		error: query.error ? apiErrorMessage(query.error) : undefined,
	};
}

export function useUpdateSessionInterface() {
	const queryClient = useQueryClient();
	const mutation = useMutation({
		mutationFn: async (mode: SessionMode) => {
			const { data, error } = await apiClient.PATCH("/api/v1/settings/session-interface", {
				body: { defaultSessionMode: mode },
			});
			if (error) throw error;
			return data;
		},
		// Refetch rather than writing the value in locally: the daemon is the source
		// of truth, and the control must not claim a change it did not persist.
		onSuccess: () => queryClient.invalidateQueries({ queryKey: settingsQueryKey }),
	});

	return {
		update: (mode: SessionMode) => mutation.mutate(mode),
		saving: mutation.isPending,
		error: mutation.error ? apiErrorMessage(mutation.error) : undefined,
	};
}
