import { memo, useCallback, useEffect, useRef, useState, type MouseEvent } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import {
	SessionsArchiveView,
	SessionsBoardGridView,
	archiveToggleOffsetClassName,
	resolveWorkflowMode,
} from "@openagents/product-ui";
import { AlertTriangle, LayoutDashboard, RotateCw } from "lucide-react";
import {
	isManagerSession,
	type WorkflowMode,
	type WorkspaceSession,
	newestActiveManager,
	managerHealth,
	workerSessions,
} from "../types/workspace";
import {
	boardLaneOrder,
	getBoardLaneView,
	type BoardLaneView,
} from "../lib/session-presentation";
import {
	useSessionUsageSummaries,
	type SessionUsageSummary,
} from "../hooks/useSessionUsageSummaries";
import { apiErrorMessage } from "../lib/api-client";
import { useRetireSession } from "../hooks/useRetireSession";
import { useRetireArchivedSessions } from "../hooks/useRetireArchivedSessions";
import { useRestoreSession } from "../hooks/useRestoreSession";
import { useTerminateSession } from "../hooks/useTerminateSession";
import { useDiscardReadySession } from "../hooks/useDiscardReadySession";
import { useMergeSessionLocal } from "../hooks/useMergeSessionLocal";
import { useCreateSessionPR } from "../hooks/useCreateSessionPR";
import { useWorkspaceQuery, workspaceQueryKey } from "../hooks/useWorkspaceQuery";
import { useSetWorkflowMode } from "../hooks/useSetWorkflowMode";
import { apiClient } from "../lib/api-client";
import type { components } from "../../api/schema";
import { NotificationCenter } from "./NotificationCenter";
import { BoardWelcome, ProjectBoardEmpty } from "./BoardEmptyStates";
import { TopbarButton, topbarProjectLabelClass } from "./TopbarButton";
import { restartProjectManager } from "../lib/restart-manager";
import { usesPreviewWorkspaceData } from "../lib/preview-mode";
import { demoBoardSessions } from "../lib/demo-board-sessions";
import { isLinuxPlatform, isMacPlatform, usesBoardActionsInPanel } from "../lib/platform";
import { cn } from "../lib/utils";
import { openAgentsBridge } from "../lib/bridge";
import { primaryPR } from "../types/workspace";
import { useUiStore } from "../stores/ui-store";
import { RestoreUnavailableDialog } from "./RestoreUnavailableDialog";
import { ConfirmDialog } from "./ConfirmDialog";
import { archiveRemoveWarning } from "./ArchiveRemoveButton";
import { DaemonStartupLoader } from "./DaemonStartupLoader";
import { useBoardPresentation } from "../hooks/useBoardPresentation";
import { useProjectManagerAction } from "../hooks/useProjectManagerAction";
import { ProjectBoardActions } from "./ProjectBoardActions";
import {
	ArchivedSessionCardAdapter,
	BoardSessionCardAdapter,
	sessionsBoardLabels,
} from "./SessionsBoardAdapters";

type SessionsBoardProps = {
	/** When set, the board shows only this project's sessions. */
	projectId?: string;
};

type UsageBySession = ReadonlyMap<string, SessionUsageSummary>;
const emptyUsageBySession: UsageBySession = new Map();

// Live merged sessions remain in-flow. A terminated runtime is archived even
// when its SCM outcome remains `merged`, which is exactly what the daemon's
// `archive` column means.
function isArchivedSession(session: WorkspaceSession): boolean {
	return (
		session.kanbanColumn === "archive" ||
		session.isTerminated === true ||
		session.status === "terminated"
	);
}

type WireConversationActivity = components["schemas"]["ConversationActivityResponse"];
type WireConversationSnapshot = components["schemas"]["ConversationSnapshotResponse"];

/** The wire-format pending approval, for the card's one-shot review action. */
function pendingWireApproval(snapshot: WireConversationSnapshot): WireConversationActivity | undefined {
	return (snapshot.activities ?? []).find(
		(activity) => activity.activityKind === "approval" && activity.status === "pending",
	);
}

/**
 * The wire-format accept decision for "Commit", mirroring the
 * composer's allow-once preference and the ApprovalCard fallbacks.
 */
