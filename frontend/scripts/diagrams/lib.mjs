/**
 * A very small scene DSL over `@excalidraw/excalidraw`.
 *
 * Excalidraw's own authoring format is a flat array of fully-populated
 * elements: every id, every `frameId`, every `children` list is explicit. That
 * is the right format to *commit* (these scenes are meant to be opened and
 * edited by hand) and the wrong one to *write* by hand, so this module keeps
 * the committed file dumb and concentrates the bookkeeping here.
 *
 * Three facts about the package shape the API:
 *
 *   1. `convertToExcalidrawElements` assigns a child's `frameId` by looking the
 *      child up in its parent frame's `children` array, and dereferences that
 *      array unconditionally. A frame with no `children` key throws. Hence
 *      `frame(name, box, fn)`: the callback runs first, the frame is emitted
 *      last, and its `children` list is every element the callback added.
 *   2. A multi-line label is one text element with embedded newlines, not one
 *      element per line. `textAlign: "center"` centres each line inside the
 *      element's `width`, which is what keeps a label editable as a single
 *      object in Excalidraw.
 *   3. Text is positioned by its top-left corner and its width comes from
 *      canvas measurement. `measureWidth` mirrors the approximation in
 *      `dom-shim.mjs` (0.55em per character) so centring computed here lands
 *      where the exported SVG draws it. The two constants are deliberately
 *      identical; change one, change both.
 */

import {
	convertToExcalidrawElements,
	exportToSvg,
} from "@excalidraw/excalidraw";

/** Must match `AVERAGE_ADVANCE_EM` in `dom-shim.mjs`. */
const AVERAGE_ADVANCE_EM = 0.55;

/** Excalidraw's line box, as a multiple of font size (16px -> 18.4px). */
const LINE_HEIGHT_EM = 1.15;

const FONT = {
	title: 28,
	box: 16,
	sub: 12,
	link: 12,
	note: 14,
	state: 18,
};

/**
 * Colour by architectural role, so a reader can tell "this is UI" from "this
 * is durable" without reading a single label.
 */
export const TONE = {
	ui: { stroke: "#1971c2", background: "#d0e8ff", text: "#0b3d6b" },
	daemon: { stroke: "#e8590c", background: "#ffe8d0", text: "#7c2d12" },
	store: { stroke: "#2b8a3e", background: "#d3f9d8", text: "#14532d" },
	external: { stroke: "#868e96", background: "#f1f3f5", text: "#495057" },
	derived: { stroke: "#f08c00", background: "#fff3bf", text: "#663c00" },
	hazard: { stroke: "#c92a2a", background: "#ffe3e3", text: "#7f1d1d" },
	plane: { stroke: "#5f3dc4", background: "#e5dbff", text: "#3b2a7a" },
};

/** Shapes the DSL knows how to draw. */
const SHAPES = { rectangle: "rectangle", diamond: "diamond", ellipse: "ellipse" };

let idCounter = 0;
const nextId = (prefix) => `${prefix}_${(idCounter += 1)}`;

/**
 * Approximate rendered width of a text element. Mirrors the `measureText` stub
 * in `dom-shim.mjs`; see the note at the top of the file.
 */
export function measureWidth(text, fontSize) {
	return String(text).length * fontSize * AVERAGE_ADVANCE_EM;
}

/** Height of a `lines`-line text block at `fontSize`. */
export function blockHeight(lines, fontSize) {
	return lines.length * fontSize * LINE_HEIGHT_EM;
}

/** Widest line of a text block, as a rendered width. */
export function blockWidth(lines, fontSize) {
	return lines.reduce((widest, line) => Math.max(widest, measureWidth(line, fontSize)), 0);
}

const splitLines = (value) => String(value ?? "").split("\n");

/**
 * One diagram. Collects raw element descriptors, then converts and exports
 * them in a single batch.
 */
