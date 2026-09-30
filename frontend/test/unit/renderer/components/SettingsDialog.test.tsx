import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useUiStore } from "../../../../src/renderer/stores/ui-store";
import type { ProjectSettingsSaveState } from "../../../../src/renderer/components/ProjectSettingsForm";
import { SettingsDialog } from "../../../../src/renderer/components/SettingsDialog";

const { postMock } = vi.hoisted(() => ({ postMock: vi.fn() }));

vi.mock("../lib/api-client", () => ({
	apiClient: { POST: postMock },
	apiErrorCode: (error: { code?: string }) => error?.code,
	apiErrorMessage: () => "request failed",
	hasTrustedApiBaseUrl: () => true,
}));

vi.mock("./ProjectSettingsForm", () => ({
	ProjectSettingsForm: ({
		onSaveState,
	}: {
		onSaveState?: (state: ProjectSettingsSaveState) => void;
	}) => (
		<button
			type="button"
			onClick={() =>
				onSaveState?.({
					phase: "pending",
				})
			}
		>
			Start pending save
		</button>
	),
}));

vi.mock("./GlobalSettingsForm", () => ({
	GlobalSettingsForm: ({ section }: { section: string }) => <div data-testid="global-settings-section">{section}</div>,
}));

// The dialog reads the cloud gate to decide whether the Cloud nav page exists;
// mocked so these tests need no QueryClientProvider (same pattern as Sidebar).
vi.mock("../hooks/useCloudGate", () => ({
	useCloudGate: () => ({ cloudEnabled: false, localEnabled: true }),
}));

describe("SettingsDialog", () => {
	beforeEach(() => {
		postMock.mockReset().mockResolvedValue({ data: {} });
		useUiStore.setState({ settingsModal: null });
	});

	function renderSettingsDialog() {
		const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
		return render(<QueryClientProvider client={queryClient}><SettingsDialog /></QueryClientProvider>);
	}

	it("does not dismiss project settings while a save is pending", async () => {
		useUiStore.getState().openProjectSettings("proj-1");
		renderSettingsDialog();

		await userEvent.click(await screen.findByRole("button", { name: "Start pending save" }));
		const closeButton = screen.getByRole("button", { name: "Close settings" });
		expect(closeButton).toBeDisabled();

		await userEvent.keyboard("{Escape}");
		expect(useUiStore.getState().settingsModal).toEqual({ scope: "project", projectId: "proj-1" });
	});

	it("opens the requested global settings page", async () => {
		useUiStore.getState().openGlobalSettings("tools");
		renderSettingsDialog();

		expect(await screen.findByTestId("global-settings-section")).toHaveTextContent("tools");
		expect(screen.getByRole("button", { name: "Tools" })).toHaveAttribute("aria-current", "page");
	});

	it("has no Mobile tab in the settings nav", async () => {
		useUiStore.getState().openGlobalSettings("general");
		renderSettingsDialog();

		const nav = await screen.findByRole("navigation", { name: "Settings sections" });
		expect(within(nav).queryByRole("button", { name: "Mobile" })).not.toBeInTheDocument();
		const labels = within(nav)
			.getAllByRole("button")
			.map((button) => button.textContent?.trim());
		expect(labels).toEqual(["General", "Harness", "Browser", "Skills", "Tools", "Shortcuts", "Updates", "Help"]);
	});

	it("mounts dialog chrome before the selected settings form", async () => {
		useUiStore.getState().openGlobalSettings("general");
		renderSettingsDialog();

		expect(screen.getByTestId("settings-dialog-body-pending")).toBeInTheDocument();
		expect(screen.queryByTestId("global-settings-section")).not.toBeInTheDocument();
		expect(await screen.findByTestId("global-settings-section")).toHaveTextContent("general");
	});

	it("does not expose Downloads as a standalone settings page", async () => {
		useUiStore.getState().openGlobalSettings("browserProfiles");
		renderSettingsDialog();

		expect(await screen.findByTestId("global-settings-section")).toHaveTextContent("browserProfiles");
		expect(screen.queryByRole("button", { name: "Downloads" })).not.toBeInTheDocument();
	});

	// The sidebar footer shortcut and this nav are read in the same order, so
	// Skills has to sit directly above Tools here too.
	it("orders the Skills tab directly above Tools in the settings nav", async () => {
		useUiStore.getState().openGlobalSettings("general");
		renderSettingsDialog();

		const nav = await screen.findByRole("navigation", { name: "Settings sections" });
		const labels = within(nav)
			.getAllByRole("button")
			.map((button) => button.textContent?.trim());
		expect(labels.indexOf("Skills")).toBe(labels.indexOf("Tools") - 1);
	});

	it("opens the Skills page from its nav tab", async () => {
		useUiStore.getState().openGlobalSettings("general");
		renderSettingsDialog();

		await userEvent.click(await screen.findByRole("button", { name: "Skills" }));

		expect(await screen.findByTestId("global-settings-section")).toHaveTextContent("skills");
		expect(screen.getByRole("button", { name: "Skills" })).toHaveAttribute("aria-current", "page");
	});



	it("traps focus and closes from Escape or the backdrop", async () => {
		useUiStore.getState().openGlobalSettings("general");
		renderSettingsDialog();

		const dialog = await screen.findByRole("dialog");
		expect(dialog).toHaveAttribute("aria-modal", "true");
		await vi.waitFor(() => expect(dialog).toContainElement(document.activeElement as HTMLElement));
		await userEvent.keyboard("{Escape}");
		await vi.waitFor(() => expect(useUiStore.getState().settingsModal).toBeNull());

		useUiStore.getState().openGlobalSettings("general");
		fireEvent.pointerDown(await screen.findByTestId("settings-dialog-overlay"));
		await vi.waitFor(() => expect(useUiStore.getState().settingsModal).toBeNull());
	});

	it("does not close when Escape is handled by a portaled nested menu", async () => {
		useUiStore.getState().openGlobalSettings("general");
		renderSettingsDialog();

		await screen.findByRole("dialog");
		fireEvent.keyDown(document.body, { key: "Escape" });
		expect(useUiStore.getState().settingsModal).not.toBeNull();

		fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
		await vi.waitFor(() => expect(useUiStore.getState().settingsModal).toBeNull());
	});
});
