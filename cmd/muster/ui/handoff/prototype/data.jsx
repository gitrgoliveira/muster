// Sample data for Muster — Beads-native model.
//
// Architecture (per https://gastownhall.github.io/beads/):
//   - Issue ID: hash-based (bd-a1b2) or hierarchical (bd-a1b2.1)
//   - Type:     bug · feature · task · epic · chore
//   - Priority: 0..4  (0 = critical, 4 = backlog)
//   - Status:   open · in_progress · closed   (mapped onto our 5 columns)
//   - Labels:   flat tags
//   - Deps:     blocks · parent-child · discovered-from · related
//   - Formulas: declarative workflow templates (TOML)
//   - Molecules: parent-child work graphs
//   - Gates:    async coordination primitives (human · timer · GitHub)
//
// Agents are LLM providers; modes are per-provider; skills compose.

const AGENTS = [
  {
    id: 'claude', name: 'Claude Code', mono: 'CC', color: '#D97757', parallel: 3,
    kind: 'cli',
    quota: { used: 4.82,  limit: 20.00, unit: '$', window: 'today',      resetIn: '8h' },
    monthly: { used: 78.4, limit: 200.0, unit: '$' },
    plan: 'Claude Max',
    rateLimit: '5h reset window',
    binary: '/opt/homebrew/bin/claude',
    version: '0.39.2',
    auth: { status: 'logged-in', as: 'you@yours.dev' },
    // Real Claude Code modes are about PERMISSIONS, not workflow stages.
    // Workflow stages (plan/build/review) are Muster concepts that map to these flags.
    modes: [
      { id: 'plan',        name: 'Plan',         desc: 'Read-only. No file writes. Native plan mode.',         cli: 'claude --permission-mode plan',              icon: '▤', native: true,  workflowStage: 'plan'   },
      { id: 'acceptEdits', name: 'Accept-edits', desc: 'Auto-accept file edits; prompt for shell.',            cli: 'claude --permission-mode acceptEdits',       icon: '▣', native: true,  workflowStage: 'build'  },
      { id: 'default',     name: 'Default',      desc: 'Interactive. Prompts before each write & shell call.', cli: 'claude',                                     icon: '◉', native: true,  workflowStage: 'agent'  },
      { id: 'bypass',      name: 'Bypass',       desc: 'Skip all permission prompts. Use with care.',          cli: 'claude --permission-mode bypassPermissions', icon: '◈', native: true,  workflowStage: 'yolo'   },
      { id: 'review',      name: 'Review',       desc: 'Muster-synthesized: --permission-mode plan + review system prompt.', cli: 'claude --permission-mode plan',  icon: '◐', native: false, workflowStage: 'review' },
    ],
  },
  {
    id: 'gemini', name: 'Gemini CLI', mono: 'GM', color: '#3B82F6', parallel: 2,
    kind: 'cli',
    quota: { used: 142_000, limit: 1_000_000, unit: 'tok', window: 'today',   resetIn: '8h' },
    monthly: { used: 2.4e6, limit: 30e6, unit: 'tok' },
    plan: 'Gemini Code Assist',
    rateLimit: '60 RPM · 1k RPD',
    binary: '/opt/homebrew/bin/gemini',
    version: '0.4.1',
    auth: { status: 'logged-in', as: 'you@yours.dev' },
    // 'gemini' default + --yolo are real flags. Other workflow stages synthesized.
    modes: [
      { id: 'default', name: 'Default', desc: 'Interactive agent loop.',                              cli: 'gemini',                    icon: '◉', native: true,  workflowStage: 'agent'  },
      { id: 'yolo',    name: 'YOLO',    desc: 'Auto-approve every tool call.',                        cli: 'gemini --yolo',             icon: '◈', native: true,  workflowStage: 'yolo'   },
      { id: 'plan',    name: 'Plan',    desc: 'Muster-synthesized via plan system prompt; no native flag.', cli: 'gemini  # + plan prompt',  icon: '▤', native: false, workflowStage: 'plan'   },
      { id: 'review',  name: 'Review',  desc: 'Muster-synthesized: read-only diff critique prompt.',   cli: 'gemini  # + review prompt', icon: '◐', native: false, workflowStage: 'review' },
    ],
  },
  {
    id: 'opencode', name: 'OpenCode', mono: 'OC', color: '#10B981', parallel: 2,
    kind: 'sdk',
    quota: { used: 0, limit: 0, unit: '$', window: 'today', resetIn: '—', selfHosted: true },
    monthly: { used: 0, limit: 0, unit: '$', selfHosted: true },
    plan: 'BYO model · routes to local Ollama',
    rateLimit: 'no upstream cap',
    sdkPackage: 'opencode-sdk',
    sdkVersion: '0.7.0',
    sdkLinked: true,
    auth: { status: 'no-auth-needed', as: 'local' },
    // OpenCode SDK — workflow stages are session input shaping (synthesized).
    modes: [
      { id: 'default', name: 'Default', desc: 'SDK session with default config.',                  cli: 'sdk.session.run()',        icon: '◉', native: true,  workflowStage: 'agent' },
      { id: 'plan',    name: 'Plan',    desc: 'Muster-synthesized via plan-shaped session input.',  cli: 'sdk.session.run({plan})',  icon: '▤', native: false, workflowStage: 'plan'  },
      { id: 'build',   name: 'Build',   desc: 'Muster-synthesized via build-shaped session input.', cli: 'sdk.session.run({build})', icon: '▣', native: false, workflowStage: 'build' },
    ],
  },
  {
    id: 'codex', name: 'Codex', mono: 'CX', color: '#8B5CF6', parallel: 1,
    kind: 'cli',
    quota: { used: 18_400, limit: 50_000, unit: 'msg', window: 'this week', resetIn: '4d' },
    monthly: { used: 62_300, limit: 250_000, unit: 'msg' },
    plan: 'ChatGPT Pro',
    rateLimit: 'weekly reset',
    binary: '/opt/homebrew/bin/codex',
    version: '0.42.0',
    auth: { status: 'logged-in', as: 'you@yours.dev' },
    // Codex CLI flags need verification; non-default workflow stages synthesized.
    modes: [
      { id: 'default', name: 'Default', desc: 'Interactive Codex session.',                       cli: 'codex',                     icon: '◉', native: true,  workflowStage: 'agent'  },
      { id: 'plan',    name: 'Plan',    desc: 'Muster-synthesized: plan-shaped system prompt.',    cli: 'codex  # + plan prompt',    icon: '▤', native: false, workflowStage: 'plan'   },
      { id: 'apply',   name: 'Apply',   desc: 'Muster-synthesized: apply-patches system prompt.',  cli: 'codex  # + apply prompt',   icon: '▣', native: false, workflowStage: 'build'  },
      { id: 'review',  name: 'Review',  desc: 'Muster-synthesized: diff-critique system prompt.',  cli: 'codex  # + review prompt',  icon: '◐', native: false, workflowStage: 'review' },
    ],
  },
];

