# Muster — Design

Spec-driven agent orchestrator. Kanban-shaped UI for queuing tickets ("beads") across multiple coding agents (Claude Code, Gemini CLI, OpenCode, Codex, …) and chaining them through plan / build / review / commit phases.

The name nods to the shape of the job — *mustering* a fleet of agent CLIs into formation around a shared spec. A Kanban skin sits on top of an orchestrator that fans work out to many CLI tools, while a "constitution" and shared skills keep everything coherent.

---

## 1. Mental model

### 1.1 Beads
A **bead** is one unit of work — what most trackers call a ticket. Beads live in columns:

  Backlog → Scheduled to run → Running → Needs review → Done

A bead is identified by `bd-<hash>`. It can spawn **sub-beads** with hierarchical IDs (`bd-a1f2.1`, `bd-a1f2.2`). Sub-beads appear when the dispatcher auto-splits a too-large task at token-budget pressure, or when the user creates them manually. Sub-beads are openable in the drawer with a synthesized view inheriting their parent's metadata.

Each bead carries:

- **id** (`bd-a1f2`)
- **type** — `bug` · `feature` · `task` · `epic` · `chore`
- **priority** — `0..4` (0 = critical, 4 = icebox)
- **status / column** — backlog · scheduled · running · review · done
- **labels** — flat tags (`oauth`, `security`, …)
- **acceptance criteria** — inline todo list, checked off as the agent completes them
- **comments** — thread of human + agent comments (persisted to `history`)
- **history** — full lifecycle event log
- **deps** — four types (see §1.7)
- **vcs** — per-bead `jj` or `git` (see §1.5)
- **pinnedAgent** — optional `bd pin <id> --for <agent>` assignment
- **steps** — ordered chain of agent invocations (see §1.2)

### 1.2 Step chain
A bead is executed as an **ordered chain of steps**. Each step is a single agent invocation:

  { agent, mode, skills[], prompt, status, loopBackTo?, loopMax? }

- **agent** — which provider runs this step (`claude`, `gemini`, …).
- **mode** — the agent's mode (see §1.3). Could be a real CLI flag OR a Muster-synthesized workflow stage.
- **skills** — capabilities loaded for this step only (see §1.4). **Per-step, no bead-level defaults.**
- **prompt** — user-editable text. Each mode has a sensible default; the user can override inline.
- **loop control** — `loopBackTo` plus `loopMax` lets a step (typically Review) bounce the chain back to an earlier step a bounded number of times. This drives "build → review → fix → re-review" cycles without writing graphs.

### 1.3 Modes — native vs Muster-synthesized
A critical distinction. Each provider exposes:

- **Native modes** — real CLI flags or SDK config. e.g. `claude --permission-mode plan`, `claude --permission-mode acceptEdits`, `gemini --yolo`.
- **Synthesized modes** — Muster workflow stages (`plan`, `build`, `review`, `agent`) that don't have a corresponding CLI flag. Muster runs the bare provider invocation and shapes behavior via system-prompt framing.

Each mode entry carries:

  { id, name, desc, cli, icon, native: bool, workflowStage: 'plan'|'build'|'agent'|'review'|'yolo' }

For Claude Code, the real modes are about **permissions**, not workflow stages:

  | Native mode    | CLI                                              | Stage mapped |
  |----------------|--------------------------------------------------|--------------|
  | Plan           | `claude --permission-mode plan`                  | plan         |
  | Accept-edits   | `claude --permission-mode acceptEdits`           | build        |
  | Default        | `claude`                                         | agent        |
  | Bypass         | `claude --permission-mode bypassPermissions`     | yolo         |

A "review" workflow stage for Claude Code is **synthesized** — it runs `claude --permission-mode plan` plus a review-shaped system prompt.

Other providers (Gemini, OpenCode, Codex) are still in verification — their non-default modes are marked synthesized until confirmed against the upstream CLI.

The UI surfaces this distinction:
- Native modes show no extra annotation.
- Synthesized modes carry a `synthesized` chip and a tooltip explaining that Muster wraps the default provider invocation with a stage-shaped system prompt.

### 1.4 Skills (per-step)
A **skill** is a loadable capability — tool pack, MCP server, or RAG index. Categories: `spec`, `code`, `web`, `doc`, `design`, `pm`, `infra`.

**Skills are applied per-step**, not at the bead level. Each step picks its own loadout via a compact chip row with a search popover. There is intentionally no "default skills" concept on the bead itself — having two scopes proved confusing and ran afoul of DRY.

Spec-system skills (Speckit, OpenSpec, Beads memory) get a violet tint because they tend to *define* what a step is.

Skills auto-discover from standard paths (agentskills.io spec):
- project: `./.agents/skills/`
- user: `~/.config/agents/skills/`
- builtin: `muster://skills/`
- url: manually imported via `bd skill add <url>`

### 1.5 VCS backend (per-bead)
**Jujutsu is not a skill.** Each bead picks its worktree backend at creation time via `Bead.VCS`:

- `jj` — `jj clone --colocate`, `jj describe`, `jj push`
- `git` — `git worktree add`, `git commit`, `git push`

Surfaced everywhere as a small badge (`⌥ jj` or `⎇ git`). Locked once a worktree exists; switching backends mid-bead would orphan the worktree.

### 1.6 Constitution
A **constitution** is a markdown blob prepended to every bead's system prompt. Universal rules: commit-message format, test-or-it-didn't-happen, dependency policy. Set once in the Orchestrator tab; a banner reminds you it's loaded inside every bead's drawer (with preview + edit link).

