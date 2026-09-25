import { useEffect, useRef } from "react";
import { useMutation, useMutationState, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { hasConfiguredManagerAgent, type WorkspaceSession } from "../types/workspace";
import { workspaceQueryKey, type WorkspaceScope } from "./useWorkspaceQuery";
import { isChatPreflightError, spawnManager, type ManagerSpawnSource } from "../lib/spawn-manager";
import { formatManagerStartupError } from "../lib/manager-startup-error";
import { useUiStore } from "../stores/ui-store";

export function useProjectManagerAction({
	projectId,
	project,
	manager,
	source,
	sessionId,
}: {
	projectId?: string;
	project?: WorkspaceScope["project"];
	manager?: WorkspaceSession;
	source: ManagerSpawnSource;
	sessionId?: string;
}) {
	const navigate = useNavigate();
	const queryClient = useQueryClient();
	const mutationKey = ["project-manager-open", projectId] as const;
	const routeKey = `${projectId ?? ""}/${sessionId ?? ""}`;
	const activeRoute = useRef<string | null>(routeKey);
	activeRoute.current = routeKey;
	useEffect(() => {
		activeRoute.current = routeKey;
		return () => { activeRoute.current = null; };
	}, [routeKey]);
	const isProjectRestarting = useUiStore((state) => projectId ? state.restartingProjectIds.has(projectId) : false);
	const isProvisioning = useUiStore((state) => projectId ? state.provisioningProjectIds.has(projectId) : false);
	const startupError = useUiStore((state) => projectId ? state.managerStartupErrors[projectId] : undefined);
	const setStartupError = useUiStore((state) => state.setManagerStartupError);
	const previousProjectId = useRef(projectId);
	useEffect(() => {
		if (previousProjectId.current && previousProjectId.current !== projectId) {
			setStartupError(previousProjectId.current, null);
		}
		previousProjectId.current = projectId;
	}, [projectId, setStartupError]);
	useEffect(() => {
		if (projectId && manager && startupError) setStartupError(projectId, null);
	}, [projectId, manager, startupError, setStartupError]);
	const mutations = useMutationState({
		filters: { mutationKey, exact: true },
		select: (mutation) => ({ status: mutation.state.status, error: mutation.state.error }),
	});
	const isSpawning = mutations.some((mutation) => mutation.status === "pending");
	const latest = mutations.at(-1);
	const error = !manager && !isSpawning && latest?.status === "error" ? latest.error : null;
	const spawnError = formatManagerStartupError(
		error ? (error instanceof Error ? error.message : "Could not spawn manager") : startupError ?? "",
	);
	const mutation = useMutation({
		mutationKey,
		mutationFn: async (mode?: "tui") => {
			if (!projectId) return;
			setStartupError(projectId, null);
			const openedSessionId = await spawnManager(projectId, source, false, mode);
			await queryClient.invalidateQueries({ queryKey: workspaceQueryKey });
			setStartupError(projectId, null);
			// A completed request belongs to its original route, even if this
			// component survived a project or session change while it was pending.
			if (activeRoute.current === routeKey) {
				void navigate({
					to: "/projects/$projectId/sessions/$sessionId",
					params: { projectId, sessionId: openedSessionId },
				});
			}
		},
	});
	const openManager = (mode?: "tui") => {
		if (!projectId || isProjectRestarting || isProvisioning) return;
		// Read the cache synchronously as well as disabling both rendered copies.
		// Two clicks in the same render must still produce just one request.
		if (queryClient.isMutating({ mutationKey, exact: true })) return;
		if (manager) {
			void navigate({ to: "/projects/$projectId/sessions/$sessionId", params: { projectId, sessionId: manager.id } });
		} else if (!hasConfiguredManagerAgent(project)) {
			if (project) useUiStore.getState().openProjectSettings(projectId);
		} else {
			mutation.mutate(mode);
		}
	};
	const openNewTask = () => {
		if (projectId && !isProjectRestarting && !isProvisioning) useUiStore.getState().requestNewTask(projectId);
	};
	return { manager, isSpawning, isProjectRestarting, isProvisioning, spawnError,
		canCreateAsTui: isChatPreflightError(error), openManager, openNewTask };
}

export type ProjectManagerAction = ReturnType<typeof useProjectManagerAction>;
