/**
 * Render logic for the Excalidraw diagrams. Not an entry point: `build.mjs`
 * generates a tiny entry that statically imports every scene plus this module,
 * bundles the lot, and calls `run`.
 *
 * The static-import requirement is not stylistic. Scenes are loaded by
 * `import()` at runtime, so esbuild cannot see them; a dynamically imported
 * scene keeps its own real `import "@excalidraw/excalidraw"` statement, and
 * Node then loads the untranspiled browser bundle that this whole pipeline
 * exists to avoid.
 *
 * Paths arrive through the environment rather than `import.meta.url`, because
 * after bundling this file executes from a cache directory and no longer knows
 * where the repository is.
 */

import { mkdirSync, writeFileSync } from "node:fs";
import { join } from "node:path";

import { Scene } from "./lib.mjs";

/**
 * Excalidraw's SVG export repeats `xmlns` on the root element, which is a hard
 * XML parse error rather than a cosmetic one: `rsvg-convert`, ImageMagick, and
 * every browser will reject the file. Drop later duplicates of any attribute
 * already present on the root tag, keeping the first.
 *
 * @param {string} svg
 * @returns {string}
 */
export function dedupeRootAttributes(svg) {
	const open = svg.indexOf("<svg");
	if (open === -1) return svg;
	const end = svg.indexOf(">", open);
	if (end === -1) return svg;

	const seen = new Set();
	const kept = [];
	for (const match of svg.slice(open, end).matchAll(/([\w:.-]+)\s*=\s*"([^"]*)"/g)) {
		if (seen.has(match[1])) continue;
		seen.add(match[1]);
		kept.push(match[0]);
	}
	return svg.slice(0, open) + `<svg ${kept.join(" ")}` + svg.slice(end);
}

/**
 * Write every scene in `scenes` as a `.excalidraw` source scene and a rendered
 * `.svg`. Bare CLI arguments narrow the run to scenes whose name contains one
 * of them.
 *
 * @param {Scene[]} scenes
 */
export async function run(scenes) {
	const outputDir = process.env.OA_DIAGRAMS_OUT;
	if (!outputDir) {
		throw new Error("OA_DIAGRAMS_OUT must be set by build.mjs");
	}

	mkdirSync(outputDir, { recursive: true });
	const filters = process.argv.slice(2).filter((argument) => !argument.startsWith("-"));

	for (const scene of scenes) {
		if (!(scene instanceof Scene)) {
			throw new Error("every scene module must default-export a Scene");
		}
		if (filters.length > 0 && !filters.some((filter) => scene.name.includes(filter))) {
			continue;
		}

		const rendered = await scene.render();
		const stem = join(outputDir, scene.name);

		writeFileSync(`${stem}.excalidraw`, `${JSON.stringify(rendered.scene, null, 2)}\n`);
		// Excalidraw's SVG export omits the XML prolog. Adding it keeps the file
		// a standalone document, which is what GitHub and `<img src>` both want.
		const svg = `<?xml version="1.0" encoding="UTF-8"?>\n${dedupeRootAttributes(rendered.svg)}`;
		writeFileSync(`${stem}.svg`, svg);

		const kilobytes = (svg.length / 1024).toFixed(1);
		console.log(`  ${scene.name}  ${rendered.elements.length} elements  ${kilobytes} KB svg`);
	}
}
