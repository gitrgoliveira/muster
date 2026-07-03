// Main app shell + Mobile preview view.

const {useState: useStateA, useEffect: useEffectA, useMemo: useMemoA} = React;

// ─── App Error Boundary ──────────────────────────────────────────────────────

class AppErrorBoundary extends React.Component {
  constructor(props) {
    super(props);
    this.state = { hasError: false, error: null };
  }
  static getDerivedStateFromError(error) {
    return { hasError: true, error };
  }
  componentDidCatch(error, info) {
    // In production, send to error reporter
    console.error('[muster] uncaught render error', error, info);
  }
  render() {
    if (!this.state.hasError) return this.props.children;
    const msg = this.state.error?.message || String(this.state.error);
    return (
      <div className="app-error-root">
        <div className="app-error-card">
          <span className="app-error-glyph" aria-hidden="true">✕</span>
          <div className="app-error-body">
            <h2 className="app-error-title">Something went wrong</h2>
            <p className="app-error-msg">{msg}</p>
            <button className="btn btn-ghost app-error-reload" onClick={() => location.reload()}>
              Reload
            </button>
          </div>
        </div>
      </div>
    );
  }
}

// ─── Toast system ─────────────────────────────────────────────────────────────
// Returns { toasts, addToast, dismissToast }. addToast({msg, undoFn}) adds a
// 6-second auto-dismiss toast with an optional Undo action.

function useToastSystem() {
  const [toasts, setToasts] = useStateA([]);
  const addToast = (msg, undoFn) => {
    const id = Date.now() + Math.random();
    setToasts(ts => [...ts, { id, msg, undoFn }]);
    setTimeout(() => {
      setToasts(ts => ts.filter(t => t.id !== id));
    }, 6000);
    return id;
  };
  const dismissToast = (id) => setToasts(ts => ts.filter(t => t.id !== id));
  return { toasts, addToast, dismissToast };
}

function ToastContainer({ toasts, onDismiss }) {
  if (!toasts.length) return null;
  return (
    <div className="toast-container" role="region" aria-live="polite" aria-label="Notifications">
      {toasts.map(t => (
        <div key={t.id} className="toast">
          <span className="toast-msg">{t.msg}</span>
          <div className="toast-actions">
            {t.undoFn && (
              <button className="toast-undo" onClick={() => { t.undoFn(); onDismiss(t.id); }}>
                Undo
              </button>
            )}
            <button className="toast-dismiss" onClick={() => onDismiss(t.id)} aria-label="Dismiss">×</button>
          </div>
        </div>
      ))}
    </div>
  );
}

function MobilePreview({tasks, onOpen}) {
  const {COLUMNS} = window.MUSTER_DATA;
  const [colIdx, setColIdx] = useStateA(2); // start on Running

  const byColumn = useMemoA(() => {
    const m = Object.fromEntries(COLUMNS.map(c => [c.id, []]));
    tasks.forEach(t => { if (m[t.column]) m[t.column].push(t); });
    return m;
  }, [tasks]);

  const col = COLUMNS[colIdx];
  const colTasks = byColumn[col.id];

  return (
    <div className="mobile-preview">
      <div className="phone">
        <div className="phone-notch"></div>
        <div className="phone-screen">
          <header className="m-head">
            <div className="m-brand">
              <span className="m-logo">M</span>
              <span className="m-title">Muster</span>
            </div>
            <button className="m-icon">⋯</button>
          </header>

          <nav className="m-cols" >
            {COLUMNS.map((c, i) => (
              <button
                key={c.id}
                className={'m-col-tab ' + (i === colIdx ? 'active' : '')}
                onClick={() => setColIdx(i)}
              >
                <span>{c.name}</span>
                <span className="m-col-count">{byColumn[c.id].length}</span>
              </button>
            ))}
          </nav>

          <div className="m-body">
            {colTasks.length === 0 && <div className="m-empty">Nothing here yet.</div>}
            {colTasks.map(t => {
              const activeStep = t.steps.find(s => s.status === 'active') ||
                                  [...t.steps].reverse().find(s => s.status === 'done');
              const A = window.MUSTER_DATA.AGENTS.find(a => a.id === activeStep?.agent);
              return (
                <article key={t.id} className="m-card" onClick={() => onOpen(t)}>
                  <header className="m-card-head">
                    <span className="m-bead-id">{t.id}</span>
                    <PriBadge n={t.priority} />
                  </header>
                  <h3 className="m-card-title">{t.title}</h3>
                  <StepRail steps={t.steps} compact />
                  <footer className="m-card-foot">
                    {A && (
                      <span className="agent-chip" style={{['--agent-color']: A.color}}>
                        <span className="agent-mono">{A.mono}</span>
                        <span className="agent-name">{A.name}</span>
                      </span>
                    )}
                    {t.column === 'running' && (
                      <TokenBar used={t.tokensUsed} budget={t.tokensBudget} />
                    )}
                  </footer>
                </article>
              );
            })}
          </div>

          <footer className="m-bar">
            <button className="m-fab">＋ New bead</button>
          </footer>
        </div>
        <div className="phone-bezel-shine"></div>
      </div>
      <div className="mobile-hint">
        <p>Swipe between columns · long-press to drag · pull to refresh.</p>
        <p className="muted">Mirrors the same data — every bead is editable from web or phone.</p>
      </div>
    </div>
  );
}

