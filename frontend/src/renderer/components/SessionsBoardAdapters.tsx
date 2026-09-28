import { useEffect, useRef, useState, type MouseEvent, type ReactNode } from "react";
import { useQueryClient } from "@tanstack/react-query";
import {
	scmUserAvatarUrl,
	SessionCardView,
	SessionUsageMetricView,
	getBoardLaneView,
	getSessionStatusView,
	type BoardPullRequestLabels,
	type BoardPullRequestProgress,
	type BoardSessionPresentation,
	type BoardColumnLabels,
	type BoardUsagePresentation,
} from "@openagents/product-ui";
import { Check, Copy, GitBranch, LoaderCircle, RotateCcw, Trash2 } from "lucide-react";
import { openAgentsBridge } from "../lib/bridge";
import { formatTimeCompact } from "../lib/format-time";
import { formatTokenCount } from "../lib/format-token-count";
import { prBrowserUrl, sessionPRDisplaySummaries } from "../lib/pr-display";
import { toBoardLane } from "../lib/session-presentation";
import { useCreateSessionPRState, clearCreateSessionPRState } from "../hooks/useCreateSessionPR";
import { useMergeSessionLocalState, clearMergeSessionLocalState } from "../hooks/useMergeSessionLocal";
import type { WorkflowMode, WorkspaceSession } from "../types/workspace";
import { canonicalTrackerIssueId, primaryPR, sessionNeedsAttention } from "../types/workspace";
import { useSessionScmSummary } from "../hooks/useSessionScmSummary";
import type { SessionUsageSummary } from "../hooks/useSessionUsageSummaries";
import {
	clearTerminateSessionState,
	useTerminateSessionState,
} from "../hooks/useTerminateSession";
import { cn } from "../lib/utils";
import { AgentAvatar } from "./AgentAvatar";
import { ArchiveRemoveButton } from "./ArchiveRemoveButton";
import { ProductExternalLink } from "./ProductExternalLink";
import { SessionTerminationPopover } from "./SessionTerminationPopover";
import { Tooltip, TooltipContent, TooltipTrigger } from "./ui/tooltip";

export function toBoardSessionPresentation(
	session: WorkspaceSession,
): BoardSessionPresentation {
	return {
		activity: session.activity,
		branch: session.branch,
		id: session.id,
		isTerminated: session.isTerminated,
		kanbanColumn: session.kanbanColumn,
		workflowMode: session.workflowMode,
		displayStatus: session.displayStatus,
		provider: session.provider,
		status: session.status,
		title: session.title,
		trackerIssueId: canonicalTrackerIssueId(session.issueId),
		updatedAt: session.updatedAt,
		lastUserMessageAt: session.lastUserMessageAt,
	};
}

export function sessionsBoardLabels(): BoardColumnLabels {
	return {
		columnAria: (label) => `${label} sessions`,
	};
}

export function BoardSessionCardAdapter({
	onOpen,
	onTerminate,
	onWorkflowModeChange,
	onReviewToCommit,
	onMergeLocal,
	onCreatePR,
	session,
	usage,
}: {
	onOpen: () => void;
	onTerminate: () => void;
	onWorkflowModeChange?: (session: WorkspaceSession, workflowMode: WorkflowMode) => void;
	onReviewToCommit?: (session: WorkspaceSession) => void;
	onMergeLocal?: (session: WorkspaceSession) => void;
	onCreatePR?: (session: WorkspaceSession) => void;
	session: WorkspaceSession;
	usage?: SessionUsageSummary;
}) {
	return (
		<DesktopSessionCard
			onOpen={onOpen}
			onTerminate={onTerminate}
			onWorkflowModeChange={onWorkflowModeChange}
			onReviewToCommit={onReviewToCommit}
			onMergeLocal={onMergeLocal}
			onCreatePR={onCreatePR}
			session={session}
			usage={usage}
		/>
	);
}