const ALL_MODES = ['plan', 'build', 'agent', 'yolo', 'apply', 'review'];

// Post-process: for each agent, expose workflow-stage aliases pointing at the
// real mode for that stage. Lets existing step refs like `mode:'agent'` work
// even though the canonical mode is now `default`.
AGENTS.forEach(a => {
  const aliases = {};
  for (const m of a.modes) {
    if (m.workflowStage && !a.modes.find(x => x.id === m.workflowStage)) {
      aliases[m.workflowStage] = m;
    }
  }
  a._stageAliases = aliases;
});

// Issue types (per Beads).
const TYPES = [
  { id: 'feature', label: 'feat',  glyph: '✦', tint: '--accent' },
  { id: 'bug',     label: 'bug',   glyph: '✕', tint: '--rose' },
  { id: 'task',    label: 'task',  glyph: '☐', tint: '--ink-3' },
  { id: 'epic',    label: 'epic',  glyph: '⌘', tint: '--violet' },
  { id: 'chore',   label: 'chore', glyph: '⊹', tint: '--ink-4' },
];

// Priority (0..4). Beads-native numeric scale.
const PRIORITIES = [
  { n: 0, label: 'crit',  tone: 'rose'   },
  { n: 1, label: 'high',  tone: 'amber'  },
  { n: 2, label: 'norm',  tone: 'ink'    },
  { n: 3, label: 'low',   tone: 'mute'   },
  { n: 4, label: 'icebox',tone: 'mute'   },
];

const SKILL_SOURCES = [
  { id: 'project', label: 'project', path: './.agents/skills/' },
  { id: 'user',    label: 'user',    path: '~/.config/agents/skills/' },
  { id: 'builtin', label: 'builtin', path: 'muster://skills/' },
  { id: 'url',     label: 'url',     path: 'https://' },
];

const SKILLS = [
  { id: 'speckit',       name: 'Speckit',             desc: 'Spec-driven dev — fleet + verify extensions.',           cat: 'spec',   icon: '◆', source: 'project', path: './.agents/skills/speckit' },
  { id: 'openspec',      name: 'OpenSpec',            desc: 'OpenAPI-style contract validation.',                       cat: 'spec',   icon: '◇', source: 'project', path: './.agents/skills/openspec' },
  { id: 'beads-memory',  name: 'Beads memory',        desc: 'Persistent memory via bd remember + bd prime.',            cat: 'spec',   icon: '◦', source: 'builtin', path: 'muster://skills/beads-memory' },
  { id: 'repo-grep',     name: 'Repo search',         desc: 'Ripgrep + ast-grep over the worktree.',                    cat: 'code',   icon: '⌕', source: 'builtin', path: 'muster://skills/repo-grep' },
  { id: 'run-tests',     name: 'Test runner',         desc: 'pnpm test / pytest with watchers and coverage.',           cat: 'code',   icon: '✓', source: 'project', path: './.agents/skills/run-tests' },
  { id: 'sql',           name: 'SQL & migrations',    desc: 'Read schemas, draft & run reversible migrations.',         cat: 'code',   icon: '⛁', source: 'user',    path: '~/.config/agents/skills/sql' },
  { id: 'browser',       name: 'Browser automation',  desc: 'Playwright — open, click, screenshot, scrape.',            cat: 'web',    icon: '◰', source: 'user',    path: '~/.config/agents/skills/browser' },
  { id: 'web-search',    name: 'Web search',          desc: 'Live web queries with citations.',                         cat: 'web',    icon: '◎', source: 'builtin', path: 'muster://skills/web-search' },
  { id: 'web-fetch',     name: 'Web fetch',           desc: 'Pull a URL into context as markdown.',                     cat: 'web',    icon: '↧', source: 'builtin', path: 'muster://skills/web-fetch' },
  { id: 'pdf-reader',    name: 'PDF reader',          desc: 'OCR + structure extraction.',                              cat: 'doc',    icon: '◫', source: 'url',     path: 'https://agentskills.io/registry/pdf-reader@1.4.0' },
  { id: 'image-gen',     name: 'Image generation',    desc: 'Generate or edit images.',                                 cat: 'doc',    icon: '◧', source: 'url',     path: 'https://agentskills.io/registry/image-gen@2.0.1' },
  { id: 'figma-read',    name: 'Figma',               desc: 'Read frames, tokens, components by URL.',                  cat: 'design', icon: '✦', source: 'url',     path: 'https://agentskills.io/registry/figma@0.9.2' },
  { id: 'linear',        name: 'Linear',              desc: 'Read + write issues, comments, cycles.',                   cat: 'pm',     icon: '◇', source: 'user',    path: '~/.config/agents/skills/linear' },
  { id: 'slack-notify',  name: 'Slack notify',        desc: 'Post status updates to channels.',                         cat: 'pm',     icon: '#',  source: 'user',    path: '~/.config/agents/skills/slack-notify' },
  { id: 'aws',           name: 'AWS console',         desc: 'Read-only by default; explicit IAM for writes.',           cat: 'infra',  icon: '☁', source: 'user',    path: '~/.config/agents/skills/aws' },
  { id: 'vercel',        name: 'Vercel',              desc: 'Inspect deploys, env vars, preview URLs.',                 cat: 'infra',  icon: '▲', source: 'project', path: './.agents/skills/vercel' },
  { id: 'sentry',        name: 'Sentry',              desc: 'Query errors, link issues to beads.',                      cat: 'infra',  icon: '◢', source: 'user',    path: '~/.config/agents/skills/sentry' },
];

const SKILL_CATEGORIES = [
  { id: 'spec',   name: 'Spec & VCS' },
  { id: 'code',   name: 'Code' },
  { id: 'web',    name: 'Web' },
  { id: 'doc',    name: 'Documents' },
  { id: 'design', name: 'Design' },
  { id: 'pm',     name: 'PM' },
  { id: 'infra',  name: 'Infra' },
];

