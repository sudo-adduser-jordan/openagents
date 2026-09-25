import { openAgentsBridge } from "./bridge";
import { setApiBaseUrl, setApiDaemonStatus } from "./api-client";

export type DaemonStatus = Awaited<ReturnType<typeof openAgentsBridge.daemon.getStatus>>;

export function applyDaemonStatus(nextStatus: DaemonStatus): void {
	setApiDaemonStatus(nextStatus);
	if (nextStatus.state === "ready" && nextStatus.port) {
		setApiBaseUrl(`http://127.0.0.1:${nextStatus.port}`);
	} else {
		setApiBaseUrl(null);
	}
}

export async function refreshDaemonStatus(): Promise<DaemonStatus> {
	const nextStatus = await readDaemonStatus();
	applyDaemonStatus(nextStatus);
	return nextStatus;
}

export function readDaemonStatus(): Promise<DaemonStatus> {
	return openAgentsBridge.daemon.getStatus();
}
