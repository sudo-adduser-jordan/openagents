export type ModelLogoKey = "model-muse" | "model-nemotron";

/**
 * Model ids that carry a brand mark of their own, mapped to the mark they get.
 *
 * Matched by exact id only. A provider namespaces its own models, so
 * `muse-spark-9` and `opencode/nemotron` are different models from the ones
 * listed here and must not inherit a mark they did not ask for. Anything absent
 * from this table — including every model without an asset — renders the
 * harness mark instead, which is the right answer for the long tail.
 */
const EXACT_MODEL_LOGOS: Readonly<Record<string, ModelLogoKey>> = {
	"muse-spark": "model-muse",
	"muse-spark-1.1": "model-muse",
	"muse-spark-1.2": "model-muse",
	"opencode/nemotron-3-ultra-free": "model-nemotron",
};

/**
 * Whether a model ref defers to the harness's own default rather than naming a
 * model. A default has no brand to show, so it falls through to the harness
 * mark like any other unmatched model.
 */
export function isDefaultModelRef(model: string | undefined): boolean {
	if (model === undefined) return true;
	const trimmed = model.trim();
	if (trimmed === "") return true;
	if (trimmed === "default") return true;
	// Parameterized harness defaults carry a provider-specific body, which may
	// be empty. Anchored on `default(` so a real model merely starting with
	// "default" is not swallowed.
	return /^default\([^)]*\)$/.test(trimmed);
}

/** The brand mark for a model id, or undefined to keep the harness mark. */
export function resolveModelLogoKey(model: string | undefined): ModelLogoKey | undefined {
	if (model === undefined) return undefined;
	const trimmed = model.trim();
	if (isDefaultModelRef(trimmed)) return undefined;
	return EXACT_MODEL_LOGOS[trimmed];
}