// VCS backends — per Bead.VCS in spec.md. Not skills.
// jj = Jujutsu (jj clone --colocate, jj describe, jj push)
// git = standard git worktree add + commit + push
const VCS_OPTIONS = [
  { id: 'jj',  label: 'Jujutsu', desc: 'jj clone --colocate · jj describe · jj push', icon: '⌥' },
  { id: 'git', label: 'Git',     desc: 'git worktree add · git commit · git push',      icon: '⎇' },
];

// Routes — .beads/routes.jsonl (multi-agent routing)
const ROUTES = [
  { pattern: 'frontend/**', target: 'frontend-repo', priority: 10 },
  { pattern: 'backend/**',  target: 'backend-repo',  priority: 10 },
  { pattern: 'billing/**',  target: 'billing-repo',  priority: 8  },
  { pattern: 'docs/**',     target: 'docs-repo',     priority: 5  },
  { pattern: '*',           target: 'main-repo',     priority: 0  },
];

// Hydration repos — bd hydrate --from <repo>
const HYDRATE_REPOS = [
  { id: 'frontend-repo', ahead: 3, lastSync: '4m ago'  },
  { id: 'backend-repo',  ahead: 0, lastSync: '12m ago' },
  { id: 'billing-repo',  ahead: 1, lastSync: '1h ago'  },
];

// Working trees Muster has attached to. Each one has a .beads/ store —
// either embedded (direct sqlite/dolt file) or server (dolt-sql-server on a
// port). Bead origin is keyed on this id.
const REPOS = [
  {
    id: 'main', name: 'octane', path: '~/code/octane',
    vcs: 'jj', vcsBranch: 'main',
    beadsMode: 'embedded',
    dbPath: '.beads/beads.db', dbSize: '2.4 MB',
    detected: { beadsVersion: '0.9.1', schemaVersion: 4, lastWrite: '2m ago' },
    status: 'connected', lastSync: '2m ago',
    counts: { backlog: 1, scheduled: 2, running: 2, review: 2, done: 1 },
    isDefault: true,
  },
  {
    id: 'frontend-repo', name: 'octane-web', path: '~/code/octane-web',
    vcs: 'git', vcsBranch: 'main',
    beadsMode: 'embedded',
    dbPath: '.beads/beads.db', dbSize: '1.1 MB',
    detected: { beadsVersion: '0.9.0', schemaVersion: 4, lastWrite: '8m ago' },
    status: 'connected', lastSync: '4m ago',
    counts: { backlog: 2, scheduled: 1, running: 0, review: 0, done: 0 },
  },
  {
    id: 'backend-repo', name: 'octane-api', path: '~/code/octane-api',
    vcs: 'jj', vcsBranch: 'release-3.2',
    beadsMode: 'server',
    dbHost: '127.0.0.1', dbPort: 3306, dbName: 'octane_api_beads',
    detected: { beadsVersion: '0.9.1', schemaVersion: 4, lastWrite: '12m ago' },
    status: 'connected', lastSync: '12m ago',
    counts: { backlog: 1, scheduled: 0, running: 0, review: 0, done: 2 },
  },
  {
    id: 'billing-repo', name: 'octane-billing', path: '~/code/octane-billing',
    vcs: 'jj', vcsBranch: 'main',
    beadsMode: 'embedded',
    dbPath: '.beads/beads.db', dbSize: '482 KB',
    detected: { beadsVersion: '0.8.4', schemaVersion: 3, lastWrite: '1h ago' },
    status: 'detached', lastSync: '1h ago',
    counts: { backlog: 0, scheduled: 1, running: 0, review: 0, done: 0 },
    note: 'schema v3 — bd migrate available',
  },
];

// Tag each TASK with its source repo. Anything not listed lives in main.
const REPO_OF_BEAD = {
  'bd-9aa1': 'frontend-repo',
  'bd-2d55': 'frontend-repo',
  'bd-4f12': 'frontend-repo',
  'bd-7c0d': 'backend-repo',
  'bd-7e21': 'backend-repo',
  'bd-4a11': 'backend-repo',
  'bd-6a02': 'backend-repo',
  'bd-8b44': 'billing-repo',
};

// Formulas — declarative workflow templates (TOML in real Beads).
const FORMULAS = [
  { id: 'speckit-flow',  name: 'speckit-flow',  desc: 'Speckit plan → build → review → commit.' },
  { id: 'bug-triage',    name: 'bug-triage',    desc: 'Repro → root-cause → fix → regression test.' },
  { id: 'migrate-v3',    name: 'migrate-v3',    desc: 'Dual-write window → backfill → cutover.' },
  { id: 'changelog-gen', name: 'changelog-gen', desc: 'Walk bd graph between tags, group by epic.' },
];

// Gates — async coordination primitives.
//   { kind: 'human' | 'timer' | 'github', label, status: 'waiting' | 'passed' | 'failed', meta? }
//
// In Beads, gates pause a formula until satisfied.

const COLUMNS = [
  { id: 'scheduled', name: 'Scheduled to run', tone: 'amber'   },
  { id: 'backlog',   name: 'Backlog',          tone: 'neutral' },
  { id: 'running',   name: 'Running',          tone: 'live'    },
  { id: 'review',    name: 'Needs review',     tone: 'violet'  },
  { id: 'done',      name: 'Done',             tone: 'green'   },
];

function modeMeta(agentId, modeId) {
  const a = AGENTS.find(x => x.id === agentId);
  if (!a) return null;
  // Try direct ID match first
  let m = a.modes.find(m => m.id === modeId);
  if (m) return m;
  // Fall back to workflow-stage alias — supports steps that say mode:'agent' for
  // an agent whose canonical mode is id:'default' with workflowStage:'agent'.
  return a._stageAliases?.[modeId] || null;
}

function typeMeta(id) { return TYPES.find(t => t.id === id) || TYPES[2]; }
function priMeta(n)   { return PRIORITIES[Math.max(0, Math.min(4, n))]; }

// Dolt sync state — surfaced in topbar.
const DOLT = {
  branch: 'main',
  remote: 'origin',
  ahead: 0,
  behind: 0,
  lastSync: '2m ago',
  status: 'clean',       // clean · ahead · behind · diverged · syncing
  server: 'running',
  port: 3306,
  writers: 4,            // active claimers
};

