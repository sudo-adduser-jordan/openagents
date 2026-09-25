import { useQueryClient } from "@tanstack/react-query";
import { useNavigate, useParams } from "@tanstack/react-router";
import { Folder, LayoutDashboard, Plus, Trash2 } from "lucide-react";
import { useEffect, useState, type ReactNode } from "react";
import { animate, LayoutGroup, motion, useMotionValue, useReducedMotion } from "motion/react";
import { NotificationCenter } from "./NotificationCenter";
import { ProjectBoardActions } from "./ProjectBoardActions";
import { useBoardPresentation } from "../hooks/useBoardPresentation";
import { useProjectManagerAction } from "../hooks/useProjectManagerAction";
import {
	isManagerSession,
	sessionIsActive,
	STANDALONE_PROJECT_KIND,
	STANDALONE_WORKSPACE_ID,
	type WorkspaceSession,
} from "../types/workspace";
import { useWorkspaceScope } from "../hooks/useWorkspaceQuery";
import {
	clearTerminateSessionState,
	useProjectTerminateSessionStates,
	useTerminateSession,
	useTerminateSessionState,
} from "../hooks/useTerminateSession";
import { sidebarOccupiesLayout, useUiStore } from "../stores/ui-store";
import { ManagerIcon } from "./icons";
import { getAgentActivityView } from "../lib/session-presentation";
import { isLinuxPlatform, isMacPlatform, usesBoardActionsInPanel } from "../lib/platform";
import { cn } from "../lib/utils";
import { SHELL_PANEL_SPRING } from "../lib/motion-spring";
import { useWindowFullScreen } from "../hooks/useWindowFullScreen";
import { StatusPill } from "./StatusPill";
import { TopbarActionError, TopbarButton, topbarHeaderClass, topbarProjectLabelClass } from "./TopbarButton";
import { SessionTerminationPopover } from "./SessionTerminationPopover";
import { TopbarOpenEditorButton } from "./TopbarOpenEditorButton";
import { Tooltip, TooltipContent, TooltipTrigger } from "./ui/tooltip";

const isMac = isMacPlatform();
const dragStyle = isMac ? ({ WebkitAppRegion: "drag" } as React.CSSProperties) : undefined;
const noDragStyle = isMac ? ({ WebkitAppRegion: "no-drag" } as React.CSSProperties) : undefined;

// The one app topbar (.dashboard-app-header). On Win/Linux the shell mounts it
// inside the framed center panel; when the platform hides the shell topbar
// (macOS), SessionView mounts the same component in-panel so Kill / Manager
// / inspector stay available. The variant is derived from the route, not props:
// a sessionId in the URL swaps the lead to the session identity (manager
// crumb + mode badge, or worker branch + status pill) and the actions to
// board/manager + inspector controls (managers open the Kanban board;
// workers open their manager); otherwise it's the dashboard crumb plus the
// Manager launcher when a project is in scope. Embedded mode contributes
// only session actions to the terminal bar; other routes retain this full bar.
// Pixel equivalents of the CSS custom properties used for titlebar clearance.
// --size-titlebar-cluster-left (72) + --size-titlebar-cluster-width (3×28+2×4=92)
// + --size-titlebar-content-gap (12) = 176; minus --size-center-panel-inset-mac (6) = 170.
// Fullscreen: --space-2 (8) + 92 + 12 = 112.
// Linux: --size-titlebar-cluster-left-linux (6) + 92 + 12 = 110; minus
// --size-center-panel-inline-inset (16) because the framed panel keeps that gutter.
const PADDING_DEFAULT = 18; // 1.125rem
const PADDING_CLEARANCE = 170;
const PADDING_CLEARANCE_FULLSCREEN = 112;
// Off-canvas the Linux cluster shifts right to clear the framed panel border, so
// measure the reserve from that inset, relative to the panel's own left edge:
// --size-titlebar-cluster-left-linux-panel (26) + --size-titlebar-cluster-width
// (92) + --size-titlebar-content-gap (12) - --size-center-panel-inline-inset (16).
const PADDING_CLEARANCE_LINUX = 114;

