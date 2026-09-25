import { describe, expect, it } from "vitest";
import {
	refreshSlowDaemonStartupDetails,
	slowDaemonStartupStatus,
} from "./daemon-startup-status";

describe("slowDaemonStartupStatus", () => {
	it("keeps a live child in starting state and refreshes captured output", () => {
		const status = slowDaemonStartupStatus({
			output: "initial recovery output",
			executablePath: "/Applications/Open Agents.app/open-agents",
			workingDirectory: "/Users/example/.open-agents",
			handshakePath: "/Users/example/.open-agents/running.json",
		});

		expect(status).toMatchObject({
			state: "starting",
			message: "Open Agents daemon is still starting. Session recovery can take a while.",
			details: "initial recovery output",
			executablePath: "/Applications/Open Agents.app/open-agents",
			workingDirectory: "/Users/example/.open-agents",
		});
		expect(status.code).toBeUndefined();

		expect(refreshSlowDaemonStartupDetails(status, "latest recovery output")).toMatchObject({
			state: "starting",
			details: "latest recovery output",
		});
	});

	it("does not rewrite an ordinary starting status", () => {
		const status = { state: "starting" as const };
		expect(refreshSlowDaemonStartupDetails(status, "output")).toBe(status);
	});
});
