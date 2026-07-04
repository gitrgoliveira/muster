# Product

## Register

product

## Users

**Primary: the solo developer running an agent fleet.** A single power user who
orchestrates many parallel coding-agent runs across their own repositories. They
live in the terminal, are fluent with `git`/`jj`, `tmux`, and CLI coding agents
(Claude Code, Gemini CLI, Codex, OpenCode), and they think in tickets and specs.

Their context is a **command center, not a form**: multiple beads in flight at
once, some running, some awaiting review, some blocked. The job to be done is to
*muster a fleet* — dispatch work against a shared spec + issue tracker, watch
runs progress at a glance, drop into any one for detail, review agent output,
and merge — without losing the thread across a dozen concurrent agents.

They value density, keyboard control, and signal-over-chrome. They will abandon
anything that hand-holds, hides state, or makes them click through modals to do
something a shortcut should do.

## Product Purpose

**Muster is "beads-central"** — a single Go binary that serves beads (issues)
over a REST + WebSocket API and runs CLI coding agents against them inside
per-bead VCS worktrees. The UI is a Kanban-shaped orchestrator: beads flow
Backlog → Scheduled → Running → Needs review → Done, each executed as an ordered
chain of agent steps (plan → build → review → commit) with per-step modes and
skills, dispatched into isolated `git`/`jj` worktrees via `tmux`.

It exists to make running a **fleet of coding agents** legible and controllable
from one place — aggregating beads across repositories, exposing each run's
lifecycle and diff, and keeping everything coherent through a shared
"constitution" and skill loadouts. Beads is the source of truth; Muster owns no
durable state of its own.

Success looks like: a solo dev can keep ten agents working in parallel and always
know, in under two seconds of looking, what is running, what needs them, and what
is stuck — and can act on any of it without leaving the keyboard.

## Brand Personality

**Restrained, precise, expert — with earned warmth.** The base is an
Anthropic-editorial calm command center: paper-cream surface, a single warm
terracotta accent (`#D97757`), Instrument Serif for display, Geist for body,
JetBrains Mono for data. Quiet, dense, confident. Nothing shouts; hierarchy and
typography do the work.

On top of that restraint, the product should feel **warmer and more delightful**
than a sterile tool — personality in the copy voice, intentional motion, and
small considered moments (a satisfying state transition, a well-timed empty
state, a bead settling into its column). Delight here is craft and character,
**not** decoration: it lives in timing, wording, and micro-interaction, never in
bright colors, bubbles, or mascots. Warmth is carried by the accent, the serif,
and the writing — not by adding chrome.

Voice: direct, literate, a little dry. Speaks to a peer who knows the domain.

## Anti-references

This must **not** look like any of:

- **Generic SaaS dashboard** — gradient cards, the hero-metric template (big
  number / small label / supporting stats), purple-on-white, identical
  icon+heading+text card grids, tiny tracked-uppercase eyebrows over every
  section.
- **Neon cyberpunk "AI tool"** — dark-mode-by-default, glowing accents,
  terminal-hacker theatrics, matrix/scanline motifs. Muster is about agents but
  refuses the aesthetic cliché of "AI product."
- **Enterprise Jira clutter** — dense uniform chrome, nested panels, config
  sprawl, everything at the same visual weight, no focal point.
- **Consumer-app playful** — rounded bubbly shapes, bright multi-color palettes,
  big friendly illustrations, mascots. (Delight comes from motion/voice/craft,
  not from going cute.)

## Design Principles

1. **Command center, at a glance.** The primary job is monitoring many things at
   once. State must be legible in under two seconds — status, ownership, and
   "does this need me?" resolved by scanning, never by clicking in.
2. **Density is a feature; noise is the enemy.** Serve the power user with
   information richness, but earn every pixel. Hierarchy and restraint keep a
   dense screen calm rather than cluttered.
3. **Keyboard-first, mouse-optional.** Every primary action has a shortcut; the
   command palette is a first-class citizen. Efficiency for experts is not a
   power-user afterthought, it is the default path.
4. **Warmth through craft, not decoration.** Personality lives in motion, copy,
   and micro-interaction — timing and wording — never in ornament. Restraint and
   delight are not in tension when delight is well-made.
5. **Color is confirmation, never the only signal.** Every status color is
   paired with a glyph or label. The interface stays readable in grayscale and
   for color-blind users by construction.

## Accessibility & Inclusion

Target **WCAG 2.1 AA**. Color is never the sole carrier of meaning — status is
always paired with a glyph or text label (DESIGN.md §9). Body text meets 4.5:1
against the paper-cream surface; the warm accent is used for emphasis at sizes
and weights that clear contrast minimums, not for body copy. Keyboard operability
is a product principle, not just an a11y checkbox: full keyboard navigation with
visible focus indicators, `Esc` to dismiss overlays, and no click-only
interactions. Honor `prefers-reduced-motion` — the intentional motion of
principle 4 must always have a reduced-motion alternative (crossfade or instant).