function CapacityStrip({capacity}) {
  const {AGENTS} = window.MUSTER_DATA;
  // Sort distressed providers to the front so the eye lands on them first.
  const sorted = [...AGENTS].sort((a, b) => {
    const pctA = a.quota.selfHosted ? 0 : (a.quota.used / a.quota.limit);
    const pctB = b.quota.selfHosted ? 0 : (b.quota.used / b.quota.limit);
    if (pctA >= 0.85 && pctB < 0.85) return -1;
    if (pctB >= 0.85 && pctA < 0.85) return 1;
    return 0;
  });
  return (
    <div className="cap-strip">
      {sorted.map(a => {
        const c = capacity.find(x => x.agent === a.id);
        const filled = c.running;
        const q = a.quota;
        const pct = q.selfHosted ? 0 : Math.min(100, (q.used / q.limit) * 100);
        const distress = pct >= 85;
        const quotaText = q.selfHosted
          ? 'local'
          : q.unit === '$'
            ? '$' + q.used.toFixed(2) + ' / $' + q.limit.toFixed(0)
            : fmtTokens(q.used) + '/' + fmtTokens(q.limit);
        return (
          <div key={a.id} className={'cap-pill ' + (distress ? 'is-distress' : '')} style={{['--agent-color']: a.color}} tabIndex={0}>
            <span className="agent-mono">{a.mono}</span>
            <span className="cap-pill-slots">
              {Array.from({length: c.limit}).map((_, i) => (
                <span key={i} className={'cap-pip ' + (i < filled ? 'on' : '')}></span>
              ))}
            </span>
            {c.queued > 0 && <span className="cap-pill-q">{c.queued}q</span>}
            <span className="cap-pill-quota">
              <span className="cap-quota-bar">
                <span className="cap-quota-bar-fill" style={{width: pct + '%'}} data-danger={pct >= 85}></span>
              </span>
            </span>

            <div className="agent-tooltip" role="tooltip">
              <div className="agent-tooltip-head">
                <span className="agent-mono" style={{['--agent-color']: a.color}}>{a.mono}</span>
                <div>
                  <div className="agent-tooltip-name">{a.name}</div>
                  <div className="agent-tooltip-plan">{a.plan}</div>
                </div>
              </div>
              <div className="agent-tooltip-rows">
                <div className="agent-tooltip-row">
                  <span>{q.window}</span>
                  <span>{fmtQuota(q)}</span>
                </div>
                {!q.selfHosted && (
                  <div className="agent-tooltip-bar">
                    <div className="agent-tooltip-bar-fill" style={{width: pct + '%', background: a.color}}></div>
                  </div>
                )}
                <div className="agent-tooltip-row muted">
                  <span>this month</span><span>{fmtQuota(a.monthly)}</span>
                </div>
                <div className="agent-tooltip-row muted">
                  <span>resets in</span><span>{q.resetIn}</span>
                </div>
                <div className="agent-tooltip-row muted">
                  <span>rate limit</span><span>{a.rateLimit}</span>
                </div>
                <div className="agent-tooltip-row muted">
                  <span>parallel</span><span>{c.running}/{c.limit} · {c.queued} queued</span>
                </div>
              </div>
            </div>
          </div>
        );
      })}
    </div>
  );
}