export function ArchivedSessionCardAdapter({
	isRestoreDisabled,
	isRestoring,
	onRemove,
	isRemoving,
	removeError,
	restoreAction,
	restoreError,
	session,
	usage,
}: {
	isRestoreDisabled: boolean;
	isRestoring: boolean;
	onRemove: () => void;
	isRemoving: boolean;
	removeError?: string;
	restoreAction: (event: MouseEvent<HTMLButtonElement>) => void;
	restoreError?: string;
	session: WorkspaceSession;
	usage?: SessionUsageSummary;
}) {
	const branch = session.branch ?? "";
	return (
		<DesktopSessionCard
			action={
				<div className="flex items-center gap-0.5">
					<ArchiveRestoreButton
						isDisabled={isRestoreDisabled}
						isRestoring={isRestoring}
						label={`Restore ${session.title}`}
						onClick={restoreAction}
					/>
					<ArchiveRemoveButton
						isRemoving={isRemoving}
						label={session.title}
						onRemove={onRemove}
					/>
				</div>
			}
			branchAction={branch ? <CopyActionButton label={`branch ${branch}`} value={branch} /> : undefined}
			footer={<CardFooterError message={restoreError ?? removeError} />}
			interactive={false}
			session={session}
			usage={usage}
		/>
	);
}