export function ShellTopbar({
	embedded = false,
	sessionAction,
	compactActions = false,
}: {
	embedded?: boolean;
	sessionAction?: ReactNode;
	compactActions?: boolean;
} = {}) {
	const navigate = useNavigate();
	const params = useParams({ strict: false }) as { projectId?: string; sessionId?: string };
	const currentSessionId = params.sessionId;
	const isSidebarOpen = useUiStore(sidebarOccupiesLayout);
	const isFullScreen = useWindowFullScreen();
	const prefersReducedMotion = useReducedMotion();
	const mac = isMacPlatform();
	const boardActionsInPanel = usesBoardActionsInPanel();
	const linux = isLinuxPlatform();
	const targetPaddingLeft =
		!embedded && !isSidebarOpen && mac
			? isFullScreen
				? PADDING_CLEARANCE_FULLSCREEN
				: PADDING_CLEARANCE
			: !embedded && !isSidebarOpen && linux
				? PADDING_CLEARANCE_LINUX
				: PADDING_DEFAULT;
	const paddingLeft = useMotionValue(targetPaddingLeft);
	useEffect(() => {
		const controls = animate(
			paddingLeft,
			targetPaddingLeft,
			prefersReducedMotion ? { duration: 0 } : SHELL_PANEL_SPRING,
		);
		return controls.stop;
	}, [targetPaddingLeft, paddingLeft, prefersReducedMotion]);
	const workspaceQuery = useWorkspaceScope(params.projectId, params.sessionId);
	const workspaceScope = workspaceQuery.data;
	const session = workspaceScope?.session;
	const isSessionRoute = Boolean(params.sessionId);
	const isManager = session ? isManagerSession(session) : false;
	const isInspectorOpen = useUiStore((state) =>
		currentSessionId ? (state.inspectorSessions[currentSessionId]?.isOpen ?? !isManager) : false,
	);
	// Project in scope: the session's workspace wins over the route param so the
	// cross-project /sessions/$sessionId route still resolves a crumb. A
	// projectId that no longer resolves (stale route after the project was
	// removed, or data still loading) shows an empty crumb — never the raw
	// route slug. "Board" is the root-board crumb only.
	const projectId = session?.workspaceId ?? params.projectId;
	const isProjectBoardRoute = !isSessionRoute && Boolean(projectId);
	const isRootBoardRoute = !isSessionRoute && !isProjectBoardRoute;
	const project = workspaceScope?.project;
	const projectLabel = project?.name ?? session?.workspaceName ?? (projectId ? "" : "Board");
	const manager = workspaceScope?.manager;
	const supportsProjectActions = project?.kind !== STANDALONE_PROJECT_KIND && projectId !== STANDALONE_WORKSPACE_ID;
	const projectActions = useProjectManagerAction({
		projectId: supportsProjectActions ? projectId : undefined,
		project: supportsProjectActions ? project : undefined,
		manager: supportsProjectActions ? manager : undefined,
		source: "topbar",
		sessionId: currentSessionId,
	});
	const { isSpawning, isProjectRestarting, isProvisioning, openNewTask, openManager } = projectActions;
	const { showProjectEmpty } = useBoardPresentation({
		projectId,
		isSuccess: workspaceQuery.isSuccess,
		isError: workspaceQuery.isError,
		hasProjects: Boolean(project),
		hasWorkerSessions: workspaceScope?.hasWorkerSessions ?? false,
	});
	const managerTooltip = isProjectRestarting ? "Restarting…" : isSpawning
		? "Spawning…" : manager ? "Open manager" : "Spawn Manager";

	const openBoard = () =>
		projectId ? void navigate({ to: "/projects/$projectId", params: { projectId } }) : void navigate({ to: "/" });

	return (
		<LayoutGroup id="shell-topbar">
		<motion.header
			className={
				embedded ? "contents" : cn(topbarHeaderClass, "workspace-topbar-container", isSessionRoute && "pr-2")
			}
			style={embedded ? undefined : { ...dragStyle, paddingLeft }}
		>
			{!embedded ? (
				<div className="flex min-w-0 items-center gap-3">
				{isSessionRoute && session ? (
					<div className="flex min-w-0 items-center gap-2.5" data-testid="session-topbar-identity">
						{isManager ? (
							<span className={cn(topbarProjectLabelClass, "inline-flex min-w-0 items-center gap-1.5")}>
								<Folder aria-hidden="true" className="size-icon-md shrink-0 text-muted-foreground" />
								<span className="max-w-content-max truncate">{projectLabel}</span>
							</span>
						) : (
							<span className={cn(topbarProjectLabelClass, "max-w-content-max truncate")}>{session.title}</span>
						)}
						<span aria-hidden="true" className="workspace-topbar__identity-separator" />
						<SessionStatusPill session={session} />
					</div>
				) : (isProjectBoardRoute && boardActionsInPanel) ||
				  (isMac && isRootBoardRoute && boardActionsInPanel) ? null : (
					<div className="inline-flex min-w-0 items-center gap-1.5" data-testid="board-topbar-label">
						<motion.span
							layoutId="topbar-project-label"
							layout="position"
							className={cn(topbarProjectLabelClass, "inline-flex items-center gap-1.5")}
							transition={{ type: "spring", stiffness: 400, damping: 40 }}
						>
							<LayoutDashboard aria-hidden="true" className="size-icon-md" />
							{"Board"}
						</motion.span>
					</div>
				)}
				</div>
			) : null}

			{!embedded ? <div className="min-w-0 flex-1" /> : null}

			<div
				className="workspace-topbar-actions flex shrink-0 items-center"
				data-compact-actions={compactActions ? "true" : "false"}
				data-testid="workspace-topbar-actions"
			>
				{!boardActionsInPanel && isProjectBoardRoute ? (
					<ProjectBoardActions actions={projectActions} placement="header" quiet={showProjectEmpty} style={noDragStyle} />
				) : null}
				{isSessionRoute ? (
					<>
						{isManager ? (
							<>
								<ProjectTerminationFeedback projectId={projectId} />
								{sessionAction ? (
									<div className="inline-flex shrink-0 items-center" style={noDragStyle}>
										{sessionAction}
									</div>
								) : null}
								<Tooltip>
									<TooltipTrigger asChild>
										<span className="inline-flex" style={noDragStyle}>
											<TopbarButton
												aria-label="New task"
												className="topbar-control--labeled"
												data-priority="primary"
												disabled={isProjectRestarting || isProvisioning}
												onClick={openNewTask}
												variant="accent"
											>
												<Plus className="size-icon-md" aria-hidden="true" />
												<span data-compact-label>{"Task"}</span>
											</TopbarButton>
										</span>
									</TooltipTrigger>
									<TooltipContent side="bottom">{"New task"}</TooltipContent>
								</Tooltip>
								<Tooltip>
									<TooltipTrigger asChild>
										<TopbarButton
											aria-label="Open Kanban"
											className="topbar-control--labeled"
											data-priority="secondary"
											onClick={openBoard}
											style={noDragStyle}
											variant="feature"
										>
											<LayoutDashboard className="size-icon-md" aria-hidden="true" />
											<span data-compact-label>{"Open Kanban"}</span>
										</TopbarButton>
									</TooltipTrigger>
									<TooltipContent side="bottom">{"Open Kanban"}</TooltipContent>
								</Tooltip>
							</>
						) : null}
						{/* Open-in-editor leads the session actions: it is the only
						    non-destructive one, and it must sit left of Kill. Kept outside
						    the local-actions group because Electron main independently
						    reports whether this session has a live workspace. */}
						{session ? (
							// Keyed per session so a stale launch error does not carry over
							// when switching sessions. The prefix keeps it distinct from the
							// kill button's key: identical sibling keys make React duplicate
							// the nodes.
							<TopbarOpenEditorButton
								key={`open-workspace-${session.id}`}
								sessionId={session.id}
								projectId={session.workspaceId}
								sessionCreatedAt={session.createdAt}
								sessionTerminated={session.isTerminated}
								style={noDragStyle}
							/>
						) : null}
						{/* Local worker actions share one tight control group. Navigation
						    remains a separate visual target in the outer top-bar row. */}
						{!isManager && session && (sessionAction || sessionIsActive(session)) ? (
							<div
								className="inline-flex shrink-0 items-center gap-1"
								data-testid="session-local-actions"
								style={noDragStyle}
							>
								{sessionAction ? <div className="inline-flex shrink-0 items-center">{sessionAction}</div> : null}
								{sessionIsActive(session) ? (
									<TopbarKillButton
										key={session.id}
										session={session}
										managerId={manager?.id}
										onKilled={(workspaceId, managerId) => {
											if (managerId) {
												void navigate({
													to: "/projects/$projectId/sessions/$sessionId",
													params: { projectId: workspaceId, sessionId: managerId },
												});
												return;
											}
											if (workspaceId === STANDALONE_WORKSPACE_ID) {
												void navigate({ to: "/" });
												return;
											}
											void navigate({ to: "/projects/$projectId", params: { projectId: workspaceId } });
										}}
									/>
								) : null}
							</div>
						) : null}
						{!isManager && supportsProjectActions ? (
							<Tooltip>
								<TooltipTrigger asChild>
									<span className="inline-flex" style={noDragStyle}>
										<TopbarButton
											aria-label="Open manager"
											className="topbar-control--labeled -mr-1"
											data-priority="secondary"
											disabled={isSpawning || isProjectRestarting || isProvisioning}
											onClick={() => void openManager()}
											variant="primary"
										>
											<ManagerIcon className="size-icon-md" aria-hidden="true" />
											<span data-compact-label>{"Manager"}</span>
										</TopbarButton>
									</span>
								</TooltipTrigger>
								<TooltipContent side="bottom">{managerTooltip}</TooltipContent>
							</Tooltip>
						) : null}
					</>
				) : null}
				{isSessionRoute ? (
					/* The pinned controls are owned by SessionView so they stay at the
					   window's right edge. Reserve their width only when the rail is closed. */
					<div
						className="session-pinned-actions-reserve"
						data-state={isInspectorOpen ? "collapsed" : "expanded"}
						data-testid="session-pinned-actions-reserve"
						aria-hidden="true"
					/>
				) : (
					<NotificationCenter style={noDragStyle} />
				)}
			</div>
		</motion.header>
	</LayoutGroup>
	);
}

