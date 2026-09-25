import { type CSSProperties } from "react";
import { Plus } from "lucide-react";
import type { ProjectManagerAction } from "../hooks/useProjectManagerAction";
import { getAgentActivityView } from "../lib/session-presentation";
import { TopbarActionError, TopbarButton } from "./TopbarButton";
import { ManagerActivityIndicator } from "./ManagerActivityIndicator";
import { ManagerIcon } from "./icons";
import { Tooltip, TooltipContent, TooltipTrigger } from "./ui/tooltip";

export function ProjectBoardActions({ actions, placement, quiet = false, style }: {
	actions: ProjectManagerAction;
	placement: "header" | "empty";
	quiet?: boolean;
	style?: CSSProperties;
}) {
	const { manager, isSpawning, isProjectRestarting, isProvisioning, spawnError, canCreateAsTui,
		openNewTask, openManager } = actions;
	const header = placement === "header";
	const busy = isSpawning || isProjectRestarting || isProvisioning;
	const activity = manager ? getAgentActivityView(manager.activity).label : undefined;
	const actionLabel = manager ? "Open manager" : "Spawn Manager";
	const busyLabel = isProjectRestarting ? "Restarting..." : isProvisioning
		? "Setting up..." : isSpawning ? "Spawning..." : undefined;
	const managerButton = (
		<Tooltip>
			<TooltipTrigger asChild>
				<span className="inline-flex" style={style}>
					<TopbarButton
						aria-label={activity ? `Manager, ${activity}` : actionLabel}
						aria-busy={busy}
						className={header ? "topbar-control--labeled" : undefined}
						data-priority={header ? "secondary" : undefined}
						disabled={busy}
						onClick={() => openManager()}
						variant={quiet ? "secondary" : "primary"}
					>
						<ManagerIcon className="size-icon-md" aria-hidden="true" />
						<span data-compact-label={header ? "" : undefined}>{busyLabel ?? "Manager"}</span>
						{manager ? <ManagerActivityIndicator session={manager} /> : null}
					</TopbarButton>
				</span>
			</TooltipTrigger>
			<TooltipContent side="bottom">{busyLabel ?? actionLabel}</TooltipContent>
		</Tooltip>
	);
	const newTaskButton = (
		<Tooltip>
			<TooltipTrigger asChild>
				<span className="inline-flex" style={style}>
					<TopbarButton
						aria-label="New task"
						className={header ? "topbar-control--labeled" : undefined}
						data-priority={header ? "primary" : undefined}
						disabled={isProjectRestarting || isProvisioning}
						onClick={openNewTask}
						variant={quiet ? "secondary" : "accent"}
					>
						<Plus className="size-icon-md" aria-hidden="true" />
						<span data-compact-label={header ? "" : undefined}>{(header ? "Task" : "New task")}</span>
					</TopbarButton>
				</span>
			</TooltipTrigger>
			<TooltipContent side="bottom">{"New task"}</TooltipContent>
		</Tooltip>
	);
	const feedback = spawnError && !quiet ? (
		<div className={header ? "contents" : "mt-3 flex flex-col items-center gap-2"}>
			<TopbarActionError role={header ? "alert" : "status"} className={header ? "max-w-content-max truncate" : "text-caption leading-body"} title={spawnError}>
				{spawnError}
			</TopbarActionError>
			{canCreateAsTui ? <TopbarButton disabled={busy} onClick={() => openManager("tui")} style={style}>{"Create as Terminal UI"}</TopbarButton> : null}
		</div>
	) : null;
	return header ? <>{feedback}{newTaskButton}{managerButton}</> : <>
		<div className="mt-5 flex items-center gap-2">{managerButton}{newTaskButton}</div>
		{feedback}
	</>;
}