export class Scene {
	/**
	 * @param {object} options
	 * @param {string} options.name    File stem, e.g. `01-application-overview`.
	 * @param {string} options.title   Heading drawn at the top of the canvas.
	 * @param {string} [options.subtitle] Secondary heading line(s).
	 * @param {string} [options.source] `source` field in the `.excalidraw` file.
	 */
	constructor({ name, title, subtitle, source }) {
		this.name = name;
		this.title = title;
		this.subtitle = subtitle;
		this.source = source;
		/** @type {object[]} Raw descriptors, in insertion order. */
		this.elements = [];
	}

	/**
	 * A text element.
	 *
	 * `align: "center"` with a `band` centres the block inside a pixel band,
	 * and also sets Excalidraw's own `textAlign` so the individual lines stay
	 * centred relative to each other when the file is reopened and edited.
	 *
	 * There is a quirk to encode here. `convertToExcalidrawElements` treats a
	 * centred text element's incoming `x` as the block's *centre* and rewrites
	 * it to a left edge (`x - width / 2`). Passing an already-computed left edge
	 * therefore shifts the label left by half its width. So this method passes
	 * the band's centre and lets the package do the conversion, and the
	 * observable result is that the widest line lands centred in the band.
	 */
	text({
		x,
		y,
		text,
		band,
		fontSize = FONT.box,
		tone = TONE.external,
		align = "left",
		background,
	}) {
		const lines = splitLines(text);
		const width = Math.max(blockWidth(lines, fontSize), 1);
		const centred = align === "center" && band;
		this.elements.push({
			id: nextId("t"),
			type: "text",
			x: centred ? x + band / 2 : x,
			y,
			width,
			height: blockHeight(lines, fontSize),
			text: lines.join("\n"),
			fontSize,
			fontFamily: 2, // 1 is "Virgil" (hand-drawn); 2 is the sans variant.
			textAlign: centred ? "center" : "left",
			verticalAlign: "top",
			strokeColor: tone.text,
			...(background ? { backgroundColor: background } : {}),
		});
		return { x, y, w: width, h: blockHeight(lines, fontSize) };
	}

	/**
	 * A labelled shape. Height is derived from the label unless given
	 * explicitly, so a column of boxes lines up without the caller doing
	 * arithmetic.
	 *
	 * Returns the shape's real bounds, which is what `link` routes against --
	 * note that a `diamond` is drawn wider than its column so the caller can
	 * lay out a state machine by column centre and still get straight edges.
	 */
	box({
		x,
		y,
		w,
		h,
		text,
		sublabel,
		tone = TONE.ui,
		shape = "rectangle",
		dashed = false,
		initial = false,
	}) {
		const kind = SHAPES[shape] ?? SHAPES.rectangle;
		const fontSize = kind === "ellipse" ? FONT.state : FONT.box;
		const lines = text === undefined ? [] : splitLines(text);
		const subLines = sublabel ? splitLines(sublabel) : [];
		const gap = subLines.length > 0 ? 6 : 0;
		const contentHeight =
			blockHeight(lines, fontSize) + gap + blockHeight(subLines, FONT.sub);

		// Widening factors: a diamond needs room for its label at the same
		// centre; an ellipse is circular so its width is its height.
		let shapeW = w;
		let shapeH = h ?? Math.max(60, contentHeight + 26);
		let shapeX = x;
		if (kind === "diamond") {
			shapeW = w * 1.5;
			shapeH = shapeH * 1.6;
			shapeX = x + (w - shapeW) / 2;
		} else if (kind === "ellipse") {
			shapeW = Math.max(w, contentHeight * 1.6, blockWidth(lines, fontSize) + 44);
			shapeH = shapeW;
		}

		this.elements.push({
			id: nextId("b"),
			type: kind,
			x: shapeX,
			y,
			width: shapeW,
			height: shapeH,
			strokeColor: tone.stroke,
			backgroundColor: tone.background,
			strokeWidth: 1,
			roughness: 0, // clean rectangles; Excalidraw's hand-drawn mode is
			//               reserved for notes so the diagrams read as schematics.
			...(dashed ? { strokeStyle: "dashed" } : {}),
			...(kind === "diamond" ? { roundness: null } : { roundness: { type: 2 } }),
		});

		if (kind === "ellipse" && initial) {
			// The filled dot that marks an entry point.
			this.elements.push({
				id: nextId("d"),
				type: "ellipse",
				x: shapeX - 34,
				y: y + shapeH / 2 - 5,
				width: 10,
				height: 10,
				strokeColor: tone.stroke,
				backgroundColor: tone.stroke,
				roughness: 0,
			});
		}

		if (lines.length > 0) {
			this.text({
				x: shapeX,
				y: y + (shapeH - contentHeight) / 2,
				text: lines.join("\n"),
				band: shapeW,
				align: "center",
				fontSize,
				tone,
			});
		}
		if (subLines.length > 0) {
			this.text({
				x: shapeX,
				y: y + (shapeH - contentHeight) / 2 + blockHeight(lines, fontSize) + gap,
				text: subLines.join("\n"),
				band: shapeW,
				align: "center",
				fontSize: FONT.sub,
				tone,
			});
		}
		return { x: shapeX, y, w: shapeW, h: shapeH };
	}