### 1.7 Dependencies (Beads-native)
Four dep types per [Beads docs](https://gastownhall.github.io/beads/core-concepts/issues):

  | Type             | Meaning                              | Affects `bd ready`? |
  |------------------|--------------------------------------|---------------------|
  | `blocks`         | Hard dep — X blocks Y                | Yes                 |
  | `parent-child`   | Epic/subtask relationship            | No                  |
  | `discovered-from`| Issue found during another's work    | No                  |
  | `related`        | Soft relationship                    | No                  |

Plus **cross-repo deps** via the `external:` prefix:

  `bd dep add bd-42 external:billing-repo/bd-100`

Surfaced in the drawer's Deps tab with separate color-coded sections per type.

### 1.8 Providers
Each LLM is added as a **provider**. Three flavors:

- **CLI tool** — Muster shells out to a binary (`claude`, `gemini`, `codex`, `aider`, …). The CLI handles its own auth, rate limits, and native modes. Muster dispatches via tmux, captures stdout, reads the worktree.
- **SDK** — Muster calls the SDK in-process (e.g. OpenCode). No subprocess, no tmux.
- **Direct API** — Muster hits the HTTP endpoint itself with an API key in Keychain.

A provider exposes: name, plan, version, auth status, daily + monthly quota, rate limit, parallel cap, and its native + synthesized modes.

### 1.9 Multi-agent routing (multi-repo)
[Beads multi-agent](https://gastownhall.github.io/beads/multi-agent) supports cross-repo routing. Muster surfaces:

- **Routes** (`.beads/routes.jsonl`) — glob patterns matched against issue title/labels, route to target repo. Priority-ordered, `*` fallback.
- **`bd routes test`** — live preview as you type a title.
- **`bd pin <id> --for <agent>`** — explicit work assignment; dispatcher respects pinned agent before falling back to capacity-based suggestion.
- **`bd hydrate`** — pull related issues from sibling repos. Per-repo with ahead-count + last-sync.

### 1.10 Dispatcher
The dispatcher is implicit — whatever picks the next bead off "Scheduled to run" and assigns it to a free provider slot. Visible in the UI via:

- **Capacity strip** at the top (per-provider slot pips + quota bar; distressed providers sort to the front)
- **Now-playing rail** (dark terminal-style strip under capacity strip — one line per running bead with agent, mode, current action, elapsed, token bar)
- **Failure policy** in the Orchestrator tab (requeue on token exhaustion, auto-split at 80% budget, escalation rules)

The dispatcher reads bead step `agent` and `mode`. The user-facing **suggested agent** for a backlog/scheduled bead is `task.pinnedAgent` if set, else `task.steps[0].agent` (via `suggestAgent`), with a capacity-loaded fallback when neither is available.

### 1.11 Concrete data shapes
Locked between UI and backend. The UI consumes / mutates these shapes via the API in `spec.md §3` — keep both files in sync when fields change.

```ts
Bead = {
  id: string;                       // "bd-a1f2" or "bd-a1f2.1" (sub-bead)
  title: string;
  desc: string;                     // markdown task statement
  type: 'feature' | 'bug' | 'task' | 'epic' | 'chore';
  priority: 0 | 1 | 2 | 3 | 4;      // 0 = critical, 4 = icebox
  column: 'backlog' | 'scheduled' | 'running' | 'review' | 'done';
  labels: string[];                 // lowercase, hyphenated
  vcs: 'jj' | 'git';                // immutable once `branch` is set
  branch: string | null;
  pinnedAgent?: string;             // provider id
  assignee?: string;                // current/last agent, derived
  repo: string;                     // attached-repo id
  estimate: 'XS' | 'S' | 'M' | 'L'; // derived from tokensBudget

  steps: Step[];
  subBeads?: SubBead[];
  acceptance: AC[];
  gates?: Gate[];

  blocks: string[];
  blockedBy: string[];
  externalDeps?: string[];          // "external:repo/bd-xxx"
  discoveredFrom?: string;          // bead id this was spawned from

  formula?: string;                 // formula id
  ready: boolean;                   // bd ready filter result
  requeued?: boolean;
  reviewer?: { agent: string; comments: number };
  nowPlaying?: { action: string; since: number; kind: 'tool' | 'thought' | 'output' };

  tokensUsed: number;
  tokensBudget: number;
  comments: number;                 // derived count
  history: HistoryEvent[];          // append-only lifecycle
  log?: RunLogLine[];               // runlog tail (server streams full)
  files?: FileChange[];
  diffPreview?: string;

  createdAt: string;
  closedAt?: string;
  openedAt?: string;
  lastActivity: string;
};

Step = {
  agent: string;
  mode: string;                     // canonical id OR workflowStage alias
  skills: string[];                 // per-step
  prompt?: string;                  // undefined = use mode default
  status: 'pending' | 'active' | 'done' | 'failed' | 'blocked';
  note?: string;
  loopBackTo?: number;              // step index
  loopMax?: number;                 // default 3
};

SubBead = {
  id: string;                       // "bd-a1f2.1"
  title: string;
  status: 'pending' | 'active' | 'done' | 'failed';
  agent?: string;
  autoSplit?: boolean;              // dispatcher-spawned vs user-created
};

AC = { text: string; done: boolean };

Gate = {
  kind: 'human' | 'timer' | 'github';
  label: string;
  status: 'waiting' | 'passed' | 'failed';
  meta?: Record<string, any>;
};

HistoryEvent = {
  at: string;                       // RFC3339 from server; UI tolerates relative strings
  kind: 'opened' | 'scheduled' | 'claimed' | 'started' | 'paused'
       | 'split' | 'review' | 'comment' | 'approved' | 'closed'
       | 'reopened' | 'requeued' | 'blocked' | 'unblocked' | 'failed' | 'discovered';
  actor: string;                    // "you@yours.dev" | "dispatcher" | <agent-id>
  agent?: string;                   // for claimed: which agent
  note?: string;
};

RunLogLine = {
  t: string;                        // HH:MM:SS
  kind: 'system' | 'tool' | 'thought' | 'output' | 'error';
  msg: string;
};

FileChange = {
  path: string;
  status: 'A' | 'M' | 'D' | 'R';
  adds: number;
  dels: number;
};
```

The live UI carries every field above. Anywhere the seed data has filler strings (`'Mon 09:14'`, `'just now'`), the backend should substitute real RFC3339 timestamps — the UI's `EVENT_KINDS` mapping and the lifecycle feed's ordering heuristic both fall back gracefully on either form.

**Field ownership.** Some fields are server-authoritative (`id`, `history`, `tokensUsed`, `closedAt`, `comments` count, `assignee` derivation, `estimate` derivation, `lastActivity`, `ready` evaluation, `requeued`, `reviewer`); the UI never writes them. Some are client-mutable via PATCH (`title`, `desc`, `labels`, `priority`, `type`, `vcs` *only while `branch` is null*, `pinnedAgent`, `acceptance`, `steps`). Some are write-once at create time (`id`, `repo`, `createdAt`). The matching Go struct (`spec.md §3.1`) is the canonical source; if the two ever drift, the Go struct wins and DESIGN.md is regenerated.

---

## 2. Page architecture

Seven top-level views, switched from the top nav:

  /board         — Kanban (5 columns; 4-column grid layout — see §3)
  /lifecycle     — Pulse counts + ready-to-dispatch + activity timeline
  /orchestrator  — Constitution + capacity + failure policy + routes + hydration
  /repos         — Working trees Muster is watching; per-repo `.beads/` store (embedded sqlite/dolt or server mode), counts, sync state, default-repo flag
  /providers     — Per-provider cards: quota, auth, CLI binary, modes, parallel
  /deps          — Dependency graph view across the visible bead set
  /modes         — Native operation modes browser (sidebar = providers, main = mode cards with native/synthesized badges)

Global topbar chrome (visible on every view):

- **Brand mark** — three-bead chain logo (first filled with accent).
- **⌘K command palette button** — tooltip `bd`, opens a fuzzy command palette (see §5.9).
- **Formulas** — opens the workflow-formula browser (Speckit-flow, bug-triage, migrate-v3, changelog-gen).
- **Memories** — opens the `bd remember` / `bd prime` agent-memory panel.
- **Dolt chip** — branch + status dot + last-sync; tooltip exposes ahead/behind, writers, server port, pull/push buttons.
- **Repo filter chip** — scopes the visible bead set to one of the attached repos (or All); links to /repos for management.
- **Search** — substring match on title + bead id.
- **+ New bead** — opens the composer.

Only the **/board** view shows the capacity strip and now-playing rail underneath the topbar; the other views replace that block with a page header.

Plus these overlays (any can open over any view):

  Task drawer        — centered modal with **non-blocking backdrop** (dim only, cards stay clickable). Tabs: Overview · Deps · Steps · Activity · Log · Files.
  New bead composer  — modal: title, instructions, type, priority (0..4), chain template, destination column
  Add provider modal — modal: CLI vs SDK vs Direct API, catalog, configure
  Add repo modal     — modal: attach an existing working tree, pick embedded vs server `.beads/`, name + default-repo toggle
  Command palette    — ⌘K fuzzy launcher (composer, view jumps, recent beads, common actions)
  Memories panel     — agent memory keys + values (read/edit/delete)
  Formulas panel     — workflow-template browser; clicking a formula seeds a composer chain
  Mobile preview     — Tweaks-panel toggle, not a route; renders a phone bezel with the same data over a dimmed overlay.

---

## 3. Board layout

The five columns are not equal. Each earns its width by the job-to-be-done.

```
┌──────────────┬──────────────────┬───────────────────┬───┐
│ Scheduled    │                  │                   │ D │
│ (top, lean)  │                  │                   │ o │
├──────────────┤  Running         │  Needs review     │ n │
│ Backlog      │  (cockpit)       │  (needs you)      │ e │
│ (mini-cards) │                  │                   │ ▶ │
└──────────────┴──────────────────┴───────────────────┴───┘
```

- **Left column** stacks Scheduled (top, full cards, "next to dispatch") and Backlog (bottom, mini-cards — type-colored left edge + priority digit + 2-line clamped title, no step rail).
- **Center columns** split Running and Needs review equally. Each column has its own collapse chevron and can independently fold to a 44px rail (same `col-collapsed-v` pattern as Done). The whole centre block is also tweakable in the Tweaks panel: **split** (equal), **stack** (Running over Review, horizontal rails collapse), **dominant** (Running 2× Review), or **tabs** (one at a time with a tab bar).
- **Done** collapses to a 44px vertical rail (`col-collapsed-v`) showing the column name, a count badge, and a stack of type-coloured pips (one per closed bead, capped at 18 + an overflow `+N` marker). Click the rail or its chevron to expand to a full column of tombstone rows.

---

## 4. Visual language

### 4.1 Typography
- **Display** — Instrument Serif, italic, generous tracking. Page titles, column headers, drawer titles, mode card headings, tab labels, the vertical "Done" label. Provides editorial warmth.
- **Body** — Geist. Card titles use sans (weight 500) for readability. Mini-card titles use sans at 13–14px.
- **Mono** — JetBrains Mono. IDs, branch names, CLI invocations, file paths, log lines, route patterns.

### 4.2 Palette
Theme is **paper**:

  --paper      #FAF7F0   page background
  --paper-2    #F4F0E7   recessed surfaces
  --paper-3    #EDE7DA   deeper recesses
  --ink        #1B1A17   primary text
  --ink-2..4              progressively lighter grays
  --rule       #DDD5C5   borders
  --accent     tweakable, default Anthropic orange

Status colors:

  --amber  #C58F2F   scheduled / requeued / waiting gates
  --green  #4A8B5F   done / approved / ready
  --violet #7B5BB5   review / spec-system skills / epic type / parent-child deps
  --rose   #B5485A   failures / P0 / bug type / blocking deps
  --live   var(--accent)  (folded into accent — one warm orange, not two)

A faint SVG grain texture sits over the whole page in `multiply` blend mode.

### 4.3 Type → left-edge accent (cards)
Instead of a head glyph, each card shows its type via a 3px left border:

  feature → accent (orange)
  bug     → rose
  task    → ink-3 (neutral)
  epic    → violet
  chore   → ink-4 (mute)

### 4.4 Priority badge (digit-only)
Priority renders as a single colored digit:

  0 (crit)   rose background
  1 (high)   amber background
  2 (norm)   ink-3 background
  3 (low)    transparent + border
  4 (icebox) transparent + border

No "high" / "norm" labels — just the digit. The number maps directly to `bd priority`.

### 4.5 Step rail — literal bead chain
Each step is a small circle ("bead") strung on a hairline thread. Beads are colored by **agent** and styled by **mode**:

  done  →   filled
  pending → outlined
  active  → filled + glow (animated)
  plan    → ringed (hollow center)
  review  → bull's-eye
  yolo    → rose halo

Sub-beads dangle as tinier secondary beads underneath the rail.

### 4.6 Density tokens
Three density presets (Tweaks panel) set `[data-density]` on `<html>`, which adjusts `--card-pad`, `--card-gap`, `--col-gap`:

  cozy     16/14/20px
  medium   12/10/16px  (default)
  compact  8/6/12px

Card titles also retune: `cozy` 17px, `medium` 15px (default), `compact` 14px. Mini-card titles: `cozy` 14px, `medium` 13px, `compact` 12px.

### 4.7 Spacing & sizing constants
Fixed primitives — do not retune per density.

  --radius      6px        all small surfaces (chips, pills, buttons, cards)
  --radius-lg   10px       drawers, modals, large panels
  --shadow-card 0 1px 0 + 0 1px 3px        resting cards
  --shadow-lift 0 4px 10px + 0 12px 28px   drawers, popovers, hovered cards

Board grid columns: `280px` left stack (Scheduled + Backlog); `minmax(0, 1fr)` per centre column; collapsed columns shrink to `44px`; expanded Done is `300px`. Drawer width: `min(640px, 50vw)`. Modal max-width: `760px` (composer), `860px` (add-provider, add-repo). Command palette: `720px × 70vh`. Tap targets follow the platform 44×44px floor; the topbar uses 32px buttons because they're keyboard-and-cursor only.

### 4.8 Motion
Restrained:
- Step bead glow on the active step — 1.8s ease-in-out
- Drawer scale-in — 220ms, ease-out-quint
- Backdrop fade — 180ms
- Now-playing pulse dot — 1.8s

No spring physics, no parallax, no decorative scroll effects.

---

## 5. Component patterns

### 5.1 Bead card (Running / Review / Scheduled)
Top to bottom:

  [pri-digit] [bd-id] [ready-pill?]                [discovered-from? requeued?]
  [Title — sans, weight 500, pretty wrap]
  [Labels — #-prefixed chips, first 3 + overflow]
  [Step rail — bead chain]
  [Review line OR Footer with agent chip]

The type-color left edge is the card's primary visual identifier. Running cards no longer carry their own now-playing line — that lives in the global rail at the top of the board. Same for token bars and sub-bead chips — global surfaces only.

### 5.2 Mini-card (Backlog)
Stripped down: priority digit + bd-id + ready-pill + 2-line clamped sans title + type-color left edge. No step rail, no labels, no foot.

### 5.3 Task drawer (rich ticket view)
Right-anchored side panel (`<aside class="drawer">`). Backdrop is **non-blocking** (`pointer-events: none`, dim only) so clicking another card on the board behind it **switches** the active bead without first closing. `Esc` closes; the tab resets to Overview whenever a new bead is opened.

Header carries: bead id, priority digit, VCS badge, pinned-agent badge (if set), `requeued` / `stale` tags, a close button, the editable title, and a meta row with a column dropdown, created-at, live-tag (running), reviewer comment count, and AC progress mini-bar.

Tabs:

- **Overview** — large editable title (`contentEditable` serif, Enter or blur commits), compact meta strip (priority, type, estimate, vcs, assignee, pinned-agent, requeued, formula, id, date), **AC checklist** (inline todo: type + Enter to add, click to toggle, × to remove), description textarea (commits on blur), labels (`#tag` chips, × to remove, `+ label` inline input with Enter/Esc), gates section (`human` / `timer` / `github` kinds, status-tinted rows), **comments** (real persistence — ⌘/Ctrl + Enter posts to `task.history` as a `comment` event), constitution banner (preview + edit link to /orchestrator).
- **Deps** — four colour-coded sections in order: `blocks` / `blocked-by` (rose, with a self-node spine showing both directions); `external` cross-repo pills; `discovered-from`; `parent-child` sub-beads with one-click open + an `+ Create sub-bead` button. Empty state lists the `bd dep add` CLI hints.
- **Steps** — a VCS toggle row (jj / git buttons, locked once `task.branch` is set), then a card per step (always expanded): agent select (tinted by agent colour), mode select (tinted by mode), CLI hint `<code>`, status pill, per-step skill chip row (with `+ skill` popover and search), prompt textarea (rows=2, expands to 4 on focus, reset-to-default button when overridden), inline loop control (`on fail, loop to <step>` + max-count input). Footer has add-step shortcut buttons: Speckit, Plan, Build, Agent, Review, OpenSpec — each appends a preset step.
- **Activity** — per-bead lifecycle timeline (newest first), tone-coloured left borders matching `EVENT_KINDS[kind].tone`, glyph + label + actor (agent monogram when applicable) + optional note + timestamp.
- **Log** — dark terminal pane streaming the run log. Header shows worktree, branch, token usage, and a `bd attach <id>` button when the bead is running. Lines are coloured by `kind`: `system`, `tool` (blue), `thought` (italic amber), `output` (green), `error` (rose).
- **Files** — sub-tabbed: **Worktree** (file list with status letter, additions/deletions, totals header) + **Diff** (unified diff with hunk / add / del / context line classes).

Footer actions are column-dependent: backlog → `Schedule →`; scheduled → `Dispatch now →`; running → `Pause` + `Send to review →`; review → `Approve` (other states render no footer).

### 5.4 Now-playing rail
Dark terminal strip under the capacity row. One line per running bead:

  ● [CC] bd-a1f2  editing src/auth/revoke.ts             38s  ─── 74%

Three colors per line kind: tool (blue), thought (italic amber), output (green).

### 5.5 Agent chip
Tiny color-coded monogram (CC / GM / OC / CX) plus the full name. On hover or focus, a tooltip pops out with quota, monthly total, rate limit, parallel slots, reset time.

### 5.6 Skill chip row (per-step)
Compact pills, click to remove. `+ skill` opens a small popover with a search input and a list of available skills — clicking adds it as a chip. No bead-level default skills picker — per-step only.

### 5.7 Routes & hydration (Orchestrator)
Table of `.beads/routes.jsonl` rules with pattern, target repo, priority. Inline add-row. Live test input that previews `→ target-repo via pattern`. Hydration block lists sibling repos with ahead-counts and one-click `bd hydrate` buttons.

### 5.8 Lifecycle view
- **Pulse strip** — 7 tinted stat cards: opened today, ready to dispatch, in flight, needs review, closed today, blocked, requeued.
- **Ready-to-dispatch** — `bd ready` filtered list, priority-sorted. Each row carries a one-click Dispatch button + suggested agent (respects `pinnedAgent` first, then capacity-based suggestion).
- **Activity timeline** — flat chronological feed across all beads, All / Today toggle.

### 5.9 Command palette (⌘K)
Centered floating panel (~720px wide, height-capped at 70vh) with a backdrop scrim. It's a **`bd` CLI command browser**, not a navigation launcher — it lists every `bd` subcommand grouped into 11 categories (Issues, Dependencies, Workflow, Formulas & Gates, Memory, Search & Labels, VCS & Worktree, Sync & Dolt, Integrations, Multi-agent, Admin & Setup). Each row shows the command (`bd create`), its argstring (`"Title" -t <type> -p <pri>`), and a one-line description. Top: search input + esc kbd. Middle: a horizontally-scrolling category filter (`All` + 11 categories). Below: scrollable list with category headers. Footer: `↑↓ navigate / ↵ select / esc close` + total count. Arrow keys move selection, hover sets selection, Enter fires.

A small set of entries carry an `action` field (`composer`, `drawer`, `lifecycle`) that the palette dispatches to the host via `onAction(action, item)` — e.g. selecting `bd create` opens the New Bead composer; `bd ready` jumps to /lifecycle; `bd show` opens the top-of-list drawer. The rest are reference-only (the palette is a discoverable index of the CLI surface).

### 5.10 Dolt chip (topbar)
A compact pill: `§` glyph + status dot (clean / ahead / behind / diverged / syncing) + branch + last-sync. Hover/focus opens a tooltip showing remote, status, ahead/behind, active writers, server port, and `bd dolt pull` / `bd dolt push` buttons. Visible on every view.

### 5.11 Repo filter chip (topbar)
Scopes the bead set to one of the attached repos (or **All**). Shows the active repo name + tiny count of in-scope beads; opens a menu listing all repos with their counts and a *Manage* link to /repos. The chip lives between the Dolt chip and the search input.

### 5.12 Repos view
Card per attached working tree. Each card surfaces: name, path, vcs (jj/git) + branch, beads mode (embedded sqlite/dolt file vs server on a port), schema version, last-write time, status dot, per-column bead counts, and a *Set as default* / *Filter board* action pair. The view also exposes an **+ Attach repo** button that opens the AddRepoModal.

### 5.13 Deps view
Level-based dependency graph. Edges are extracted from `task.blocks` (hard, rose, solid) and `task.discoveredFrom` (soft, amber, dashed). Connected beads are arranged into columns by iterative topo propagation; within each level, nodes sort by priority. Bezier connectors join one level to the next (4px arrowheads at the target end). Hover dims unrelated nodes and edges to 10% opacity; clicking opens the drawer. A separate **Independent** grid below catches beads with no edges. A small legend strip across the top names the edge types and the column tints.

### 5.14 Memories & Formulas panels
Two centred floating panels surfaced from the topbar (matching the command-palette chrome — backdrop scrim + centred surface + close button):
- **Memories** — `bd remember` key/value pairs. Search input + count, scrollable list (each row: code-styled key + relative timestamp + agent monogram (or human actor) + `×` remove), a two-input compose row (key + value textarea, ⌘/Ctrl + Enter to commit) with a live `bd remember "key" "value"` CLI hint, and a footer button `bd prime — load all memories into next dispatch`.
- **Formulas** — sidebar lists every formula (`speckit-flow`, `bug-triage`, `migrate-v3`, `changelog-gen`). Main pane shows: title + description, **Step chain** (agent monogram + mode + skill chips + description per step, with a loop tag where a step loops back), **Gates** (`human` / `timer` / `github` after step N), **Variables** (`$SPEC_PATH`, `$FROM_TAG`, …), and a `bd cook <formula> --title "…" $VAR=…` CLI block. Clicking a formula in the New Bead composer's chain-template grid pre-fills the step list with that formula's chain.

---

## 6. Interaction notes

### 6.1 Drag and reorder
Cards are HTML5-draggable (`draggable={true}`). Drop targets:
- **A column** — appends to the end on drop.
- **Above / below a specific card** — the card-level `onDragOver` records `{columnId, beforeId, position}` based on whether the pointer is in the top or bottom half of the hovered card. A 2px accent-coloured bar renders as `drop-above` / `drop-below` on the target card.
- A `dragend` listener clears all drag state defensively (covers ESC-cancel).

`onMove(id, columnId, beforeId?, position?)` is the host's contract. The host re-orders the in-memory `tasks` array directly; the backend persists via `POST /api/v1/beads/{id}/move`.

### 6.2 Keyboard
- `Esc` closes the drawer.
- AC items: type + Enter to add new criterion.
- Labels: + button → text input → Enter to commit, Esc to cancel.
- Title: click the contenteditable title to edit; Enter or blur commits.
- Comments: ⌘/Ctrl + Enter to post.

### 6.3 Card click while drawer is open
Because the backdrop is non-blocking, clicking any card on the board **switches** the active bead. No close-and-reopen dance.

### 6.4 Sub-bead click
See §6.8 (Sub-bead drawer synthesis) for the full mechanism.

### 6.5 Tweaks
See §6.9 (Tweaks persistence contract). Currently exposed: `accent`, `density`, `centerLayout`, `mobilePreview`.

### 6.6 Command palette
⌘/Ctrl + K toggles the command palette (see §5.9 for the surface). Arrow keys navigate, Enter fires, `Esc` closes. The palette is global — it works from any view, including when the drawer is open.

### 6.7 Repo scope
The topbar's repo filter chip scopes the board, lifecycle, and deps views to one repo (or All). The chip's *Manage* affordance jumps to /repos. Switching repos via a /repos card also flips the filter chip and routes to /board so the user lands on the new scope's work.

### 6.8 Sub-bead drawer synthesis
Sub-beads don't carry their own full task rows. When the user clicks a sub-bead id (in the parent's Deps tab, the activity feed, etc.), `findTaskAny(id)` in `app.jsx` looks the id up across every `task.subBeads[]` array and, if found, synthesises a viewable task object that inherits the parent's type / priority / vcs / labels, derives `column` from the sub-bead's status, builds a single-step chain, and marks `_isSubBead: true`. The drawer renders normally; the backend should still treat this id as a first-class bead at the API layer (it is, in `Bead.ParentID`).

### 6.9 Tweaks persistence contract
Tweaks state lives in `TWEAK_DEFAULTS` (the `EDITMODE-BEGIN`/`END` block in `Muster.html`) and is mirrored by `useTweaks({...})` in `app.jsx`. Both lists of defaults must stay in lockstep — the host parses the JSON block as authoritative and merges edits back to disk via `__edit_mode_set_keys`. Currently exposed tweaks: `accent` (curated swatch list), `density`, `centerLayout`, `mobilePreview`.

---

## 7. Non-goals and deferred decisions

- **Realtime collaboration.** Single-user prototype. No presence, no live cursors, no per-actor conflict resolution.
- **Full markdown editing** for descriptions / constitution. Plain textareas for now — monospace, no preview pane.
- **Cost analytics** beyond per-provider daily/monthly. No usage-by-bead/agent/time charts.
- **Auto-merge sub-beads to parent.** Out of scope for v1. Each sub-bead has its own worktree; merging is manual or a dedicated step in the parent's chain.
- **Verifying non-Claude provider CLI flags.** Gemini, OpenCode, Codex non-default modes are marked synthesised; their actual native flags need verification (tracked as spec.md §15 open question 1).
- **Mobile composing.** The mobile preview is read-only — swipe between columns, open the drawer, that's it. No new-bead / dispatch from the phone view.
- **Free-form colour picker.** Accent is a curated swatch list. Free colour picking is out of scope.

---

## 8. File map

  Muster.html         shell, fonts, edit-mode defaults (TWEAK_DEFAULTS), script tags
  styles.css          all CSS (tokens + components)
  data.jsx            window.MUSTER_DATA — seed AGENTS (modes carry native flag + workflowStage; _stageAliases populated post-load), SKILLS + SKILL_CATEGORIES + SKILL_SOURCES, TYPES, PRIORITIES, COLUMNS, VCS_OPTIONS, FORMULAS, ROUTES, HYDRATE_REPOS, REPOS, DOLT, EVENT_KINDS, TASKS (with history/acceptance/comments/vcs/pinnedAgent/externalDeps/repo), modeMeta/typeMeta/priMeta/suggestAgent helpers
  app.jsx             top-level <App>, view routing, topbar, CapacityStrip, NowPlayingRail, DoltChip, MobilePreview, dispatch + move + update handlers, sub-bead synthesis (findTaskAny), Tweaks panel mount
  kanban.jsx          KanbanBoard, Column, Card, MiniCard, DoneRow, DoneRail, StepRail, AgentChip, ModeDot, TokenBar, PriBadge, ReadyDot, GateChip, fmtTokens/fmtQuota/fmtElapsed
  drawer.jsx          TaskDrawer + OverviewTab, DepsTab, StepsTab (StepCard + SkillChipRow + SkillPicker), ActivityTab, RunLogTab, FilesTab, AcceptanceCriteria, CommentThread, GatesSection, LabelAddInput
  lifecycle.jsx       LifecycleView (pulse strip + ready-to-dispatch + activity feed), bdReady predicate
  composer.jsx        NewBeadComposer (chain templates: speckit-flow / plan-build / autonomous / review-only)
  add-provider.jsx    AddProviderModal (CLI catalog + Direct-API catalog), keychain note for API keys
  repos.jsx           ReposView, RepoCard, AddRepoModal (with mock probe), RepoFilterChip (topbar), RepoGlyph SVGs
  depgraph.jsx        DepGraph — level-based dependency layout (blocks + discovered-from edges, independent grid below)
  command-palette.jsx CommandPalette (BD_COMMANDS + CMD_CATEGORIES), MemoriesPanel, FormulasPanel (FORMULA_DETAILS)
  orchestrator.jsx    OrchestratorView (constitution + capacity + failure policy + RoutesSection with hydration), ProvidersView, ModesView
  tweaks-panel.jsx    starter tweaks shell — TweaksPanel + Tweak* controls (Slider/Toggle/Radio/Select/Text/Number/Color/Button) + useTweaks hook

---

## 9. Accessibility & keyboard

- **Focus.** Every interactive surface is keyboard-reachable. The drawer, command palette, and modals trap focus while open and restore on close. The non-blocking drawer is the lone exception: by design it does not trap focus, so Tab continues to the board behind it (clicking a card switches the active bead).
- **Visible focus.** Default browser focus rings are kept; the design intentionally does not suppress them. Custom focusable elements (`tabIndex={0}` on agent chips, dolt chip) receive the same outline.
- **Roles & labels.** All non-button clickables that act as buttons carry `role="button"` and `aria-label`. The drawer is `<aside>` with a labelled close (`aria-label="Close"`). Tooltips use `role="tooltip"`.
- **Drag fallback.** Drag is the primary reorder gesture but every card row also opens via Enter on focus (the wrapping element is `<article onClick>` and stays keyboard-activatable). Moving a bead between columns without drag is via the drawer's column dropdown.
- **Colour contrast.** Body text on `--paper` clears WCAG AA (`--ink` on `--paper` ≈ 14.8:1). Muted text (`--ink-3` ≈ 4.9:1, `--ink-4` ≈ 3.0:1) is reserved for de-emphasised metadata, never primary content. Status tints (amber/violet/rose/green) are always paired with a glyph or label so colour is never the only carrier of meaning.
- **Motion.** Honour `prefers-reduced-motion`. The step-bead glow, pulse dot, and drawer scale-in all collapse to instant transitions when reduced motion is set. (Currently a TODO in `styles.css`; flagged in §12.)
- **Keyboard shortcuts (global)**

  | Key             | Action                                |
  |-----------------|---------------------------------------|
  | ⌘/Ctrl + K      | Toggle command palette                |
  | Esc             | Close topmost overlay (palette → drawer → modal) |
  | ⌘/Ctrl + Enter  | Post comment from the drawer textarea |
  | Enter           | Commit AC item / label / title        |
  | Esc             | Cancel inline AC / label / title edit |
  | ↑ ↓             | Navigate command palette results      |
  | Enter           | Activate selected palette command     |

---

## 10. State catalogue

For every list / detail surface, the production build must implement:

| Surface              | Empty                                              | Loading                                | Error                                                  | Stale / read-only        |
|----------------------|----------------------------------------------------|----------------------------------------|--------------------------------------------------------|--------------------------|
| Board column         | `—` glyph centred                                  | skeleton card (3 rows) with grain      | toast + retry button on the column header              | n/a                      |
| Drawer · Overview AC | "No acceptance criteria yet — `bd update <id> --ac \"…\"`" | n/a (loaded with bead)                 | inline rose banner above the AC list                   | input disabled, read-only checkboxes |
| Drawer · Deps        | "No dependencies yet." + `bd dep add` CLI hints    | n/a                                    | inline rose banner                                     | pills disabled           |
| Drawer · Files       | "No worktree yet — task hasn't started running."   | spinner + "loading diff…"              | "Failed to read worktree" + retry                      | n/a                      |
| Drawer · Log         | "No log yet."                                      | live cursor (when running)             | "stream dropped — reconnecting…" toast                 | "agent disconnected" tag |
| Lifecycle · Ready    | "Queue is clear — everything ready has been dispatched." | skeleton row (3)                       | rose banner on the panel                               | n/a                      |
| Lifecycle · Activity | "Nothing recent."                                  | skeleton row (5)                       | rose banner on the panel                               | n/a                      |
| Providers            | "No providers configured — `+ Add another provider`." | skeleton card                          | per-card auth/expired/error pill                       | offline tag              |
| Repos                | "No repos attached — `+ Add a repo`."              | per-card scanning spinner              | `error` status dot + note row                          | `detached` status        |
| Add repo · Probe     | "Paste a path or pick one above"                   | three-line CLI-style "probing…" rows   | "Probe failed." + reason                               | n/a                      |
| Command palette      | "No commands match \"<q>\""                        | n/a                                    | n/a                                                    | n/a                      |
| Deps graph           | "No beads to graph."                               | n/a                                    | n/a                                                    | n/a                      |

A **stale** badge appears on the drawer header when `task.lastActivity` resolves to >2 days ago (current heuristic checks `Fri / Thu / Wed` substrings; replace with real elapsed-ms once timestamps are RFC3339 from the server).

---

## 11. Mock → backend handover map

The UI ships with seed data in `data.jsx` and a handful of mocked surfaces. This table is the canonical "what to wire up first" reference for the backend implementor.

| UI code                                  | Mock today                                              | Real source (spec.md endpoint)                                            |
|------------------------------------------|---------------------------------------------------------|---------------------------------------------------------------------------|
| `window.MUSTER_DATA.TASKS`               | hand-authored array                                     | `GET /api/v1/beads` + WS `bead.updated` / `bead.moved` events             |
| `app.jsx` `onMove`                       | in-memory array splice                                  | `POST /api/v1/beads/{id}/move` (returns updated bead)                     |
| `app.jsx` `onDispatch`                   | local history append + step flip                        | `POST /api/v1/beads/{id}/dispatch` (server sets `claimed` + `started`)    |
| `drawer.jsx` `RunLogTab` interval-pump   | timed `setInterval` that injects fake lines             | `GET /api/v1/beads/{id}/runlog?since=` + WS `runlog.line`                 |
| `drawer.jsx` `FilesTab`                  | static `task.files` + `task.diffPreview`                | `GET /api/v1/beads/{id}/worktree` + `GET /api/v1/beads/{id}/diff`         |
| `drawer.jsx` `CommentThread.submit`      | in-memory history append                                | `POST /api/v1/beads/{id}/comments` (returns the persisted event)          |
| `orchestrator.jsx` `RoutesSection`       | inline state + no persistence                           | `GET/PUT/POST/DELETE /api/v1/routes` + `POST /api/v1/routes/test`         |
| `orchestrator.jsx` hydrate buttons       | `setTimeout(1800)` no-op                                | `POST /api/v1/hydrate` (with optional `dry-run`)                          |
| `orchestrator.jsx` capacity ± buttons    | local state edit                                        | `PATCH /api/v1/providers/{id}` (`parallel_max`)                           |
| `repos.jsx` `mockProbe`                  | path-string heuristic, 700ms delay                      | `POST /api/v1/repos/probe` (returns the parsed `.beads/` config + counts) |
| `repos.jsx` `AddRepoModal` submit        | unshift into `window.MUSTER_DATA.REPOS`                 | `POST /api/v1/repos` (server attaches + scans, emits `repo.added`)        |
| `add-provider.jsx` `submit`              | calls `onAdd` no-op                                     | `POST /api/v1/providers` + interactive `POST /api/v1/providers/{id}/login`|
| `command-palette.jsx` `BD_COMMANDS`      | hard-coded command catalogue                            | static — the palette is intentionally a documentation surface for `bd`    |
| `command-palette.jsx` `MemoriesPanel`    | `SAMPLE_MEMORIES` + local state                         | `GET / POST / DELETE /api/v1/memories` (per repo)                         |
| `command-palette.jsx` `FormulasPanel`    | `FORMULA_DETAILS` inline                                | `GET /api/v1/formulas` + `GET /api/v1/formulas/{id}`                      |
| `app.jsx` `DoltChip`                     | static `DOLT` object                                    | `GET /api/v1/orchestrator/status` + WS `dolt.tick`                        |
| `app.jsx` `CapacityStrip`                | derived from seed CAPACITY array                        | `GET /api/v1/orchestrator/capacity` + WS `capacity.changed`               |
| `app.jsx` `NowPlayingRail`               | reads `task.nowPlaying` written from seed data          | last `runlog.line` per running bead, kind-tagged                          |

**Cut order.** Wire (1) beads CRUD, (2) WS bead/runlog events, (3) move/dispatch, (4) worktree/diff, (5) providers + capacity, (6) routes/hydrate, (7) memories, (8) repos probe + attach. Until each is live, the UI keeps its mocked path — the gating predicate is "does this UI element write through `onUpdate` / `onMove` / `onDispatch`?" If yes, those handlers are the single seam to swap for `fetch()`.

---

## 12. Production-readiness checklist

Outstanding items before shipping the UI:

- [ ] Replace relative-string timestamps (`'Mon 09:14'`, `'just now'`) with `Intl.RelativeTimeFormat` over real RFC3339 input. Lifecycle's ordering heuristic in `lifecycle.jsx` (the `Fri/Sat/Sun/Mon` score map) goes away with this.
- [ ] Honour `prefers-reduced-motion` for the step-bead glow, pulse dot, drawer scale-in, and now-playing pulse.
- [ ] Add an error boundary at the `<App>` root that surfaces caught errors as a banner instead of blanking the page.
- [ ] Replace `mockProbe` in `repos.jsx` with the real `POST /api/v1/repos/probe`.
- [ ] Replace `RunLogTab`'s fake `setInterval` stream with the real WS-driven runlog.
- [ ] Pull `BD_COMMANDS` out of `command-palette.jsx` and source from `GET /api/v1/cli/commands` so the palette stays in sync with the installed `bd` version.
- [ ] Verify Gemini / OpenCode / Codex CLI flags against upstream and demote `native: false` where appropriate in `data.jsx`.
- [ ] Add a `data-testid` on every drag handle, drop indicator, dispatch button, and tab so E2E can target without leaning on text content.
- [ ] Confirm focus restoration when closing the drawer with Esc (the trigger card should re-receive focus).
- [ ] Confirm the topbar collapses correctly below 960px viewports (the search input + RepoFilterChip currently overflow).