// task shape:
//   { id, title, desc, type, priority(0..4), labels[], column,
//     branch, worktree,
//     deps: { blocks[], blockedBy[], parent?, children[], discoveredFrom?, related[] },
//     ready: bool,
//     formula?: id,
//     gates?: [{kind,label,status,meta?}],
//     skills[], steps[], subBeads[],
//     tokensUsed, tokensBudget,
//     nowPlaying?: { action, since, kind: 'tool'|'thought'|'output' },
//     requeued?, createdAt, log[], files[], diffPreview }

const TASKS = [
  {
    id: 'bd-a1f2',
    title: 'Refactor auth middleware for OAuth refresh tokens',
    desc: 'Existing middleware swallows 401s when the access token is expired. Need a refresh dance with concurrency-safe locking, plus session-scoped revocation.',
    type: 'feature',
    vcs: 'jj',
    priority: 0,
    labels: ['oauth', 'security', 'middleware'],
    column: 'running',
    ready: true,
    branch: 'jj/bd-a1f2-oauth-refresh',
    skills: ['repo-grep', 'run-tests', 'beads-memory'],
    formula: 'speckit-flow',
    blocks: ['bd-c411'],
    blockedBy: [],
    related: ['bd-3e80'],
    subBeads: [
      { id: 'bd-a1f2.1', title: 'Token refresh queue with singleflight lock',     status: 'done',    agent: 'claude' },
      { id: 'bd-a1f2.2', title: 'Session-scoped revocation cascade',              status: 'active',  agent: 'claude' },
      { id: 'bd-a1f2.3', title: 'Migration script for legacy session tokens',     status: 'pending', agent: 'claude' },
      { id: 'bd-a1f2.4', title: 'Backfill audit log for revoked sessions',        status: 'pending', agent: 'gemini' },
    ],
    tokensUsed: 184_320,
    tokensBudget: 250_000,
    createdAt: 'Mon 09:14',
    nowPlaying: { action: 'editing src/auth/revoke.ts', since: 38, kind: 'tool' },
    steps: [
      { agent: 'claude', mode: 'plan',   skills: ['speckit'], status: 'done',   note: 'Spec ratified, 3 acceptance tests' },
      { agent: 'claude', mode: 'plan',   skills: [],          status: 'done',   note: '6 atomic steps planned' },
      { agent: 'claude', mode: 'build',  skills: [],          status: 'active', note: 'Implementing token refresh queue' },
      { agent: 'gemini', mode: 'review', skills: [],          status: 'pending' },
      { agent: 'claude', mode: 'agent',  skills: [], status: 'pending' },
    ],
    log: [
      { t: '09:14:02', kind: 'system', msg: 'Task claimed by Claude Code · worktree wt-a1f2' },
      { t: '09:14:11', kind: 'tool',   msg: 'read_file src/auth/middleware.ts' },
      { t: '09:14:14', kind: 'tool',   msg: 'read_file src/auth/tokens.ts' },
      { t: '09:14:22', kind: 'thought',msg: 'The refresh path needs a singleflight lock so concurrent 401s don\'t each spawn a refresh.' },
      { t: '09:14:31', kind: 'tool',   msg: 'edit_file src/auth/refresh-queue.ts (new)' },
      { t: '09:14:48', kind: 'tool',   msg: 'edit_file src/auth/middleware.ts' },
      { t: '09:15:02', kind: 'tool',   msg: 'shell: pnpm test auth' },
      { t: '09:15:31', kind: 'output', msg: '  ✓ refresh queue dedupes concurrent 401s (412ms)' },
      { t: '09:15:31', kind: 'output', msg: '  ✓ revoked refresh tokens 401 immediately (88ms)' },
      { t: '09:15:31', kind: 'output', msg: '  ✗ session-scoped revocation cascades (timeout)' },
      { t: '09:15:34', kind: 'thought',msg: 'Cascade test times out — revoke is async, need to await flush.' },
      { t: '09:15:39', kind: 'tool',   msg: 'edit_file src/auth/revoke.ts' },
    ],
    files: [
      { path: 'src/auth/middleware.ts',    status: 'M', adds: 24, dels: 11 },
      { path: 'src/auth/refresh-queue.ts', status: 'A', adds: 87, dels:  0 },
      { path: 'src/auth/revoke.ts',        status: 'M', adds:  9, dels:  2 },
      { path: 'src/auth/tokens.ts',        status: 'M', adds:  3, dels:  3 },
      { path: 'test/auth/refresh.spec.ts', status: 'A', adds: 64, dels:  0 },
    ],
    diffPreview: `@@ src/auth/middleware.ts @@
-  if (res.status === 401) {
-    await refresh(token);
-    return retry(req);
-  }
+  if (res.status === 401) {
+    const fresh = await refreshQueue.dedupe(token.sub, () => refresh(token));
+    if (!fresh) throw new SessionRevoked(token.sub);
+    return retry(req, fresh);
+  }`,
  },
  {
    id: 'bd-c411',
    pinnedAgent: 'gemini',
    title: 'Wire Beads dependency graph into changelog generator',
    desc: 'Generate CHANGELOG entries by walking the bead graph between two tags. Group by epic, surface blocked-by chains.',
    type: 'feature',
    vcs: 'jj',
    priority: 1,
    labels: ['changelog', 'release'],
    column: 'running',
    ready: false,
    branch: 'jj/bd-c411-changelog',
    skills: ['repo-grep', 'beads-memory'],
    formula: 'changelog-gen',
    blocks: [],
    blockedBy: ['bd-a1f2'],
    subBeads: [
      { id: 'bd-c411.1', title: 'Walk bd graph topology, group by epic',  status: 'active',  agent: 'gemini' },
      { id: 'bd-c411.2', title: 'Markdown renderer with epic headings',   status: 'pending', agent: 'gemini' },
    ],
    tokensUsed: 42_110,
    tokensBudget: 250_000,
    createdAt: 'Mon 11:02',
    nowPlaying: { action: 'walking bd graph v0.4.0..HEAD · 127 beads', since: 502, kind: 'thought' },
    steps: [
      { agent: 'claude', mode: 'plan',   skills: ['speckit'], status: 'done' },
      { agent: 'gemini', mode: 'plan',   skills: [],          status: 'done' },
      { agent: 'gemini', mode: 'agent',  skills: [],          status: 'active', note: 'Walking bd graph from v0.4.0..HEAD' },
      { agent: 'claude', mode: 'review', skills: [],          status: 'pending' },
      { agent: 'claude', mode: 'agent',  skills: [], status: 'pending' },
    ],
    log: [
      { t: '11:02:14', kind: 'system', msg: 'Task claimed by Gemini CLI · worktree wt-c411' },
      { t: '11:02:33', kind: 'tool',   msg: 'shell: bd export --since v0.4.0 --json' },
      { t: '11:02:41', kind: 'output', msg: '127 beads · 14 epics · 8 chains' },
      { t: '11:02:55', kind: 'thought',msg: 'Group by epic root, then topo-sort within each group.' },
      { t: '11:03:08', kind: 'tool',   msg: 'edit_file scripts/changelog.ts' },
    ],
    files: [
      { path: 'scripts/changelog.ts',     status: 'M', adds: 134, dels: 42 },
      { path: 'scripts/__tests__/changelog.spec.ts', status: 'A', adds: 89, dels: 0 },
    ],
  },
  {
    id: 'bd-7c0d',
    externalDeps: ['external:billing-repo/bd-202', 'external:backend-repo/bd-189'],
    title: 'Migrate legacy invoice schema → v3',
    desc: 'Backfill v3 fields from v2 rows, dual-write window, then cutover.',
    type: 'epic',
    vcs: 'jj',
    priority: 0,
    labels: ['migration', 'billing'],
    column: 'scheduled',
    ready: true,
    branch: null,
    skills: ['sql', 'run-tests', 'sentry'],
    formula: 'migrate-v3',
    blocks: ['bd-9aa1'],
    blockedBy: [],
    children: ['bd-8b44.1', 'bd-8b44.2', 'bd-8b44.3'],
    gates: [
      { kind: 'human', label: '@alice approves dual-write plan', status: 'waiting' },
    ],
    tokensUsed: 0,
    tokensBudget: 400_000,
    createdAt: 'Mon 08:30',
    steps: [
      { agent: 'claude', mode: 'plan',   skills: ['speckit'],  status: 'done' },
      { agent: 'gemini', mode: 'plan',   skills: ['openspec'], status: 'pending' },
      { agent: 'claude', mode: 'plan',   skills: [],           status: 'pending' },
      { agent: 'claude', mode: 'build',  skills: [],           status: 'pending' },
      { agent: 'codex',  mode: 'review', skills: [],           status: 'pending' },
      { agent: 'claude', mode: 'agent',  skills: [],  status: 'pending' },
    ],
  },
  {
    id: 'bd-9aa1',
    pinnedAgent: 'opencode',
    title: 'Add full-text search to /docs route',
    desc: 'Postgres tsvector + GIN index. Snippets with <mark> highlighting.',
    type: 'feature',
    vcs: 'jj',
    priority: 2,
    labels: ['search', 'docs'],
    column: 'scheduled',
    ready: false,
    branch: null,
    skills: ['sql', 'browser', 'run-tests'],
    blocks: [],
    blockedBy: ['bd-7c0d'],
    tokensUsed: 0,
    tokensBudget: 200_000,
    createdAt: 'Mon 10:45',
    steps: [
      { agent: 'opencode', mode: 'plan',   skills: [], status: 'pending' },
      { agent: 'opencode', mode: 'build',  skills: [], status: 'pending' },
      { agent: 'gemini',   mode: 'review', skills: [], status: 'pending' },
    ],
  },
  {
    id: 'bd-8b44',
    title: 'Token budget exhausted on schema migration',
    desc: 'Returned to queue at 92% of budget. Likely needs to be split into 3 sub-beads.',
    type: 'task',
    vcs: 'jj',
    priority: 1,
    labels: ['migration', 'requeue'],
    column: 'scheduled',
    ready: true,
    branch: 'jj/bd-8b44-stuck',
    skills: [],
    blocks: [],
    blockedBy: [],
    subBeads: [
      { id: 'bd-8b44.1', title: 'Split: dual-write window setup',          status: 'pending', agent: 'claude', autoSplit: true },
      { id: 'bd-8b44.2', title: 'Split: backfill v3 fields from v2 rows',  status: 'pending', agent: 'claude', autoSplit: true },
      { id: 'bd-8b44.3', title: 'Split: cutover + drop v2 columns',        status: 'pending', agent: 'claude', autoSplit: true },
    ],
    tokensUsed: 184_000,
    tokensBudget: 200_000,
    requeued: true,
    createdAt: 'Sun 23:40',
    steps: [
      { agent: 'claude', mode: 'plan',  skills: [], status: 'done' },
      { agent: 'claude', mode: 'agent', skills: [], status: 'failed', note: 'Token budget exhausted at 92%' },
    ],
  },
  {
    id: 'bd-3e80',
    pinnedAgent: 'codex',
    title: 'Fix flaky test in payments/checkout.spec.ts',
    desc: 'Race between stripe webhook stub and order-finalize. Add deterministic clock.',
    type: 'bug',
    vcs: 'jj',
    priority: 1,
    labels: ['flake', 'payments', 'test'],
    column: 'review',
    ready: true,
    branch: 'jj/bd-3e80-flaky-checkout',
    skills: ['run-tests', 'sentry'],
    formula: 'bug-triage',
    blocks: [],
    blockedBy: [],
    discoveredFrom: 'bd-a1f2',
    gates: [
      { kind: 'human', label: '2 review comments awaiting reply', status: 'waiting' },
    ],
    tokensUsed: 58_440,
    tokensBudget: 150_000,
    createdAt: 'Sun 22:11',
    reviewer: { agent: 'claude', comments: 2 },
    steps: [
      { agent: 'codex',  mode: 'plan',   skills: [],          status: 'done' },
      { agent: 'codex',  mode: 'apply',  skills: [],          status: 'done' },
      { agent: 'claude', mode: 'review', skills: [],          status: 'active', note: '2 comments, awaiting human' },
      { agent: 'codex',  mode: 'apply',  skills: [], status: 'pending' },
    ],
    files: [
      { path: 'test/payments/checkout.spec.ts', status: 'M', adds: 22, dels: 31 },
      { path: 'test/helpers/clock.ts',          status: 'A', adds: 41, dels: 0 },
    ],
  },
  {
    id: 'bd-d091',
    title: 'Surface stale beads in dashboard',
    desc: 'Beads with no status change in >7 days surface a "stale" pill in the backlog header.',
    type: 'chore',
    vcs: 'git',
    priority: 3,
    labels: ['dashboard', 'hygiene'],
    column: 'review',
    ready: true,
    branch: 'jj/bd-d091-stale-beads',
    skills: [],
    blocks: [],
    blockedBy: [],
    tokensUsed: 22_410,
    tokensBudget: 80_000,
    createdAt: 'Sun 18:02',
    reviewer: { agent: 'gemini', comments: 0 },
    steps: [
      { agent: 'opencode', mode: 'plan',  skills: [], status: 'done' },
      { agent: 'opencode', mode: 'agent', skills: [], status: 'done' },
      { agent: 'gemini',   mode: 'review',skills: [], status: 'active', note: 'no comments — autoclose in 4h' },
    ],
  },
  {
    id: 'bd-b210',
    title: 'Implement audit log for admin actions',
    desc: 'Append-only audit table, RLS by tenant, surface in /admin/audit.',
    type: 'feature',
    vcs: 'jj',
    priority: 2,
    labels: ['admin', 'security'],
    column: 'backlog',
    ready: true,
    branch: null,
    skills: [],
    blocks: [],
    blockedBy: [],
    tokensUsed: 0,
    tokensBudget: 300_000,
    createdAt: 'Mon 07:55',
    steps: [
      { agent: 'claude', mode: 'plan',  skills: ['speckit'], status: 'pending' },
      { agent: 'claude', mode: 'plan',  skills: [],          status: 'pending' },
      { agent: 'claude', mode: 'build', skills: [],          status: 'pending' },
    ],
  },
  {
    id: 'bd-4f12',
    externalDeps: ['external:frontend-repo/bd-441'],
    title: 'Upgrade Tailwind 3 → 4 across packages',
    desc: 'Codemod arbitrary values, replace removed plugins, regenerate tokens.',
    type: 'chore',
    vcs: 'git',
    priority: 3,
    labels: ['deps', 'design-system'],
    column: 'backlog',
    ready: true,
    branch: null,
    skills: [],
    blocks: [],
    blockedBy: [],
    tokensUsed: 0,
    tokensBudget: 200_000,
    createdAt: 'Fri 16:20',
    steps: [
      { agent: 'gemini', mode: 'plan',   skills: [], status: 'pending' },
      { agent: 'gemini', mode: 'agent',  skills: [], status: 'pending' },
      { agent: 'claude', mode: 'review', skills: [], status: 'pending' },
    ],
  },
  {
    id: 'bd-2d55',
    title: 'Mobile share sheet for article reader',
    desc: 'Native share on iOS/Android via Web Share API, fallback popover on desktop.',
    type: 'feature',
    vcs: 'git',
    priority: 3,
    labels: ['mobile', 'reader'],
    column: 'backlog',
    ready: true,
    branch: null,
    skills: ['browser', 'figma-read', 'run-tests'],
    blocks: [],
    blockedBy: [],
    tokensUsed: 0,
    tokensBudget: 100_000,
    createdAt: 'Fri 14:02',
    steps: [
      { agent: 'opencode', mode: 'agent',  skills: [], status: 'pending' },
      { agent: 'claude',   mode: 'review', skills: [], status: 'pending' },
    ],
  },
  {
    id: 'bd-7e21',
    title: 'Reproduce intermittent 502 from /api/feed under load',
    desc: 'Caller reports spiky 502s at ~2k RPM. Likely connection pool exhaustion.',
    type: 'bug',
    vcs: 'jj',
    priority: 1,
    labels: ['perf', 'feed'],
    column: 'backlog',
    ready: false,
    branch: null,
    skills: ['sentry', 'run-tests'],
    blocks: [],
    blockedBy: ['bd-7c0d'],
    discoveredFrom: 'bd-c411',
    tokensUsed: 0,
    tokensBudget: 150_000,
    createdAt: 'Mon 06:11',
    steps: [
      { agent: 'claude', mode: 'plan',   skills: [], status: 'pending' },
      { agent: 'claude', mode: 'agent',  skills: [], status: 'pending' },
    ],
  },
  {
    id: 'bd-5e91',
    title: 'Stream embeddings to Pinecone in batches',
    desc: 'Replace blocking upserts with a 200-row stream, retry with jitter.',
    type: 'task',
    vcs: 'jj',
    priority: 1,
    labels: ['pinecone', 'embeddings'],
    column: 'done',
    ready: true,
    branch: 'jj/bd-5e91-pinecone-stream',
    skills: ['run-tests'],
    blocks: [],
    blockedBy: [],
    tokensUsed: 91_220,
    tokensBudget: 150_000,
    closedAt: 'Thu 17:42',
    createdAt: 'Thu 13:30',
    steps: [
      { agent: 'claude', mode: 'plan',   skills: ['speckit'], status: 'done' },
      { agent: 'claude', mode: 'agent',  skills: [],          status: 'done' },
      { agent: 'codex',  mode: 'review', skills: [],          status: 'done' },
      { agent: 'claude', mode: 'agent',  skills: [], status: 'done' },
    ],
  },
  {
    id: 'bd-6a02',
    title: 'Deprecate /v1 API surface',
    desc: 'Add Sunset headers, log callers, prep migration doc.',
    type: 'chore',
    vcs: 'jj',
    priority: 2,
    labels: ['api', 'sunset'],
    column: 'done',
    ready: true,
    branch: 'jj/bd-6a02-v1-sunset',
    skills: [],
    blocks: [],
    blockedBy: [],
    tokensUsed: 34_990,
    tokensBudget: 120_000,
    closedAt: 'Thu 12:08',
    createdAt: 'Thu 09:10',
    steps: [
      { agent: 'gemini', mode: 'plan',   skills: [], status: 'done' },
      { agent: 'gemini', mode: 'agent',  skills: [], status: 'done' },
      { agent: 'claude', mode: 'review', skills: [], status: 'done' },
    ],
  },
  {
    id: 'bd-4a11',
    title: 'Strip PII from outbound webhooks',
    desc: 'Redact email / phone / address before posting to subscriber endpoints.',
    type: 'task',
    vcs: 'jj',
    priority: 1,
    labels: ['privacy', 'webhooks'],
    column: 'done',
    ready: true,
    branch: 'jj/bd-4a11-webhook-pii',
    skills: [],
    blocks: [],
    blockedBy: [],
    tokensUsed: 18_220,
    tokensBudget: 80_000,
    closedAt: 'Wed 16:00',
    createdAt: 'Wed 09:33',
    steps: [
      { agent: 'codex',  mode: 'plan',  skills: [],          status: 'done' },
      { agent: 'codex',  mode: 'apply', skills: [],          status: 'done' },
      { agent: 'claude', mode: 'review',skills: [],          status: 'done' },
      { agent: 'codex',  mode: 'apply', skills: [], status: 'done' },
    ],
  },
];

