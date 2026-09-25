import { describe, expect, it } from "vitest";
import { resolveDesktopDataDir } from "./data-dir";

describe("resolveDesktopDataDir", () => {
	it("resolves one absolute data directory against the daemon launch cwd", () => {
		expect(resolveDesktopDataDir({ OPEN_AGENTS_DATA_DIR: "relative-data" }, "/home/open-agents", "/work/checkout", false)).toBe(
			"/work/checkout/relative-data",
		);
		expect(resolveDesktopDataDir({}, "/home/open-agents", "/work/checkout", true)).toBe("/home/open-agents/.open-agents/data");
		expect(resolveDesktopDataDir({}, "/home/open-agents", "/work/checkout", false)).toBe("/home/open-agents/.open-agents/dev/data");
	});
});
