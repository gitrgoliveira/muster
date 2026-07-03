---
target: whole app (holistic)
total_score: 33
p0_count: 1
p1_count: 4
timestamp: 2026-07-03T14-44-45Z
slug: cmd-muster-ui-index-html
---
Method: dual-agent (A: design-review · B: detector + browser evidence)
Target: Muster web UI (whole app, holistic) — `cmd/muster/ui/`

## Design Health Score

| # | Heuristic | Score | Key Issue |
|---|-----------|-------|-----------|
| 1 | Visibility of System Status | 4 | Capacity strip, now-playing rail, live dots, token bars — "what's happening" resolves at a glance. |
| 2 | Match System / Real World | 4 | Speaks the user's exact language (bd ready, worktree, jj/git, dispatch); CLI hints everywhere. |
| 3 | User Control and Freedom | 3 | Esc closes overlays, but no undo on dispatch/move and no keyboard path for drag reorder. |
| 4 | Consistency and Standards | 3 | Strong token system; docked for drawer doc/impl drift (spec 640px side-panel vs coded 92vw centered modal). |
| 5 | Error Prevention | 3 | VCS locks after branch; dispatch disabled when blocked — but Approve&close / Dispatch fire with no confirm or undo. |
| 6 | Recognition Rather Than Recall | 4 | Suggested-agent chips, per-mode default prompts, template rails, CLI hints — very low recall burden. |
| 7 | Flexibility and Efficiency | 3 | ⌘K palette + density presets, but ⌘K is the ONLY global shortcut and card keyboard-activation is broken. |
| 8 | Aesthetic and Minimalist Design | 4 | Genuinely restrained; density earns its "command center" claim. |
| 9 | Error Recovery | 2 | No error boundary (a JS error blanks the page), no retry/requeue at failure, runlog reconnect state unbuilt. |
| 10 | Help and Documentation | 3 | ⌘K doubles as a searchable 106-command bd index — clever inline docs; no first-run onboarding. |
| **Total** | | **33/40** | **Strong** (above the typical 20–32 band) |

> The 33 scores UX/usability. A pure WCAG-AA audit would score lower — the accessibility gaps below (P0/P1) are real and measured.

## Anti-Patterns Verdict

**Does this look AI-generated? No — and convincingly so.** This is authored, coherent, on-brand work.

- **LLM assessment:** Clears every anti-reference. No gradient text, no hero-metric template, no icon+heading+text card grid, no neon/cyberpunk, no consumer-playful bubble. The gradients that exist are sub-6%-opacity tonal washes over paper, not purple-on-white SaaS. Real domain invention carries it: the bead-chain step rail (literal beads on a hairline thread), digit-only priority badges, a 3px type-color card edge instead of a head glyph, the dark now-playing terminal rail, an asymmetric column layout. Voice has character ("archive is quiet", "Queue is clear — everything ready has been dispatched"). Instrument Serif italic for column heads/brand is a deliberate, well-executed warmth move.
- **Deterministic scan:** Detector exit 2, but only **one** finding — `layout-transition` at `tweaks-panel.jsx:104` (a segmented-control thumb animating `width`; true positive, low impact). This near-clean result is **low-signal, not a clean bill**: the detector can't see `styles.css` (207KB, where all real styling lives via `className`), so it had almost no CSS surface to inspect. The real evidence came from code review + browser measurement, not the scanner.
- **Visual evidence:** Live render **succeeded** (React+Babel compiled, 0 console errors, `#root` populated). No user-visible overlay was injected — B measured the live DOM directly instead. Screenshot confirmed a crowded, right-clipped topbar and tight board density.

## Overall Impression

The taste here is real — this is the rare agent-tool UI that doesn't scream "AI made it," and the board genuinely delivers the "ten agents in flight, I can see all of it" cockpit promise. The problem isn't aesthetics; it's that **the UI asserts accessibility and resilience it doesn't actually implement.** DESIGN.md §9 claims "every card stays keyboard-activatable" — the code has no such thing. PRODUCT.md promises reduced-motion and WCAG AA — neither holds. And the scariest moment for a fleet operator, a run dying at 2am, has no first-class surface at all. The single biggest opportunity: **make the keyboard-first, AA-accessible, failure-resilient product that the design already describes.**

