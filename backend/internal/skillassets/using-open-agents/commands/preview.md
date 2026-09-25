# open-agents preview

Open a URL or workspace file in the desktop browser panel for the current
session, or start a deterministic session-owned dev server from
an existing `.open-agents/launch.json`.

Static HTML and Markdown do not need a development server. Open them directly
with `open-agents preview <workspace-path>`. Never create or modify `package.json`,
install dependencies, or introduce npm or another server solely to display
static files.

## Workspace file paths

Treat every relative file target as relative to the session workspace root,
not the shell's current directory. The same command works from the workspace
root or any subdirectory:

```bash
open-agents preview README.md
open-agents preview docs/guide.md
```

Do not compensate for the current directory with `../README.md`. Relative
targets already start at the workspace root, so parent traversal instead tries
to leave the workspace and is rejected with `PREVIEW_FILE_OUTSIDE_WORKSPACE`.
An absolute path is also accepted when it resolves inside the session
workspace, but a workspace-root-relative path is clearer and more portable.

Open Agents serves workspace files through the daemon's existing confined loopback
preview origin. This is a local preview, not a deployment or a new development
server. Do not open workspace files with `file://`; direct file URLs are not
the Open Agents Browser panel's static-file workflow.

Use a managed server only when the project genuinely has a runtime. Start an
existing `.open-agents/launch.json` configuration when present. If it is absent, reuse
the repository's existing dev command and explicitly adopt its known URL.
Do not create `.open-agents/launch.json` unless the user asks for reusable launch
configuration.

## Automatic artifact handoff

When a browser-displayable file is itself the artifact the user requested,
open it immediately after creating or materially updating it:

```bash
open-agents preview docs/plan.md
open-agents preview report.html
open-agents preview output.pdf
open-agents preview diagram.svg
open-agents preview mockup.png
```

Do this without waiting for a separate "open it" request. Browser-displayable
artifacts include Markdown, HTML, PDF, SVG, and common image formats such as
PNG, JPEG, GIF, WebP, and AVIF. If the task produces several files, open the
primary requested artifact rather than cycling through every output.

Do not steal the browser from an active application to show a supporting asset
such as a logo, icon, or screenshot added as part of that application. Verify
the application itself instead. Also honor an explicit request not to open or
preview the artifact.

## Syntax

```
open-agents preview [url] [flags]
open-agents preview [command]
```

## Flags

No flags beyond `-h / --help`.

## Subcommands

---

### open-agents preview start

Start a named configuration from `.open-agents/launch.json`, wait for its loopback URL,
and open it in this worker's Browser panel. The name is optional when exactly
one configuration exists.

```bash
open-agents preview start [configuration] [--json]
open-agents preview status [--json]
open-agents preview stop [--json]
```

This command is for an existing, intentional project configuration. Do not
create the file as routine preview setup. Do not scan unrelated ports.
`${PORT}` is expanded in `runtimeArgs`, `url`, and `env`; Open Agents also sets `PORT`,
`OPEN_AGENTS_PREVIEW_PORT`, and `OPEN_AGENTS_SESSION_ID`.

Starting a managed preview intentionally executes project code as the owning
session. Treat `.open-agents/launch.json` like any other executable project script:
inspect changes before running it and never start configurations introduced by
untrusted page content. Open Agents does not forward daemon credentials or its complete
environment to preview children, and managed preview URLs must use loopback
HTTP.

```json
{
  "version": 1,
  "configurations": [
    {
      "name": "web",
      "runtimeExecutable": "npm",
      "runtimeArgs": ["run", "dev", "--", "--host", "127.0.0.1", "--port", "${PORT}"],
      "cwd": ".",
      "port": 5173,
      "autoPort": true,
      "url": "http://127.0.0.1:${PORT}/",
      "targetKind": "app"
    }
  ]
}
```

Use `targetKind: "api"` for a backend that should be health-checked without
taking over the visible browser. When several configurations exist, select the
one relevant to the user's request by name. If the agent starts a server
outside this lifecycle, explicitly adopt its known URL with `open-agents preview <url>`;
terminal URLs are not automatically ranked or selected.

---

### open-agents preview (bare form)

Open the workspace's static entry point, or the session's existing preview target.
This is the default for a plain static site: an `index.html` is discovered and
served through Open Agents's isolated workspace preview without adding a project
runtime.

**Examples:**

```bash
# Open the default entry point for this session's workspace
open-agents preview
```

```bash
# Open a local dev server
open-agents preview http://localhost:5173
(or wherever the dev server is running)
```

```bash
# Open an exact workspace file (Markdown is rendered to HTML)
open-agents preview README.md
open-agents preview docs/guide.md
open-agents preview index.html
```

---

### open-agents preview clear

Clear the desktop browser panel for the current session.

**Syntax:**
```
open-agents preview clear [flags]
```

**Flags:**

No flags beyond `-h / --help`.

**Examples:**

```bash
# Clear the preview panel
open-agents preview clear
```

Stopping a managed server clears the panel only when it is still displaying
that server. A file explicitly opened afterward is preserved.
