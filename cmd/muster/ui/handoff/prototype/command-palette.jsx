// Command Palette — ⌘K surface for bd CLI commands.
// Groups all 106 bd commands into searchable categories.
// Each entry shows the command, description, and optional shortcut.

const {useState: useStateCP, useEffect: useEffectCP, useRef: useRefCP, useMemo: useMemoCP} = React;

const BD_COMMANDS = [
  // ── Issues ────────────────────────────────────────────────
  { cmd: 'bd create',    args: '"Title" -t <type> -p <pri>', desc: 'Create a new bead',              cat: 'issues', action: 'composer' },
  { cmd: 'bd list',      args: '--status open --json',       desc: 'List beads with filters',        cat: 'issues' },
  { cmd: 'bd show',      args: '<id> --json',                desc: 'Show bead details',              cat: 'issues', action: 'drawer' },
  { cmd: 'bd update',    args: '<id> --claim',               desc: 'Update a bead (claim, edit)',    cat: 'issues' },
  { cmd: 'bd close',     args: '<id> --reason "Done"',       desc: 'Close a bead',                  cat: 'issues' },
  { cmd: 'bd delete',    args: '<id>',                       desc: 'Delete a bead (if not running)', cat: 'issues' },
  { cmd: 'bd reopen',    args: '<id>',                       desc: 'Reopen a closed bead',           cat: 'issues' },
  { cmd: 'bd edit',      args: '<id>',                       desc: 'Edit bead in $EDITOR',           cat: 'issues' },
  { cmd: 'bd rename',    args: '<id> "New title"',           desc: 'Rename a bead',                  cat: 'issues' },
  { cmd: 'bd duplicate', args: '<id>',                       desc: 'Clone a bead with new ID',       cat: 'issues' },
  { cmd: 'bd priority',  args: '<id> <0-4>',                 desc: 'Set priority (0=crit, 4=ice)',   cat: 'issues' },

  // ── Dependencies ──────────────────────────────────────────
  { cmd: 'bd dep',       args: 'add <id> <other>',           desc: 'Add a dependency between beads',  cat: 'deps' },
  { cmd: 'bd blocked',   args: '<id>',                       desc: 'Show what blocks a bead',         cat: 'deps' },
  { cmd: 'bd children',  args: '<id>',                       desc: 'List sub-beads of a parent',      cat: 'deps' },
  { cmd: 'bd graph',     args: '<id>',                       desc: 'Visualize the dependency graph',  cat: 'deps' },
  { cmd: 'bd ready',     args: '--json',                     desc: 'Show unblocked, dispatchable beads', cat: 'deps', action: 'lifecycle' },
  { cmd: 'bd orphans',   args: '',                           desc: 'Find beads with no parent or deps', cat: 'deps' },

  // ── Workflow ──────────────────────────────────────────────
  { cmd: 'bd assign',    args: '<id> --to <agent>',          desc: 'Pin a bead to a specific agent',  cat: 'workflow' },
  { cmd: 'bd state',     args: '<id>',                       desc: 'Show bead state details',         cat: 'workflow' },
  { cmd: 'bd set-state', args: '<id> in_progress',           desc: 'Manually set bead status',        cat: 'workflow' },
  { cmd: 'bd statuses',  args: '',                           desc: 'List all valid statuses',         cat: 'workflow' },
  { cmd: 'bd defer',     args: '<id> --until "2d"',          desc: 'Defer a bead until later',        cat: 'workflow' },
  { cmd: 'bd undefer',   args: '<id>',                       desc: 'Un-defer a bead',                 cat: 'workflow' },
  { cmd: 'bd promote',   args: '<id>',                       desc: 'Promote bead priority',           cat: 'workflow' },
  { cmd: 'bd supersede', args: '<id> <replacement>',         desc: 'Mark bead as superseded',         cat: 'workflow' },
  { cmd: 'bd batch',     args: 'close --label done',         desc: 'Bulk operations on beads',        cat: 'workflow' },
  { cmd: 'bd ship',      args: '<id>',                       desc: 'Close + push + clean up worktree', cat: 'workflow' },
  { cmd: 'bd human',     args: '<id>',                       desc: 'Flag bead as needing human input', cat: 'workflow' },

  // ── Formulas & Molecules ──────────────────────────────────
  { cmd: 'bd formula',   args: 'list',                       desc: 'List available workflow formulas',  cat: 'formulas' },
  { cmd: 'bd formula',   args: 'show <name>',                desc: 'Show formula definition (TOML)',    cat: 'formulas' },
  { cmd: 'bd mol',       args: 'create --formula <name>',    desc: 'Create a molecule from a formula',  cat: 'formulas' },
  { cmd: 'bd mol',       args: 'list',                       desc: 'List active molecules',             cat: 'formulas' },
  { cmd: 'bd cook',      args: '<formula> --title "..."',    desc: 'Shorthand: create bead + molecule', cat: 'formulas' },
  { cmd: 'bd gate',      args: '<id> pass',                  desc: 'Pass a gate on a formula step',     cat: 'formulas' },
  { cmd: 'bd gate',      args: '<id> fail',                  desc: 'Fail a gate on a formula step',     cat: 'formulas' },

  // ── Memory ────────────────────────────────────────────────
  { cmd: 'bd remember',  args: '"key" "value"',              desc: 'Store a persistent memory',         cat: 'memory' },
  { cmd: 'bd recall',    args: '"key"',                      desc: 'Retrieve a stored memory',          cat: 'memory' },
  { cmd: 'bd memories',  args: '',                           desc: 'List all stored memories',          cat: 'memory' },
  { cmd: 'bd prime',     args: '',                           desc: 'Load memories into agent context',  cat: 'memory' },
  { cmd: 'bd forget',    args: '"key"',                      desc: 'Delete a stored memory',            cat: 'memory' },
  { cmd: 'bd context',   args: '<id>',                       desc: 'Show full context for a bead',      cat: 'memory' },

  // ── Labels & Search ───────────────────────────────────────
  { cmd: 'bd label',     args: 'add <id> <label>',           desc: 'Add label to a bead',              cat: 'search' },
  { cmd: 'bd tag',       args: '<id> <tag>',                 desc: 'Tag a bead',                       cat: 'search' },
  { cmd: 'bd search',    args: '"query"',                    desc: 'Full-text search across beads',    cat: 'search' },
  { cmd: 'bd query',     args: '"status:open type:bug"',     desc: 'Structured query with filters',    cat: 'search' },
  { cmd: 'bd sql',       args: '"SELECT * FROM beads"',      desc: 'Raw SQL against the Dolt DB',      cat: 'search' },
  { cmd: 'bd count',     args: '--status open',              desc: 'Count beads matching filters',     cat: 'search' },
  { cmd: 'bd find-duplicates', args: '',                     desc: 'Detect similar/duplicate beads',   cat: 'search' },
  { cmd: 'bd stale',     args: '',                           desc: 'Find beads with no recent activity', cat: 'search' },

  // ── VCS & Worktree ────────────────────────────────────────
  { cmd: 'bd branch',    args: '<id>',                       desc: 'Show or create bead branch',       cat: 'vcs' },
  { cmd: 'bd worktree',  args: '<id>',                       desc: 'Show worktree status',             cat: 'vcs' },
  { cmd: 'bd diff',      args: '<id>',                       desc: 'Show worktree diff',               cat: 'vcs' },
  { cmd: 'bd merge-slot', args: '<id>',                      desc: 'Prepare a merge slot for review',  cat: 'vcs' },
  { cmd: 'bd vc',        args: 'status',                     desc: 'VCS status across worktrees',      cat: 'vcs' },

  // ── Sync & Dolt ───────────────────────────────────────────
  { cmd: 'bd dolt',      args: 'push',                       desc: 'Push beads DB to Dolt remote',     cat: 'sync' },
  { cmd: 'bd dolt',      args: 'pull',                       desc: 'Pull beads DB from Dolt remote',   cat: 'sync' },
  { cmd: 'bd dolt',      args: 'start',                      desc: 'Start the Dolt SQL server',        cat: 'sync' },
  { cmd: 'bd backup',    args: 'sync',                       desc: 'Push a Dolt-native backup',        cat: 'sync' },
  { cmd: 'bd backup',    args: 'restore',                    desc: 'Restore from backup',              cat: 'sync' },
  { cmd: 'bd import',    args: '--from github',              desc: 'Import issues from external source', cat: 'sync' },
  { cmd: 'bd export',    args: '--json > beads.jsonl',       desc: 'Export beads to JSONL',             cat: 'sync' },
  { cmd: 'bd federation', args: 'status',                    desc: 'Show federation status',            cat: 'sync' },

  // ── Integrations ──────────────────────────────────────────
  { cmd: 'bd github',    args: 'sync',                       desc: 'Sync with GitHub Issues/PRs',      cat: 'integrations' },
  { cmd: 'bd gitlab',    args: 'sync',                       desc: 'Sync with GitLab issues',          cat: 'integrations' },
  { cmd: 'bd jira',      args: 'import',                     desc: 'Import from Jira',                 cat: 'integrations' },
  { cmd: 'bd linear',    args: 'sync',                       desc: 'Sync with Linear issues',          cat: 'integrations' },
  { cmd: 'bd notion',    args: 'sync',                       desc: 'Sync with Notion databases',       cat: 'integrations' },
  { cmd: 'bd ado',       args: 'sync',                       desc: 'Sync with Azure DevOps',           cat: 'integrations' },

  // ── Admin & Setup ─────────────────────────────────────────
  { cmd: 'bd init',      args: '--quiet',                    desc: 'Initialize beads in current dir',  cat: 'admin' },
  { cmd: 'bd setup',     args: '',                           desc: 'Interactive setup wizard',          cat: 'admin' },
  { cmd: 'bd config',    args: 'get <key>',                  desc: 'Read/write config values',          cat: 'admin' },
  { cmd: 'bd doctor',    args: '',                           desc: 'Diagnose database + config issues', cat: 'admin' },
  { cmd: 'bd audit',     args: '',                           desc: 'Audit trail for all bead changes',  cat: 'admin' },
  { cmd: 'bd lint',      args: '',                           desc: 'Lint beads for consistency issues',  cat: 'admin' },
  { cmd: 'bd gc',        args: '',                           desc: 'Garbage-collect old worktrees',     cat: 'admin' },
  { cmd: 'bd compact',   args: '',                           desc: 'Compact the database',              cat: 'admin' },
  { cmd: 'bd migrate',   args: '',                           desc: 'Run pending schema migrations',     cat: 'admin' },
  { cmd: 'bd upgrade',   args: '',                           desc: 'Upgrade bd CLI to latest',          cat: 'admin' },
  { cmd: 'bd version',   args: '',                           desc: 'Show bd version',                   cat: 'admin' },
  { cmd: 'bd completion', args: 'bash',                      desc: 'Generate shell completions',        cat: 'admin' },
  { cmd: 'bd where',     args: '',                           desc: 'Show beads directory location',     cat: 'admin' },
  { cmd: 'bd info',      args: '',                           desc: 'Show project info & stats',         cat: 'admin' },
  { cmd: 'bd rules',     args: '',                           desc: 'Show active workflow rules',        cat: 'admin' },
  { cmd: 'bd hooks',     args: 'list',                       desc: 'List configured hooks',             cat: 'admin' },

  // ── Multi-agent ───────────────────────────────────────────
  { cmd: 'bd swarm',     args: '<formula>',                  desc: 'Fan out work to multiple agents',   cat: 'multiagent' },
  { cmd: 'bd ping',      args: '<agent>',                    desc: 'Ping an agent to check liveness',   cat: 'multiagent' },
  { cmd: 'bd preflight', args: '<id>',                       desc: 'Pre-dispatch validation checks',    cat: 'multiagent' },
];

