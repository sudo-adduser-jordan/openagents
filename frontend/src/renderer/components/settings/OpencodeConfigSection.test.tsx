import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { createElement } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";


import { OpencodeConfigSection } from "./OpencodeConfigSection";

const getMock = vi.hoisted(() => vi.fn());
const putMock = vi.hoisted(() => vi.fn());

vi.mock("../../lib/api-client", () => ({
	apiClient: { GET: getMock, PUT: putMock },
	apiErrorMessage: (error: unknown, fallback: string) => {
		if (error instanceof Error) return error.message;
		const message = (error as { message?: unknown } | null)?.message;
		return typeof message === "string" && message !== "" ? message : fallback;
	},
}));

/** The textarea always renders; wait for the served text to land in it. */
async function loadedEditor() {
	const editor = (await screen.findByRole("textbox", { name: "OpenCode configuration" })) as HTMLTextAreaElement;
	await waitFor(() => expect(editor).not.toBeDisabled());
	return editor;
}

function renderSection() {
	const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
	return render(
		createElement(QueryClientProvider, { client }, createElement(OpencodeConfigSection, {})),
	);
}

describe("OpencodeConfigSection", () => {
	beforeEach(() => {
		getMock.mockReset();
		putMock.mockReset();
	});

	afterEach(() => {
		vi.restoreAllMocks();
	});

	it("shows the served document and the path it is editing", async () => {
		getMock.mockResolvedValue({
			data: { path: "/home/u/.config/opencode/opencode.jsonc", exists: true, content: "{\n  // hi\n}" },
		});
		renderSection();

		const editor = await loadedEditor();
		expect(editor.value).toBe("{\n  // hi\n}");
		expect(screen.getByText("/home/u/.config/opencode/opencode.jsonc")).toBeInTheDocument();
	});

	// A file opencode will not load is a problem the user has to be able to see
	// and repair, so the text stays editable and the warning is shown, not thrown.
	it("keeps a broken file editable and surfaces the warning", async () => {
		getMock.mockResolvedValue({
			data: {
				path: "/p/opencode.jsonc",
				exists: true,
				content: '{"permission": {',
				warning: "This file is not valid JSONC and will not load: unexpected end of JSON input",
			},
		});
		renderSection();

		expect(await screen.findByRole("alert")).toHaveTextContent("not valid JSONC");
		const editor = await loadedEditor();
		expect(editor.value).toBe('{"permission": {');
	});

	it("only enables Save once the text actually changes", async () => {
		getMock.mockResolvedValue({ data: { path: "/p", exists: true, content: "{}" } });
		renderSection();

		const editor = await loadedEditor();
		const save = screen.getByRole("button", { name: "Save" });
		expect(save).toBeDisabled();

		fireEvent.change(editor, { target: { value: '{"a":1}' } });
		expect(save).toBeEnabled();
	});

	it("sends the draft verbatim so comments survive", async () => {
		getMock.mockResolvedValue({ data: { path: "/p", exists: true, content: "{}" } });
		putMock.mockResolvedValue({ data: { path: "/p", exists: true, content: '{"a":1}' } });
		renderSection();

		const editor = await loadedEditor();
		fireEvent.change(editor, { target: { value: '{\n  // keep me\n  "a": 1\n}' } });
		fireEvent.click(screen.getByRole("button", { name: "Save" }));

		await waitFor(() => expect(putMock).toHaveBeenCalledTimes(1));
		expect(putMock).toHaveBeenCalledWith("/api/v1/settings/opencode-config", {
			body: { content: '{\n  // keep me\n  "a": 1\n}' },
		});
	});

	it("reports a rejected save instead of claiming success", async () => {
		getMock.mockResolvedValue({ data: { path: "/p", exists: true, content: "{}" } });
		putMock.mockResolvedValue({
			error: { message: "opencode.jsonc is not valid JSONC: unexpected end of JSON input" },
		});
		renderSection();

		const editor = await loadedEditor();
		fireEvent.change(editor, { target: { value: "{" } });
		fireEvent.click(screen.getByRole("button", { name: "Save" }));

		expect(await screen.findByRole("alert")).toHaveTextContent("not valid JSONC");
	});
});
