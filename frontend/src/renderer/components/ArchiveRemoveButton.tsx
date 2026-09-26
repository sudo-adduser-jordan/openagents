import { Trash2 } from "lucide-react";
import { useState } from "react";

import { Tooltip, TooltipContent, TooltipTrigger } from "./ui/tooltip";
import { cn } from "../lib/utils";

/**
 * The remove affordance on an archived card.
 *
 * The confirmation names what goes, because the honest answer is "more than the
 * card": the session row, its change log, the PR facts and conversation turns
 * that cascade from it, and its number, which is retired so it is never reused.
 * The worktree directory is *not* touched, and saying so matters too -- a user
 * deciding whether to remove a task should not have to guess.
 *
 * Removal is offered rather than refused when something would be lost, per the
 * product decision behind it. The warning is the safeguard, so it has to be
 * specific enough to act on.
 */
export function ArchiveRemoveButton({
	label,
	onRemove,
	isRemoving,
}: {
	label: string;
	onRemove: () => void;
	isRemoving: boolean;
}) {
	const [confirming, setConfirming] = useState(false);

	if (!confirming) {
		return (
			<Tooltip>
				<TooltipTrigger asChild>
					<span className="inline-flex">
						<button
							aria-label={label}
							className="grid size-control-board-sm shrink-0 place-items-center rounded-md text-passive transition-colors hover:bg-error/10 hover:text-error focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-accent/50 disabled:cursor-not-allowed disabled:opacity-35"
							disabled={isRemoving}
							onClick={(event) => {
								event.stopPropagation();
								setConfirming(true);
							}}
							type="button"
						>
							<Trash2 className={cn("size-icon-md", isRemoving && "animate-spin")} aria-hidden="true" />
						</button>
					</span>
				</TooltipTrigger>
				<TooltipContent side="top">{"Remove session for good"}</TooltipContent>
			</Tooltip>
		);
	}

	return (
		<span className="flex items-center gap-1" data-testid="archive-remove-confirm">
			<span className="text-2xs text-settings-muted">Remove permanently?</span>
			<button
				aria-label={`Confirm removing ${label}`}
				className="rounded-sm bg-danger-strong px-1.5 py-0.5 text-2xs font-semibold text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/60"
				onClick={(event) => {
					event.stopPropagation();
					onRemove();
				}}
				type="button"
			>
				Yes
			</button>
			<button
				aria-label="Keep session"
				className="rounded-sm border border-border/80 px-1.5 py-0.5 text-2xs font-medium text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/60"
				onClick={(event) => {
					event.stopPropagation();
					setConfirming(false);
				}}
				type="button"
			>
				No
			</button>
		</span>
	);
}

/** What removal destroys, stated plainly rather than as a generic warning. */
export const archiveRemoveWarning =
	"Removes the session, its pull request history, and its conversation history. The worktree folder is left on disk.";
