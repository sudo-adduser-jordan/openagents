import path from "node:path";

/** Resolve the Open Agents data directory, honoring an explicit `OPEN_AGENTS_DATA_DIR` override. */
export function resolveDesktopDataDir(
	env: Record<string, string | undefined>,
	homeDir: string,
	launchWorkingDirectory: string,
	isPackaged: boolean,
): string {
	const configured = env.OPEN_AGENTS_DATA_DIR?.trim();
	if (configured) return path.resolve(launchWorkingDirectory, configured);
	return path.resolve(homeDir, ".open-agents", isPackaged ? "data" : path.join("dev", "data"));
}
