import type { ForgeConfig } from "@electron-forge/shared-types";
import { AutoUnpackNativesPlugin } from "@electron-forge/plugin-auto-unpack-natives";
import { VitePlugin } from "@electron-forge/plugin-vite";
import { rebuild } from "@electron/rebuild";
import electronPackage from "electron/package.json";
import MakerNSIS from "./makers/maker-nsis";
import MakerAppImage from "./makers/maker-appimage";
import { existsSync, writeFileSync } from "node:fs";
import { spawnSync } from "node:child_process";
import path from "node:path";

// Default GitHub release target (production). Builds cut by CI must NOT rely
// on a redirect or an older repository identity: workflows set
// OPEN_AGENTS_RELEASE_REPO to the repository they run in, and
// build-artifacts.yml asserts the baked app-update.yml matches it.
const DEFAULT_RELEASE_REPO = "sudo-adduser-jordan/open-agents";

// The packaged binary name (no extension). Single source of truth: the packager
// names the exe/ELF from this, and the NSIS + deb makers must point their
// shortcut/launcher at the SAME name. Drift here means a broken Start menu
// shortcut on Windows (#2414) or "could not find the Electron app binary" on deb.
const EXECUTABLE_NAME = "open-agents";
const AUTH_PROTOCOL = {
	name: "Open Agents authentication callback",
	schemes: ["open-agents"],
};
const AUTH_PROTOCOL_MIME_TYPE = "x-scheme-handler/open-agents";
const PACKAGED_EXTERNAL_DEPENDENCIES = [
	"/node_modules/better-sqlite3",
	"/node_modules/bindings",
	"/node_modules/file-uri-to-path",
];

function ignoreFromVitePackage(file: string): boolean {
	if (!file) return false;
	if (file.startsWith("/.vite")) return false;
	if (file === "/node_modules") return false;
	return !PACKAGED_EXTERNAL_DEPENDENCIES.some(
		(dependency) => file === dependency || file.startsWith(`${dependency}/`),
	);
}

async function prepareNativeDependencies(platform: NodeJS.Platform, arch: string): Promise<void> {
	// Rebuild in the source tree, where prebuild-install and its helper packages
	// are available. The Vite package intentionally carries only the resulting
	// native runtime, not the install-time download toolchain.
	await rebuild({
		buildPath: process.cwd(),
		electronVersion: electronPackage.version,
		platform,
		arch,
		onlyModules: ["better-sqlite3"],
		force: true,
	});
}

export function extraResourcesForPlatform(platform: NodeJS.Platform): string[] {
	return [
		"daemon",
		"agent-browser",
		...(platform === "linux" ? ["tmux"] : []),
		"assets/icon.png",
		"assets/icon.ico",
		"assets/trayIconTemplate.png",
		"assets/trayIconTemplate@2x.png",
		"app-update.yml",
	];
}

// parseReleaseRepo turns an "owner/repo" string (from OPEN_AGENTS_RELEASE_REPO) into the
// publisher-github { owner, name } shape, falling back to the production default
// when unset or malformed.
function parseReleaseRepo(value: string | undefined): { owner: string; name: string } {
	const [owner, name] = (value || DEFAULT_RELEASE_REPO).split("/");
	if (!owner || !name) {
		const [defOwner, defName] = DEFAULT_RELEASE_REPO.split("/");
		return { owner: defOwner, name: defName };
	}
	return { owner, name };
}

