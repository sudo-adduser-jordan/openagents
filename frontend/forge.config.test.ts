import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { mkdtempSync, mkdirSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
// postMake's dmg/zip branches only need to prove they call the right
// maker-dmg functions with the right gates; the functions' own behavior
// (sealDmg's credential matrix, verifyMacArtifact's script invocation) is
// covered by makers/maker-dmg.test.ts. Mocking the whole module keeps this
// suite from spawning codesign/xcrun/bash indirectly through the real chain.
// vi.mock's factory is hoisted above the rest of the file (including plain
// const declarations), so the mock fns themselves must go through vi.hoisted.
const { sealDmg, verifyDmg, verifyMacArtifact, isSigningConfigured } = vi.hoisted(() => ({
	sealDmg: vi.fn<(path: string) => Promise<boolean>>(),
	verifyDmg: vi.fn<(path: string) => Promise<void>>(async () => undefined),
	verifyMacArtifact: vi.fn<(path: string) => Promise<void>>(async () => undefined),
	isSigningConfigured: vi.fn<() => boolean>(),
}));
vi.mock("./makers/maker-dmg", async (importOriginal) => {
	const actual = await importOriginal<typeof import("./makers/maker-dmg")>();
	return { ...actual, sealDmg, verifyDmg, verifyMacArtifact, isSigningConfigured };
});

import config, { canonicalDarwinZipPath, extraResourcesForPlatform } from "./forge.config";

// Minimal synthetic Mach-O headers (thin little-endian + fat big-endian), the
// two on-disk layouts the signing selector must tell apart. Full parser
// coverage lives in makers/macho-archs.test.ts; here the fixtures exist so the
// per-file signing decision is exercised against real file bytes.
describe("canonicalDarwinZipPath", () => {
	it("uses the Open Agents platform and architecture release identity", () => {
		expect(canonicalDarwinZipPath("/out/Open Agents-darwin-arm64-0.13.0.zip")).toBe(
			"/out/open-agents-darwin-arm64-0.13.0.zip",
		);
	});
});

let fixtureDir: string;

beforeEach(() => {
	fixtureDir = mkdtempSync(join(tmpdir(), "forge-signing-"));
});

afterEach(() => {
	rmSync(fixtureDir, { recursive: true, force: true });
});

describe("native runtime resources", () => {
	it("fails packaging when the macOS helper was not copied into Resources", async () => {
		mkdirSync(join(fixtureDir, "Open Agents.app", "Contents", "Resources"), { recursive: true });
		const hook = config.hooks?.postPackage;
		expect(hook).toBeTypeOf("function");
		if (typeof hook !== "function") return;
		await expect(hook(config, { platform: "darwin", arch: "arm64", outputPaths: [fixtureDir] })).rejects.toThrow("packaged macOS update helper missing");
	});

	it("bundles the native update helper only on macOS", () => {
		expect(extraResourcesForPlatform("darwin")).toContain("update-helper");
		expect(extraResourcesForPlatform("linux")).not.toContain("update-helper");
		expect(extraResourcesForPlatform("win32")).not.toContain("update-helper");
	});

	it.each(["darwin", "linux"] as const)("bundles tmux on %s", (platform) => {
		expect(extraResourcesForPlatform(platform)).toContain("tmux");
	});

	it("does not bundle tmux on Windows", () => {
		expect(extraResourcesForPlatform("win32")).not.toContain("tmux");
	});
});

type MacSignOptions = {
	identity?: string;
	optionsForFile?: typeof macSignOptionsForFile;
};

async function loadMacSignOptions(env: {
	APPLE_SIGNING_IDENTITY?: string;
	CSC_LINK?: string;
}): Promise<MacSignOptions | undefined> {
	vi.stubEnv("APPLE_SIGNING_IDENTITY", env.APPLE_SIGNING_IDENTITY ?? "");
	vi.stubEnv("CSC_LINK", env.CSC_LINK ?? "");
	vi.resetModules();
	const { default: envConfig } = await import("./forge.config");
	return envConfig.packagerConfig?.osxSign as MacSignOptions | undefined;
}

afterEach(() => {
	vi.unstubAllEnvs();
	vi.resetModules();
	sealDmg.mockReset();
	verifyDmg.mockReset().mockImplementation(async () => undefined);
	verifyMacArtifact.mockReset().mockImplementation(async () => undefined);
	isSigningConfigured.mockReset();
});

describe("macOS signing", () => {
	it.each([
		{
			name: "an explicit signing identity",
			env: { APPLE_SIGNING_IDENTITY: "Developer ID Application: Open Agents (TEAMID)" },
			identity: "Developer ID Application: Open Agents (TEAMID)",
		},
		{
			name: "a CSC_LINK certificate",
			env: { CSC_LINK: "base64-certificate" },
			identity: undefined,
		},
	])("wires the signing config when signing with $name", async ({ env, identity }) => {
		const signOptions = await loadMacSignOptions(env);

		expect(signOptions?.identity).toBe(identity);
		expect(signOptions?.optionsForFile).toBeUndefined();
	});

	it("leaves unsigned local packages unsigned", async () => {
		await expect(loadMacSignOptions({})).resolves.toBeUndefined();
	});
});

describe("postMake artifact verification", () => {
	// The dmg branch already ran verify-mac-artifact.sh through
	// sealDmg/verifyDmg; these cases prove the zip branch — the artifact
	// electron-updater installs auto-updates from, and per-arch what CI ships
	// for x64 — gets the same verification instead of shipping unverified.
	function darwinResult(artifacts: string[]) {
		return [{ artifacts, packageJSON: {}, platform: "darwin" as const, arch: "x64" as const }];
	}

	it("verifies a signed zip with the canonical script, gated on isSigningConfigured", async () => {
		isSigningConfigured.mockReturnValue(true);
		const makeResults = darwinResult(["/out/make/zip/open-agents-darwin-x64.zip"]);

		await config.hooks?.postMake?.({} as never, makeResults);

		expect(verifyMacArtifact).toHaveBeenCalledWith("/out/make/zip/open-agents-darwin-x64.zip");
	});

	it("skips zip verification for an unsigned local build", async () => {
		isSigningConfigured.mockReturnValue(false);
		const makeResults = darwinResult(["/out/make/zip/open-agents-darwin-x64.zip"]);

		await config.hooks?.postMake?.({} as never, makeResults);

		expect(verifyMacArtifact).not.toHaveBeenCalled();
	});

	it("still seals and verifies the dmg through the existing sealDmg/verifyDmg path", async () => {
		isSigningConfigured.mockReturnValue(true);
		sealDmg.mockResolvedValue(true);
		const makeResults = darwinResult(["/out/make/app.dmg"]);

		await config.hooks?.postMake?.({} as never, makeResults);

		expect(sealDmg).toHaveBeenCalledWith("/out/make/app.dmg");
		expect(verifyDmg).toHaveBeenCalledWith("/out/make/app.dmg");
		expect(verifyMacArtifact).not.toHaveBeenCalled();
	});

	it("verifies both the dmg and the zip when a make run produces both", async () => {
		isSigningConfigured.mockReturnValue(true);
		sealDmg.mockResolvedValue(true);
		const makeResults = darwinResult([
			"/out/make/zip/open-agents-darwin-x64.zip",
			"/out/make/app.dmg",
		]);

		await config.hooks?.postMake?.({} as never, makeResults);

		expect(verifyMacArtifact).toHaveBeenCalledWith("/out/make/zip/open-agents-darwin-x64.zip");
		expect(verifyDmg).toHaveBeenCalledWith("/out/make/app.dmg");
	});

	it("never verifies non-darwin artifacts", async () => {
		const makeResults = [
			{
				artifacts: ["/out/make/open-agents.exe"],
				packageJSON: {},
				platform: "win32" as const,
				arch: "x64" as const,
			},
		];

		await config.hooks?.postMake?.({} as never, makeResults);

		expect(sealDmg).not.toHaveBeenCalled();
		expect(verifyDmg).not.toHaveBeenCalled();
		expect(verifyMacArtifact).not.toHaveBeenCalled();
	});
});

describe("packaged authentication callback registration", () => {
	it("declares open-agents in the macOS bundle and Linux package metadata", () => {
		expect(config.packagerConfig?.protocols).toEqual([
			{
				name: "Open Agents authentication callback",
				schemes: ["open-agents"],
			},
		]);

		const makers = config.makers as Array<{
			name?: string;
			config?: { options?: { mimeType?: string[] } };
		}>;
		for (const name of [
			"@electron-forge/maker-deb",
			"@electron-forge/maker-rpm",
		]) {
			const maker = makers.find((candidate) => candidate.name === name);
			expect(maker?.config?.options?.mimeType).toEqual([
				"x-scheme-handler/open-agents",
			]);
		}
	});
});

describe("packaged native dependencies", () => {
	it("keeps the SQLite runtime available to the Vite main bundle", () => {
		const ignore = config.packagerConfig?.ignore;
		expect(ignore).toBeTypeOf("function");
		if (typeof ignore !== "function") return;

		expect(ignore("/.vite/build/main.js")).toBe(false);
		expect(ignore("/node_modules")).toBe(false);
		expect(ignore("/node_modules/better-sqlite3/build/Release/better_sqlite3.node")).toBe(false);
		expect(ignore("/node_modules/bindings/bindings.js")).toBe(false);
		expect(ignore("/node_modules/file-uri-to-path/index.js")).toBe(false);
		expect(ignore("/node_modules/react/index.js")).toBe(true);
		expect(ignore("/src/main.ts")).toBe(true);
		expect(config.hooks?.prePackage).toBeTypeOf("function");
	});
});
