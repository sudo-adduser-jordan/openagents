import { useEffect, useRef } from "react";
import { useMutation, useMutationState, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { hasConfiguredOrchestratorAgent, type WorkspaceSession } from "../types/workspace";
import { workspaceQueryKey, type WorkspaceScope } from "./useWorkspaceQuery";
import { isChatPreflightError, spawnOrchestrator, type OrchestratorSpawnSource } from "../lib/spawn-orchestrator";
import { formatOrchestratorStartupError } from "../lib/orchestrator-startup-error";
import { useUiStore } from "../stores/ui-store";

export function useProjectOrchestratorAction({
	projectId,
	project,
	orchestrator,
	source,
	sessionId,
}: {
	projectId?: string;
	project?: WorkspaceScope["project"];
	orchestrator?: WorkspaceSession;
	source: OrchestratorSpawnSource;
	sessionId?: string;
}) {
	const navigate = useNavigate();
	const queryClient = useQueryClient();
	const mutationKey = ["project-orchestrator-open", projectId] as const;
	const routeKey = `${projectId ?? ""}/${sessionId ?? ""}`;
	const activeRoute = useRef<string | null>(routeKey);
	activeRoute.current = routeKey;
	useEffect(() => {
		activeRoute.current = routeKey;
		return () => { activeRoute.current = null; };
	}, [routeKey]);
	const isProjectRestarting = useUiStore((state) => projectId ? state.restartingProjectIds.has(projectId) : false);
	const isProvisioning = useUiStore((state) => projectId ? state.provisioningProjectIds.has(projectId) : false);
	const startupError = useUiStore((state) => projectId ? state.orchestratorStartupErrors[projectId] : undefined);
	const setStartupError = useUiStore((state) => state.setOrchestratorStartupError);
	const previousProjectId = useRef(projectId);
	useEffect(() => {
		if (previousProjectId.current && previousProjectId.current !== projectId) {
			setStartupError(previousProjectId.current, null);
		}
		previousProjectId.current = projectId;
	}, [projectId, setStartupError]);
	useEffect(() => {
		if (projectId && orchestrator && startupError) setStartupError(projectId, null);
	}, [projectId, orchestrator, startupError, setStartupError]);
	const mutations = useMutationState({
		filters: { mutationKey, exact: true },
		select: (mutation) => ({ status: mutation.state.status, error: mutation.state.error }),
	});
	const isSpawning = mutations.some((mutation) => mutation.status === "pending");
	const latest = mutations.at(-1);
	const error = !orchestrator && !isSpawning && latest?.status === "error" ? latest.error : null;
	const spawnError = formatOrchestratorStartupError(
		error ? (error instanceof Error ? error.message : "Could not spawn orchestrator") : startupError ?? "",
	);
	const mutation = useMutation({
		mutationKey,
		mutationFn: async (mode?: "tui") => {
			if (!projectId) return;
			setStartupError(projectId, null);
			const openedSessionId = await spawnOrchestrator(projectId, source, false, mode);
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
	const openOrchestrator = (mode?: "tui") => {
		if (!projectId || isProjectRestarting || isProvisioning) return;
		// Read the cache synchronously as well as disabling both rendered copies.
		// Two clicks in the same render must still produce just one request.
		if (queryClient.isMutating({ mutationKey, exact: true })) return;
		if (orchestrator) {
			void navigate({ to: "/projects/$projectId/sessions/$sessionId", params: { projectId, sessionId: orchestrator.id } });
		} else if (!hasConfiguredOrchestratorAgent(project)) {
			if (project) useUiStore.getState().openProjectSettings(projectId);
		} else {
			mutation.mutate(mode);
		}
	};
	const openNewTask = () => {
		if (projectId && !isProjectRestarting && !isProvisioning) useUiStore.getState().requestNewTask(projectId);
	};
	return { orchestrator, isSpawning, isProjectRestarting, isProvisioning, spawnError,
		canCreateAsTui: isChatPreflightError(error), openOrchestrator, openNewTask };
}

export type ProjectOrchestratorAction = ReturnType<typeof useProjectOrchestratorAction>;