	/**
	 * A labelled frame. The callback runs before the frame is emitted, because
	 * Excalidraw resolves each child's `frameId` from the frame's `children`
	 * list -- the frame has to know its contents before it exists.
	 *
	 * The callback receives the scene itself, so every DSL call works inside a
	 * frame exactly as it does outside one.
	 */
	frame({ x, y, w, h, name }, fn) {
		const before = this.elements.length;
		fn(this);
		const children = this.elements.slice(before).map((element) => element.id);
		this.elements.splice(before, 0, {
			id: nextId("f"),
			type: "frame",
			x,
			y,
			width: w,
			height: h,
			name,
			// A frame with no children still needs the key: Excalidraw
			// dereferences it unconditionally.
			children,
		});
		return { x, y, w, h };
	}

	/**
	 * An arrow between two rectangles returned by `box`, routed orthogonally.
	 *
	 * The dominant axis picks the edges: mostly-below leaves the source's
	 * bottom and enters the target's top, otherwise it leaves right and enters
	 * left. `bend` is how far past the source edge the first leg runs before
	 * turning; vary it when several edges converge on one target, so their
	 * shared horizontal run does not overprint.
	 */
	link(a, b, { label, tone = TONE.external, dashed = false, at = "mid", head = "arrow", bend = 24 } = {}) {
		const acx = a.x + a.w / 2;
		const acy = a.y + a.h / 2;
		const bcx = b.x + b.w / 2;
		const bcy = b.y + b.h / 2;
		const dx = bcx - acx;
		const dy = bcy - acy;
		const BEND = bend;

		let points;
		if (Math.abs(dy) >= Math.abs(dx)) {
			const down = dy >= 0;
			const exitY = (down ? a.y + a.h : a.y) + (down ? BEND : -BEND);
			points = [
				[acx, down ? a.y + a.h : a.y],
				[acx, exitY],
				[bcx, exitY],
				[bcx, down ? b.y : b.y + b.h],
			];
		} else {
			const right = dx >= 0;
			const exitX = (right ? a.x + a.w : a.x) + (right ? BEND : -BEND);
			points = [
				[right ? a.x + a.w : a.x, acy],
				[exitX, acy],
				[exitX, bcy],
				[right ? b.x : b.x + b.w, bcy],
			];
		}
		return this.edge({ points, label, tone, dashed, head, at });
	}