function DesktopSessionCard({
	action,
	branchAction,
	footer,
	interactive = true,
	onOpen,
	onTerminate,
	onWorkflowModeChange,
	onReviewToCommit,
	onMergeLocal,
	onCreatePR,
	session,
	usage,
}: {
	action?: ReactNode;
	branchAction?: ReactNode;
	footer?: ReactNode;
	interactive?: boolean;
	onOpen?: () => void;
	onTerminate?: () => void;
	onWorkflowModeChange?: (session: WorkspaceSession, workflowMode: WorkflowMode) => void;
	onReviewToCommit?: (session: WorkspaceSession) => void;
	onMergeLocal?: (session: WorkspaceSession) => void;
	onCreatePR?: (session: WorkspaceSession) => void;
	session: WorkspaceSession;
	usage?: SessionUsageSummary;
}) {
	const queryClient = useQueryClient();
	const [confirmOpen, setConfirmOpen] = useState(false);
	const [mergeConfirmOpen, setMergeConfirmOpen] = useState(false);
	const summaries = sessionPRDisplaySummaries(session, useSessionScmSummary(session.id).data);
	const termination = useTerminateSessionState(session.id);
	const mergeLocal = useMergeSessionLocalState(session.id);
	const createPR = useCreateSessionPRState(session.id);
	const showTerminate = interactive && session.isTerminated !== true && onTerminate;
	const keepTerminateVisible = session.status === "merged";
	const usagePresentation = toUsagePresentation(usage);
	// Lanes are daemon-derived; the card groups by the same derivation the
	// board uses for placement, so actions never appear on a card whose lane
	// does not own them.
	const boardLane = interactive && session.isTerminated !== true
		? toBoardLane(session.kanbanColumn, session.status, session.workflowMode, session.displayStatus)
		: undefined;
	// The daemon reports a finished pre-PR session as "Awaiting PR". That text
	// is replaced here with the delivery-stage actions: confirm a finished plan,
	// or review the pending edit so the agent commits and waits on PR approval.
	const pausedAwaitingPR = session.displayStatus === "Awaiting PR";
	const canAdvance = interactive && session.isTerminated !== true;
	const statusAction =
		pausedAwaitingPR && canAdvance && session.workflowMode === "planning" && onWorkflowModeChange ? (
			<div className="flex min-w-0 items-center gap-1.5">
				<WorkflowStageActionButton
					label="Build"
					onClick={() => onWorkflowModeChange(session, "building")}
					title="Approve this plan and let the worker start building"
				/>
			</div>
		) : boardLane === "review" && onReviewToCommit ? (
			// Review-lane Commit: resolve the pending approval so the agent
			// commits; the lane itself follows only once the daemon observes
			// the resulting PR facts. Exactly one button per review card. The
			// daemon's status phrase stays visible beside it (it names the loop
			// that is turning).
			<div className="flex min-w-0 items-center gap-1.5">
				<DeliveryStatusLabel session={session} lane={boardLane} />
				<WorkflowStageActionButton
					label="Commit"
					onClick={() => onReviewToCommit(session)}
					title="Approve the pending edit so the agent commits and opens a pull request"
				/>
			</div>
		) : boardLane === "ready" ? (
			// Ready-lane delivery: the daemon's status phrase stays visible
			// next to the two actions, so "Approved"/"Mergeable"/"Merged" keeps
			// explaining why the card is ready.
			<div className="flex min-w-0 items-center gap-1.5">
				<DeliveryStatusLabel session={session} lane={boardLane} />
				{onMergeLocal ? (
					<SessionTerminationPopover
						onConfirm={() => {
							setMergeConfirmOpen(false);
							onMergeLocal(session);
						}}
						onOpenChange={(open) => {
							if (open) clearMergeSessionLocalState(queryClient, session.id);
							setMergeConfirmOpen(open);
						}}
						open={mergeConfirmOpen}
						session={session}
						title={`Merge ${session.title} into dev?`}
						body="Merges the session branch into local dev, removes the branch, and archives the session. Refuses on a dirty checkout or conflicts."
						confirmLabel="Yes, merge into dev"
						trigger={
							<WorkflowStageActionButton
								label={mergeLocal.isPending ? "Merging…" : "Merge local"}
								disabled={mergeLocal.isPending || createPR.isPending}
								title="Merge this branch into local dev, remove it, and archive the session"
							/>
						}
					/>
				) : null}
				{onCreatePR && primaryPR(session)?.url ? (
					<WorkflowStageActionButton
						label={createPR.isPending ? "Opening…" : "Open PR"}
						onClick={() => {
							clearCreateSessionPRState(queryClient, session.id);
							onCreatePR(session);
						}}
						disabled={mergeLocal.isPending || createPR.isPending}
						title="Push the branch if needed and open its pull request against dev"
					/>
				) : null}
			</div>
		) : undefined;

	const terminationOverlay = showTerminate ? (
		<Tooltip>
			<TooltipTrigger asChild>
				<span className="inline-flex">
					<SessionTerminationPopover
						onConfirm={() => {
							setConfirmOpen(false);
							onTerminate();
						}}
						onOpenChange={setConfirmOpen}
						open={confirmOpen}
						session={session}
						trigger={
							<button
								aria-label={
									termination.isPending
										? `Killing ${session.title}`
										: `Terminate ${session.title}`
								}
								className={cn(
									"inline-flex size-control-md items-center justify-center rounded-sm text-passive transition-[color,background-color,opacity] hover:bg-error/10 hover:text-error focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/60",
									keepTerminateVisible || termination.isPending
										? "opacity-100"
										: "pointer-events-none opacity-0 group-hover:pointer-events-auto group-hover:opacity-100 group-focus-within:pointer-events-auto group-focus-within:opacity-100",
								)}
								onClick={(event) => {
									event.stopPropagation();
									clearTerminateSessionState(queryClient, session.id);
									// Force the confirm open instead of toggling it, so repeated
									// trash taps keep the dialog up rather than dismissing it.
									setConfirmOpen(true);
								}}
								disabled={termination.isPending}
								type="button"
							>
								{termination.isPending ? (
									<LoaderCircle className="size-icon-sm animate-spin" aria-hidden="true" />
								) : (
									<Trash2 className="size-icon-sm" aria-hidden="true" />
								)}
							</button>
						}
					/>
				</span>
			</TooltipTrigger>
			<TooltipContent side="bottom">
				{termination.isPending ? "Killing session" : "Terminate session"}
			</TooltipContent>
		</Tooltip>
	) : undefined;

	return (
		<>
			<SessionCardView
				action={action}
				branchAction={branchAction}
				branchIcon={<GitBranch aria-hidden="true" className="size-icon-2xs shrink-0" />}
				error={termination.error ?? undefined}
				externalLink={ProductExternalLink}
				footer={footer ?? <CardFooterError message={mergeLocal.error ?? createPR.error ?? undefined} />}
				interactive={interactive}
			labels={{
				formatTime: formatTimeCompact,
				intakeIssue: (id) => `Intake issue: ${id}`,
				pr: pullRequestLabels(),
				updatedAt: (timestamp: string) =>
					`Last message ${formatTimeCompact(timestamp)}`,
			}}
			onOpen={onOpen}
			overlay={terminationOverlay}
			prs={summaries.map((pr) => ({
				commentCount: pr.review.unresolvedBy.reduce((count, reviewer) => count + reviewer.count, 0),
				number: pr.number,
				reviewers: Array.from(
					new Map(
						pr.review.unresolvedBy.map((reviewer) => [reviewer.reviewerId, reviewer]),
					).values(),
				).map((reviewer) => ({
					avatarUrl: scmUserAvatarUrl(pr.provider, prBrowserUrl(pr), reviewer.reviewerId),
					id: reviewer.reviewerId,
				})),
				state: pr.state,
				url: prBrowserUrl(pr),
			}))}
			renderAvatar={(provider) => <AgentAvatar provider={provider} />}
			session={toBoardSessionPresentation(session)}
			statusAction={statusAction}
			renderUsage={(usage) => (
				<Tooltip>
					<TooltipTrigger asChild>
						<SessionUsageMetricView usage={usage} />
					</TooltipTrigger>
					<TooltipContent side="top">{usage.accessibleLabel}</TooltipContent>
				</Tooltip>
			)}
			usage={usagePresentation}
			/>
		</>
	);
}

