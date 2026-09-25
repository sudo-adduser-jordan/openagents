# Open Agents start and distribution history

The earlier bootstrapper design and npm delivery proposal are obsolete. This page
preserves the distribution decision as design history; it is not an operator
runbook and must not be used to reconstruct a publication flow.

## Current contract

- Install the desktop app from [GitHub Releases](https://github.com/sudo-adduser-jordan/open-agents/releases). The release repository is `sudo-adduser-jordan/open-agents`.
- The desktop identity is **Open Agents**: the app is `Open Agents.app`, its bundle identifier is `dev.openagents.desktop`, its executable is `open-agents`, and its URL scheme is `open-agents://`.
- The CLI is `open-agents`. Configuration uses the `OPEN_AGENTS_*` environment prefix.
- Persistent state belongs under `~/.open-agents`; managed workspace metadata belongs under `.open-agents`.
- Managed branches use the `open-agents/*` namespace and managed refs use `refs/open-agents/*`.
- Open Agents has no compatibility command aliases, state migration, or import path for an earlier product installation. A new installation starts with the canonical state root.

## Publication history

A previously published npm package remains frozen externally as a historical
artifact. It is not an Open Agents install path, package identity, or supported
distribution channel. This repository does not unpublish it, document it as a
new-product installer, or build compatibility behavior around it.

For current release and macOS signing rules, see
[`frontend/docs/desktop-release.md`](../frontend/docs/desktop-release.md). For the
supported command surface, see [`docs/cli/README.md`](cli/README.md).
