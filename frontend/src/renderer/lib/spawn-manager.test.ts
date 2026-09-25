import { describe, expect, it, vi, beforeEach } from "vitest";
import { isChatPreflightError, ManagerSpawnError, spawnManager } from "./spawn-manager";
import { apiClient } from "./api-client";

vi.mock("./api-client", () => ({
	apiClient: { POST: vi.fn() },
	apiErrorCode: (error: unknown) =>
		typeof error === "object" && error !== null && "code" in error
			? String((error as { code: unknown }).code)
			: undefined,
	apiErrorRequestId: (error: unknown) =>
		typeof error === "object" && error !== null && "requestId" in error
			? String((error as { requestId: unknown }).requestId)
			: undefined,
	apiErrorMessage: (error: unknown, fallback = "Request failed") => {
		if (typeof error === "object" && error !== null && "message" in error) {
			const body = error as { code?: unknown; message: unknown };
			const message = String(body.message);
			return typeof body.code === "string" && body.code !== "" ? `${message} (${body.code})` : message;
		}
		return fallback;
	},
}));

describe("spawnManager", () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it("sends clean:true through to the request body when asked", async () => {
		(apiClient.POST as ReturnType<typeof vi.fn>).mockResolvedValue({
			data: { manager: { id: "proj-9" } },
			error: undefined,
			response: { status: 201 },
		});
		const id = await spawnManager("proj", "restore_dialog", true);
		expect(id).toBe("proj-9");
		expect(apiClient.POST).toHaveBeenCalledWith("/api/v1/managers", {
			body: { projectId: "proj", clean: true },
		});
	});

	it("defaults clean to false / omitted for the existing call sites", async () => {
		(apiClient.POST as ReturnType<typeof vi.fn>).mockResolvedValue({
			data: { manager: { id: "proj-1" } },
			error: undefined,
			response: { status: 201 },
		});
		await spawnManager("proj", "board");
		expect(apiClient.POST).toHaveBeenCalledWith("/api/v1/managers", {
			body: { projectId: "proj", clean: false },
		});
	});

	it("sends mode only when the user explicitly chooses it", async () => {
		(apiClient.POST as ReturnType<typeof vi.fn>).mockResolvedValue({
			data: { manager: { id: "proj-2" } },
			error: undefined,
			response: { status: 201 },
		});
		await spawnManager("proj", "board", false, "tui");
		expect(apiClient.POST).toHaveBeenCalledWith("/api/v1/managers", {
			body: { projectId: "proj", clean: false, mode: "tui" },
		});
	});

	it("accepts project_clone as a first-class manager spawn source", async () => {
		(apiClient.POST as ReturnType<typeof vi.fn>).mockResolvedValue({
			data: { manager: { id: "proj-8" } },
			error: undefined,
			response: { status: 201 },
		});
		await spawnManager("proj", "project_clone");
		expect(apiClient.POST).toHaveBeenCalledWith("/api/v1/managers", {
			body: { projectId: "proj", clean: false },
		});
	});

	it("surfaces daemon spawn error messages and codes", async () => {
		(apiClient.POST as ReturnType<typeof vi.fn>).mockResolvedValue({
			data: undefined,
			error: {
				code: "CHAT_DRIVER_UNAVAILABLE",
				message: "chat driver is unavailable",
				requestId: "request-42",
			},
			response: { status: 400 },
		});

		const error = await spawnManager("proj", "board").catch((caught: unknown) => caught);
		expect(error).toBeInstanceOf(ManagerSpawnError);
		expect(error).toMatchObject({
			code: "CHAT_DRIVER_UNAVAILABLE",
			requestId: "request-42",
			status: 400,
		});
		expect((error as Error).message).toBe("chat driver is unavailable (CHAT_DRIVER_UNAVAILABLE)");
		expect(isChatPreflightError(error)).toBe(true);
	});
});