const config: ForgeConfig = {
	packagerConfig: {
		asar: true,
		// The Vite plugin normally packages only .vite. better-sqlite3 must stay
		// external so Electron can load its native binary, so include its minimal
		// runtime dependency tree explicitly; AutoUnpackNativesPlugin then places
		// the .node binary outside app.asar.
		ignore: ignoreFromVitePackage,
		appBundleId: "dev.openagents.desktop",
		name: "Open Agents",
		executableName: EXECUTABLE_NAME,
		protocols: [AUTH_PROTOCOL],
		// App icon. electron-packager appends the per-platform extension
		// (.ico on Windows); Linux menu icons come from the
		// deb/rpm makers below, and the runtime window icon from src/main.ts.
		icon: "assets/icon",
		extraResource: extraResourcesForPlatform(process.platform),
	},
	hooks: {
		// electron-forge does not generate app-update.yml (electron-builder does);
		// electron-updater reads it from the app's Resources dir at runtime to know
		// which GitHub repo to pull from, else it throws ENOENT during download.
		// Generate it in prePackage and ship it via extraResource above.
		// owner/repo are baked from OPEN_AGENTS_RELEASE_REPO at build time.
		prePackage: async (_forgeConfig, platform, arch) => {
			await prepareNativeDependencies(platform as NodeJS.Platform, arch);
			const { owner, name } = parseReleaseRepo(process.env.OPEN_AGENTS_RELEASE_REPO);
			const yml = [
				"provider: github",
				`owner: ${owner}`,
				`repo: ${name}`,
				"updaterCacheDirName: open-agents-updater",
				"",
			].join("\n");
			writeFileSync("app-update.yml", yml);
		},
		packageAfterPrune: async (_forgeConfig, buildPath) => {
			const nativeModule = path.join(
				buildPath,
				"node_modules",
				"better-sqlite3",
				"build",
				"Release",
				"better_sqlite3.node",
			);
			if (!existsSync(nativeModule)) {
				throw new Error("Packaged app is missing the better-sqlite3 native runtime");
			}
		},
		// Assert the native resource survived Electron Packager's copy/asar
		// pipeline. A source build succeeding is not enough: a missing extraResource
		// would otherwise publish an app that silently fell back to machine tmux.
		postPackage: async (_forgeConfig, packageResult) => {
			if (packageResult.platform !== "linux") return;
			for (const outputPath of packageResult.outputPaths) {
				const resourcesPath = path.join(outputPath, "resources");
				const binary = path.join(resourcesPath, "tmux", "bin", "tmux");
				if (!existsSync(binary)) throw new Error(`packaged tmux missing from ${binary}`);
				const version = spawnSync(binary, ["-V"], { encoding: "utf8" });
				if (version.status !== 0 || version.stdout.trim() !== "tmux 3.5a") {
					throw new Error(`packaged tmux failed verification at ${binary}: ${version.stderr || version.stdout}`);
				}
			}
		},
	},
	rebuildConfig: {},
	makers: [
		// Windows installer: NSIS via electron-builder (see makers/maker-nsis.ts).
		// Replaces Squirrel.Windows, which only does per-user installs with no
		// custom install dir or proper uninstaller (issue #401).
		new MakerNSIS(
			{
				appId: "dev.openagents.desktop",
				productName: "Open Agents",
				// Match the packaged binary name so the Start menu shortcut targets
				// the real "open-agents.exe" (not "Open Agents.exe").
				executableName: EXECUTABLE_NAME,
				icon: "assets/icon.ico",
			},
			["win32"],
		),
		// Linux fetch-and-run artifact for `open-agents start`: a single self-contained
		// AppImage the Go bootstrapper downloads and runs directly (see
		// makers/maker-appimage.ts). The deb/rpm makers below stay for users who
		// prefer a system package.
		new MakerAppImage(
			{
				appId: "dev.openagents.desktop",
				productName: "Open Agents",
				icon: "assets/icon.png",
				protocols: [AUTH_PROTOCOL],
			},
			["linux"],
		),
		{
			name: "@electron-forge/maker-deb",
			config: {
				options: {
					// Must match packagerConfig.executableName, or the deb maker
					// looks for the package name and fails with "could not find
					// the Electron app binary". (Both are "open-agents".)
					bin: EXECUTABLE_NAME,
					icon: "assets/icon.png",
					maintainer: "Open Agents",
					homepage: "https://github.com/sudo-adduser-jordan/open-agents",
					mimeType: [AUTH_PROTOCOL_MIME_TYPE],
				},
			},
		},
		{
			name: "@electron-forge/maker-rpm",
			config: {
				options: {
					icon: "assets/icon.png",
					// rpmbuild rejects a spec with an empty License field.
					license: "MIT",
					homepage: "https://github.com/sudo-adduser-jordan/open-agents",
					mimeType: [AUTH_PROTOCOL_MIME_TYPE],
				},
			},
		},
	],
	publishers: [
		{
			name: "@electron-forge/publisher-github",
			// Release target is build-time overridable so a fork run publishes to the
			// fork without a source edit. OPEN_AGENTS_RELEASE_REPO is "owner/repo";
			// it defaults to sudo-adduser-jordan/open-agents.
			config: {
				repository: parseReleaseRepo(process.env.OPEN_AGENTS_RELEASE_REPO),
				prerelease: process.env.OPEN_AGENTS_RELEASE_PRERELEASE === "true",
				draft: false,
				// Ask GitHub to compose the body from the PRs merged since the last
				// release. Without it the publisher creates the release with an empty
				// body, and the app's new "what's new" section has nothing to show:
				// electron-updater reads release notes from the release body, so an
				// empty body means users get told nothing about what changed.
				generateReleaseNotes: true,
			},
		},
	],
	plugins: [
		new AutoUnpackNativesPlugin({}),
		new VitePlugin({
			build: [
				{ entry: "src/main.ts", config: "vite.main.config.ts", target: "main" },
				{ entry: "src/preload.ts", config: "vite.preload.config.ts", target: "preload" },
				{ entry: "src/annotate-preload.ts", config: "vite.preload.config.ts", target: "preload" },
			],
			renderer: [{ name: "main_window", config: "vite.renderer.config.ts" }],
		}),
	],
};

export default config;
