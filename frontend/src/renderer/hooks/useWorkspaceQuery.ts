import { useQuery, useQueryClient, type Query } from "@tanstack/react-query";
import type { TraySessionEntry } from "../../shared/tray";
import { useEffect, useMemo } from "react";
import type { components } from "../../api/schema";
import { apiClient, apiErrorCode, hasTrustedApiBaseUrl } from "../lib/api-client";
import { mockWorkspaces } from "../lib/mock-data";
import { usesPreviewWorkspaceData } from "../lib/preview-mode";
import { toReviewerHarnessId } from "../lib/reviewer-harnesses";
import {
	type PRState,
	type PullRequestFacts,
	toAgentProvider,
	toKanbanColumn,
	toProjectKind,
	toSessionActivity,
	toSessionStatus,
	newestActiveManager,
	attentionZone,
	workerSessions,
	type WorkspaceSession,
	type WorkspaceSummary,
	STANDALONE_PROJECT_KIND,
	STANDALONE_WORKSPACE_ID,
} from "../types/workspace";

const AD_HOC_AGENTS_WORKSPACE_NAME = "Ad hoc agents";

function placeStandaloneWorkspaceLast(workspaces: WorkspaceSummary[]): WorkspaceSummary[] {
	const standalone = workspaces.find((workspace) => workspace.id === STANDALONE_WORKSPACE_ID);
	if (!standalone) return workspaces;
	return [
		...workspaces.filter((workspace) => workspace.id !== STANDALONE_WORKSPACE_ID),
		standalone,
	];
}

function toPullRequestFacts(pr: components["schemas"]["SessionPRFacts"]): PullRequestFacts {
	return {
		url: pr.url,
		number: pr.number,
		state: pr.state as PRState,
		ci: pr.ci,
		review: pr.review,
		mergeability: pr.mergeability,
		reviewComments: pr.reviewComments,
		updatedAt: pr.updatedAt,
	};
}

function toWorkspaceSession(
	session: components["schemas"]["ControllersSessionView"],
	project: Pick<WorkspaceSummary, "id" | "name">,
): WorkspaceSession {
	const statusReadiness = session.statusReadiness ?? "ready";
	const status =
		statusReadiness === "ready" ? toSessionStatus(session.status, session.isTerminated) : "unknown";
	const scmStatus = session.scmStatus ? toSessionStatus(session.scmStatus) : undefined;
	const kanbanColumn = toKanbanColumn(session.kanbanColumn, status);
	const activity = statusReadiness === "ready" ? toSessionActivity(session.activity) : undefined;
	return {
		id: session.id,
		terminalHandleId: session.terminalHandleId,
		terminalGeneration: session.terminalGeneration,
		workspaceId: project.id,
		workspaceName: project.name,
		title: session.displayName ?? session.issueId ?? session.id,
		issueId: session.issueId,
		provider: toAgentProvider(session.harness),
		reviewerHarness: toReviewerHarnessId(session.reviewerHarness),
		reviewerConfig: session.reviewerConfig
			? {
				model: session.reviewerConfig.model ?? undefined,
				mode: session.reviewerConfig.mode ?? undefined,
				permissions: session.reviewerConfig.permissions ?? undefined,
			}
			: undefined,
		autoReviewEnabled: session.autoReviewEnabled ?? false,
		kind: session.kind === "manager" ? "manager" : session.kind === "worker" ? "worker" : undefined,
		mode: session.mode === "chat" ? "chat" : "tui",
		branch: session.branch || undefined,
		status,
		scmStatus,
		kanbanColumn,
		workflowMode: session.workflowMode ?? undefined,
		displayStatus: session.displayStatus || undefined,
		statusReadiness,
		isTerminated: session.isTerminated,
		chatProviderPreserved: session.chatProviderPreserved,
		terminateOnPrMerge: session.terminateOnPrMerge ?? true,
		autoInjectReview: session.autoInjectReview ?? true,
		autoInjectCI: session.autoInjectCI ?? true,
		createdAt: session.createdAt,
		updatedAt: session.updatedAt,
		lastUserMessageAt: session.lastUserMessageAt ?? undefined,
		activity,
		previewUrl: session.previewUrl,
		previewRevision: session.previewRevision,
		isPinned: session.isPinned ?? false,
		pinnedAt: session.pinnedAt ?? undefined,
		prs: (session.prs ?? []).map(toPullRequestFacts),
	};
}