// Active agent capacity snapshot
const CAPACITY = [
  { agent: 'claude',   running: 2, queued: 3, limit: 3 },
  { agent: 'gemini',   running: 1, queued: 2, limit: 2 },
  { agent: 'opencode', running: 0, queued: 1, limit: 2 },
  { agent: 'codex',    running: 0, queued: 0, limit: 1 },
];

// Lifecycle event kinds.
// Each kind has a single-glyph marker and a tone hint, used in feed + timeline.
const EVENT_KINDS = {
  opened:     { glyph: '○', label: 'opened',     tone: 'neutral' },
  scheduled:  { glyph: '◌', label: 'scheduled',  tone: 'amber'   },
  claimed:    { glyph: '◉', label: 'claimed',    tone: 'accent'  },
  started:    { glyph: '▸', label: 'started',    tone: 'live'    },
  paused:     { glyph: '⏸', label: 'paused',     tone: 'mute'    },
  split:      { glyph: '⊢', label: 'split',      tone: 'violet'  },
  review:     { glyph: '◐', label: 'review',     tone: 'violet'  },
  comment:    { glyph: '»',  label: 'comment',    tone: 'violet'  },
  approved:   { glyph: '✔', label: 'approved',   tone: 'green'   },
  closed:     { glyph: '◉', label: 'closed',     tone: 'green'   },
  reopened:   { glyph: '↻', label: 'reopened',   tone: 'amber'   },
  requeued:   { glyph: '↻', label: 'requeued',   tone: 'amber'   },
  blocked:    { glyph: '◊', label: 'blocked',    tone: 'rose'    },
  unblocked:  { glyph: '◈', label: 'unblocked',  tone: 'green'   },
  failed:     { glyph: '×',  label: 'failed',     tone: 'rose'    },
  discovered: { glyph: '↪', label: 'discovered', tone: 'mute'    },
};