## What's Working

1. **The now-playing rail + capacity strip as a persistent "fleet vitals" band** (`app.jsx:88-197`). Distressed providers sort to the front; each running bead gets one mono terminal line with agent/action/elapsed/token-%. Dense without noise — the product thesis made visual.
2. **Color-is-never-sole-signal, honored by construction.** Every status tint pairs with a glyph or word (ready-pill text, gate icons ☻/◷/⎇, filled/outlined/ringed step-bead shapes, +/−/@@ diff prefixes). Genuinely readable in grayscale — rare to see done this thoroughly.
3. **The command palette as a living `bd` index** (`command-palette.jsx`). ⌘K reframed from "nav launcher" to "searchable documentation of 106 subcommands" fits a CLI-fluent power user perfectly and doubles as onboarding-by-exploration.

## Priority Issues

**[P0] The keyboard-first board is not keyboard-operable, and focus is invisible.**
- **What:** `Card`/`MiniCard` are `<article onClick>` with no `tabIndex`, `role`, or `onKeyDown` (`kanban.jsx:167-231, 234-255`) — unreachable by Tab, un-openable by keyboard. Independently, B measured only **one** real focus ring in the whole app (`.bead-link:focus-visible`, `styles.css:1275`); `outline: none` appears ~30× and the primary New-bead button shows no focus box-shadow. DESIGN.md §9 claims cards "open via Enter on focus" — false.
- **Why it matters:** The primary user is keyboard-first (PRODUCT.md principle 3) and the primary object on the primary view can't be driven or seen by keyboard. Breaks the core promise for power users AND screen-reader users; drag is the only reorder gesture.
- **Fix:** Add `tabIndex={0}`, `role="button"`, `aria-label`, `onKeyDown` (Enter/Space→open) to cards; restore a global `:focus-visible` ring and stop blanket `outline:none`.
- **Suggested command:** `/impeccable harden`

**[P1] Measured color-contrast failures — including the primary CTA.**
- **What (computed, deterministic):** `--accent #D97757` as text on paper = **2.92:1** (fails); **white on `--accent`** (the New-bead button label) = **3.12:1** (fails 4.5 body); `--amber` text = 2.67:1; `--green` text = 3.81:1; `--ink-4` = 2.75:1. Compounded by tiny type — status pills render at **8.5–9.5px** (ready-pill 3.33:1, blocked-pill 4.14:1 at those sizes).
- **Why it matters:** PRODUCT.md targets WCAG AA. The main dispatch CTA and status pills — the things you read most — are the ones failing. Sub-11px text at ~3:1 is hard to read for everyone, not just low-vision users.
- **Fix:** Darken accent-as-text usages (or only use accent on fills with a darker ink label), lift `--amber`/`--green` text tones, and floor status-pill text at ≥11px. Keep the palette identity; adjust the ramp.
- **Suggested command:** `/impeccable colorize` (then `/impeccable audit` for the full sweep)

**[P1] No `prefers-reduced-motion` for ~15 infinite animations.**
- **What:** Zero `prefers-reduced-motion` in `styles.css`, yet infinite `pulse`/`blink`(×8+)/`beadGlow`/`nowPulse` plus `drawerIn`/`cardIn`/`colIn` entrances. Self-flagged TODO (DESIGN.md §12).
- **Why it matters:** Promised in PRODUCT.md and a WCAG 2.1 AA obligation (2.3.3). Continuously-blinking dots on a dense screen are a vestibular/attention hazard.
- **Fix:** One `@media (prefers-reduced-motion: reduce)` block setting `animation: none`/crossfade on the glow/pulse/blink/scale-in classes.
- **Suggested command:** `/impeccable animate`

**[P1] No failure-recovery surface when a run dies.**
- **What:** Failed steps get a pill and rose log lines, but there's no error boundary (a JS error blanks the page), no retry/requeue/split at the failure moment, and the "stream dropped — reconnecting" runlog state (DESIGN.md §10) isn't built.
- **Why it matters:** Fleet operators live with failures — this is the highest-anxiety moment and the UI offers no reassurance or recovery. The craft budget is spent on the launch (bead-glow, dispatch); the failure end-state is absent.
- **Fix:** Add an `<App>` error boundary (banner, not blank); a failed-run banner in the drawer header with Requeue / Split / Re-dispatch; implement the reconnect toast.
- **Suggested command:** `/impeccable harden`