const CMD_CATEGORIES = [
  { id: 'issues',       name: 'Issues',            icon: '○' },
  { id: 'deps',         name: 'Dependencies',      icon: '◊' },
  { id: 'workflow',     name: 'Workflow',           icon: '▸' },
  { id: 'formulas',     name: 'Formulas & Gates',   icon: 'ƒ' },
  { id: 'memory',       name: 'Memory',             icon: '◦' },
  { id: 'search',       name: 'Search & Labels',    icon: '⌕' },
  { id: 'vcs',          name: 'VCS & Worktree',     icon: '⎇' },
  { id: 'sync',         name: 'Sync & Dolt',        icon: '§' },
  { id: 'integrations', name: 'Integrations',       icon: '↔' },
  { id: 'multiagent',   name: 'Multi-agent',        icon: '⊕' },
  { id: 'admin',        name: 'Admin & Setup',      icon: '⚙' },
];

function CommandPalette({ open, onClose, onAction }) {
  const [query, setQuery] = useStateCP('');
  const [catFilter, setCatFilter] = useStateCP('all');
  const [selectedIdx, setSelectedIdx] = useStateCP(0);
  const inputRef = useRefCP(null);
  const listRef = useRefCP(null);

  // Focus input on open
  useEffectCP(() => {
    if (open) {
      setQuery('');
      setCatFilter('all');
      setSelectedIdx(0);
      setTimeout(() => inputRef.current?.focus(), 60);
    }
  }, [open]);

  // Keyboard nav
  useEffectCP(() => {
    if (!open) return;
    const handler = (e) => {
      if (e.key === 'Escape') { onClose(); return; }
      if (e.key === 'ArrowDown') { e.preventDefault(); setSelectedIdx(i => i + 1); }
      if (e.key === 'ArrowUp')   { e.preventDefault(); setSelectedIdx(i => Math.max(0, i - 1)); }
      if (e.key === 'Enter') {
        e.preventDefault();
        const item = filtered[Math.min(selectedIdx, filtered.length - 1)];
        if (item) execCommand(item);
      }
    };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [open, selectedIdx, query, catFilter]);

  const filtered = useMemoCP(() => {
    let items = BD_COMMANDS;
    if (catFilter !== 'all') items = items.filter(c => c.cat === catFilter);
    if (query.trim()) {
      const q = query.toLowerCase().trim();
      items = items.filter(c =>
        c.cmd.toLowerCase().includes(q) ||
        c.desc.toLowerCase().includes(q) ||
        c.args.toLowerCase().includes(q) ||
        c.cat.toLowerCase().includes(q)
      );
    }
    return items;
  }, [query, catFilter]);

  // Clamp selection
  useEffectCP(() => {
    if (selectedIdx >= filtered.length) setSelectedIdx(Math.max(0, filtered.length - 1));
  }, [filtered.length]);

  // Scroll selected into view
  useEffectCP(() => {
    const el = listRef.current?.children[selectedIdx];
    if (el) el.scrollIntoView({ block: 'nearest' });
  }, [selectedIdx]);

  const execCommand = (item) => {
    if (item.action) {
      onAction(item.action, item);
    }
    onClose();
  };

  if (!open) return null;

  // Group filtered by category for display
  const grouped = [];
  let lastCat = null;
  filtered.forEach((item, i) => {
    if (item.cat !== lastCat) {
      const catMeta = CMD_CATEGORIES.find(c => c.id === item.cat);
      grouped.push({ type: 'header', cat: item.cat, name: catMeta?.name || item.cat, icon: catMeta?.icon || '·' });
      lastCat = item.cat;
    }
    grouped.push({ type: 'item', ...item, flatIdx: i });
  });

  return (
    <>
      <div className="cmdpal-backdrop" onClick={onClose}></div>
      <div className="cmdpal">
        <div className="cmdpal-head">
          <span className="cmdpal-icon">⌘</span>
          <input
            ref={inputRef}
            className="cmdpal-input"
            placeholder="Search bd commands…"
            value={query}
            onChange={e => { setQuery(e.target.value); setSelectedIdx(0); }}
          />
          <kbd className="cmdpal-kbd">esc</kbd>
        </div>

        <div className="cmdpal-cats">
          <button
            className={'cmdpal-cat ' + (catFilter === 'all' ? 'active' : '')}
            onClick={() => { setCatFilter('all'); setSelectedIdx(0); }}
          >All</button>
          {CMD_CATEGORIES.map(c => (
            <button
              key={c.id}
              className={'cmdpal-cat ' + (catFilter === c.id ? 'active' : '')}
              onClick={() => { setCatFilter(c.id); setSelectedIdx(0); }}
            >
              <span className="cmdpal-cat-icon">{c.icon}</span>
              {c.name}
            </button>
          ))}
        </div>

        <div className="cmdpal-list" ref={listRef}>
          {grouped.map((g, i) => {
            if (g.type === 'header') {
              return (
                <div key={'h-' + g.cat} className="cmdpal-group-head">
                  <span className="cmdpal-group-icon">{g.icon}</span>
                  <span>{g.name}</span>
                  <span className="cmdpal-group-line"></span>
                </div>
              );
            }
            const isSelected = g.flatIdx === selectedIdx;
            return (
              <button
                key={g.cmd + '-' + g.args + '-' + i}
                className={'cmdpal-item ' + (isSelected ? 'selected' : '')}
                onMouseEnter={() => setSelectedIdx(g.flatIdx)}
                onClick={() => execCommand(g)}
              >
                <code className="cmdpal-cmd">{g.cmd}</code>
                <span className="cmdpal-args">{g.args}</span>
                <span className="cmdpal-desc">{g.desc}</span>
                {g.action && <span className="cmdpal-action-hint">↵</span>}
              </button>
            );
          })}
          {filtered.length === 0 && (
            <div className="cmdpal-empty">
              No commands match "<strong>{query}</strong>"
            </div>
          )}
        </div>

        <div className="cmdpal-foot">
          <span className="cmdpal-foot-hint">
            <kbd>↑↓</kbd> navigate
            <kbd>↵</kbd> select
            <kbd>esc</kbd> close
          </span>
          <span className="cmdpal-foot-count">{filtered.length} commands</span>
        </div>
      </div>
    </>
  );
}

// ── Memories panel (small floating surface) ──────────────────────────────

const SAMPLE_MEMORIES = [
  { key: 'auth-pattern',      value: 'Always use singleflight lock for token refresh. See bd-a1f2 for reference implementation.', agent: 'claude', at: 'Mon 09:38' },
  { key: 'test-convention',   value: 'Name test files as <module>.spec.ts, not <module>.test.ts. Use vitest, not jest.', agent: 'you@yours.dev', at: 'Sun 14:20' },
  { key: 'billing-v3-schema', value: 'v3 adds: tenant_id (uuid), billing_cycle (enum: monthly|annual), grace_period_days (int, default 7)', agent: 'gemini', at: 'Sat 11:05' },
  { key: 'deploy-checklist',  value: '1. pnpm typecheck  2. pnpm test  3. bd lint  4. jj push  5. vercel deploy --prod', agent: 'you@yours.dev', at: 'Fri 09:12' },
  { key: 'api-sunset-policy', value: 'Add Sunset header 90 days before removal. Log callers for 30 days. See RFC 8594.', agent: 'codex', at: 'Thu 12:30' },
];

function MemoriesPanel({ open, onClose }) {
  const [memories, setMemories] = useStateCP(SAMPLE_MEMORIES);
  const [newKey, setNewKey] = useStateCP('');
  const [newVal, setNewVal] = useStateCP('');
  const [search, setSearch] = useStateCP('');

  const filtered = memories.filter(m =>
    !search || m.key.includes(search.toLowerCase()) || m.value.toLowerCase().includes(search.toLowerCase())
  );

  const addMemory = () => {
    if (!newKey.trim() || !newVal.trim()) return;
    setMemories([{ key: newKey.trim(), value: newVal.trim(), agent: 'you@yours.dev', at: 'just now' }, ...memories]);
    setNewKey(''); setNewVal('');
  };
  const removeMemory = (key) => setMemories(memories.filter(m => m.key !== key));

  if (!open) return null;

  return (
    <>
      <div className="mem-backdrop" onClick={onClose}></div>
      <div className="mem-panel">
        <header className="mem-head">
          <h3 className="mem-title">Memories</h3>
          <span className="mem-sub">bd remember · bd recall · bd prime</span>
          <button className="icon-btn mem-close" onClick={onClose}>×</button>
        </header>

        <div className="mem-search-row">
          <input
            className="mem-search"
            placeholder="Filter memories…"
            value={search}
            onChange={e => setSearch(e.target.value)}
          />
          <span className="mem-count">{filtered.length} stored</span>
        </div>

        <div className="mem-list">
          {filtered.map(m => {
            const {AGENTS} = window.MUSTER_DATA;
            const A = AGENTS.find(a => a.id === m.agent);
            return (
              <div key={m.key} className="mem-item">
                <div className="mem-item-head">
                  <code className="mem-key">{m.key}</code>
                  <span className="mem-when">{m.at}</span>
                  {A
                    ? <span className="agent-mono sm" style={{['--agent-color']: A.color}}>{A.mono}</span>
                    : <span className="mem-actor">{m.agent}</span>}
                  <button className="mem-remove" onClick={() => removeMemory(m.key)} title="bd forget">×</button>
                </div>
                <div className="mem-value">{m.value}</div>
              </div>
            );
          })}
          {filtered.length === 0 && <div className="mem-empty">No memories match.</div>}
        </div>

        <div className="mem-add">
          <div className="mem-add-row">
            <input className="mem-add-key" placeholder="key" value={newKey} onChange={e => setNewKey(e.target.value)} />
            <textarea className="mem-add-val" placeholder="value — what should agents remember?" rows={2} value={newVal} onChange={e => setNewVal(e.target.value)} onKeyDown={e => { if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) addMemory(); }} />
          </div>
          <div className="mem-add-foot">
            <code className="mem-cli-hint">bd remember "{newKey || 'key'}" "{newVal || 'value'}"</code>
            <button className="btn btn-primary mem-add-btn" onClick={addMemory} disabled={!newKey.trim() || !newVal.trim()}>Remember</button>
          </div>
        </div>

        <div className="mem-prime-row">
          <button className="btn btn-ghost mem-prime-btn">
            ◦ bd prime — load all memories into next dispatch
          </button>
        </div>
      </div>
    </>
  );
}