export const workspaceQueryKey = ["workspaces"] as const;
export function workspaceStatusesChecking(workspaces: WorkspaceSummary[] | undefined): boolean {
	return workspaces?.some((workspace) => workspace.sessions.some((session) => session.statusReadiness === "checking")) ?? false;
}

function toLocalWorkspaceSession(
	session: components["schemas"]["ControllersSessionView"],
	workspaceId: string,
	workspaceName: string,
): WorkspaceSession {
	const status = toSessionStatus(session.status, session.isTerminated);
	const scmStatus = session.scmStatus ? toSessionStatus(session.scmStatus) : undefined;
	const kanbanColumn = toKanbanColumn(session.kanbanColumn, status);
	const activity = toSessionActivity(session.activity);
	return {
		id: session.id,
		terminalHandleId: session.terminalHandleId,
		terminalGeneration: session.terminalGeneration,
		workspaceId,
		workspaceName,
		title: session.displayName ?? session.issueId ?? session.id,
		issueId: session.issueId,
		provider: toAgentProvider(session.harness),
		reviewerHarness: toReviewerHarnessId(session.reviewerHarness),
		reviewerConfig: session.reviewerConfig ? {
			model: session.reviewerConfig.model ?? undefined,
			mode: session.reviewerConfig.mode ?? undefined,
			permissions: session.reviewerConfig.permissions ?? undefined,
		} : undefined,
		autoReviewEnabled: session.autoReviewEnabled ?? false,
		kind: session.kind === "manager" ? "manager" : session.kind === "worker" ? "worker" : undefined,
		// Carried through verbatim: the session surface must render from
		// the mode this session was created with, not from the current default.
		mode: session.mode === "chat" ? "chat" : "tui",
		branch: session.branch || undefined,
		status,
		scmStatus,
		kanbanColumn,
		workflowMode: session.workflowMode ?? undefined,
		displayStatus: session.displayStatus || undefined,
		isTerminated: session.isTerminated,
		terminateOnPrMerge: session.terminateOnPrMerge ?? true,
		autoInjectReview: session.autoInjectReview ?? true,
		autoInjectCI: session.autoInjectCI ?? true,
		createdAt: session.createdAt,
		updatedAt: session.updatedAt,
		lastUserMessageAt: session.lastUserMessageAt ?? undefined,
		activity,
		previewUrl: session.previewUrl,
		previewRevision: session.previewRevision,
		isPinned: session.isPinned ?? false,
		pinnedAt: session.pinnedAt ?? undefined,
		prs: (session.prs ?? []).map(toPullRequestFacts),
	};
}

// e2e seam (dev:web only): the Playwright fake-agent harness injects
// `window.__openAgentsFakeAgent` (see e2e/support/fake-bridge.ts) to drive a
// deterministic, mutable session timeline off the SSE refetch path. Compiled
// out of the packaged build — the packaged renderer never sets VITE_NO_ELECTRON
// and always hits the real daemon.
type FakeAgentSeam = { snapshot: () => WorkspaceSummary[] };