**[P1] The drawer implementation contradicts its own interaction model.**
- **What:** Spec (§4.7/§5.3) says a right-anchored `min(640px,50vw)` side-panel with a non-blocking backdrop so "clicking a card behind switches the active bead." Code renders a **centered** panel at `width:min(960px,92vw)` (`styles.css:742-760`) — near-fullscreen, covering the board.
- **Why it matters:** The "switch beads without close/reopen" delight (§6.3) depends on the board staying visible behind the drawer. A 92vw centered modal defeats it, and the drift will confuse the backend implementor.
- **Fix:** Pick a canonical model. Side-panel → restore right-anchor + 640px. Centered-modal → update DESIGN.md and give it a proper blocking scrim + focus trap (which also fixes a screen-reader issue).
- **Suggested command:** `/impeccable layout`

## Persona Red Flags

**Alex (impatient power user)** — mostly served, three breaks:
- ⌘K is the *only* global shortcut — no `n` (new), `j/k` (nav), `/` (search focus), `1–7` (views). Keyboard-*available*, not keyboard-*first*.
- Cards can't be opened or moved by keyboard (P0) — can't drive the board without a mouse.
- Dispatch/Approve have no undo — a fat-fingered Approve has no recourse.

**Sam (accessibility / keyboard / screen reader)** — several hard breaks:
- Board cards not focusable and not announced (P0) — the core object is invisible to AT.
- `outline:none` without replacement on selects/inputs in the Steps tab (only one real focus ring app-wide).
- `contentEditable` title (`drawer.jsx:355-365`) with only `data-placeholder` — no `role="textbox"`/`aria-label`.
- No `prefers-reduced-motion` (P1). *Credit where due:* agent chips are `tabIndex={0}`, drawer close has `aria-label`, color-never-sole-signal is real.

**Riley (edge cases / stress)** — the State Catalogue (§10) is largely unbuilt: no skeletons, no per-column error+retry, no loading states — pulling the network shows blank regions. Rendered overlap bug in the older `modes.png` (provider title colliding with subtext). Timestamp ordering is a fragile string heuristic (`lastActivity.includes('Fri')`).

## Minor Observations

- **Topbar overflows at 1440px** (`scrollWidth 1669`) — the New-bead CTA is clipped hard against the right edge. §12 flags overflow <960px, but B measured it at desktop width. → `/impeccable adapt`
- **Irreversible actions, no undo** — prefer a lightweight "Dispatched bd-a1f2 · Undo" toast over a modal confirm, to keep the keyboard-fast feel with a safety net.
- **Dead code:** orphaned `updateStep`/`removeStep`/`addStepTemplate` block at module top level (`drawer.jsx:879-899`); `StepEditor` (drawer.jsx:55) appears unused vs `StepCard`. Delete both.
- **Provider card clipping** (`providers.png`) — "TODAY $4.82/$20.00" and "resets in 8h" rows clip against the card edge; vertical rhythm slightly too tight.
- **Brand:** code renders "muster" consistently (brand mark, title, wordmark). Some committed screenshots under `screens/` are from an older iteration that read differently — stale design-scratch captures, not shipped assets.

## Questions to Consider

1. If the drawer is now a 92vw centered modal, what is the non-blocking backdrop *for*? Commit to the side-panel and get the "switch beads live" magic, or drop the pretense and give it a proper scrim + focus trap.
2. The product's scariest moment is a run failing, yet the craft budget goes to the launch. Should "earned warmth" show up *most* at failure — a calm, well-worded recovery state — where the solo dev actually needs it?
3. Seven nav tabs + a 7-card pulse strip + a 6-button add-step row: where does "density is a feature" tip into "everything at the same weight" — the exact Jira anti-reference? The board earns its density; does the pulse strip?
4. Keyboard-first is principle 3, but ⌘K is the only global key. Is this keyboard-first, or keyboard-available?
