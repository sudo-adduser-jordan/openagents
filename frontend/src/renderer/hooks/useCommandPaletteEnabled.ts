import { useQuery } from "@tanstack/react-query";
import { openAgentsBridge } from "../lib/bridge";
import { isCommandPaletteEnabled } from "../lib/build-channel";

export function useAppVersion(): string | undefined {
	const { data } = useQuery({
		queryKey: ["app-version"],
		queryFn: () => openAgentsBridge.app.getVersion(),
		staleTime: Infinity,
	});
	return typeof data === "string" ? data : undefined;
}

export function useCommandPaletteEnabled(): boolean {
	return isCommandPaletteEnabled(useAppVersion());
}
