// @vitest-environment node
import { describe, it, expect, beforeEach, afterEach } from "vitest";
import { mkdtemp, rm, writeFile, readdir } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import {
	readUiSettings,
	writeUiSettings,
	UI_SETTINGS_FILE_NAME,
	DEFAULT_UI_SETTINGS,
} from "./ui-settings";

describe("ui-settings", () => {
	let dir: string;
	beforeEach(async () => {
		dir = await mkdtemp(path.join(os.tmpdir(), "open-agents-ui-settings-"));
	});
	afterEach(async () => {
		await rm(dir, { recursive: true, force: true });
	});

	it("returns safe defaults when no file exists", async () => {
		expect(await readUiSettings(dir)).toEqual(DEFAULT_UI_SETTINGS);
	});

	it("merges a partial write with previously persisted settings instead of replacing them", async () => {
		await writeUiSettings(dir, { soundNotificationsEnabled: false });
		expect(await readUiSettings(dir)).toEqual({ ...DEFAULT_UI_SETTINGS, soundNotificationsEnabled: false });
	});

	it("merges the terminal shell without resetting other UI settings", async () => {
		await writeUiSettings(dir, { soundNotificationsEnabled: false });
		await writeUiSettings(dir, { terminalShell: { kind: "git-bash" } });
		expect(await readUiSettings(dir)).toEqual({
			soundNotificationsEnabled: false,
			terminalShell: { kind: "git-bash" },
		});
	});

	it("falls back to defaults on garbage", async () => {
		await writeFile(path.join(dir, UI_SETTINGS_FILE_NAME), "{not json", "utf8");
		expect(await readUiSettings(dir)).toEqual(DEFAULT_UI_SETTINGS);
	});

	it("atomic write leaves no temp file behind", async () => {
		await writeUiSettings(dir, { soundNotificationsEnabled: false });
		const entries = await readdir(dir);
		expect(entries).toEqual([UI_SETTINGS_FILE_NAME]);
	});
});