// Confirmation is modal, but teardown progress is not: confirming closes the
// dialog and returns to the project's manager while the daemon finishes.
// Mutation-cache state is filtered by worker ID so rapid route switches never
// carry another worker's Killing/error state into the current topbar.
export function TopbarKillButton({
	session,
	managerId,
	onKilled,
}: {
	session: WorkspaceSession;
	managerId?: string;
	onKilled: (workspaceId: string, managerId?: string) => void;
}) {
	const [confirmOpen, setConfirmOpen] = useState(false);
	const queryClient = useQueryClient();
	const kill = useTerminateSession();
	const { error, isPending } = useTerminateSessionState(session.id);

	const confirmKill = () => {
		setConfirmOpen(false);
		kill.mutate(session);
		onKilled(session.workspaceId, managerId);
	};

	return (
		<div className="inline-flex items-center gap-1.5" style={noDragStyle}>
			<Tooltip>
				<TooltipTrigger asChild>
					<span className="inline-flex">
						<SessionTerminationPopover
							onConfirm={confirmKill}
							onOpenChange={setConfirmOpen}
							open={confirmOpen}
							session={session}
							trigger={
								<TopbarButton
									aria-label={isPending ? "Killing..." : "Kill session"}
									disabled={isPending}
									onClick={() => {
										clearTerminateSessionState(queryClient, session.id);
										// Force the confirm open rather than letting the trigger toggle
										// it: a second trash tap would otherwise dismiss the dialog, so
										// the delete "needed" several clicks to land on the Yes button.
										setConfirmOpen(true);
									}}
									variant="killIcon"
								>
									<Trash2 className="size-icon-md" aria-hidden="true" />
								</TopbarButton>
							}
						/>
					</span>
				</TooltipTrigger>
				<TooltipContent side="bottom">{"Kill session"}</TooltipContent>
			</Tooltip>
			{error ? <TopbarActionError>{error}</TopbarActionError> : null}
		</div>
	);
}

function ProjectTerminationFeedback({ projectId }: { projectId: string | undefined }) {
	const states = useProjectTerminateSessionStates(projectId);
	if (states.length === 0) return null;

	return (
		<div aria-label="Session termination status" className="flex max-w-content-max items-center gap-2">
			{states.map((state) =>
				state.error ? (
					<TopbarActionError className="max-w-48 truncate" key={state.session.id} title={state.error}>
						{state.session.title}: {state.error}
					</TopbarActionError>
				) : (
					<span
						className="max-w-40 truncate text-caption text-muted-foreground"
						key={state.session.id}
						role="status"
						title={`Killing ${state.session.title}…`}
					>
						{`Killing ${state.session.title}…`}
					</span>
				),
			)}
		</div>
	);
}
function SessionStatusPill({ session }: { session: WorkspaceSession }) {
	const { label, tone, breathe } = getAgentActivityView(session.activity);
	return (
		<StatusPill label={label} tone={tone} breathe={breathe} leading="none" className="px-2 py-1 text-micro" />
	);
}
