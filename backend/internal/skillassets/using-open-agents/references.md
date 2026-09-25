# Quick Reference

Natural-language-to-command mappings for common Open Agents tasks.

| You want to... | Command |
|---|---|
| Show me this webpage / open this page | `open-agents preview "<url>"` |
| Start an existing configured dev app | `open-agents preview start [configuration]` |
| Check or stop the worker's managed dev app | `open-agents preview status` / `open-agents preview stop` |
| Show this Markdown or HTML file without a server | `open-agents preview "<workspace-path>"` |
| Hand off a newly created browser-displayable artifact | `open-agents preview "<workspace-path>"` immediately after writing the primary artifact |
| Inspect and verify this webpage as the agent | `open-agents browser open "<url>"`, then `open-agents browser snapshot` |
| Click or fill a page element | `open-agents browser snapshot`, then `open-agents browser click <ref>` or `open-agents browser fill <ref> "<text>"` |
| Check frontend runtime failures | `open-agents browser errors` and `open-agents browser console` |
| Diagnose a request/API/CORS/auth/redirect failure when normal page evidence is insufficient | `open-agents browser network start`, reproduce once, then `open-agents browser network stop` |
| Check network capture without enabling it | `open-agents browser network status` or `open-agents browser network list` |
| Open the user's real Chromium debugging surface | `open-agents browser devtools open` |
| Close the shared DevTools window when explicitly requested | `open-agents browser devtools close` |
| Capture the page | `open-agents browser screenshot [path]` |
| Spawn a worker on issue N | `open-agents spawn --project <p> --issue N --name "<=20 chars>" --prompt "..."` |
| Message a running agent | `open-agents send --session <id> --message "..."` |
| Kill a session | `open-agents session kill <id>` |
| List sessions | `open-agents session ls` |
| Register a repo as a project | `open-agents project add --path <abs-path> --name <name>` |
| List projects | `open-agents project ls` |
| Rename a session | `open-agents session rename <id> "<name>"` |
| Restore a killed session | `open-agents session restore <id>` |
| Clean up terminated sessions | `open-agents session cleanup` |
| Make a Docker container this session starts survive Open Agents cleanup | `docker run --label open-agents.session=$OPEN_AGENTS_SESSION_ID --label open-agents.spare=true ...` |
| See a session's details | `open-agents session get <id>` |
| Open the desktop app | `open-agents start` |
| Check the daemon is up | `open-agents status` |
| Run health checks | `open-agents doctor` |
| Clear the preview panel | `open-agents preview clear` |
| List orchestrator sessions | `open-agents orchestrator ls` |
| Claim an existing PR for the current session | `open-agents session claim-pr <pr-ref>` (`OPEN_AGENTS_SESSION_ID`) |
| Claim an existing PR for another session | `open-agents session claim-pr <id> <pr-ref>` |
| Submit a code review verdict | `open-agents review submit <session-id> --run <run-id> --verdict approved` |
| Configure a project's default branch or model | `open-agents project set-config <id> --default-branch <branch> --model <model>` |