function wireAllowOnceDecision(detail: Record<string, unknown> | undefined): { id: string } | undefined {
	const raw = detail?.decisions;
	if (!Array.isArray(raw)) return undefined;
	const options = raw.filter(
		(entry): entry is Record<string, unknown> => Boolean(entry) && typeof entry === "object",
	);
	const picked =
		options.find((entry) => entry.kind === "allow_once") ??
		options.find((entry) => typeof entry.id === "string" && /(allow|approve|accept)/i.test(entry.id)) ??
		options[0];
	return picked && typeof picked.id === "string" && picked.id !== "" ? { id: picked.id } : undefined;
}

const isMac = isMacPlatform();
const dragStyle = isMac ? ({ WebkitAppRegion: "drag" } as React.CSSProperties) : undefined;
const noDragStyle = isMac ? ({ WebkitAppRegion: "no-drag" } as React.CSSProperties) : undefined;

export function SessionsBoard({ projectId }: SessionsBoardProps) {
	const navigate = useNavigate();
	const queryClient = useQueryClient();
	// Lanes follow the delivery order the user asked for: planning -> building
	// -> review -> ready. Planning and Building split the daemon's pre-PR
	// `building` column by workflow mode; Review groups the validating and
	// in-review feedback loop.
	const columns: BoardLaneView[] = boardLaneOrder.map((lane) => getBoardLaneView(lane));
	const workspaceQuery = useWorkspaceQuery();
	const liveUsageBySession = useSessionUsageSummaries(projectId).data ?? emptyUsageBySession;
	// Evaluated at render so platform mocks in tests can flip the in-panel chrome.
	const boardActionsInPanel = usesBoardActionsInPanel();
	/** Bell lives in the board action row when the shell topbar does not host it. */
	const boardOwnsNotificationCenter = isLinuxPlatform() || boardActionsInPanel;
	const all = workspaceQuery.data ?? [];
	const workspaces = projectId ? all.filter((workspace) => workspace.id === projectId) : all;
	const workspace = projectId ? workspaces[0] : undefined;
	// Board chrome stays route-oriented; project context remains in the sidebar.
	const boardLabel = "Board";
	const liveSessions = workspaces.flatMap((workspace) => workerSessions(workspace.sessions));
	const demoWorkspaceId = projectId ?? workspaces[0]?.id;
	const sessions = usesPreviewWorkspaceData && demoWorkspaceId && liveSessions.length === 0
		? demoBoardSessions(demoWorkspaceId)
		: liveSessions;
	const usageBySession = usesPreviewWorkspaceData
		? new Map<string, SessionUsageSummary>(
				sessions.map((session, index) => [
						session.id,
						liveUsageBySession.get(session.id) ?? {
							estimatedCost: null,
							sessionId: session.id,
							processedTokens: [18_400, 46_700, 12_900, 81_200, 3_100][index % 5],
							totalTokens: 100_000,
							incomplete: false,
					},
				]),
			)
		: liveUsageBySession;
	const manager = projectId ? newestActiveManager(workspaces[0]?.sessions ?? []) : undefined;
	const projectActions = useProjectManagerAction({ projectId, project: workspace, manager, source: "board" });
	const { isProjectRestarting, isProvisioning } = projectActions;
	const setProjectRestarting = useUiStore((state) => state.setProjectRestarting);
	const setManagerReplacementError = useUiStore((state) => state.setManagerReplacementError);
	const health = workspace ? managerHealth(workspace, isProjectRestarting) : { state: "ok" as const };

	const archived = sessions
		.filter(isArchivedSession)
		.sort((left, right) => right.updatedAt.localeCompare(left.updatedAt));
	const activeSessions = sessions.filter((candidate) => !isArchivedSession(candidate));
	const boardLabels = sessionsBoardLabels();
	const { showStartup, showWelcome, showProjectEmpty, workspaceStartupState } = useBoardPresentation({
		projectId,
		isSuccess: workspaceQuery.isSuccess,
		isError: workspaceQuery.isError,
		hasProjects: workspaces.length > 0,
		hasWorkerSessions: liveSessions.length > 0,
	});
	const hasArchive = archived.length > 0;
	const terminateSession = useTerminateSession();
	const discardReadySession = useDiscardReadySession();
	const activeProjectIdRef = useRef(projectId);
	activeProjectIdRef.current = projectId;

	const openSession = useCallback((session: WorkspaceSession) =>
		void navigate({
			to: "/projects/$projectId/sessions/$sessionId",
			params: { projectId: session.workspaceId, sessionId: session.id },
		}), [navigate]);

	const setWorkflowMode = useSetWorkflowMode();
	// Resolve the requested stage against the session role before persisting it.
	// A manager can never enter the worker building stage, so an unnormalized
	// "building" write would be accepted by the daemon and then read back as
	// "manager" — a click that looks like it did nothing.
	const changeWorkflowMode = useCallback(
		(session: WorkspaceSession, requestedMode: WorkflowMode) => {
			const role = isManagerSession(session) ? "manager" : "worker";
			const currentMode = resolveWorkflowMode(role, session.workflowMode);
			const nextMode = resolveWorkflowMode(role, requestedMode);
			if (currentMode === nextMode) return;
			setWorkflowMode.mutate({ sessionId: session.id, workflowMode: nextMode });
		},
		[setWorkflowMode],
	);
	const mergeSessionLocal = useMergeSessionLocal();
	const createSessionPR = useCreateSessionPR();
	// Push the branch if needed and open exactly one pull request against
	// dev, then hand its URL to the browser. Shared by the Ready Open-PR
	// action and the Review Commit fallback below: the daemon de-duplicates
	// (durable facts, then the provider listing, then the creation race), so
	// either entry point is safe to retry. Failures surface on the card via
	// mutation state; nothing is optimistic and the session stays alive.
	const ensureSessionPR = useCallback(
		async (session: WorkspaceSession) => {
			try {
				const result = await createSessionPR.mutateAsync(session);
				if (result.prUrl) await openAgentsBridge.app.openExternal(result.prUrl);
			} catch {
				// The card footer reports the failure via mutation state.
			}
		},
		[createSessionPR],
	);
	const reviewToCommit = useCallback(
		async (session: WorkspaceSession) => {
			// Two-link Commit chain toward Ready. With a pending edit, approve
			// it so the agent commits and return: the push must wait for that
			// commit to land, so the next Commit click (now with nothing
			// pending) pulls the delivery link. With nothing pending the work
			// is already committed — push the branch and ensure exactly one
			// pull request against dev, then let the daemon's observed PR
			// facts move the card. Only an approval-resolution failure hands
			// the session to the composer; a PR-leg failure stays on the card
			// footer so the lane never advances without real facts.
			if (usesPreviewWorkspaceData) {
				openSession(session);
				return;
			}
			const { data, error } = await apiClient.GET("/api/v1/sessions/{sessionId}/conversation", {
				params: { path: { sessionId: session.id } },
			});
			const approval = error || !data ? undefined : pendingWireApproval(data);
			const decision = approval?.requestId ? wireAllowOnceDecision(approval.detail) : undefined;
			if (approval?.requestId && decision) {
				const { error: resolveError } = await apiClient.POST(
					"/api/v1/sessions/{sessionId}/conversation/approvals/{requestId}/resolve",
					{
						params: { path: { sessionId: session.id, requestId: approval.requestId } },
						body: { decisionId: decision.id },
					},
				);
				if (!resolveError) {
					void queryClient.invalidateQueries({ queryKey: workspaceQueryKey });
					return;
				}
				openSession(session);
				return;
			}
			await ensureSessionPR(session);
		},
		[ensureSessionPR, openSession, queryClient],
	);

	// Merge local is end-of-life: the daemon merges, verifies, removes the
	// branch, and terminates, so the card settles into archive on the
	// invalidation. Failures surface on the card; nothing is optimistic.
	const requestMergeLocal = useCallback(
		(session: WorkspaceSession) => mergeSessionLocal.mutate(session),
		[mergeSessionLocal],
	);
	// Ready-lane discard: terminate the session when it is still alive, then
	// retire its record so the finished card leaves the board. Failures
	// surface on the card via mutation state; nothing is optimistic.
	const requestDiscardReady = useCallback(
		(session: WorkspaceSession) => discardReadySession.mutate(session),
		[discardReadySession],
	);
	// Open PR leaves the session alive for review: ensure exactly one PR
	// exists, then hand its URL to the browser. Failures surface on the card.
	const requestCreatePR = useCallback(
		(session: WorkspaceSession) => {
			if (usesPreviewWorkspaceData) {
				const url = primaryPR(session)?.url;
				if (url) void openAgentsBridge.app.openExternal(url);
				return;
			}
			void ensureSessionPR(session);
		},
		[ensureSessionPR],
	);

	const restartManager = async () => {
		if (!projectId) return;
		await restartProjectManager({
			projectId,
			queryClient,
			navigate,
			setProjectRestarting,
			setManagerReplacementError,
		});
	};

	const actions = projectId ? (
		<>
			<ProjectBoardActions actions={projectActions} placement="header" quiet={showProjectEmpty} />
			{boardOwnsNotificationCenter ? (
				<>
					<NotificationCenter />
				</>
			) : null}
		</>
	) : boardOwnsNotificationCenter ? (
		<NotificationCenter />
	) : undefined;

	return (
		<div className="relative flex h-full min-h-0 flex-col bg-background text-foreground" data-testid="board">
			{/* macOS: shell topbar is hidden on board routes, so the project/"Board"
			    crumb + New task / Manager / bell live in this in-panel row.
			    Win/Linux keep the crumb and actions in the framed ShellTopbar.
			    Welcome skips the row — a dangling "Board" above the import
			    chooser was review feedback on #2432. */}
			{!showWelcome && boardActionsInPanel && (boardLabel || actions) ? (
				<div
					className="workspace-topbar-container center-panel-titlebar flex h-toolbar shrink-0 items-center gap-2 border-b border-border-strong pr-1"
					style={dragStyle}
				>
					{boardLabel ? (
						<span
							className={cn(topbarProjectLabelClass, "inline-flex items-center gap-1.5")}
							data-testid="board-topbar-label"
						>
							<LayoutDashboard aria-hidden="true" className="size-icon-md" />
							{boardLabel}
						</span>
					) : null}
					<div className="min-w-0 flex-1" />
					{actions ? (
						<div className="workspace-topbar-actions flex shrink-0 items-center" style={noDragStyle}>
							{actions}
						</div>
					) : null}
				</div>
			) : null}

			{/* Reserve only the collapsed archive bar. Expanded archive overlays the
			    board so lane height (and Needs You scrollbars) stay stable. */}
			<div className={cn("min-h-0 flex-1 overflow-hidden", hasArchive && archiveToggleOffsetClassName)}>
				{projectId && health.state !== "ok" ? (
					<div className="mx-3 my-3 flex items-center gap-3 rounded-md border border-border bg-surface px-3 py-2 text-xs text-muted-foreground">
						<AlertTriangle className="size-icon-base shrink-0 text-warning" aria-hidden="true" />
						<span className="min-w-0 flex-1">{health.message}</span>
						{health.state === "restart_needed" || health.state === "duplicates" ? (
							<TopbarButton disabled={isProjectRestarting} onClick={() => void restartManager()} variant="primary">
								<RotateCw className="size-3.5" aria-hidden="true" />
								{"Restart"}
							</TopbarButton>
						) : null}
					</div>
				) : null}
			{workspace?.folderMissing ? (
				<div className="mx-3 my-3 flex items-center gap-3 rounded-md border border-border bg-surface px-3 py-2 text-xs text-muted-foreground">
					<AlertTriangle className="size-icon-base shrink-0 text-warning" aria-hidden="true" />
					<span className="min-w-0 flex-1">{"Folder missing"}</span>
				</div>
			) : null}
			{projectId && isProvisioning ? (
				<div
					className="mx-3 my-3 flex items-center gap-3 rounded-md border border-border bg-surface px-3 py-2 text-xs text-muted-foreground"
					role="status"
				>
					<span
						className="size-icon-base shrink-0 animate-spin rounded-full border-2 border-current border-r-transparent"
						aria-hidden="true"
					/>
					<span className="min-w-0 flex-1">
						{"Setting up the project — starting the manager…"}
					</span>
				</div>
			) : null}
			{workspaceStartupState === "error" || workspaceQuery.isError ? (
				<p className="py-10 text-center text-xs text-passive">{"Could not load sessions."}</p>
			) : showWelcome ? (
				<BoardWelcome />
			) : showProjectEmpty ? (
				<ProjectBoardEmpty actions={<ProjectBoardActions actions={projectActions} placement="empty" />} />
				) : (
					<SessionsBoardGridView
						columns={columns}
						key={projectId ?? "all"}
						labels={boardLabels}
							renderSessionCard={(session) => (
								<BoardSessionCardAdapter
								onOpen={() => openSession(session)}
									onTerminate={() => terminateSession.mutate(session)}
								onDiscardReady={() => requestDiscardReady(session)}
									onWorkflowModeChange={(_session, workflowMode) => changeWorkflowMode(session, workflowMode)}
									onReviewToCommit={() => void reviewToCommit(session)}
									onMergeLocal={() => requestMergeLocal(session)}
									onCreatePR={() => requestCreatePR(session)}
									session={session}
								usage={usageBySession.get(session.id)}
							/>
						)}
						sessions={activeSessions}
					/>
				)}
			</div>

			{hasArchive ? (
				<BoardArchivePanel
					activeProjectIdRef={activeProjectIdRef}
					projectId={projectId}
					sessions={archived}
					usageBySession={usageBySession}
				/>
			) : null}
			{showStartup ? <DaemonStartupLoader /> : null}
		</div>
	);
}

