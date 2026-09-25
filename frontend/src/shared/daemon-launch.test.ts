import { describe, expect, it } from "vitest";
import { bundledDaemonIdentityError, resolveDaemonLaunch } from "./daemon-launch";

describe("resolveDaemonLaunch", () => {
	it("uses OPEN_AGENTS_DAEMON_COMMAND when configured", () => {
		expect(
			resolveDaemonLaunch({ OPEN_AGENTS_DAEMON_COMMAND: "/tmp/open-agents daemon" }, false, "/resources", "/app", "/home/user", "darwin"),
		).toEqual({
			command: "/tmp/open-agents daemon",
			args: [],
			cwd: "/app",
			shell: true,
			source: "configured",
		});
	});

	it("runs the backend daemon from source in non-Windows dev without an explicit command", () => {
		expect(resolveDaemonLaunch({}, false, "/resources", "/repo/frontend", "/home/user", "darwin")).toEqual({
			command: "go",
			args: ["run", "./cmd/open-agents", "daemon"],
			cwd: "/repo/frontend/../backend",
			shell: false,
			source: "dev",
		});
	});

	it("uses the prebuilt daemon exe in Windows dev", () => {
		expect(resolveDaemonLaunch({}, false, "/resources", "C:\\repo\\frontend", "C:\\Users\\alice", "win32")).toEqual({
			command: "C:\\repo\\frontend/daemon/open-agents.exe",
			args: ["daemon"],
			cwd: "C:\\repo\\frontend",
			shell: false,
			source: "dev",
		});
	});

	it("uses the versioned daemon exe in Windows dev when build-daemon wrote one", () => {
		expect(
			resolveDaemonLaunch(
				{ OPEN_AGENTS_DEV_DAEMON_BINARY: "C:\\repo\\frontend\\daemon\\dev-123\\open-agents.exe" },
				false,
				"/resources",
				"C:\\repo\\frontend",
				"C:\\Users\\alice",
				"win32",
			),
		).toEqual({
			command: "C:\\repo\\frontend\\daemon\\dev-123\\open-agents.exe",
			args: ["daemon"],
			cwd: "C:\\repo\\frontend",
			shell: false,
			source: "dev",
		});
	});

	it("uses the bundled daemon binary for packaged macOS/Linux builds", () => {
		expect(
			resolveDaemonLaunch(
				{},
				true,
				"/Applications/Open Agents.app/Contents/Resources",
				"/app",
				"/Users/alice",
				"darwin",
			),
		).toEqual({
			command: "/Applications/Open Agents.app/Contents/Resources/daemon/open-agents",
			args: ["daemon"],
			cwd: "/Users/alice/.open-agents",
			shell: false,
			source: "bundled",
		});
	});

	it("uses the bundled daemon exe for packaged Windows builds", () => {
		expect(
			resolveDaemonLaunch(
				{},
				true,
				"C:\\Program Files\\Open Agents\\resources",
				"C:\\Program Files\\Open Agents\\resources\\app.asar",
				"C:\\Users\\alice",
				"win32",
			),
		).toEqual({
			command: "C:\\Program Files\\Open Agents\\resources/daemon/open-agents.exe",
			args: ["daemon"],
			cwd: "C:\\Users\\alice/.open-agents",
			shell: false,
			source: "bundled",
		});
	});
});

describe("bundledDaemonIdentityError", () => {
	const samePath = (a: string, b: string): boolean => a === b;
	const appImage = "/home/user/Apps/open-agents.AppImage";
	// The bundled command under AppImage: a random FUSE mount, different per launch.
	const launchCommand = "/tmp/.mount_agent-mDQfUL/resources/daemon/open-agents";

	it("accepts the same install across two AppImage mounts (relaunch-to-update)", () => {
		const probe = {
			executablePath: "/tmp/.mount_agent-1Qs4N6/resources/daemon/open-agents",
			appImagePath: appImage,
		};
		expect(bundledDaemonIdentityError(probe, launchCommand, appImage, samePath)).toBeNull();
	});

	it("rejects a daemon from a different AppImage install", () => {
		const other = "/home/user/Apps/open-agents-nightly.AppImage";
		const probe = { executablePath: "/tmp/.mount_agent-1Qs4N6/resources/daemon/open-agents", appImagePath: other };
		expect(bundledDaemonIdentityError(probe, launchCommand, appImage, samePath)).toBe(
			`Another Open Agents daemon is already running from ${other}; expected ${appImage}. Stop the other daemon before using this app.`,
		);
	});

	it("fails closed under AppImage when the daemon does not report its install identity", () => {
		const probe = { executablePath: "/tmp/.mount_agent-1Qs4N6/resources/daemon/open-agents" };
		expect(bundledDaemonIdentityError(probe, launchCommand, appImage, samePath)).toBe(
			"An older Open Agents daemon is already running, but it does not report its install identity. Stop it and restart this app.",
		);
	});

	it("compares executable paths outside AppImage", () => {
		const command = "/opt/Open Agents/resources/daemon/open-agents";
		expect(bundledDaemonIdentityError({ executablePath: command }, command, undefined, samePath)).toBeNull();
		expect(bundledDaemonIdentityError({ executablePath: "/other/open-agents" }, command, undefined, samePath)).toBe(
			`Another Open Agents daemon is already running from /other/open-agents; expected ${command}. Stop the other daemon before using this app.`,
		);
	});

	it("fails closed outside AppImage when the daemon does not report its binary path", () => {
		expect(bundledDaemonIdentityError({}, "/opt/open-agents/resources/daemon/open-agents", undefined, samePath)).toBe(
			"An older Open Agents daemon is already running, but it does not report its binary path. Stop it and restart this app.",
		);
	});
});
