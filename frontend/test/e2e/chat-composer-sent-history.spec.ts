import { expect, test } from "@playwright/test";
import { agentReadiness } from "../../src/renderer/test/agent-readiness-fixtures";
import { installFakeAgent } from "./support/fake-bridge";

const projectId = "chat-composer-history";
const sessionId = "chat-composer-history-worker";
const completedAt = "2026-08-26T06:00:00Z";

test("chat composer recalls sent messages bash-style with arrow keys @T0", async ({ page }) => {
	await installFakeAgent(page, {
		projectId,
		projectName: projectId,
		workers: [{ id: sessionId, provider: "codex", title: "Composer history", mode: "chat" }],
	});
	await page.route("http://127.0.0.1:8080/api/v1/**", async (route) => {
		const pathname = new URL(route.request().url()).pathname;
		if (pathname === "/api/v1/agents/readiness" || pathname === "/api/v1/agents/readiness/ensure") {
			await route.fulfill({ json: { agents: [agentReadiness("codex", "Codex")] } });
			return;
		}
		if (pathname === `/api/v1/projects/${projectId}`) {
			await route.fulfill({
				json: {
					status: "ok",
					project: { id: projectId, agent: "codex", config: { worker: { agent: "codex" } } },
				},
			});
			return;
		}
		if (pathname === `/api/v1/sessions/${sessionId}/conversation`) {
			await route.fulfill({
				json: {
					conversationId: "conversation-chat-composer-history",
					sessionId,
					harness: "codex",
					mode: "chat",
					controller: "ready",
					latestSequence: 4,
					oldestSequence: 1,
					hasMoreBefore: false,
					turns: [
						{
							id: "turn-1",
							state: "completed",
							requestedAt: completedAt,
							startedAt: completedAt,
							completedAt,
						},
					],
					messages: [
						{
							kind: "message",
							id: "message-1",
							turnId: "turn-1",
							sequence: 1,
							revision: 0,
							role: "user",
							origin: "human",
							text: "first prompt",
							streaming: false,
							createdAt: completedAt,
						},
						{
							kind: "message",
							id: "message-2",
							turnId: "turn-1",
							sequence: 2,
							revision: 0,
							role: "assistant",
							origin: "provider",
							text: "agent answer",
							streaming: false,
							createdAt: completedAt,
						},
						{
							kind: "message",
							id: "message-3",
							turnId: "turn-1",
							sequence: 3,
							revision: 0,
							role: "user",
							origin: "human",
							text: "second prompt",
							streaming: false,
							createdAt: completedAt,
						},
					],
					activities: [],
					settings: {},
				},
			});
			return;
		}
		if (pathname === `/api/v1/sessions/${sessionId}/conversation/models`) {
			await route.fulfill({ json: { models: [], selected: {} } });
			return;
		}
		if (pathname === `/api/v1/sessions/${sessionId}/conversation/skills`) {
			await route.fulfill({ json: { skills: [] } });
			return;
		}
		if (pathname === `/api/v1/sessions/${sessionId}/workspace/files`) {
			await route.fulfill({ json: { files: [], truncated: false } });
			return;
		}
		if (pathname === `/api/v1/sessions/${sessionId}/interface-transition`) {
			await route.fulfill({ json: { supported: true, targetMode: "tui" } });
			return;
		}
		await route.fulfill({ json: { status: "ok" } });
	});

	await page.goto(`/#/projects/${projectId}/sessions/${sessionId}`);
	const composer = page.getByRole("combobox", { name: "Message the agent" });
	// Cold dev-server loads can take a while; wait generously for first paint.
	await expect(composer).toBeVisible({ timeout: 30000 });
	await expect(composer).toHaveText("");

	// Recall only fires from an empty composer: existing text is left alone.
	await composer.fill("a fresh draft");
	await composer.press("ArrowUp");
	await expect(composer).toHaveText("a fresh draft");
	await composer.fill("");

	await composer.press("ArrowUp");
	await expect(composer).toHaveText("second prompt");
	await composer.press("ArrowUp");
	await expect(composer).toHaveText("first prompt");
	// Clamped at the oldest entry.
	await composer.press("ArrowUp");
	await expect(composer).toHaveText("first prompt");
	await composer.press("ArrowDown");
	await expect(composer).toHaveText("second prompt");
	// Escaping a recall restores the empty draft.
	await composer.press("Escape");
	await expect(composer).toHaveText("");
});
