import AsyncStorage from "@react-native-async-storage/async-storage";
import * as SecureStore from "expo-secure-store";
import type { Endpoint } from "./endpoints";

/**
 * A paired machine.
 *
 * Endpoints are a list because the phone races them, and they are refreshed on
 * every successful connect — that is what lets a rotated tunnel hostname or a
 * new LAN address heal itself without the user re-pairing.
 */
export type Host = {
	/** Stable, daemon-issued. Every probe answer is checked against this before
	 * the endpoint is trusted or the token is presented. */
	id: string;
	/** Hostname by default; the user can rename it. */
	name: string;
	platform: string;
	endpoints: Endpoint[];
	/** Connection token. Lives in the device keystore, never in AsyncStorage. */
	token: string;
	lastConnected: number;
};

/**
 * Enough for a developer's machines without letting stale pairings pile up.
 *
 * Storage only. There is no host switcher in the UI yet: activeHost() answers
 * with an explicit selection where one exists and the most recent machine
 * otherwise, and the only things that set a selection today are pairing and
 * manual connect. So a second machine is reachable by pairing with it, not by
 * choosing it from a list — the plumbing is here, the picker is not.
 */
export const MAX_HOSTS = 10;

const HOSTS_KEY = "openAgents.hosts";
const ACTIVE_HOST_KEY = "openAgents.activeHost";
const tokenKey = (id: string) => `openAgents.hostToken.${id}`;

/** What is written to AsyncStorage: everything except the token. */
export type HostMetadata = Omit<Host, "token">;
type StoredHost = HostMetadata;

function isStoredHost(v: unknown): v is StoredHost {
	if (typeof v !== "object" || v === null) return false;
	const h = v as Record<string, unknown>;
	// An empty id is valid: a machine migrated from the single-server config has
	// not been issued one yet and adopts it on first connect.
	return typeof h.id === "string" && Array.isArray(h.endpoints);
}

async function readStored(): Promise<StoredHost[]> {
	try {
		const raw = await AsyncStorage.getItem(HOSTS_KEY);
		if (!raw) return [];
		const parsed: unknown = JSON.parse(raw);
		if (!Array.isArray(parsed)) return [];
		return parsed.filter(isStoredHost);
	} catch {
		// Corrupted storage must not brick the app; the user re-pairs instead.
		return [];
	}
}

function sortAndCap(hosts: StoredHost[]): StoredHost[] {
	return [...hosts].sort((a, b) => b.lastConnected - a.lastConnected).slice(0, MAX_HOSTS);
}

async function writeStored(hosts: StoredHost[]): Promise<void> {
	await AsyncStorage.setItem(HOSTS_KEY, JSON.stringify(sortAndCap(hosts)));
}

/** Every paired machine, most recently connected first. */
export async function loadHosts(): Promise<Host[]> {
	const stored = sortAndCap(await readStored());
	return Promise.all(
		stored.map(async (h) => ({
			...h,
			token: (await SecureStore.getItemAsync(tokenKey(h.id))) ?? "",
		})),
	);
}

/** One machine by id, or null. */
export async function findHost(id: string): Promise<Host | null> {
	return (await loadHosts()).find((h) => h.id === id) ?? null;
}

/** Adds or replaces a machine. Re-pairing the same machine updates it in place
 * rather than adding a second entry for it. */
export async function saveHost(host: Host): Promise<void> {
	const { token, ...rest } = host;
	const others = (await readStored()).filter((h) => h.id !== host.id);
	await writeStored([rest, ...others]);
	if (token) {
		await SecureStore.setItemAsync(tokenKey(host.id), token);
	} else {
		await SecureStore.deleteItemAsync(tokenKey(host.id));
	}
}

/**
 * Replaces a machine's endpoint list, leaving its token alone.
 *
 * Called after every successful connect with whatever the daemon now
 * advertises, so a rotated tunnel hostname or a changed LAN address is picked
 * up without the user doing anything.
 */
export async function updateHostEndpoints(id: string, endpoints: Endpoint[]): Promise<void> {
	const stored = await readStored();
	const next = stored.map((h) => (h.id === id ? { ...h, endpoints } : h));
	await writeStored(next);
}

/** Records a successful connection, so the list orders by recency. */
export async function touchHost(id: string, at: number = Date.now()): Promise<void> {
	const stored = await readStored();
	await writeStored(stored.map((h) => (h.id === id ? { ...h, lastConnected: at } : h)));
}

/**
 * Forgets a machine.
 *
 * Both tiers are cleared: wiping only the AsyncStorage entry would leave the
 * token in the keystore, and a later re-pair of the same machine would silently
 * resurrect it.
 */
/**
 * The machine the app is talking to.
 *
 * Selection used to be emergent: loadHosts is ordered most-recent-first and
 * callers took the head. Nothing owned it, so nothing could change it — which
 * is why a manual connection snapped back to the previous machine on reload,
 * and why forgetting a server left it reachable.
 *
 * Recency remains the fallback, so a first pairing needs no explicit selection
 * and a stale pointer cannot strand the app with no host at all.
 */
export async function activeHost(): Promise<Host | null> {
	const host = await activeHostMetadata();
	if (!host) return null;
	return { ...host, token: (await SecureStore.getItemAsync(tokenKey(host.id))) ?? "" };
}

/** The selected machine without opening the token store. Cleanup uses this so
 * a keychain read failure cannot prevent the user from forgetting a server. */
export async function activeHostMetadata(): Promise<HostMetadata | null> {
	const hosts = sortAndCap(await readStored());
	if (hosts.length === 0) return null;
	const selected = await AsyncStorage.getItem(ACTIVE_HOST_KEY);
	return hosts.find((h) => h.id === selected) ?? hosts[0];
}

/** Point the app at a machine, overriding recency until it changes again. */
export async function setActiveHost(id: string): Promise<void> {
	await AsyncStorage.setItem(ACTIVE_HOST_KEY, id);
}

export async function clearActiveHost(): Promise<void> {
	await AsyncStorage.removeItem(ACTIVE_HOST_KEY);
}

export async function removeHost(id: string): Promise<void> {
	const stored = await readStored();
	await writeStored(stored.filter((h) => h.id !== id));
	await SecureStore.deleteItemAsync(tokenKey(id));
	// A pointer at a machine that no longer exists would resolve to nothing;
	// clearing it falls back to recency instead.
	if ((await AsyncStorage.getItem(ACTIVE_HOST_KEY)) === id) await clearActiveHost();
}

/**
 * Gives a host the stable identity it just reported.
 *
 * A newly paired host can be stored before the daemon has issued its id. It
 * connects once to learn that id, then every endpoint it races is checked
 * against the stored value.
 *
 * The token is moved to the new key as part of the same operation. Only a host
 * that genuinely has no identity is rekeyed; a host that already has one must
 * never be renamed by whatever answered.
 */
export async function adoptHostIdentity(oldId: string, hostId: string): Promise<void> {
	if (oldId !== "" || hostId === "") return;

	const stored = await readStored();
	const target = stored.find((h) => h.id === "");
	if (!target) return;

	const token = (await SecureStore.getItemAsync(tokenKey(""))) ?? "";
	await writeStored(stored.map((h) => (h.id === "" ? { ...h, id: hostId } : h)));
	if (token) {
		await SecureStore.setItemAsync(tokenKey(hostId), token);
		await SecureStore.deleteItemAsync(tokenKey(""));
	}
}
