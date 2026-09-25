import { beforeEach, describe, expect, it, vi } from "vitest";

const buildForge = vi.fn<(forge: { dir: string }, options: any) => Promise<string[]>>(
	async () => ["/out/make/open-agents.AppImage"],
);
vi.mock("app-builder-lib", () => ({ buildForge }));

import MakerAppImage from "./maker-appimage";

const makeOptions = {
	dir: "/tmp/app/Open Agents-linux-x64",
	makeDir: "/tmp/app/make",
	appName: "Open Agents",
	targetPlatform: "linux" as const,
	targetArch: "x64" as const,
	forgeConfig: {} as never,
	packageJSON: {},
};

beforeEach(() => {
	buildForge.mockClear();
});

describe("MakerAppImage", () => {
	it("targets Linux and is supported for cross-builds", () => {
		const maker = new MakerAppImage();
		expect(maker.name).toBe("appimage");
		expect(maker.defaultPlatforms).toEqual(["linux"]);
		expect(maker.isSupportedOnCurrentPlatform()).toBe(true);
	});

	it("writes callback protocols into the AppImage desktop entry", async () => {
		const protocols = [
			{
				name: "Open Agents authentication callback",
				schemes: ["open-agents"],
			},
		];
		const maker = new MakerAppImage(
			{ appId: "dev.openagents.desktop", protocols },
			["linux"],
		);
		await maker.prepareConfig(makeOptions.targetArch);
		await maker.make(makeOptions);

		const [, options] = buildForge.mock.calls[0];
		expect(options.linux).toEqual(["appImage:x64"]);
		expect(options.config.protocols).toEqual(protocols);
		expect(options.config.publish).toBeNull();
		expect(options.config.appImage.artifactName).toBe("open-agents-linux-${arch}-${version}.${ext}");
	});

	// Pinned exactly rather than asserted "not legacy": app-builder-lib treats null
	// the same as "0.0.0", so a looser check would stay green on a regression. See
	// the rationale in maker-appimage.ts.
	it("pins the appimage toolset so the artifact does not need libfuse2 (#4006)", async () => {
		const maker = new MakerAppImage({ appId: "dev.openagents.desktop" }, ["linux"]);
		await maker.prepareConfig(makeOptions.targetArch);
		await maker.make(makeOptions);

		const [, options] = buildForge.mock.calls[0];
		expect(options.config.toolsets?.appimage).toBe("1.0.3");
	});
});