// Sample lifecycle history for visible beads. Each entry:
//   { at, kind, actor, note?, agent? }
// `at` strings are calendar-ish; the lifecycle view sorts by their index in this
// array (newest events live at the bottom of each task's history; newest beads
// have the latest events). The feed assembles + reverses.
const HISTORY_BY_BEAD = {
  'bd-a1f2': [
    { at: 'Mon 08:55', kind: 'opened',    actor: 'you@yours.dev' },
    { at: 'Mon 09:02', kind: 'scheduled', actor: 'you@yours.dev' },
    { at: 'Mon 09:14', kind: 'claimed',   actor: 'dispatcher', agent: 'claude' },
    { at: 'Mon 09:14', kind: 'started',   actor: 'claude',     note: 'plan step' },
    { at: 'Mon 09:21', kind: 'comment',   actor: 'claude',     note: '3 acceptance tests drafted' },
    { at: 'Mon 09:34', kind: 'started',   actor: 'claude',     note: 'build step' },
    { at: 'Mon 10:02', kind: 'discovered',actor: 'claude',     note: 'spawned bd-3e80 (flaky checkout)' },
  ],
  'bd-c411': [
    { at: 'Mon 10:55', kind: 'opened',    actor: 'you@yours.dev' },
    { at: 'Mon 10:58', kind: 'blocked',   actor: 'dispatcher', note: 'waits on bd-a1f2' },
    { at: 'Mon 11:02', kind: 'claimed',   actor: 'dispatcher', agent: 'gemini' },
    { at: 'Mon 11:02', kind: 'started',   actor: 'gemini',     note: 'agent step · walking bd graph' },
  ],
  'bd-7c0d': [
    { at: 'Mon 08:24', kind: 'opened',    actor: 'you@yours.dev' },
    { at: 'Mon 08:30', kind: 'scheduled', actor: 'you@yours.dev' },
  ],
  'bd-8b44': [
    { at: 'Sun 23:40', kind: 'opened',    actor: 'you@yours.dev' },
    { at: 'Sun 23:55', kind: 'claimed',   actor: 'dispatcher', agent: 'claude' },
    { at: 'Sun 23:55', kind: 'started',   actor: 'claude' },
    { at: 'Mon 02:11', kind: 'failed',    actor: 'claude',     note: 'token budget exhausted at 92%' },
    { at: 'Mon 02:11', kind: 'requeued',  actor: 'dispatcher' },
    { at: 'Mon 06:30', kind: 'split',     actor: 'dispatcher', note: 'auto-split into 3 sub-beads' },
  ],
  'bd-3e80': [
    { at: 'Sun 22:11', kind: 'opened',    actor: 'claude',     note: 'auto-created by bd-a1f2' },
    { at: 'Sun 22:14', kind: 'claimed',   actor: 'dispatcher', agent: 'codex' },
    { at: 'Sun 22:14', kind: 'started',   actor: 'codex' },
    { at: 'Mon 07:48', kind: 'review',    actor: 'claude' },
    { at: 'Mon 09:15', kind: 'comment',   actor: 'claude',     note: 'requested deterministic clock test' },
    { at: 'Mon 09:42', kind: 'comment',   actor: 'claude',     note: 'second iteration looks good' },
  ],
  'bd-d091': [
    { at: 'Sun 17:30', kind: 'opened',    actor: 'you@yours.dev' },
    { at: 'Sun 18:02', kind: 'claimed',   actor: 'dispatcher', agent: 'opencode' },
    { at: 'Sun 18:02', kind: 'started',   actor: 'opencode' },
    { at: 'Sun 20:14', kind: 'review',    actor: 'gemini',     note: 'no comments' },
  ],
  'bd-9aa1': [
    { at: 'Mon 10:38', kind: 'opened',    actor: 'you@yours.dev' },
    { at: 'Mon 10:40', kind: 'blocked',   actor: 'dispatcher', note: 'waits on bd-7c0d' },
    { at: 'Mon 10:45', kind: 'scheduled', actor: 'you@yours.dev' },
  ],
  'bd-b210': [
    { at: 'Mon 07:55', kind: 'opened',    actor: 'you@yours.dev' },
  ],
  'bd-4f12': [
    { at: 'Fri 16:20', kind: 'opened',    actor: 'you@yours.dev' },
  ],
  'bd-2d55': [
    { at: 'Fri 14:02', kind: 'opened',    actor: 'you@yours.dev' },
  ],
  'bd-7e21': [
    { at: 'Mon 06:00', kind: 'opened',    actor: 'gemini',     note: 'discovered while running bd-c411' },
    { at: 'Mon 06:11', kind: 'blocked',   actor: 'dispatcher', note: 'waits on bd-7c0d' },
  ],
  'bd-5e91': [
    { at: 'Thu 13:30', kind: 'opened',    actor: 'you@yours.dev' },
    { at: 'Thu 13:42', kind: 'claimed',   actor: 'dispatcher', agent: 'claude' },
    { at: 'Thu 13:42', kind: 'started',   actor: 'claude' },
    { at: 'Thu 15:58', kind: 'review',    actor: 'codex' },
    { at: 'Thu 17:42', kind: 'closed',    actor: 'you@yours.dev', note: 'approved' },
  ],
  'bd-6a02': [
    { at: 'Thu 09:10', kind: 'opened',    actor: 'you@yours.dev' },
    { at: 'Thu 09:18', kind: 'claimed',   actor: 'dispatcher', agent: 'gemini' },
    { at: 'Thu 09:18', kind: 'started',   actor: 'gemini' },
    { at: 'Thu 11:34', kind: 'review',    actor: 'claude' },
    { at: 'Thu 12:08', kind: 'closed',    actor: 'you@yours.dev', note: 'shipped' },
  ],
  'bd-4a11': [
    { at: 'Wed 09:33', kind: 'opened',    actor: 'you@yours.dev' },
    { at: 'Wed 09:40', kind: 'claimed',   actor: 'dispatcher', agent: 'codex' },
    { at: 'Wed 09:40', kind: 'started',   actor: 'codex' },
    { at: 'Wed 14:52', kind: 'review',    actor: 'claude' },
    { at: 'Wed 16:00', kind: 'closed',    actor: 'you@yours.dev', note: 'approved' },
  ],
};