// ── Formulas browser ─────────────────────────────────────────────────────

const FORMULA_DETAILS = {
  'speckit-flow': {
    steps: [
      { agent: 'claude', mode: 'plan', skills: ['speckit'], desc: 'Draft spec + acceptance criteria' },
      { agent: 'claude', mode: 'plan', skills: [], desc: 'Decompose into atomic steps' },
      { agent: 'claude', mode: 'build', skills: [], desc: 'Implement step by step' },
      { agent: 'gemini', mode: 'review', skills: [], desc: 'Review diff against spec', loopTo: 2 },
      { agent: 'claude', mode: 'agent', skills: [], desc: 'Final fixups + commit' },
    ],
    gates: [{ kind: 'human', label: 'Spec approval before build', after: 0 }],
    vars: ['$SPEC_PATH', '$TEST_CMD'],
  },
  'bug-triage': {
    steps: [
      { agent: 'codex', mode: 'plan', skills: [], desc: 'Reproduce + root cause analysis' },
      { agent: 'codex', mode: 'apply', skills: [], desc: 'Fix + regression test' },
      { agent: 'claude', mode: 'review', skills: [], desc: 'Review fix', loopTo: 1 },
      { agent: 'codex', mode: 'apply', skills: [], desc: 'Apply review feedback' },
    ],
    gates: [],
    vars: ['$ERROR_LOG', '$SENTRY_URL'],
  },
  'migrate-v3': {
    steps: [
      { agent: 'claude', mode: 'plan', skills: ['speckit'], desc: 'Draft migration plan' },
      { agent: 'gemini', mode: 'plan', skills: ['openspec'], desc: 'Validate API contracts' },
      { agent: 'claude', mode: 'plan', skills: [], desc: 'Plan dual-write window' },
      { agent: 'claude', mode: 'build', skills: [], desc: 'Implement dual-write + backfill' },
      { agent: 'codex', mode: 'review', skills: [], desc: 'Review migration safety' },
      { agent: 'claude', mode: 'agent', skills: [], desc: 'Cutover + cleanup' },
    ],
    gates: [
      { kind: 'human', label: 'DBA approves dual-write plan', after: 2 },
      { kind: 'timer', label: '24h soak window', after: 3 },
    ],
    vars: ['$SOURCE_TABLE', '$TARGET_SCHEMA', '$SOAK_HOURS'],
  },
  'changelog-gen': {
    steps: [
      { agent: 'claude', mode: 'plan', skills: ['speckit'], desc: 'Define changelog format' },
      { agent: 'gemini', mode: 'agent', skills: [], desc: 'Walk bd graph between tags' },
      { agent: 'claude', mode: 'review', skills: [], desc: 'Review generated changelog' },
    ],
    gates: [],
    vars: ['$FROM_TAG', '$TO_TAG'],
  },
};

