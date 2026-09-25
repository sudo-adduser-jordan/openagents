import { apiClient, apiErrorCode, apiErrorMessage, apiErrorRequestId } from "./api-client";
import type { ManagerSpawnSource } from "./manager-spawn-sources";
import type { SessionMode } from "../types/conversation";

export type { ManagerSpawnSource };

const CHAT_PREFLIGHT_CODES = new Set([
	"SESSION_MODE_UNSUPPORTED",
	"CHAT_DRIVER_UNAVAILABLE",
	"CHAT_DRIVER_INCOMPATIBLE",
	"CHAT_AUTH_REQUIRED",
]);

/** A rejected manager spawn without flattening the daemon's error envelope. */
export class ManagerSpawnError extends Error {
	constructor(
		message: string,
		readonly code?: string,
		readonly requestId?: string,
		readonly status?: number,
	) {
		super(message);
		this.name = "ManagerSpawnError";
	}
}

export function isChatPreflightCode(code?: string): boolean {
	return Boolean(code && CHAT_PREFLIGHT_CODES.has(code));
}

export function isChatPreflightError(error: unknown): error is ManagerSpawnError {
	return error instanceof ManagerSpawnError && isChatPreflightCode(error.code);
}

/** Spawn the project's manager session via the daemon API. When clean is
 *  true the daemon first tears down any active manager for the project, then
 *  re-spawns one on the canonical branch (reattaching the existing branch). */
export async function spawnManager(
	projectId: string,
	_source: ManagerSpawnSource,
	clean = false,
	mode?: SessionMode,
): Promise<string> {
	const { data, error, response } = await apiClient.POST("/api/v1/managers", {
		body: { projectId, clean, ...(mode ? { mode } : {}) },
	});

	if (error || !data?.manager?.id) {
		const message = error
			? apiErrorMessage(error, `Failed to spawn manager (${response.status})`)
			: `Failed to spawn manager (${response.status})`;
		throw new ManagerSpawnError(
			message,
			apiErrorCode(error),
			apiErrorRequestId(error),
			response.status,
		);
	}

	return data.manager.id;
}
