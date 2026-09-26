/**
 * Generates the Excalidraw architecture diagrams in `docs/assets/diagrams/`.
 *
 *     npm run diagrams
 *     npm run diagrams -- 02-state      # only scenes matching a substring
 *
 * This is a three-step dance rather than a plain `node generate.mjs`:
 *
 *   1. **Write a static entry.** Scenes have to be reachable from a static
 *      `import` statement so esbuild can see them. A runtime `import()` would
 *      leave each scene's own `import "@excalidraw/excalidraw"` intact, and
 *      Node would then load the untranspiled browser bundle.
 *   2. **Bundle.** `@excalidraw/excalidraw`'s dist contains JSON imports with no
 *      `with { type: "json" }` attribute, which Node's ESM resolver rejects
 *      outright. esbuild resolves and inlines them. jsdom stays external
 *      because bundling it to ESM turns its internal `require("node:fs")` into
 *      an unsupported dynamic require.
 *   3. **Preload the DOM.** The same package reads `document`, `FontFace` and a
 *      2D canvas context at import time, before any scene code runs, so the
 *      shim is installed via `--import` rather than imported by the bundle.
 *
 * Neither the generated entry nor the bundle is written into the working tree:
 * both go under `node_modules/.cache/`, which git already ignores.
 */

import { spawnSync } from "node:child_process";
import { mkdirSync, readdirSync, writeFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

import { build } from "esbuild";

const diagramsDir = dirname(fileURLToPath(import.meta.url));
const frontendRoot = resolve(diagramsDir, "../..");
const repoRoot = resolve(frontendRoot, "..");

/** Committed output. A sibling of the existing `docs/assets/readme/` images. */
const outputDir = join(repoRoot, "docs/assets/diagrams");

const cacheDir = join(frontendRoot, "node_modules/.cache/open-agents-diagrams");
const entryPath = join(cacheDir, "entry.mjs");
const bundlePath = join(cacheDir, "bundle.mjs");
const shimPath = join(diagramsDir, "dom-shim.mjs");

const scenesDir = join(diagramsDir, "scenes");
const sceneFiles = readdirSync(scenesDir)
	.filter((file) => file.endsWith(".mjs"))
	.sort();

if (sceneFiles.length === 0) {
	throw new Error(`No scene modules found in ${scenesDir}`);
}

mkdirSync(cacheDir, { recursive: true });
writeFileSync(
	entryPath,
	[
		`import { run } from ${JSON.stringify(join(diagramsDir, "generate.mjs"))};`,
		...sceneFiles.map((file, index) => `import scene${index} from ${JSON.stringify(join(scenesDir, file))};`),
		`await run([${sceneFiles.map((_, index) => `scene${index}`).join(", ")}]);`,
		"",
	].join("\n"),
);

await build({
	entryPoints: [entryPath],
	outfile: bundlePath,
	bundle: true,
	platform: "node",
	format: "esm",
	target: "node22",
	external: ["jsdom"],
	logLevel: "warning",
});

console.log(`Generating ${sceneFiles.length} Excalidraw diagram(s) into docs/assets/diagrams:`);
const result = spawnSync(process.execPath, ["--import", shimPath, bundlePath, ...process.argv.slice(2)], {
	cwd: repoRoot,
	stdio: "inherit",
	env: { ...process.env, OA_DIAGRAMS_OUT: outputDir },
});

if (result.error) {
	console.error(`Failed to run the diagram generator: ${result.error.message}`);
	process.exit(1);
}
process.exit(result.status ?? 1);