function FormulasPanel({ open, onClose }) {
  const {FORMULAS, AGENTS} = window.MUSTER_DATA;
  const [selected, setSelected] = useStateCP(FORMULAS[0]?.id);
  const formula = FORMULAS.find(f => f.id === selected);
  const detail = FORMULA_DETAILS[selected];

  if (!open) return null;

  return (
    <>
      <div className="form-backdrop" onClick={onClose}></div>
      <div className="form-panel">
        <header className="form-head">
          <h3 className="form-title">Formulas</h3>
          <span className="form-sub">bd formula · bd mol · bd cook</span>
          <button className="icon-btn form-close" onClick={onClose}>×</button>
        </header>

        <div className="form-body">
          <aside className="form-sidebar">
            {FORMULAS.map(f => (
              <button
                key={f.id}
                className={'form-side-item ' + (selected === f.id ? 'active' : '')}
                onClick={() => setSelected(f.id)}
              >
                <span className="form-side-icon">ƒ</span>
                <div className="form-side-text">
                  <span className="form-side-name">{f.name}</span>
                  <span className="form-side-desc">{f.desc}</span>
                </div>
              </button>
            ))}
          </aside>

          <main className="form-main">
            {formula && detail && (
              <>
                <div className="form-main-head">
                  <span className="form-formula-icon">ƒ</span>
                  <div>
                    <h4 className="form-formula-name">{formula.name}</h4>
                    <p className="form-formula-desc">{formula.desc}</p>
                  </div>
                </div>

                <div className="form-section">
                  <span className="form-section-label">Step chain</span>
                  <div className="form-steps">
                    {detail.steps.map((s, i) => {
                      const A = AGENTS.find(a => a.id === s.agent);
                      return (
                        <div key={i} className="form-step">
                          <span className="form-step-num">{i + 1}</span>
                          <span className="agent-mono" style={{['--agent-color']: A?.color}}>{A?.mono}</span>
                          <span className="form-step-mode" data-mode={s.mode}>{s.mode}</span>
                          {s.skills?.length > 0 && s.skills.map(sk => (
                            <span key={sk} className="form-step-skill">{sk}</span>
                          ))}
                          <span className="form-step-desc">{s.desc}</span>
                          {s.loopTo !== undefined && (
                            <span className="loop-tag">↻ {s.loopTo + 1}</span>
                          )}
                          {i < detail.steps.length - 1 && <span className="form-step-conn"></span>}
                        </div>
                      );
                    })}
                  </div>
                </div>

                {detail.gates.length > 0 && (
                  <div className="form-section">
                    <span className="form-section-label">Gates</span>
                    <div className="form-gates">
                      {detail.gates.map((g, i) => (
                        <div key={i} className="form-gate">
                          <span className="form-gate-icon">{g.kind === 'human' ? '☻' : g.kind === 'timer' ? '◷' : '⎇'}</span>
                          <span className="form-gate-label">{g.label}</span>
                          <span className="form-gate-after">after step {g.after + 1}</span>
                        </div>
                      ))}
                    </div>
                  </div>
                )}

                {detail.vars.length > 0 && (
                  <div className="form-section">
                    <span className="form-section-label">Variables</span>
                    <div className="form-vars">
                      {detail.vars.map(v => <code key={v} className="form-var">{v}</code>)}
                    </div>
                  </div>
                )}

                <div className="form-cli-block">
                  <span className="form-section-label">Use this formula</span>
                  <code className="form-cli">bd cook {formula.name} --title "My task" {detail.vars.map(v => v + '=...').join(' ')}</code>
                </div>
              </>
            )}
          </main>
        </div>
      </div>
    </>
  );
}

Object.assign(window, { CommandPalette, MemoriesPanel, FormulasPanel, BD_COMMANDS, CMD_CATEGORIES });