async function fetchWorkspaces(): Promise<WorkspaceSummary[]> {
	if (usesPreviewWorkspaceData) {
		const fake =
			typeof window !== "undefined"
				? (window as unknown as { __openAgentsFakeAgent?: FakeAgentSeam }).__openAgentsFakeAgent
				: undefined;
		return fake ? fake.snapshot() : mockWorkspaces;
	}
	if (!hasTrustedApiBaseUrl()) {
		throw new Error("Open Agents daemon API is not ready");
	}

	const [{ data: projectsData, error: projectsError }, { data: sessionsData, error: sessionsError }] =
		await Promise.all([apiClient.GET("/api/v1/projects"), apiClient.GET("/api/v1/sessions")]);

	if (projectsError || sessionsError) {
		throw projectsError ?? sessionsError;
	}

	const sessions = sessionsData?.sessions ?? [];
	const projects = (projectsData?.projects ?? []).map((project) => {
		const kind = toProjectKind(project.kind);
		return {
			id: project.id,
			name: project.name,
			kind,
			path: project.path,
			folderMissing: project.folderMissing,
			managerAgent: project.managerAgent ? toAgentProvider(project.managerAgent) : undefined,
			sessions: sessions
				.filter((session) => session.projectId === project.id)
				.map((session) => toWorkspaceSession(session, project)),
		};
	});
	const standalone: WorkspaceSummary = {
		id: STANDALONE_WORKSPACE_ID,
		name: AD_HOC_AGENTS_WORKSPACE_NAME,
		kind: STANDALONE_PROJECT_KIND,
		path: "Not attached to a project",
		sessions: sessions
			.filter((session) => !session.projectId)
			.map((session) => toLocalWorkspaceSession(session, STANDALONE_WORKSPACE_ID, AD_HOC_AGENTS_WORKSPACE_NAME)),
	};
	return standalone.sessions.length > 0 ? placeStandaloneWorkspaceLast([...projects, standalone]) : projects;
}

// Shared so route loaders can prefetch via queryClient.ensureQueryData (paired
// with the router's defaultPreload: "intent") and the hook reads the same cache.
export const workspaceQueryOptions = {
	queryKey: workspaceQueryKey,
	queryFn: fetchWorkspaces,
	retry: 1,
	staleTime: 10_000,
	refetchInterval: (query: Query<WorkspaceSummary[]>) =>
		workspaceStatusesChecking(query.state.data) ? 300 : 15_000,
};

type WorkspaceSubscriptionOptions = {
	subscribed?: boolean;
};

export function useWorkspaceQuery(options: WorkspaceSubscriptionOptions = {}) {
	const local = useQuery({ ...workspaceQueryOptions, subscribed: options.subscribed });
	return local;
}

/**
 * Subscribe a detail surface to one session instead of the complete workspace
 * tree. TanStack Query applies structural sharing to the selected value, so an
 * activity update elsewhere no longer redraws the open session workspace.
 */
export function useWorkspaceSession(sessionId: string) {
	const queryClient = useQueryClient();
	const selectLocalSession = useMemo(
		() => (workspaces: WorkspaceSummary[]) =>
			workspaces.flatMap((workspace) => workspace.sessions).find((session) => session.id === sessionId),
		[sessionId],
	);
	const local = useQuery({ ...workspaceQueryOptions, select: selectLocalSession });
	const localWorkspaces = useQuery({ ...workspaceQueryOptions, subscribed: false, enabled: Boolean(sessionId) });
	const direct = useQuery({
		queryKey: ["session", sessionId],
		enabled: Boolean(sessionId) && local.data === undefined,
		retry: (attempt, error) => apiErrorCode(error) === "SESSION_NOT_FOUND" && attempt < 4,
		retryDelay: 250,
		queryFn: async () => {
			const { data, error } = await apiClient.GET("/api/v1/sessions/{sessionId}", {
				params: { path: { sessionId } },
			});
			if (error) throw error;
			const session = data?.session;
			if (!session) return undefined;
			const project = session.projectId
				? localWorkspaces.data?.find((workspace) => workspace.id === session.projectId) ??
					({ id: session.projectId, name: "" } satisfies Pick<WorkspaceSummary, "id" | "name">)
				: ({ id: STANDALONE_WORKSPACE_ID, name: AD_HOC_AGENTS_WORKSPACE_NAME } satisfies Pick<WorkspaceSummary, "id" | "name">);
			return toWorkspaceSession(session, project);
		},
	});
	const resolvedDirectSession = useMemo(() => {
		if (!direct.data) return undefined;
		const workspace = localWorkspaces.data?.find((candidate) => candidate.id === direct.data?.workspaceId);
		if (!workspace || direct.data.workspaceName === workspace.name) return direct.data;
		return { ...direct.data, workspaceName: workspace.name };
	}, [direct.data, localWorkspaces.data]);
	useEffect(() => {
		if (!resolvedDirectSession) return;
		queryClient.setQueryData<WorkspaceSummary[]>(workspaceQueryKey, (current) => {
			if (!current) return current;
			let changed = false;
			const next = current.map((workspace) => {
				if (workspace.id !== resolvedDirectSession.workspaceId) return workspace;
				if (workspace.sessions.some((session) => session.id === resolvedDirectSession.id)) return workspace;
				changed = true;
				return { ...workspace, sessions: [...workspace.sessions, resolvedDirectSession] };
			});
			return changed ? next : current;
		});
	}, [queryClient, resolvedDirectSession]);
	return {
		...local,
		data: local.data ?? resolvedDirectSession,
		isLoading: local.isLoading || direct.isLoading,
	};
}

