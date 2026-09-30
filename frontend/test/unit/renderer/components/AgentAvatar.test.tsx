import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { AgentAvatar } from "../../../../src/renderer/components/AgentAvatar";

describe("AgentAvatar", () => {
	it("renders the opencode brand asset", () => {
		render(<AgentAvatar provider="opencode" />);

		expect(screen.getByRole("img", { name: "opencode" })).toHaveAttribute(
			"src",
			expect.stringContaining("data:image/svg+xml"),
		);
		// The inlined asset carries the opencode brand title, so this is the
		// real logo rather than the lettered fallback tile.
		expect(screen.getByRole("img", { name: "opencode" })).toHaveAttribute("src", expect.stringContaining("opencode"));
	});

	it("prefers a listed model's own mark over the harness's", () => {
		render(<AgentAvatar provider="opencode" model="muse-spark" />);

		// The harness still supplies the accessible name, so a model mark never
		// reads to a screen reader as a differently-named agent.
		expect(screen.getByRole("img", { name: "opencode" })).toHaveAttribute("src", expect.stringContaining("muse"));
	});

	it("keeps the harness mark for an unlisted model", () => {
		render(<AgentAvatar provider="opencode" model="muse-spark-9" />);

		expect(screen.getByRole("img", { name: "opencode" })).toHaveAttribute("src", expect.stringContaining("opencode"));
	});

	it("keeps the harness mark when the session runs on the harness default", () => {
		render(<AgentAvatar provider="opencode" model="default" />);

		expect(screen.getByRole("img", { name: "opencode" })).toHaveAttribute("src", expect.stringContaining("opencode"));
	});
});