/**
 * Restore state lives here so expand/collapse in SessionsArchiveView does not
 * re-render the kanban columns. In-flight restores are invalidated on project
 * change or unmount so completion cannot navigate after the user left.
 */
const BoardArchivePanel = memo(function BoardArchivePanel({
	activeProjectIdRef,
	projectId,
	sessions,
	usageBySession,
}: {
	activeProjectIdRef: React.MutableRefObject<string | undefined>;
	projectId?: string;
	sessions: WorkspaceSession[];
	usageBySession: UsageBySession;
}) {
	const retireSession = useRetireSession();
	const clearArchive = useRetireArchivedSessions();
	const [clearArchiveOpen, setClearArchiveOpen] = useState(false);
	const showGlobalToast = useUiStore((state) => state.showGlobalToast);
	const navigate = useNavigate();
	const queryClient = useQueryClient();
	const restoreSessionById = useRestoreSession();
	const [restoringSessionId, setRestoringSessionId] = useState<string | undefined>();
	const [restoreErrors, setRestoreErrors] = useState<Record<string, string>>({});
	const [restoreUnavailableSession, setRestoreUnavailableSession] = useState<WorkspaceSession | undefined>();
	const restoreGenerationRef = useRef(0);

	useEffect(() => {
		setRestoringSessionId(undefined);
		setRestoreErrors({});
		setRestoreUnavailableSession(undefined);
		setClearArchiveOpen(false);
		restoreGenerationRef.current += 1;
	}, [projectId]);

	useEffect(() => {
		const generation = restoreGenerationRef.current;
		return () => {
			// Invalidate in-flight restores if this panel unmounts (e.g. project with
			// no archive) so completion cannot navigate after the user left.
			if (restoreGenerationRef.current === generation) {
				restoreGenerationRef.current += 1;
			}
		};
	}, []);

	const restoreArchivedSession = async (event: MouseEvent<HTMLButtonElement>, session: WorkspaceSession) => {
		event.stopPropagation();
		if (restoringSessionId) return;
		const restoreProjectId = projectId;
		const generation = restoreGenerationRef.current;
		const isStillActiveProject = () =>
			generation === restoreGenerationRef.current &&
			(!restoreProjectId || activeProjectIdRef.current === restoreProjectId);
		setRestoringSessionId(session.id);
		setRestoreErrors((current) => {
			const next = { ...current };
			delete next[session.id];
			return next;
		});
		try {
			const result = await restoreSessionById(session.id);
			if (!isStillActiveProject()) return;
			if (result.status === "success") {
				void navigate({
					to: "/projects/$projectId/sessions/$sessionId",
					params: { projectId: session.workspaceId, sessionId: session.id },
				});
				return;
			}
			if (result.status === "not_resumable") {
				setRestoreUnavailableSession(session);
				return;
			}
			setRestoreErrors((current) => ({ ...current, [session.id]: result.message }));
		} finally {
			if (isStillActiveProject()) {
				setRestoringSessionId(undefined);
			}
		}
	};

	const runClearArchive = async () => {
		const targets = sessions.map((session) => session.id);
		if (targets.length === 0) {
			setClearArchiveOpen(false);
			return;
		}
		const result = await clearArchive.mutateAsync(targets);
		setClearArchiveOpen(false);
		// The summary goes to a global toast rather than the archive bar. This
		// panel is gated on hasArchive, so clearing the last session unmounts
		// it -- an inline summary would be destroyed by the success it reports,
		// which is exactly the case worth reporting. A partial failure names
		// the sessions that need a retry, so a transient toast is enough.
		if (result.failed.length === 0) {
			showGlobalToast(
				"Archive cleared",
				`Removed ${result.removed.length} archived ${pluralize(result.removed.length, "session")}.`,
			);
			return;
		}
		// A user who cleared 12 and sees only "12 removed" when one failed
		// would be misled, so name the shortfall.
		showGlobalToast(
			"Archive partly cleared",
			`Removed ${result.removed.length} · ${result.failed.length} failed: ${result.failed
				.map((failure) => failure.sessionId)
				.join(", ")}`,
			"error",
		);
	};

	return (
		<>
			<SessionsArchiveView
				headerAction={
					<button
						aria-label="Clear archive"
						className="rounded-sm border border-border/80 px-1.5 py-0.5 text-2xs font-medium text-foreground transition-colors hover:bg-error/10 hover:text-error focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/60"
						disabled={clearArchive.isPending}
						onClick={() => setClearArchiveOpen(true)}
						type="button"
					>
						Clear archive
					</button>
				}
				labels={{
					archive: "Archive",
					archiveAria: (sessions.length === 1 ? `Archive, ${sessions.length} session` : `Archive, ${sessions.length} sessions`),
					archivedSessions: "Archived sessions",
				}}
				renderSessionCard={(session) => (
					<ArchivedSessionCardAdapter
						isRestoreDisabled={restoringSessionId !== undefined}
						isRestoring={restoringSessionId === session.id}
						onRemove={() => retireSession.mutate(session.id)}
						isRemoving={retireSession.isPending && retireSession.variables === session.id}
						removeError={retireSession.isError && retireSession.variables === session.id
							? apiErrorMessage(retireSession.error, `Failed to remove ${session.title}`)
							: undefined}
						restoreAction={(event) => void restoreArchivedSession(event, session)}
						restoreError={restoreErrors[session.id]}
						session={session}
						usage={usageBySession.get(session.id)}
					/>
				)}
				resetKey={projectId}
				sessions={sessions}
			/>
			{restoreUnavailableSession ? (
				<RestoreUnavailableDialog
					open={true}
					session={restoreUnavailableSession}
					onOpenChange={(open) => {
						if (!open) setRestoreUnavailableSession(undefined);
					}}
					onRecreated={async () => {
						await queryClient.invalidateQueries({ queryKey: workspaceQueryKey });
					}}
				/>
			) : null}
			<ConfirmDialog
				busy={clearArchive.isPending}
				confirmLabel="Clear archive"
				description={
					<>
						<p>
							{`Permanently remove ${sessions.length} archived ${pluralize(sessions.length, "session")} and their records.`}
						</p>
						<p className="mt-2">{archiveRemoveWarning}</p>
					</>
				}
				destructive
				// The dialog cannot close under a run in progress: the sweep
				// owns the archive until it reports, and a close mid-run would
				// leave the summary to arrive against a panel the user left.
				onOpenChange={(open) => {
					if (open) return;
					if (clearArchive.isPending) return;
					setClearArchiveOpen(false);
				}}
				onConfirm={() => void runClearArchive()}
				open={clearArchiveOpen}
				title="Clear the archive?"
			/>
		</>
	);
});

function pluralize(count: number, noun: string): string {
	return count === 1 ? noun : `${noun}s`;
}
