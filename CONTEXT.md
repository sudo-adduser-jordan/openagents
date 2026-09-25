# Domain Glossary

Canonical vocabulary for Open Agents. Terms only — no implementation
details, no decisions (those live in `docs/adr/`).

## Daemon surfaces

- **Loopback Listener** — the daemon's only supported HTTP surface, bound to
  `127.0.0.1`. It serves the desktop app and CLI and remains unauthenticated.
