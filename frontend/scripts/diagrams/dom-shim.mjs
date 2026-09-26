/**
 * A browser, in just enough pieces, for Excalidraw's scene builder.
 *
 * `@excalidraw/excalidraw` is a browser bundle. It touches `document`,
 * `FontFace`, and a 2D canvas context at *import* time -- before any diagram
 * code runs -- so the shim has to be installed as a Node preload
 * (`node --import ./dom-shim.mjs <bundle>`) rather than imported by the
 * generator itself. See `build.mjs`, which esbuilds the scene modules first
 * because the package's dist contains JSON imports that carry no import
 * attribute and therefore cannot be loaded by Node's ESM resolver directly.
 *
 * What each piece is for:
 *
 *   - jsdom supplies the DOM. Its `HTMLCanvasElement.getContext()` returns
 *     `null`, which Excalidraw dereferences immediately (`"filter" in ctx`),
 *     so the canvas is stubbed below.
 *   - `measureText` is the stub that matters. Excalidraw measures every label
 *     to lay out and to wrap text, so the stub approximates a sans-serif
 *     average advance of 0.55em. The width tracks `ctx.font` so that
 *     measurement stays proportional to font size, which is what makes
 *     DSL-side centring agree with the rendered SVG. Diagrams keep labels
 *     short, so nothing here needs to be exact -- and the `.excalidraw` scene
 *     is the source of truth: Excalidraw re-measures with real fonts when the
 *     file is opened.
 *   - `FontFace` is a no-op. Excalidraw registers its bundled fonts through it
 *     during module init and then awaits `document.fonts`; nothing in the
 *     export path consumes the result.
 *
 * `localStorage` is deliberately not copied across: jsdom throws a
 * `SecurityError` on property *access* for an opaque origin, and Excalidraw
 * reads it during init.
 */

import { JSDOM } from "jsdom";

/** Average glyph advance as a fraction of font size, for sans-serif labels. */
const AVERAGE_ADVANCE_EM = 0.55;

/** Fallback font size when the caller has not set `ctx.font` yet. */
const DEFAULT_FONT_SIZE = 16;

const dom = new JSDOM("<!doctype html><html><body></body></html>", {
	pretendToBeVisual: true,
	// A real origin keeps `localStorage` reachable, which Excalidraw probes
	// during init even though the export path never reads it.
	url: "https://localhost/",
});
const { window } = dom;

const COPIED_GLOBALS = [
	"window",
	"document",
	"navigator",
	"HTMLElement",
	"HTMLCanvasElement",
	"Element",
	"Node",
	"SVGElement",
	"Image",
	"Blob",
	"File",
	"FileReader",
	"URL",
	"MutationObserver",
	"ResizeObserver",
	"DOMParser",
	"XMLSerializer",
	"TextEncoder",
	"TextDecoder",
	"CSS",
	"structuredClone",
];

/** DOM methods that must keep their original `this` binding. */
const BOUND_GLOBALS = new Set([
	"getComputedStyle",
	"requestAnimationFrame",
	"cancelAnimationFrame",
	"matchMedia",
]);

for (const name of COPIED_GLOBALS) {
	const value = window[name];
	if (value === undefined) continue;
	try {
		globalThis[name] = BOUND_GLOBALS.has(name) ? value.bind(window) : value;
	} catch {
		// A host that refuses the copy is a global Excalidraw can live without.
	}
}

globalThis.devicePixelRatio ??= 1;

window.matchMedia ??= () => ({
	matches: false,
	addListener() {},
	removeListener() {},
	addEventListener() {},
	removeEventListener() {},
});

/**
 * Read the pixel size out of a CSS `font` shorthand (`italic 600 16px Inter`).
 * Returns 0 when there is no px component, which is the "font not set yet"
 * case Excalidraw uses before its first measure.
 */
function fontSizeOf(font) {
	const match = /(\d+(?:\.\d+)?)px/.exec(String(font ?? ""));
	return match ? Number(match[1]) : 0;
}

const context2d = new Proxy(
	{
		canvas: null,
		font: `${DEFAULT_FONT_SIZE}px sans-serif`,
		measureText(text) {
			const size = fontSizeOf(this.font) || DEFAULT_FONT_SIZE;
			const width = String(text).length * size * AVERAGE_ADVANCE_EM;
			return {
				width,
				actualBoundingBoxAscent: size * 0.8,
				actualBoundingBoxDescent: size * 0.2,
				fontBoundingBoxAscent: size * 0.8,
				fontBoundingBoxDescent: size * 0.2,
			};
		},
		fillText() {},
		strokeText() {},
		clearRect() {},
		save() {},
		restore() {},
		scale() {},
		translate() {},
		getImageData: () => ({ data: new Uint8ClampedArray(4) }),
	},
	{
		get(target, property) {
			if (property in target) return target[property];
			// Excalidraw feature-detects `ctx.filter`; any truthy string reads as
			// "the 2D context can blur", which is the conservative answer here.
			return typeof property === "string" && property.startsWith("filter")
				? "blur(0px)"
				: undefined;
		},
	},
);

window.HTMLCanvasElement.prototype.getContext = function getContext() {
	context2d.canvas = this;
	return context2d;
};
window.HTMLCanvasElement.prototype.toDataURL = () =>
	"data:image/png;base64,";

class FontFaceStub {
	constructor(family, source, descriptors) {
		this.family = family;
		this.source = source;
		this.status = "loaded";
		Object.assign(this, descriptors);
	}

	load() {
		return Promise.resolve(this);
	}
}

globalThis.FontFace ??= FontFaceStub;
window.FontFace ??= FontFaceStub;
if (!("fonts" in window.document)) {
	Object.defineProperty(window.document, "fonts", {
		configurable: true,
		value: {
			add() {},
			delete() {},
			has: () => true,
			forEach() {},
			check: () => true,
			ready: Promise.resolve(),
		},
	});
}