function NowPlayingRail({tasks, onOpen}) {
  const {AGENTS} = window.MUSTER_DATA;
  const running = tasks.filter(t => t.column === 'running');
  return (
    <div className="nowplaying-rail">
      {running.length === 0 && <div className="np-empty">No beads running. Schedule one to begin.</div>}
      {running.map(t => {
        const activeStep = t.steps.find(s => s.status === 'active');
        const agentId = activeStep?.agent || t.assignee;
        const A = AGENTS.find(a => a.id === agentId);
        const action = t.nowPlaying?.action || activeStep?.note || 'working…';
        const kind = t.nowPlaying?.kind || 'tool';
        const since = t.nowPlaying?.since || 0;
        const pct = Math.min(100, (t.tokensUsed / t.tokensBudget) * 100);
        const warn = pct >= 70 && pct < 85;
        const danger = pct >= 85;
        return (
          <div key={t.id} className="np-row" onClick={() => onOpen(t)} style={{['--agent-color']: A?.color || '#888'}}>
            <span className="np-now-pulse"></span>
            <span className="np-agent">{A?.mono || '??'}</span>
            <span className="np-bead">{t.id}</span>
            <span className={'np-action kind-' + kind} title={action}>{action}</span>
            <span className="np-elapsed">{fmtElapsed(since)}</span>
            <span className="np-tokens" title={fmtTokens(t.tokensUsed) + ' / ' + fmtTokens(t.tokensBudget) + ' tokens'}>
              <span className="np-tok-bar"><span className="np-tok-bar-fill" style={{width: pct + '%'}} data-warn={warn} data-danger={danger}></span></span>
              <span className="np-tok-pct">{Math.round(pct)}%</span>
            </span>
          </div>
        );
      })}
    </div>
  );
}

function DoltChip() {
  const {DOLT} = window.MUSTER_DATA;
  if (!DOLT) return null;
  return (
    <div className="dolt-chip" tabIndex={0} role="button">
      <span className="dolt-glyph">§</span>
      <span className="dolt-status">
        <span className="dolt-dot" data-status={DOLT.status}></span>
        <span className="dolt-branch">{DOLT.branch}</span>
      </span>
      <span className="dolt-meta">· {DOLT.lastSync}</span>
      <div className="dolt-tip" role="tooltip">
        <div className="dolt-tip-row"><span>branch</span><strong>{DOLT.branch}</strong></div>
        <div className="dolt-tip-row"><span>remote</span><strong>{DOLT.remote}</strong></div>
        <div className="dolt-tip-row"><span>status</span><strong>{DOLT.status}</strong></div>
        <div className="dolt-tip-row muted"><span>ahead</span><span>{DOLT.ahead}</span></div>
        <div className="dolt-tip-row muted"><span>behind</span><span>{DOLT.behind}</span></div>
        <div className="dolt-tip-row muted"><span>writers</span><span>{DOLT.writers} active</span></div>
        <div className="dolt-tip-row muted"><span>server</span><span>:{DOLT.port} {DOLT.server}</span></div>
        <div className="dolt-tip-row muted"><span>last sync</span><span>{DOLT.lastSync}</span></div>
        <div className="dolt-tip-actions">
          <button>bd dolt pull</button>
          <button>bd dolt push</button>
        </div>
      </div>
    </div>
  );
}

