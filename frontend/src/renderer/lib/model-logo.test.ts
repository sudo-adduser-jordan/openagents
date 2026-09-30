import { describe, expect, it } from "vitest";
import { isDefaultModelRef, resolveModelLogoKey } from "./model-logo";

describe("resolveModelLogoKey", () => {
	it("maps every listed model id to its own mark", () => {
		expect(resolveModelLogoKey("muse-spark")).toBe("model-muse");
		expect(resolveModelLogoKey("muse-spark-1.1")).toBe("model-muse");
		expect(resolveModelLogoKey("muse-spark-1.2")).toBe("model-muse");
		expect(resolveModelLogoKey("opencode/nemotron-3-ultra-free")).toBe("model-nemotron");
	});

	it("falls back to the harness mark for anything unlisted", () => {
		expect(resolveModelLogoKey("opencode/big-pickle")).toBeUndefined();
		expect(resolveModelLogoKey("opencode/space-bunny-free")).toBeUndefined();
		expect(resolveModelLogoKey(undefined)).toBeUndefined();
	});

	it("never prefix-matches a listed id", () => {
		// A provider namespaces its own models: these are different models that
		// must not inherit a mark they did not ask for.
		expect(resolveModelLogoKey("muse-spark-9")).toBeUndefined();
		expect(resolveModelLogoKey("muse-spark-preview")).toBeUndefined();
		expect(resolveModelLogoKey("opencode/nemotron")).toBeUndefined();
		expect(resolveModelLogoKey("opencode/nemotron-3-ultra-free-2")).toBeUndefined();
	});

	it("does not treat a harness or provider name as a model id", () => {
		expect(resolveModelLogoKey("muse")).toBeUndefined();
		expect(resolveModelLogoKey("nemotron")).toBeUndefined();
		expect(resolveModelLogoKey("opencode")).toBeUndefined();
	});

	it("keeps the harness mark for harness defaults", () => {
		expect(resolveModelLogoKey("default")).toBeUndefined();
		expect(resolveModelLogoKey("")).toBeUndefined();
		expect(resolveModelLogoKey("default(fast)")).toBeUndefined();
	});

	it("ignores surrounding whitespace and case-sensitive ids", () => {
		expect(resolveModelLogoKey("  muse-spark  ")).toBe("model-muse");
		expect(resolveModelLogoKey("MUSE-SPARK")).toBeUndefined();
	});
});

describe("isDefaultModelRef", () => {
	it("treats an absent or empty model as the harness default", () => {
		expect(isDefaultModelRef(undefined)).toBe(true);
		expect(isDefaultModelRef("")).toBe(true);
		expect(isDefaultModelRef("   ")).toBe(true);
	});

	it("recognizes plain and parameterized defaults", () => {
		expect(isDefaultModelRef("default")).toBe(true);
		expect(isDefaultModelRef("default()")).toBe(true);
		expect(isDefaultModelRef("default(slow)")).toBe(true);
		expect(isDefaultModelRef("  default(gpt-4o)  ")).toBe(true);
	});

	it("does not treat a real model as a default", () => {
		expect(isDefaultModelRef("muse-spark")).toBe(false);
		expect(isDefaultModelRef("defaulted")).toBe(false);
		expect(isDefaultModelRef("my-default")).toBe(false);
		expect(isDefaultModelRef("defaulted(gpt-4o)")).toBe(false);
	});
});
