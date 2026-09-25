import type { OpenAgentsBridge } from "../preload";

declare global {
	interface Window {
		openAgents?: OpenAgentsBridge;
	}

	interface ImportMetaEnv {
		readonly VITE_OPEN_AGENTS_POSTHOG_KEY?: string;
		readonly VITE_OPEN_AGENTS_POSTHOG_HOST?: string;
	}
}

export {};
