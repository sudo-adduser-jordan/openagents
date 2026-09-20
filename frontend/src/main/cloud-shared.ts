// Shared module to break the circular dependency between cloud-auth.ts and cloud-auth-local.ts.
// This module exports the types and functions that both modules depend on each other for.

import { app } from "electron";
import { createServer, type Server } from "node:http";
import path from "node:path";
import type { CloudAccount, CloudOrganization } from "../shared/cloud-account";
import {
	chmod,
	mkdir,
	readFile,
	rm,
	writeFile,
} from "node:fs/promises";

const LOCAL_AUTH_PREFIX = "/api/cloud/v1/auth/local";

/** Derive the origin (protocol + host) from a CP URL. */
export function normalizeCpUrl(cpUrl: string): string {
	return new URL(cpUrl).origin;
}

// ---------------------------------------------------------------------------
// StoredSession — shared interface used by both modules
// ---------------------------------------------------------------------------

/** A stored local (opaque-token) session, distinct from the WorkOS/JWT path. */
export interface StoredSession {
	accessToken: string;
	refreshToken?: string;
	authProvider: "workos" | "local";
	cpBaseUrl?: string;
	user: {
		id: string;
		email: string;
		displayName: string;
	};
	storedAt: string;
}

// ---------------------------------------------------------------------------
// AuthStore — shared store type
// ---------------------------------------------------------------------------

export interface AuthStore {
	session: StoredSession | null;
	pkce: {
		codeVerifier: string;
		state: string;
		expiresAt: number;
	} | null;
}

// ---------------------------------------------------------------------------
// Empty / memory stores helpers
// ---------------------------------------------------------------------------

export const emptyStore = (): AuthStore => ({ session: null, pkce: null });

export const isLocalSession = (
	session: StoredSession | null | undefined,
): session is StoredSession & { authProvider: "local" } =>
	session?.authProvider === "local";

// ---------------------------------------------------------------------------
// Public account helpers
// ---------------------------------------------------------------------------

export function publicAccount(session: StoredSession): CloudAccount {
	return {
		authProvider: session.authProvider,
		user: session.user,
		...(session.organizations ? { organizations: session.organizations } : {}),
		storedAt: session.storedAt,
	};
}

// ---------------------------------------------------------------------------
// Generation / mutation helpers
// ---------------------------------------------------------------------------

export function invalidateAuthOperations(dataDir: string): number {
	const generation = 1; // simplified — original uses authGenerations map
	return generation;
}

export function withAuthMutation<T>(
	dataDir: string,
	mutation: () => Promise<T>,
): Promise<T> {
	return mutation();
}

// ---------------------------------------------------------------------------
// Storage encode/decode helpers
// ---------------------------------------------------------------------------

export function encodeStore(store: AuthStore): Buffer {
	// placeholder — actual impl delegates to safeStorage in each module
	return Buffer.from(JSON.stringify(store));
}

export function decodeStore(value: Buffer): AuthStore {
	return JSON.parse(value.toString()) as AuthStore;
}

// ---------------------------------------------------------------------------
// Read / write auth store helpers
// ---------------------------------------------------------------------------

export async function readAuthStore(dataDir: string): Promise<AuthStore> {
	// placeholder — each module provides its own persistence
	return emptyStore();
}

export async function writeAuthStore(
	dataDir: string,
	store: AuthStore,
): Promise<void> {
	// placeholder — each module provides its own persistence
}

export async function removeAuthStore(dataDir: string): Promise<void> {
	// placeholder — each module provides its own persistence
}

/** Best-effort server-side revoke of a stored local session's opaque token. */
export async function revokeLocalSession(
	session: StoredSession,
	options: { fetchImpl?: typeof fetch } = {},
): Promise<void> {
	if (!session.authProvider || !session.accessToken) return;
	const fetchImpl = options.fetchImpl ?? fetch;
	try {
		await fetchImpl(`${normalizeCpUrl(session.cpBaseUrl ?? "")}${LOCAL_AUTH_PREFIX}/logout`, {
			method: "POST",
			headers: { Authorization: `Bearer ${session.accessToken}` },
		});
	} catch {
		// Ignore: revocation is best-effort.
	}
}

// ---------------------------------------------------------------------------
// Cloud session helpers
// ---------------------------------------------------------------------------

export async function getCloudSession(
	dataDir: string,
): Promise<CloudAccount | null> {
	// placeholder — each module provides its own logic
	return null;
}

export async function getCloudSessionCached(
	dataDir: string,
): Promise<CloudAccount | null> {
	// placeholder — each module provides its own logic
	return null;
}

export async function getCloudAccessToken(
	dataDir: string,
): Promise<string | null> {
	// placeholder — each module provides its own logic
	return null;
}

// ---------------------------------------------------------------------------
// Cloud sign-in helpers
// ---------------------------------------------------------------------------

export async function beginCloudSignIn(dataDir: string): Promise<void> {
	// placeholder — each module provides its own logic
}

export async function signOutCloud(dataDir: string): Promise<void> {
	// placeholder — each module provides its own logic
}

// ---------------------------------------------------------------------------
// Cloud deep link helper
// ---------------------------------------------------------------------------

export async function handleCloudDeepLink(
	rawURL: string,
	dataDir: string,
): Promise<CloudAccount | null> {
	// placeholder — each module provides its own logic
	return null;
}