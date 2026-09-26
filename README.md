<div align="center">
  <img src="assets/open-agents-logo.svg" alt="Open Agents" width="144" height="144" />

### Open Agents

#### Plan, run, and supervise coding agents from one place.

[![GitHub stars](https://img.shields.io/github/stars/sudo-adduser-jordan/open-agents?style=flat&logo=github)](https://github.com/sudo-adduser-jordan/open-agents/stargazers)
![Top 6k repositories](https://img.shields.io/badge/Top%206k%20repositories-181717?style=flat&logo=github&logoColor=white)
[![GitHub release](https://img.shields.io/github/v/release/sudo-adduser-jordan/open-agents?style=flat&logo=github)](https://github.com/sudo-adduser-jordan/open-agents/releases/latest)
[![GitHub downloads](https://img.shields.io/github/downloads/sudo-adduser-jordan/open-agents/total?style=flat&logo=github)](https://github.com/sudo-adduser-jordan/open-agents/releases)
[![License: Apache-2.0](https://img.shields.io/badge/license-Apache--2.0-blue?style=flat)](LICENSE)
[![Discord](https://img.shields.io/badge/Discord-555?style=flat&logo=discord&logoColor=white)](https://discord.com/invite/UZv7JjxbwG)

[**Download Open Agents**](#install) &nbsp;&bull;&nbsp; [Documentation](https://orchestrator.inc/docs) &nbsp;&bull;&nbsp; [Releases](https://github.com/sudo-adduser-jordan/open-agents/releases) &nbsp;&bull;&nbsp; [Contributing](CONTRIBUTING.md) &nbsp;&bull;&nbsp; [Discord](https://discord.com/invite/UZv7JjxbwG)
</div>

## Install

Download the latest Open Agents desktop app for your platform. Open Agents checks for updates automatically.

| Platform              | Download                                                                                                                      |
| --------------------- | ----------------------------------------------------------------------------------------------------------------------------- |
| macOS (Apple silicon) | [Download](https://github.com/sudo-adduser-jordan/open-agents/releases/latest/download/open-agents-darwin-arm64.dmg)   |
| macOS (Intel)         | [Download](https://github.com/sudo-adduser-jordan/open-agents/releases/latest/download/open-agents-darwin-x64.dmg)     |
| Windows               | [Download](https://github.com/sudo-adduser-jordan/open-agents/releases/latest/download/open-agents-win32-x64.exe)      |
| Linux (AppImage)      | [Download](https://github.com/sudo-adduser-jordan/open-agents/releases/latest/download/open-agents-linux-x64.AppImage) |
| Linux (Debian/Ubuntu) | [Download](https://github.com/sudo-adduser-jordan/open-agents/releases/latest/download/open-agents-linux-x64.deb)      |
| Linux (Fedora/RHEL)   | [Download](https://github.com/sudo-adduser-jordan/open-agents/releases/latest/download/open-agents-linux-x64.rpm)      |

The desktop app runs the daemon for you, so no CLI is required. See the [installation guide](https://orchestrator.inc/docs/installation) for agent CLI setup and troubleshooting.

## Architecture

Five diagrams of how Open Agents is put together. Each is generated from code in `frontend/scripts/diagrams/`, so it can be rebuilt when the architecture moves — see [`docs/assets/diagrams/README.md`](docs/assets/diagrams/README.md). Open any image for a full-size copy, or the `.excalidraw` link to edit it.

### 1. Application overview

Which processes exist, who is allowed to talk to whom, and where state actually lives. Every other diagram is a zoom-in on one of the frames below.

<img src="docs/assets/diagrams/01-application-overview.svg" alt="Open Agents application overview: the desktop app, CLI, loopback daemon, adapters, external systems, and on-disk state" width="100%" />

[Open full size](docs/assets/diagrams/01-application-overview.svg) &nbsp;&bull;&nbsp; [Edit in Excalidraw](docs/assets/diagrams/01-application-overview.excalidraw)

### 2. Session state machines

The two machines that are constantly mistaken for one: the durable `activity_state` written by a single reducer, and the derived display status recomputed on every read.

<img src="docs/assets/diagrams/02-session-state-machines.svg" alt="Session state machines: the durable activity_state machine on the left, the derived display-status precedence ladder on the right" width="100%" />

[Open full size](docs/assets/diagrams/02-session-state-machines.svg) &nbsp;&bull;&nbsp; [Edit in Excalidraw](docs/assets/diagrams/02-session-state-machines.excalidraw)

### 3. Frontend

Main owns processes, preload owns capability, renderer owns presentation. Every byte crossing into the renderer goes through the bridge.

<img src="docs/assets/diagrams/03-frontend-architecture.svg" alt="Frontend architecture: the Electron main process, preload contextBridge, React renderer, and the transports across the loopback socket" width="100%" />

[Open full size](docs/assets/diagrams/03-frontend-architecture.svg) &nbsp;&bull;&nbsp; [Edit in Excalidraw](docs/assets/diagrams/03-frontend-architecture.excalidraw)

### 4. Backend

The Go request path, the single write path allowed to move a durable fact, the ports the core depends on, and the CDC pipeline that fans changes back out.

<img src="docs/assets/diagrams/04-backend-architecture.svg" alt="Backend architecture: the inbound httpd and service layer, the session_manager and lifecycle write path, ports and adapters, and the SQLite change_log CDC pipeline" width="100%" />

[Open full size](docs/assets/diagrams/04-backend-architecture.svg) &nbsp;&bull;&nbsp; [Edit in Excalidraw](docs/assets/diagrams/04-backend-architecture.excalidraw)

### 5. Session interface handoff

The TUI ↔ Chat handoff as a saga: the session row is the single commit point, everything before it is reversible, and messages that arrive while no controller is live survive whoever ends up owning the session.

<img src="docs/assets/diagrams/05-interface-handoff-saga.svg" alt="The TUI to Chat interface handoff saga: forward path, commit point, failure and recovery paths, and the fences around the no-controller gap" width="100%" />

[Open full size](docs/assets/diagrams/05-interface-handoff-saga.svg) &nbsp;&bull;&nbsp; [Edit in Excalidraw](docs/assets/diagrams/05-interface-handoff-saga.excalidraw)

For the reasoning behind these boundaries, start with [docs/architecture.md](docs/architecture.md) and [docs/STATUS.md](docs/STATUS.md).

## Screenshots

<img src="docs/assets/readme/hero.png" alt="Open Agents Kanban showing worker sessions grouped by live status" width="100%" />

<img src="docs/assets/readme/tui.png" alt="A worker agent's native terminal interface supervised inside Open Agents" width="100%" />

<img src="docs/assets/readme/browser.png" alt="A worker controlling its isolated in-app browser preview" width="100%" />

## License

Open Agents is available under the [Apache License 2.0](LICENSE).
