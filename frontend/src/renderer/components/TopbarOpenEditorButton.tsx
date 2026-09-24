import { ChevronDown, Code2, FolderOpen, SquareTerminal } from "lucide-react";
import { useState } from "react";
import type { OpenTarget, OpenTargetId } from "../../shared/editor-handoff";
import { useEditorHandoffState, useOpenSessionTarget } from "../hooks/useEditorHandoff";
import { TopbarActionError, TopbarButton } from "./TopbarButton";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuLabel,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from "./ui/dropdown-menu";
import { Tooltip, TooltipContent, TooltipTrigger } from "./ui/tooltip";
import {
	AndroidStudioIcon,
	AntigravityIcon,
	CursorIcon,
	JetBrainsIcon,
	SublimeIcon,
	VSCodeIcon,
	VSCodiumIcon,
	WindsurfIcon,
	ZedIcon,
} from "./icons";

const editorIcons: Record<string, typeof VSCodeIcon> = {
	vscode: VSCodeIcon,
	"vscode-insiders": VSCodeIcon,
	vscodium: VSCodiumIcon,
	cursor: CursorIcon,
	windsurf: WindsurfIcon,
	zed: ZedIcon,
	antigravity: AntigravityIcon,
	sublime: SublimeIcon,
	"android-studio": AndroidStudioIcon,
	intellij: JetBrainsIcon,
	webstorm: JetBrainsIcon,
	pycharm: JetBrainsIcon,
	goland: JetBrainsIcon,
	phpstorm: JetBrainsIcon,
	rubymine: JetBrainsIcon,
	clion: JetBrainsIcon,
	rider: JetBrainsIcon,
	fleet: JetBrainsIcon,
};

const editorColors: Record<string, string> = {
	vscode: "#1F9CF0",
	"vscode-insiders": "#1F9CF0",
	vscodium: "#2F80ED",
	antigravity: "#4285F4",
	sublime: "#FF9800",
	"android-studio": "#3DDC84",
};

function TargetIcon({ target, className }: { target?: OpenTarget; className?: string }) {
	if (target?.kind === "file_manager") return <FolderOpen className={className} aria-hidden="true" />;
	if (target?.kind === "terminal") return <SquareTerminal className={className} aria-hidden="true" />;
	const Icon = (target && editorIcons[target.id]) || Code2;
	const color = target ? editorColors[target.id] : undefined;
	return <Icon className={className} style={color ? { color } : undefined} aria-hidden="true" />;
}

// Electron main owns the complete handoff: it resolves the loopback-only
// workspace path, launches the native target, and persists editor preference.
// This renderer receives only safe target metadata and availability status.
export function TopbarOpenEditorButton({
	sessionId,
	projectId,
	sessionCreatedAt,
	sessionTerminated,
	style,
}: {
	sessionId: string;
	projectId: string;
	sessionCreatedAt?: string;
	sessionTerminated?: boolean;
	style?: React.CSSProperties;
}) {
	const stateQuery = useEditorHandoffState(sessionId, { sessionCreatedAt, sessionTerminated });
	const open = useOpenSessionTarget();
	const state = stateQuery.data;
	const [menuOpen, setMenuOpen] = useState(false);
	const targets = state?.targets ?? [];
	const editors = targets.filter((target) => target.kind === "editor");
	const preferred = editors.find((target) => target.id === state?.preferredEditorId);
	const safeTargets = targets.filter((target) => target.kind !== "editor");
	const workspaceAvailable = state?.workspaceAvailable === true;
	const busy = stateQuery.isPending || open.isPending;
	const mainDisabled = busy || !workspaceAvailable || !preferred;
	const menuDisabled = busy || !workspaceAvailable || targets.length === 0;

	const launch = (targetId?: OpenTargetId) => {
		open.reset();
		open.mutate({ sessionId, projectId, ...(targetId ? { targetId } : {}) });
	};
	const launchError = open.error instanceof Error ? open.error.message : null;
	const workspaceError = !stateQuery.isPending && !workspaceAvailable
		? state?.unavailableReason ?? "Session workspace is not available."
		: null;
	const visibleActionError = launchError ?? workspaceError;
	const noEditorInstalled = !stateQuery.isPending && workspaceAvailable && editors.length === 0;
	const mainTitle = stateQuery.isPending
		? "Preparing workspace…"
		: (workspaceError
			?? (preferred
				? `Open this session's workspace in ${preferred.name}`
				: (noEditorInstalled ? "No editor installed" : "Choose an installed editor or use a native fallback")));

	return (
		<>
			{visibleActionError ? (
				<TopbarActionError className="max-w-content-max truncate" title={visibleActionError}>
					{visibleActionError}
				</TopbarActionError>
			) : null}
			<div
				className="inline-flex items-center gap-0 rounded-md transition-colors hover:bg-interactive-hover data-[state=open]:bg-interactive-hover"
				data-state={menuOpen ? "open" : "closed"}
				style={style}
			>
				<Tooltip>
					<TooltipTrigger asChild>
						<span className="inline-flex">
							<TopbarButton
								aria-label={stateQuery.isPending
									? "Preparing workspace…"
									: preferred
										? `Open in ${preferred.name}`
										: (noEditorInstalled ? "No editor installed" : "Choose editor")}
								className="hover:bg-transparent"
								disabled={mainDisabled}
								onClick={() => launch()}
								variant="icon"
							>
								<TargetIcon target={preferred} className="size-icon-md" />
							</TopbarButton>
						</span>
					</TooltipTrigger>
					<TooltipContent side="bottom">{mainTitle}</TooltipContent>
				</Tooltip>
				<DropdownMenu onOpenChange={setMenuOpen}>
					<Tooltip>
						<TooltipTrigger asChild>
							<span className="inline-flex">
								<DropdownMenuTrigger asChild>
									<TopbarButton
										aria-label="Open workspace options"
										className="hover:bg-transparent"
										disabled={menuDisabled}
										variant="icon"
									>
										<ChevronDown className="size-icon-sm" aria-hidden="true" />
									</TopbarButton>
								</DropdownMenuTrigger>
							</span>
						</TooltipTrigger>
						<TooltipContent side="bottom">{"Open workspace options"}</TooltipContent>
					</Tooltip>
					<DropdownMenuContent align="end" className="min-w-52">
						{safeTargets.map((target) => (
							<DropdownMenuItem key={target.id} onSelect={() => launch(target.id)}>
								<TargetIcon target={target} className="size-icon-sm" />
								{`Open in ${target.name}`}
							</DropdownMenuItem>
						))}
						{safeTargets.length > 0 && editors.length > 0 ? <DropdownMenuSeparator /> : null}
						{editors.length > 0 ? <DropdownMenuLabel>{"Open with"}</DropdownMenuLabel> : null}
						{editors.map((editor) => (
							<DropdownMenuItem key={editor.id} onSelect={() => launch(editor.id)}>
								<TargetIcon target={editor} className="size-icon-sm" />
								{editor.name}
							</DropdownMenuItem>
						))}
					</DropdownMenuContent>
				</DropdownMenu>
			</div>
		</>
	);
}