// Augment TASKS with derived/light Beads fields.
//   - history (lifecycle log)
//   - assignee (who currently owns/is-on-it)
//   - estimate (S/M/L derived from token budget)
//   - lastActivity (most recent history event's `at`)
//   - comments (drawn from history + reviewer.comments)
//   - acceptance (acceptance criteria — a few sample beads only)
const ACCEPTANCE_BY_BEAD = {
  'bd-a1f2': [
    { text: 'Concurrent 401s dedupe via singleflight refresh', done: true },
    { text: 'Revoked refresh tokens return 401 immediately',   done: true },
    { text: 'Session-scoped revocation cascades to all devices', done: false },
    { text: 'Migration script handles legacy session tokens',   done: false },
  ],
  'bd-c411': [
    { text: 'Walks bd graph from tag→tag',                      done: false },
    { text: 'Groups entries by epic root, topo-sort within',    done: false },
    { text: 'Markdown output passes shellcheck on CI',          done: false },
  ],
  'bd-7c0d': [
    { text: 'Dual-write window plan reviewed by @alice',        done: false },
    { text: 'Backfill job is resumable from any chunk',         done: false },
    { text: 'Cutover requires zero downtime',                   done: false },
  ],
  'bd-3e80': [
    { text: 'Test passes 1000 iterations without flake',        done: true },
    { text: 'Deterministic clock helper extracted',             done: true },
  ],
};

