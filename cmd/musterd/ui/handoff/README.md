# Muster — Implementation Handoff

This bundle is everything Claude Code needs to implement **Muster**, a spec-driven agent orchestrator (Kanban-shaped UI for queuing tickets — "beads" — across multiple coding-agent CLIs).

It is split into two independent implementation tracks. You can pick up either one without touching the other:

| Track    | Canonical spec | What you build                                                                                       |
|----------|----------------|------------------------------------------------------------------------------------------------------|
| **UI**   | `DESIGN.md`    | A React (or framework-of-your-choice) application that matches the HTML prototype in `prototype/`.   |
| **Backend** | `spec.md`   | A single Go binary `musterd` that serves the UI + an HTTP/WS API at `:7766`. |

The bridge between the two tracks is **DESIGN.md §11 — Mock → backend handover map**, which lists every place the UI is currently mocked and the spec.md endpoint that replaces each mock.

---

## What's in this bundle

```
README.md            ← you are here
DESIGN.md            ← UI spec (~46 KB) — read this for the frontend track
spec.md              ← Backend spec (~71 KB) — read this for the backend track
prototype/           ← the high-fidelity HTML prototype, viewable in a browser
  Muster.html          shell + script tags
  styles.css           all CSS (tokens + components, ~6k lines)
  *.jsx                React components (Babel-transpiled in the browser)
```

**The prototype is a reference, not production code.** It exists so you can see and click the design. The HTML uses unpinned Babel-in-browser; the production UI should be built with a real toolchain (Vite + React, or whatever the target codebase uses).

To view the prototype:

```bash
# from inside prototype/
python3 -m http.server 8080
# then open http://localhost:8080/Muster.html
```

The prototype seeds itself from `data.jsx`'s `window.MUSTER_DATA` — every shape there matches a type in DESIGN.md §1.11 and spec.md §3.

---

## Suggested reading order

1. **DESIGN.md §1 (Mental model)** — establishes the vocabulary: beads, steps, modes, skills, VCS, constitution, providers, dispatcher. Both tracks need this.
2. **DESIGN.md §2 (Page architecture)** — the seven views + the overlays.
3. **spec.md §0–§1** — backend goals/non-goals + the high-level diagram.
4. **For UI**: DESIGN.md §3–§12 in order. §11 (mock-seam table) is the single most useful page when wiring the backend in.
5. **For backend**: spec.md §3 (data model) → §4 (HTTP/WS API) → §5 (dispatcher) → §6 (step execution) → §7 (tmux/CLI adapters). Then §17 (testing) before writing a line of code.

---

## Status conventions

- Both md files use **§** for section refs and explicit numbered sub-sections so cross-references are unambiguous.
- Any item the human still owes a decision on is in `DESIGN.md §7` (UI deferred) and `spec.md §15` (backend open questions). Don't invent answers — flag and ask.
- `DESIGN.md §12` and `spec.md §20` (milestones) are the punch-lists. Tick items off as you ship.

---

## Stack & dependencies

### Backend (spec.md §16)

- Go 1.26+, single binary, embedded UI via `go:embed`.
- Key libs: `dolthub/go-mysql-server` (Beads), `coder/websocket`, `go-chi/chi`, `spf13/viper`, `zalando/go-keyring`, `prometheus/client_golang`, `google/uuid`.
- Runtime deps on the host: `tmux >= 3.2`, `jj >= 0.20`, `git >= 2.40`.

### UI (DESIGN.md §8 file map)

- Pick a real toolchain (Vite + React is the path of least resistance — the prototype is already React).
- Fonts in use: **Instrument Serif** (display, italic), **Geist** (body), **JetBrains Mono** (mono). All from Google Fonts.
- CSS tokens live in `prototype/styles.css` lines 5–32 — lift them wholesale into your design-system layer.

---

## Asset & brand notes

- **No external brand assets.** The three-bead chain logo is inline SVG in `prototype/Muster.html` (head, `<link rel="icon">`) and reproduced in `app.jsx` (the `<div class="brand-mark">` block). No image files to copy.
- The palette is paper-cream with a single warm accent (`#D97757`). It is intentionally restrained — no gradient backgrounds, no decorative iconography.
- All status colours (amber/green/violet/rose) are paired with a glyph or label so colour is never the sole carrier of meaning (DESIGN.md §9).

---

## Where to start (concrete first PR)

If you're picking up the backend cold, the first PR should be **spec.md §20 M0 — Skeleton**:
- A `musterd serve` that binds `127.0.0.1:7766`, serves the embedded UI, exposes `GET /api/v1/beads` returning an in-memory array, and pushes optimistic mutations via WS at `/api/v1/stream`.
- Use the seed data from `prototype/data.jsx`'s `TASKS` array as the in-memory fixture so the UI renders without changes.

If you're picking up the UI cold, the first PR should be **DESIGN.md §12 — Production-readiness checklist** item 1 (timestamps) plus standing up a real Vite + React project that imports the JSX files unchanged. Once that runs, work through the §11 handover map endpoint by endpoint.

---

## Contact

If anything in the specs contradicts itself or the prototype, the order of authority is:

1. The Go struct in `spec.md §3`
2. The TypeScript shape in `DESIGN.md §1.11`
3. The JSX prototype

Flag the drift and ask the human; do **not** silently pick one.