function App() {
  const {TASKS: SEED_TASKS, CAPACITY: SEED_CAP, FRAMEWORKS} = window.MUSTER_DATA;

  const { toasts, addToast, dismissToast } = useToastSystem();
  const [tasks, setTasks] = useStateA(SEED_TASKS);
  const [capacity, setCapacity] = useStateA(SEED_CAP);
  const [repos, setRepos] = useStateA(window.MUSTER_DATA.REPOS);
  const [repoFilter, setRepoFilter] = useStateA('all');
  const [constitution, setConstitution] = useStateA(`# Constitution

These rules apply to every bead, on top of the per-task instructions.

1. Keep diffs small — if a change touches more than 8 files, split into sub-beads.
2. Never disable a failing test. Either fix the cause or document why it's flaky.
3. Prefer pure functions and explicit return types over inferred any.
4. Add a regression test for every bug fix — reference the bead id in the test name.
5. Run \`pnpm typecheck\` before handoff to review.
6. Commit messages follow Conventional Commits, scoped to the bead id.
7. Do not introduce new dependencies without a one-line justification in the bead.`);
  const [openTaskId, setOpenTaskId] = useStateA(null);
  const [composerOpen, setComposerOpen] = useStateA(false);
  const [addProviderOpen, setAddProviderOpen] = useStateA(false);
  const [addRepoOpen, setAddRepoOpen] = useStateA(false);
  const [cmdPalOpen, setCmdPalOpen] = useStateA(false);
  const [memoriesOpen, setMemoriesOpen] = useStateA(false);
  const [formulasOpen, setFormulasOpen] = useStateA(false);
  const [view, setView] = useStateA('board'); // board | lifecycle | orchestrator | providers | modes | repos
  const [filter, setFilter] = useStateA('');
  const [tweaks, setTweak] = useTweaks({
    accent: '#D97757',
    density: 'medium',
    centerLayout: 'split',
    mobilePreview: false,
  });

  // Apply tweaks to root
  useEffectA(() => {
    document.documentElement.style.setProperty('--accent', tweaks.accent);
    document.documentElement.setAttribute('data-density', tweaks.density);
  }, [tweaks.accent, tweaks.density]);

  // ⌘K command palette trigger
  useEffectA(() => {
    const handler = (e) => {
      if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
        e.preventDefault();
        setCmdPalOpen(o => !o);
      }
    };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, []);

  // Sub-bead lookup: synthesize a viewable task from any parent's subBeads array
  // when the ID isn't a top-level task. Keeps clicking sub-beads from "closing"
  // the drawer when the bead isn't yet a full task row.
  function findTaskAny(id) {
    const direct = tasks.find(t => t.id === id);
    if (direct) return direct;
    for (const parent of tasks) {
      const sb = (parent.subBeads || []).find(b => b.id === id);
      if (sb) {
        return {
          ...sb,
          parentId: parent.id,
          type: parent.type,
          column: sb.status === 'done' ? 'done'
                : sb.status === 'active' ? 'running'
                : 'scheduled',
          priority: parent.priority,
          vcs: parent.vcs,
          labels: parent.labels || [],
          steps: [{ agent: sb.agent || parent.assignee || 'claude', mode: 'default', skills: [], status: sb.status === 'done' ? 'done' : sb.status === 'active' ? 'active' : 'pending' }],
          history: [
            { at: 'recent', kind: 'opened', actor: sb.autoSplit ? 'dispatcher' : 'you@yours.dev', note: sb.autoSplit ? 'auto-split from ' + parent.id : 'sub-bead of ' + parent.id },
          ],
          blockedBy: [],
          blocks: [],
          subBeads: [],
          acceptance: [],
          tokensUsed: 0,
          tokensBudget: 80_000,
          createdAt: 'recent',
          lastActivity: 'recent',
          desc: 'Sub-bead of ' + parent.id + '. ' + (sb.autoSplit ? 'Auto-split by dispatcher when parent exhausted its budget.' : ''),
          _isSubBead: true,
        };
      }
    }
    return null;
  }
  const openTask = findTaskAny(openTaskId);

  const onOpen = (t) => setOpenTaskId(t.id);
  const onClose = () => setOpenTaskId(null);

  // Dispatch a ready bead: move to running, append claim+start events,
  // mark first pending step active, set assignee. In a real build this
  // shells out to the agent CLI.
  const onDispatch = (task, agentId) => {
    const now = 'just now';
    setTasks(ts => ts.map(t => {
      if (t.id !== task.id) return t;
      const newHistory = [...(t.history || []),
        { at: now, kind: 'claimed', actor: 'dispatcher', agent: agentId },
        { at: now, kind: 'started', actor: agentId, note: t.steps[0]?.mode + ' step' },
      ];
      // Find first pending step; if none, reset the last failed step to active
      // (this handles requeued beads whose steps all finished or failed).
      let activated = false;
      let newSteps = t.steps.map(s => {
        if (!activated && s.status === 'pending') { activated = true; return {...s, status: 'active'}; }
        return s;
      });
      if (!activated) {
        // No pending — flip the last failed step back to active, or activate the last done one.
        const lastFailedIdx = (() => { for (let i = newSteps.length - 1; i >= 0; i--) if (newSteps[i].status === 'failed') return i; return -1; })();
        const idx = lastFailedIdx >= 0 ? lastFailedIdx : newSteps.length - 1;
        if (idx >= 0) newSteps = newSteps.map((s, i) => i === idx ? {...s, status: 'active'} : s);
      }
      return {
        ...t,
        column: 'running',
        assignee: agentId,
        history: newHistory,
        lastActivity: now,
        steps: newSteps,
        requeued: false,
        nowPlaying: { action: 'claiming worktree…', since: 0, kind: 'tool' },
      };
    }));
  };

  const onMove = (id, colId, beforeId, position) => {
    // Snapshot current tasks for undo (tasks is the current state via closure).
    const snapshot = tasks;
    const movingTask = tasks.find(t => t.id === id);

    // Toast for irreversible "Dispatch now" and "Approve & close" actions.
    if (movingTask) {
      if (colId === 'running' && (movingTask.column === 'scheduled' || movingTask.column === 'backlog')) {
        addToast(`Dispatched ${id} · Undo`, () => setTasks(snapshot));
      } else if (colId === 'done') {
        addToast(`Closed ${id} · Undo`, () => setTasks(snapshot));
      }
    }

    setTasks(ts => {
      const moving = ts.find(t => t.id === id);
      if (!moving) return ts;
      const without = ts.filter(t => t.id !== id);
      const updated = {...moving, column: colId};

      if (!beforeId || position === 'append') {
        // Append: place at the end of that column.
        const others = without;
        // Insert after the last task in colId (or at the end if column is empty).
        const lastIdxInCol = (() => {
          for (let i = others.length - 1; i >= 0; i--) if (others[i].column === colId) return i;
          return -1;
        })();
        const out = [...others];
        out.splice(lastIdxInCol + 1, 0, updated);
        return out;
      }

      // Insert above/below a specific card
      const targetIdx = without.findIndex(t => t.id === beforeId);
      if (targetIdx === -1) return [...without, updated];
      const insertIdx = position === 'above' ? targetIdx : targetIdx + 1;
      const out = [...without];
      out.splice(insertIdx, 0, updated);
      return out;
    });
  };
  const onUpdate = (next) => {
    setTasks(ts => ts.map(t => t.id === next.id ? next : t));
  };

  const filtered = tasks
    .filter(t => repoFilter === 'all' || t.repo === repoFilter)
    .filter(t => filter
      ? (t.title.toLowerCase().includes(filter.toLowerCase()) ||
         t.id.toLowerCase().includes(filter.toLowerCase()))
      : true);

  return (
    <div className="app">
      <header className="topbar">
        <div className="brand">
          <div className="brand-mark">
            {/* Bead-chain logo. Three beads, the first filled with accent. */}
            <svg viewBox="0 0 36 16" aria-hidden="true">
              <circle cx="6"  cy="8" r="5" fill="var(--accent)"/>
              <circle cx="18" cy="8" r="5" fill="none" stroke="var(--ink)" strokeWidth="1.5"/>
              <circle cx="30" cy="8" r="5" fill="none" stroke="var(--ink)" strokeWidth="1.5"/>
              <line x1="11" y1="8" x2="13" y2="8" stroke="var(--ink)" strokeWidth="1.5"/>
              <line x1="23" y1="8" x2="25" y2="8" stroke="var(--ink)" strokeWidth="1.5"/>
            </svg>
          </div>
          <div className="brand-text">
            <span className="brand-name">muster</span>
            <span className="brand-sub">spec-driven agent orchestrator</span>
          </div>
        </div>

        <nav className="topnav">
          <button className={view === 'board' ? 'active' : ''} onClick={() => setView('board')}>Board</button>
          <button className={view === 'lifecycle' ? 'active' : ''} onClick={() => setView('lifecycle')}>Lifecycle</button>
          <button className={view === 'orchestrator' ? 'active' : ''} onClick={() => setView('orchestrator')}>Orchestrator</button>
          <button className={view === 'repos' ? 'active' : ''} onClick={() => setView('repos')}>Repos</button>
          <button className={view === 'providers' ? 'active' : ''} onClick={() => setView('providers')}>Providers</button>
          <button className={view === 'deps' ? 'active' : ''} onClick={() => setView('deps')}>Deps</button>
          <button className={view === 'modes' ? 'active' : ''} onClick={() => setView('modes')}>Modes</button>
        </nav>

        <div className="topbar-tools">
          <button className="topbar-cmd-btn" onClick={() => setCmdPalOpen(true)} title="Command palette (⌘K)">
            <span className="topbar-cmd-icon">⌘</span>
            <span>bd</span>
            <kbd>K</kbd>
          </button>
          <button className="topbar-tool-btn" onClick={() => setFormulasOpen(true)} title="Browse workflow formulas">
            <span className="topbar-tool-icon">ƒ</span>
            Formulas
          </button>
          <button className="topbar-tool-btn" onClick={() => setMemoriesOpen(true)} title="Agent memories (bd remember)">
            <span className="topbar-tool-icon">◦</span>
            Memories
          </button>
          <DoltChip />
          <RepoFilterChip
            value={repoFilter}
            onChange={setRepoFilter}
            onManage={() => setView('repos')}
          />
          <input
            className="search"
            placeholder="Search beads…"
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
          />
          <button className="btn btn-primary new-bead" onClick={() => setComposerOpen(true)}>＋ New bead</button>
        </div>
      </header>

      {view === 'board' && (
        <div className="capacity-row">
          <CapacityStrip capacity={capacity} />
          <div className="meta-right">
            <span className="meta-dot live"></span>
            <strong>{tasks.filter(t => t.column === 'running').length}</strong> running
            <span className="meta-sep">·</span>
            <strong>{tasks.filter(t => t.column === 'scheduled').length}</strong> queued
            <span className="meta-sep">·</span>
            <strong>{tasks.filter(t => t.column === 'review').length}</strong> need review
          </div>
        </div>
      )}
      {view === 'board' && <NowPlayingRail tasks={tasks} onOpen={onOpen} />}

      <main className="main">
        {view === 'board' && (
          <KanbanBoard tasks={filtered} onOpen={onOpen} onMove={onMove} centerLayout={tweaks.centerLayout} />
        )}
        {view === 'lifecycle' && (
          <div className="page-pad lifecycle-pad">
            <LifecycleView tasks={tasks} onOpen={onOpen} onDispatch={onDispatch} />
          </div>
        )}
        {view === 'orchestrator' && (
          <div className="page-pad">
            <div className="page-h">
              <h1 className="page-title">Orchestrator</h1>
              <p className="page-sub">Global rules, capacity limits, and dispatcher policy. Constitution applies to every bead.</p>
            </div>
            <OrchestratorView
              capacity={capacity}
              setCapacity={setCapacity}
              constitution={constitution}
              setConstitution={setConstitution}
            />
          </div>
        )}
        {view === 'providers' && (
          <div className="page-pad">
            <div className="page-h">
              <h1 className="page-title">Providers</h1>
              <p className="page-sub">Each configured LLM provider — quota, rate limits, native modes, and connection details.</p>
            </div>
            <ProvidersView onAddProvider={() => setAddProviderOpen(true)} />
          </div>
        )}
        {view === 'deps' && (
          <DepGraph tasks={filtered} onOpen={onOpen} />
        )}
        {view === 'modes' && (
          <div className="page-pad">
            <div className="page-h">
              <h1 className="page-title">Modes</h1>
              <p className="page-sub">Each provider's native operation modes. Step the dispatcher picks lives in the bead's chain.</p>
            </div>
            <ModesView />
          </div>
        )}
        {view === 'repos' && (
          <div className="page-pad">
            <div className="page-h">
              <h1 className="page-title">Repos</h1>
              <p className="page-sub">Working trees Muster is watching. Each one has its own <code>.beads/</code> store; embedded and server modes are both supported.</p>
            </div>
            <ReposView
              onAddRepo={() => setAddRepoOpen(true)}
              onPickRepo={(id) => { setRepoFilter(id); setView('board'); }}
              repoFilter={repoFilter}
            />
          </div>
        )}
      </main>

      <TaskDrawer task={openTask} onClose={onClose} onUpdate={onUpdate} onMove={onMove} onOpenBead={(id) => setOpenTaskId(id)} constitution={constitution} onEditConstitution={() => setView('orchestrator')} />

      {composerOpen && (
        <NewBeadComposer
          onClose={() => setComposerOpen(false)}
          onCreate={(t) => setTasks(ts => [t, ...ts])}
        />
      )}

      {addProviderOpen && (
        <AddProviderModal
          onClose={() => setAddProviderOpen(false)}
          onAdd={(p) => { /* in a real build, persist */ }}
        />
      )}

      {addRepoOpen && (
        <AddRepoModal
          onClose={() => setAddRepoOpen(false)}
          onAdd={(repo) => {
            // Push the new repo into the shared data store so the filter chip,
            // board badges, and routing all see it without a reload.
            window.MUSTER_DATA.REPOS = [...window.MUSTER_DATA.REPOS, repo];
            setRepos(window.MUSTER_DATA.REPOS);
            setRepoFilter(repo.id);
            setView('repos');
          }}
        />
      )}

      {tweaks.mobilePreview && (
        <div className="mobile-overlay" onClick={(e) => { if (e.target.classList.contains('mobile-overlay')) setTweak('mobilePreview', false); }}>
          <button className="mobile-close" onClick={() => setTweak('mobilePreview', false)}>× close</button>
          <MobilePreview tasks={tasks} onOpen={onOpen} />
        </div>
      )}

      <CommandPalette
        open={cmdPalOpen}
        onClose={() => setCmdPalOpen(false)}
        onAction={(action, item) => {
          if (action === 'composer') setComposerOpen(true);
          if (action === 'lifecycle') setView('lifecycle');
          if (action === 'drawer' && tasks.length) setOpenTaskId(tasks[0].id);
        }}
      />

      <MemoriesPanel open={memoriesOpen} onClose={() => setMemoriesOpen(false)} />
      <FormulasPanel open={formulasOpen} onClose={() => setFormulasOpen(false)} />

      <ToastContainer toasts={toasts} onDismiss={dismissToast} />

      <TweaksPanel title="Tweaks">
        <TweakSection label="Layout">
          <TweakSelect
            label="Center pane"
            value={tweaks.centerLayout}
            onChange={(v) => setTweak('centerLayout', v)}
            options={[
              { value: 'split',    label: 'Split — Running | Review' },
              { value: 'stack',    label: 'Stack — Running over Review' },
              { value: 'dominant', label: 'Dominant — Running 2× Review' },
              { value: 'tabs',     label: 'Tabs — one at a time' },
            ]}
          />
          <TweakRadio
            label="Density"
            value={tweaks.density}
            onChange={(v) => setTweak('density', v)}
            options={['cozy', 'medium', 'compact']}
          />
        </TweakSection>
        <TweakSection label="Appearance">
          <TweakColor
            label="Accent"
            value={tweaks.accent}
            onChange={(v) => setTweak('accent', v)}
            options={['#D97757', '#3B82F6', '#10B981', '#8B5CF6', '#E11D48', '#0F172A']}
          />
        </TweakSection>
        <TweakSection label="Preview">
          <TweakToggle
            label="Mobile companion"
            value={tweaks.mobilePreview}
            onChange={(v) => setTweak('mobilePreview', v)}
          />
        </TweakSection>
      </TweaksPanel>
    </div>
  );
}

ReactDOM.createRoot(document.getElementById('root')).render(
  <AppErrorBoundary>
    <App />
  </AppErrorBoundary>
);