TASKS.forEach(t => {
  // Estimate from token budget.
  if (t.tokensBudget >= 350_000)      t.estimate = 'L';
  else if (t.tokensBudget >= 180_000) t.estimate = 'M';
  else if (t.tokensBudget >= 90_000)  t.estimate = 'S';
  else                                t.estimate = 'XS';

  // Assignee = active or last-touched agent.
  const active = t.steps.find(s => s.status === 'active');
  t.assignee = active?.agent
           ?? [...t.steps].reverse().find(s => s.status === 'done')?.agent
           ?? null;

  // Open date alias.
  t.openedAt = t.openedAt || t.createdAt;

  // History — either supplied above, or synthesized from createdAt.
  t.history = HISTORY_BY_BEAD[t.id] || [
    { at: t.openedAt, kind: 'opened', actor: 'you@yours.dev' },
  ];
  // If done, ensure a `closed` event exists.
  if (t.column === 'done' && !t.history.some(h => h.kind === 'closed')) {
    t.history = [...t.history, { at: t.closedAt || 'recent', kind: 'closed', actor: 'you@yours.dev' }];
  }
  t.lastActivity = t.history[t.history.length - 1]?.at || t.openedAt;

  // Acceptance criteria — sample data on a few beads.
  t.acceptance = ACCEPTANCE_BY_BEAD[t.id] || [];

  // Comment count — derived from history `comment` events plus reviewer.comments.
  t.comments = t.history.filter(h => h.kind === 'comment').length + (t.reviewer?.comments || 0);

  // Source repo — defaulting to the primary repo when unspecified.
  t.repo = REPO_OF_BEAD[t.id] || 'main';
});

// Suggested agent for dispatch — best-fit by remaining capacity, then by speed.
function suggestAgent(task) {
  // Prefer the agent of the first pending step; otherwise pick the least-loaded.
  const firstStep = task.steps[0];
  if (firstStep) return firstStep.agent;
  const sorted = [...CAPACITY].sort((a, b) =>
    (a.running / a.limit) - (b.running / b.limit));
  return sorted[0]?.agent || AGENTS[0].id;
}

window.MUSTER_DATA = {
  AGENTS, COLUMNS, TASKS, CAPACITY, SKILLS, SKILL_CATEGORIES, SKILL_SOURCES,
  ALL_MODES, TYPES, PRIORITIES, FORMULAS, DOLT, EVENT_KINDS,
  VCS_OPTIONS, ROUTES, HYDRATE_REPOS, REPOS,
  modeMeta, typeMeta, priMeta, suggestAgent,
};