/** Daemon status phrase shown beside delivery actions, mirroring the card
 * label tone (closed-without-merge red, mergeable green, attention amber,
 * lane tone otherwise) so action rows never restyle what the card means. A
 * daemon too old to send displayStatus falls back to the status label, so
 * legacy cards keep their text; only the finished-build "Awaiting PR" text is
 * dropped, as before. */
function DeliveryStatusLabel({ session, lane }: { session: WorkspaceSession; lane: "review" | "ready" }) {
	const label =
		session.displayStatus && session.displayStatus !== "Awaiting PR"
			? session.displayStatus
			: !session.displayStatus
				? getSessionStatusView(session.status).label
				: "";
	if (!label) return null;
	return (
		<span
			className={cn(
				"min-w-0 shrink truncate text-2xs font-medium",
				session.displayStatus === "Closed without merge"
					? "text-status-exited"
					: session.status === "mergeable" || session.displayStatus === "Mergeable"
						? "text-success"
						: sessionNeedsAttention(session)
							? "text-status-needs-you"
							: getBoardLaneView(lane).titleClassName,
			)}
			data-kanban-column={lane}
		>
			{label}
		</span>
	);
}
/** Compact card action used in place of the daemon status label. */
function WorkflowStageActionButton({
	label,
	onClick,
	title,
	disabled,
}: {
	label: string;
	onClick?: () => void;
	/** Carries the meaning a one-word label drops. Falls back to no tooltip. */
	title?: string;
	disabled?: boolean;
}) {
	return (
		<button
			type="button"
			onClick={(event) => {
				event.stopPropagation();
				onClick?.();
			}}
			title={title}
			disabled={disabled}
			className="inline-flex min-w-0 items-center rounded-sm border border-border/80 px-1.5 py-px text-2xs font-medium leading-none text-foreground transition-colors hover:border-foreground/30 hover:bg-surface focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/60 disabled:cursor-wait disabled:opacity-60"
		>
			<span className="truncate">{label}</span>
		</button>
	);
}

function pullRequestLabels(): BoardPullRequestLabels {
	return {
		progress: (progress) => pullRequestProgressLabel(progress),
		short: "PR",
		states: {
			closed: "closed",
			draft: "draft",
			merged: "merged",
			open: "open",
		},
	};
}

