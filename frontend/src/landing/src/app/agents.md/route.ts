import { COMPANY } from "@openagents/shared/constants";
import { buildWhenToUseSection, MARKDOWN_HEADERS } from "@/lib/llms";

export const dynamic = "force-static";

export function GET() {
  const baseUrl = COMPANY.MARKETING_URL;
  const docsUrl = COMPANY.DOCS_URL;

  const lines: string[] = [
    `# ${COMPANY.NAME} for AI agents`,
    "",
    `This page is the machine-readable entry point for AI agents working inside ${COMPANY.NAME} (${baseUrl}). ${COMPANY.NAME} is a local desktop application and daemon, not a hosted agent API.`,
    "",
    ...buildWhenToUseSection(),
    "",
    "## Capabilities",
    "",
    `Inside an ${COMPANY.NAME}-managed session, an agent can use the installed local \`open-agents\` CLI to:`,
    "",
    "- **Tasks**: create, list, inspect, and update work tracked by the local Open Agents daemon.",
    "- **Workspaces**: work in branch- or PR-scoped Git worktrees managed by Open Agents.",
    "- **Agents**: launch and inspect coding-agent sessions in isolated workspaces.",
    "- **Terminals**: open a PTY in a workspace, optionally running a one-off command.",
    "- **Projects and previews**: inspect registered repositories and open local preview URLs in Open Agents's browser panel.",
    "",
    "## How to operate",
    "",
    "- Run `open-agents help` or `open-agents <command> --help` for the commands installed with the current Open Agents version.",
    "- The CLI is a thin client of the local daemon; do not bypass it by reading Open Agents's SQLite state directly.",
    "- Open Agents's primary daemon listener is loopback-only. Do not treat this website as an API endpoint.",
    "",
    "## Learn more",
    "",
    `- [Docs](${docsUrl}/)`,
    `- [Quickstart](${docsUrl}/quickstart/)`,
    `- [CLI reference](${docsUrl}/cli/)`,
    `- [Download](${baseUrl}/download/)`,
    `- [Site llms.txt](${baseUrl}/llms.txt)`,
    `- [GitHub](${COMPANY.GITHUB_URL})`,
  ];

  return new Response(lines.join("\n"), { headers: MARKDOWN_HEADERS });
}
