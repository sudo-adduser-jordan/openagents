# Architecture diagrams

Five diagrams of the Open Agents system, generated from code so they can be
regenerated when the architecture moves. Each exists as an editable
[Excalidraw](https://excalidraw.com) scene (`.excalidraw`) and as a committed
SVG (`.svg`) for anywhere that cannot render a scene.

| # | Scene | What it answers |
|---|-------|-----------------|
| 01 | [`01-application-overview`](./01-application-overview.excalidraw) | Which processes exist, who may talk to whom, and where state lives. |
| 02 | [`02-session-state-machines`](./02-session-state-machines.excalidraw) | The durable `activity_state` machine vs. the derived display status. |
| 03 | [`03-frontend-architecture`](./03-frontend-architecture.excalidraw) | The Electron main / preload / renderer split and every transport across the socket. |
| 04 | [`04-backend-architecture`](./04-backend-architecture.excalidraw) | The Go request path, the single write path, ports/adapters, and the CDC pipeline. |
| 05 | [`05-interface-handoff-saga`](./05-interface-handoff-saga.excalidraw) | The TUI ↔ Chat controller handoff, its commit point, and its failure paths. |

The scene files are the source of truth. The SVGs are build output, but they
are committed so the docs render without a build step.

## Regenerating

```sh
cd frontend
npm install
npm run diagrams
```

`npm run diagrams` writes to `docs/assets/diagrams/`. Pass a substring to
regenerate only matching scenes:

```sh
npm run diagrams -- handoff
```

## How it works

The generator lives in `frontend/scripts/diagrams/`:

| File | Role |
|------|------|
| `lib.mjs` | The scene DSL: `Scene`, `TONE`, and the `box` / `frame` / `link` / `edge` / `note` / `caption` primitives. |
| `scenes/*.mjs` | One file per diagram, written against the DSL. This is where content changes go. |
| `generate.mjs` | Turns a scene into `.excalidraw` JSON and `.svg`. |
| `dom-shim.mjs` | A jsdom + canvas + `FontFace` shim, enough for Excalidraw to import in Node. |
| `build.mjs` | Bundles the scenes with esbuild, preloads the shim, runs the generator. |

The three-step dance in `build.mjs` is load-bearing: Excalidraw ships an
unattributed-JSON browser bundle, so it has to be bundled for Node; the shim has
to be preloaded with `--import` because the package touches `document` at import
time; and `jsdom` has to stay external because bundling it to ESM breaks its own
internal `require("node:fs")` calls.

## Editing a diagram

Do not hand-edit the `.excalidraw` or `.svg` files — the next `npm run diagrams`
overwrites them. Change the matching file in `scenes/` and regenerate.

If you would rather edit visually: open the `.excalidraw` file in Excalidraw,
make your changes, and export a scene JSON. Transcribing the result back into
the DSL is usually easier than trying to reverse-engineer coordinates, since the
DSL computes text widths and box heights for you.

### Text metrics are approximate

The `measureText` stub in `dom-shim.mjs` estimates glyph advance at
`0.55em` per character, so SVG text widths are approximate. Box and frame
geometry is computed from that same estimate, which keeps the SVG and the scene
consistent. The scene file is the accurate artifact: it re-renders exactly when
opened in Excalidraw with a real font.