function pullRequestProgressLabel(
	{ closed, draft, merged, open, total }: BoardPullRequestProgress,
): string {
	return [
		(total === 1 ? `${merged} of ${total} PR merged` : `${merged} of ${total} PRs merged`),
		open > 0 ? (open === 1 ? `${open} open` : `${open} open`) : undefined,
		draft > 0 ? (draft === 1 ? `${draft} draft` : `${draft} drafts`) : undefined,
		closed > 0 ? (closed === 1 ? `${closed} closed` : `${closed} closed`) : undefined,
	]
		.filter((part): part is string => part !== undefined)
		.join(" · ");
}

// Keep the board metric scannable with a compact token count. The full
// token summary remains available from the hover tooltip and to screen readers.
function toUsagePresentation(
	usage: SessionUsageSummary | undefined,
): BoardUsagePresentation | undefined {
	const processedTokens = usage?.processedTokens ?? null;
	if (!usage || processedTokens === null || processedTokens <= 0) {
		return undefined;
	}
	const compactTokens = formatTokenCount(processedTokens).replace(/ tok$/, "");
	const accessibleTokens = `${processedTokens.toLocaleString("en-US")} tokens`;
	return {
		accessibleLabel: accessibleTokens,
		compactLabel: compactTokens,
	};
}

function ArchiveRestoreButton({
	label,
	onClick,
	isRestoring,
	isDisabled,
}: {
	label: string;
	onClick: (event: MouseEvent<HTMLButtonElement>) => void;
	isRestoring: boolean;
	isDisabled: boolean;
}) {
	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<span className="inline-flex">
					<button
						aria-label={label}
						className="grid size-control-board-sm shrink-0 place-items-center rounded-md text-passive transition-colors hover:bg-interactive-hover hover:text-foreground focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-accent/50 disabled:cursor-not-allowed disabled:opacity-35"
						disabled={isDisabled}
						onClick={onClick}
						type="button"
					>
						<RotateCcw className={cn("size-icon-md", isRestoring && "animate-spin")} aria-hidden="true" />
					</button>
				</span>
			</TooltipTrigger>
			<TooltipContent side="top">
				{isRestoring ? "Restoring session" : "Restore session"}
			</TooltipContent>
		</Tooltip>
	);
}

function CardFooterError({ message }: { message?: string }) {
	return message ? (
		<div className="border-t border-border px-2 py-1.5 text-2xs text-destructive" role="alert">
			{message}
		</div>
	) : null;
}

function CopyActionButton({ label, value }: { label: string; value: string }) {
	const [copied, setCopied] = useState(false);
	const copiedTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
	useEffect(
		() => () => {
			if (copiedTimeoutRef.current !== null) clearTimeout(copiedTimeoutRef.current);
		},
		[],
	);
	const buttonLabel = copied ? `Copied ${label}` : `Copy ${label}`;
	const copyValue = async (event: MouseEvent<HTMLButtonElement>) => {
		event.stopPropagation();
		try {
			await openAgentsBridge.clipboard.writeText(value);
		} catch {
			return;
		}
		setCopied(true);
		if (copiedTimeoutRef.current !== null) clearTimeout(copiedTimeoutRef.current);
		copiedTimeoutRef.current = setTimeout(() => {
			setCopied(false);
			copiedTimeoutRef.current = null;
		}, 1_500);
	};
	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<button
					aria-label={buttonLabel}
					className="inline-flex size-4 shrink-0 items-center justify-center rounded-sm text-passive transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
					onClick={(event) => void copyValue(event)}
					type="button"
				>
					{copied ? (
						<Check className="size-icon-2xs text-success" aria-hidden="true" />
					) : (
						<Copy className="size-icon-2xs" aria-hidden="true" />
					)}
				</button>
			</TooltipTrigger>
			<TooltipContent side="bottom">{buttonLabel}</TooltipContent>
		</Tooltip>
	);
}
