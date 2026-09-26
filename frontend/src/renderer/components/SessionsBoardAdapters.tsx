import { useEffect, useRef, useState, type MouseEvent, type ReactNode } from "react";
import { useQueryClient } from "@tanstack/react-query";
import {
	scmUserAvatarUrl,
	SessionCardView,
	SessionUsageMetricView,
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
import type { WorkflowMode, WorkspaceSession } from "../types/workspace";
import { canonicalTrackerIssueId } from "../types/workspace";
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
	session,
	usage,
}: {
	onOpen: () => void;
	onTerminate: () => void;
	onWorkflowModeChange?: (session: WorkspaceSession, workflowMode: WorkflowMode) => void;
	onReviewToCommit?: (session: WorkspaceSession) => void;
	session: WorkspaceSession;
	usage?: SessionUsageSummary;
}) {
	return (
		<DesktopSessionCard
			onOpen={onOpen}
			onTerminate={onTerminate}
			onWorkflowModeChange={onWorkflowModeChange}
			onReviewToCommit={onReviewToCommit}
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
			footer={<ArchiveRestoreError message={restoreError ?? removeError} />}
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
	session: WorkspaceSession;
	usage?: SessionUsageSummary;
}) {
	const queryClient = useQueryClient();
	const [confirmOpen, setConfirmOpen] = useState(false);
	const summaries = sessionPRDisplaySummaries(session, useSessionScmSummary(session.id).data);
	const termination = useTerminateSessionState(session.id);
	const showTerminate = interactive && session.isTerminated !== true && onTerminate;
	const keepTerminateVisible = session.status === "merged";
	const usagePresentation = toUsagePresentation(usage);
	// The daemon reports a finished pre-PR session as "Awaiting PR". That text
	// is replaced here with the delivery-stage actions: confirm a finished plan,
	// or review the pending edit so the agent commits and waits on PR approval.
	const pausedAwaitingPR = session.displayStatus === "Awaiting PR";
	const canAdvance = interactive && session.isTerminated !== true;
	const statusAction =
		pausedAwaitingPR && canAdvance ? (
			<div className="flex min-w-0 items-center gap-1.5">
				{session.workflowMode === "planning" && onWorkflowModeChange ? (
					<WorkflowStageActionButton
						label="Build"
						onClick={() => onWorkflowModeChange(session, "building")}
						title="Approve this plan and let the worker start building"
					/>
				) : null}
				{onReviewToCommit ? (
					<WorkflowStageActionButton
						label="Commit"
						onClick={() => onReviewToCommit(session)}
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
		<SessionCardView
			action={action}
			branchAction={branchAction}
			branchIcon={<GitBranch aria-hidden="true" className="size-icon-2xs shrink-0" />}
			error={termination.error ?? undefined}
			externalLink={ProductExternalLink}
			footer={footer}
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
	);
}

/** Compact card action used in place of the "Awaiting PR" status label. */
function WorkflowStageActionButton({
	label,
	onClick,
	title,
}: {
	label: string;
	onClick: () => void;
	/** Carries the meaning a one-word label drops. Falls back to no tooltip. */
	title?: string;
}) {
	return (
		<button
			type="button"
			onClick={(event) => {
				event.stopPropagation();
				onClick();
			}}
			title={title}
			className="inline-flex min-w-0 items-center rounded-sm border border-border/80 px-1.5 py-px text-2xs font-medium leading-none text-foreground transition-colors hover:border-foreground/30 hover:bg-surface focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/60"
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

function ArchiveRestoreError({ message }: { message?: string }) {
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
