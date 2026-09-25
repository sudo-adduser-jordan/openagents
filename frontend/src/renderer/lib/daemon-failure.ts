import type { DaemonStatus } from "../../shared/daemon-status";

export function daemonFailureMessage(status: DaemonStatus): string {
	// Prefer the daemon-provided English diagnostic when present.
	if (status.message) return status.message;
	if (status.state === "starting") return "Open Agents daemon is starting.";
	return "Open Agents daemon is not ready.";
}

export function daemonFailureTitle(status: DaemonStatus): string {
	switch (status.code) {
		case "not_ready":
		case "port_unconfirmed":
			return "Open Agents daemon is not ready yet";
		case "not_configured":
			return "Open Agents daemon is not configured";
		case "daemon_unreachable":
			return "Open Agents daemon is unreachable";
		case "identity_mismatch":
			return "Open Agents daemon identity check failed";
		case "binary_missing":
			return "Open Agents daemon binary is missing";
		case "spawn_failed":
		case "exited":
		default:
			return "Open Agents daemon failed to start";
	}
}

export function daemonFailureHint(status: DaemonStatus): string {
	switch (status.code) {
		case "binary_missing":
			return "Run npm run build:daemon to rebuild the daemon.";
		case "spawn_failed":
		case "exited":
			return "";
		case "not_ready":
			return "The daemon has not passed its readiness check yet. Open details below for more information.";
		case "not_configured":
			return "Set OPEN_AGENTS_DAEMON_COMMAND or run the desktop app from a source checkout.";
		case "daemon_unreachable":
		case "identity_mismatch":
			return "Stop the conflicting daemon, then restart the desktop app.";
		default:
			return "Check the terminal where you ran npm run dev for details.";
	}
}
