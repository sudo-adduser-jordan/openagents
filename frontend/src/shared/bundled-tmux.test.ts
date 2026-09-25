import { describe, expect, it } from "vitest";
import { bundledTmuxBinaryPath, stableBundledTmuxBinaryPath } from "./bundled-tmux";

describe("bundledTmuxBinaryPath", () => {
	it.each(["darwin", "linux"] as const)("uses the packaged tmux on %s", (platform) => {
		expect(bundledTmuxBinaryPath(true, "/opt/open-agents/resources", platform)).toBe(
			"/opt/open-agents/resources/tmux/bin/tmux",
		);
	});

	it("does not override tmux in development", () => {
		expect(bundledTmuxBinaryPath(false, "/opt/open-agents/resources", "darwin")).toBeNull();
	});

	it("does not require tmux on Windows", () => {
		expect(bundledTmuxBinaryPath(true, "C:\\Open Agents\\resources", "win32")).toBeNull();
	});
});

describe("stableBundledTmuxBinaryPath", () => {
	it.each(["darwin", "linux"] as const)("uses durable versioned Open Agents storage on %s", (platform) => {
		expect(stableBundledTmuxBinaryPath(true, "/home/me/.open-agents", "0.10.3", platform, "arm64")).toBe(
			`/home/me/.open-agents/runtime/tmux/0.10.3-${platform}-arm64/tmux`,
		);
	});

	it("sanitizes version components rather than allowing path traversal", () => {
		expect(stableBundledTmuxBinaryPath(true, "/home/me/.open-agents", "../next build", "linux", "x64")).toBe(
			"/home/me/.open-agents/runtime/tmux/.._next_build-linux-x64/tmux",
		);
	});

	it("does not stage a Windows binary", () => {
		expect(stableBundledTmuxBinaryPath(true, "C:\\Open Agents", "0.10.3", "win32", "x64")).toBeNull();
	});
});