	/**
	 * A polyline arrow from absolute points, for edges that do not connect two
	 * boxes (guard rails, feedback loops, cross-cutting annotations).
	 */
	edge({ points, label, tone = TONE.external, dashed = false, at = "mid", head = "arrow", tail = null, labelFont = FONT.link }) {
		const xs = points.map(([px]) => px);
		const ys = points.map(([, py]) => py);
		const minX = Math.min(...xs);
		const minY = Math.min(...ys);
		this.elements.push({
			id: nextId("a"),
			type: "arrow",
			x: minX,
			y: minY,
			width: Math.max(...xs) - minX,
			height: Math.max(...ys) - minY,
			points: points.map(([px, py]) => [px - minX, py - minY]),
			strokeColor: tone.stroke,
			strokeWidth: 1,
			strokeStyle: dashed ? "dashed" : "solid",
			startArrowhead: tail,
			endArrowhead: head === "none" ? null : head,
			roundness: { type: 2 },
		});

		if (label) {
			// Label a horizontal run when there is one: vertical text is
			// effectively unreadable at this size.
			const index =
				at === "start" ? 0 : at === "end" ? points.length - 2 : Math.floor((points.length - 1) / 2);
			const [px, py] = points[Math.max(0, Math.min(index, points.length - 1))];
			const [nextX, nextY] = points[Math.min(index + 1, points.length - 1)];
			const width = measureWidth(label, labelFont) + 12;
			const horizontal = nextY === py;
			this.text({
				x: horizontal ? (px + nextX) / 2 - width / 2 : px - width / 2 - 8,
				y: horizontal ? py - 20 : (py + nextY) / 2 - FONT.link,
				text: label,
				band: width,
				align: "center",
				fontSize: labelFont,
				tone,
				background: "#ffffff",
			});
		}
		return { x: minX, y: minY, w: Math.max(...xs) - minX, h: Math.max(...ys) - minY };
	}

	/**
	 * A note. Reserved for the rules that are the actual point of a diagram --
	 * invariants that are invisible in the code because they are spread across
	 * it. Drawn hand-drawn to read as commentary rather than as structure.
	 */
	note({ x, y, w, text, tone = TONE.derived, size = FONT.note }) {
		const lines = splitLines(text);
		const height = blockHeight(lines, size) + 26;
		this.elements.push({
			id: nextId("n"),
			type: "rectangle",
			x,
			y,
			width: w,
			height,
			strokeColor: tone.stroke,
			backgroundColor: tone.background,
			strokeWidth: 1,
			roughness: 1,
			roundness: { type: 3 },
		});
		this.text({ x: x + 14, y: y + 13, text, fontSize: size, tone });
		return { x, y, w, h: height };
	}

	/** A small caption, e.g. an edge condition sitting beside an arrow. */
	caption({ x, y, text, tone = TONE.external, align = "left", band }) {
		return this.text({ x, y, text, band, align, fontSize: FONT.sub, tone });
	}

	/** The document heading. Returns the y coordinate content should start at. */
	heading() {
		const subtitle = this.subtitle ? splitLines(this.subtitle) : [];
		const height = blockHeight([this.title], FONT.title) + (subtitle.length ? 8 : 0) + blockHeight(subtitle, 14);
		this.text({ x: 60, y: 48, text: this.title, fontSize: FONT.title, tone: TONE.plane });
		if (subtitle.length > 0) {
			this.text({ x: 60, y: 48 + blockHeight([this.title], FONT.title) + 8, text: subtitle.join("\n"), fontSize: 14, tone: TONE.external });
		}
		return 48 + height + 28;
	}

	/**
	 * Convert, then export a `.svg` string and a `.excalidraw` scene object.
	 * Both are produced in one place so every diagram round-trips identically.
	 */
	async render() {
		const elements = convertToExcalidrawElements(this.elements);
		const appState = {
			exportBackground: true,
			viewBackgroundColor: "#ffffff",
			gridSize: null,
		};
		const svgRoot = await exportToSvg({ elements, appState, files: null });
		return {
			elements,
			svg: new XMLSerializer().serializeToString(svgRoot),
			scene: {
				type: "excalidraw",
				version: 2,
				source: this.source,
				elements,
				appState,
				files: {},
			},
		};
	}
}