export type WorkspaceScope = {
	project?: Pick<WorkspaceSummary, "id" | "kind" | "name" | "managerAgent">;
	hasWorkerSessions: boolean;
	session?: WorkspaceSession;
	manager?: WorkspaceSession;
};

function selectWorkspaceScope(
	workspaces: WorkspaceSummary[],
	projectId: string | undefined,
	sessionId: string | undefined,
): WorkspaceScope {
	const session = sessionId
		? workspaces.flatMap((workspace) => workspace.sessions).find((candidate) => candidate.id === sessionId)
		: undefined;
	const resolvedProjectId = session?.workspaceId ?? projectId;
	const workspace = resolvedProjectId ? workspaces.find((candidate) => candidate.id === resolvedProjectId) : undefined;
	// Do not carry the project's complete sessions array into shell chrome. With
	// React Query's structural sharing, this small metadata projection retains
	// its identity when another session in the same project streams an update.
	const project = workspace
		? {
				id: workspace.id,
				kind: workspace.kind,
				name: workspace.name,
				managerAgent: workspace.managerAgent,
			}
		: undefined;
	return {
		project, session,
		hasWorkerSessions: workspace ? workerSessions(workspace.sessions).length > 0 : false,
		manager: workspace ? newestActiveManager(workspace.sessions) : undefined,
	};
}

/**
 * Subscribe shell chrome to just the routed project and session. This avoids
 * redrawing the topbar for streamed activity from every other project.
 */
export function useWorkspaceScope(projectId?: string, sessionId?: string) {
	const selectLocalScope = useMemo(
		() => (workspaces: WorkspaceSummary[]) => selectWorkspaceScope(workspaces, projectId, sessionId),
		[projectId, sessionId],
	);
	const local = useQuery({ ...workspaceQueryOptions, select: selectLocalScope });
	return local;
}

function selectTraySessions(workspaces: WorkspaceSummary[]): TraySessionEntry[] {
	const entries: TraySessionEntry[] = [];
	for (const workspace of workspaces) {
		for (const session of workerSessions(workspace.sessions)) {
			const zone = attentionZone(session);
			if ((zone === "merge" && session.status === "merged") || (zone !== "action" && zone !== "merge")) continue;
			entries.push({
				projectId: session.workspaceId,
				projectName: workspace.name,
				sessionId: session.id,
				title: session.title,
				zone,
			});
		}
	}
	return entries;
}

/**
 * The tray lives for the whole app lifetime, but only attention-worthy worker
 * sessions affect its native payload. Select that compact projection at the
 * query boundary so ordinary streamed activity does not wake the runtime.
 */
export function useWorkspaceTraySessions() {
	const local = useQuery({ ...workspaceQueryOptions, select: selectTraySessions });
	return local;
}
