// Lifecycle view — open/close tracking + dispatcher.
// Mounts at /lifecycle (top-nav). Three regions:
//   1. Pulse strip — counts of today's lifecycle events + current state.
//   2. Ready-to-dispatch — bd-ready beads with one-click [Dispatch] action.
//   3. Activity timeline — chronological flat feed of every bead's history.

const {useState: useStateL, useMemo: useMemoL} = React;

// Roughly "today" — beads whose history contains a `Mon` or `today` token.
// In a real build this would parse real timestamps.
const TODAY_TOKENS = ['Mon', 'today', 'just now', 'recent'];
function isTodayAt(at) {
  return TODAY_TOKENS.some(tok => (at || '').includes(tok));
}

function bdReady(t) {
  if (t.column !== 'backlog' && t.column !== 'scheduled') return false;
  if (t.blockedBy?.length) return false;
  if (t.gates?.some(g => g.status === 'waiting')) return false;
  return true;
}

function PulseStat({label, value, sub, tone, glyph}) {
  return (
    <div className={'pulse-stat tone-' + (tone || 'neutral')}>
      <div className="pulse-row">
        <span className="pulse-glyph">{glyph}</span>
        <span className="pulse-value">{value}</span>
      </div>
      <div className="pulse-label">{label}</div>
      {sub && <div className="pulse-sub">{sub}</div>}
    </div>
  );
}

function ReadyDispatchItem({task, onDispatch, onOpen, suggestAgentFn}) {
  const {AGENTS} = window.MUSTER_DATA;
  const fn = suggestAgentFn || window.MUSTER_DATA.suggestAgent;
  const suggestedId = fn(task);
  const A = AGENTS.find(a => a.id === suggestedId);
  const isPinned = !!task.pinnedAgent;
  const blockedDeps = task.blockedBy?.filter(id => !!id) || [];

  return (
    <div className={'rd-item type-edge-' + task.type}>
      <div className="rd-main" onClick={() => onOpen(task)}>
        <div className="rd-head">
          <PriBadge n={task.priority} />
          <span className="rd-id">{task.id}</span>
          <span className="rd-est">{task.estimate}</span>
          {task.formula && <span className="rd-formula">ƒ {task.formula}</span>}
        </div>
        <div className="rd-title">{task.title}</div>
        <div className="rd-sub">
          <span className="rd-step-count">{task.steps.length} steps</span>
          {task.skills?.length > 0 && (
            <>
              <span className="rd-sep">·</span>
              <span className="rd-skills">{task.skills.length} skills</span>
            </>
          )}
          {blockedDeps.length > 0 && (
            <>
              <span className="rd-sep">·</span>
              <span className="rd-blocked">blocked by {blockedDeps.join(', ')}</span>
            </>
          )}
        </div>
      </div>
      <div className="rd-actions">
        <span className="rd-suggested" style={{['--agent-color']: A?.color}}>
          <span className="rd-sug-label">{isPinned ? 'pinned' : 'suggests'}</span>
          {isPinned && <span className="rd-pin-icon">⊣</span>}
          <span className="agent-mono" style={{['--agent-color']: A?.color}}>{A?.mono}</span>
        </span>
        <button
          className="btn btn-primary rd-dispatch"
          onClick={(e) => { e.stopPropagation(); onDispatch(task, A?.id); }}
          disabled={blockedDeps.length > 0}
          title={blockedDeps.length > 0 ? 'Blocked — resolve deps first' : `Dispatch to ${A?.name}`}
        >
          Dispatch ↗
        </button>
      </div>
    </div>
  );
}

function ActivityRow({event, task, onOpen}) {
  const {EVENT_KINDS, AGENTS} = window.MUSTER_DATA;
  const K = EVENT_KINDS[event.kind] || { glyph: '·', label: event.kind, tone: 'neutral' };
  const isAgent = ['claude', 'gemini', 'opencode', 'codex'].includes(event.actor);
  const A = isAgent ? AGENTS.find(a => a.id === event.actor) : null;
  const agentForKind = event.agent ? AGENTS.find(a => a.id === event.agent) : null;
  return (
    <li className={'act-row kind-' + event.kind + ' tone-' + K.tone} onClick={() => onOpen(task)}>
      <span className="act-when">{event.at}</span>
      <span className="act-kind"><span className="act-glyph">{K.glyph}</span> {K.label}</span>
      <span className="act-bead">
        <span className="act-bead-id">{task.id}</span>
        <span className="act-bead-title">{task.title}</span>
      </span>
      <span className="act-actor">
        {A
          ? <span className="agent-mono" style={{['--agent-color']: A.color}}>{A.mono}</span>
          : <span className="act-human">{event.actor}</span>}
        {agentForKind && (
          <>
            <span className="act-arrow">→</span>
            <span className="agent-mono" style={{['--agent-color']: agentForKind.color}}>{agentForKind.mono}</span>
          </>
        )}
      </span>
      {event.note && <span className="act-note">{event.note}</span>}
    </li>
  );
}

function LifecycleView({tasks, onOpen, onDispatch}) {
  const [activityScope, setActivityScope] = useStateL('all'); // all | today

  // Pulse counts.
  const pulse = useMemoL(() => {
    const open       = tasks.filter(t => t.column !== 'done').length;
    const running    = tasks.filter(t => t.column === 'running').length;
    const review     = tasks.filter(t => t.column === 'review').length;
    const closed     = tasks.filter(t => t.column === 'done').length;
    const ready      = tasks.filter(bdReady).length;
    const blocked    = tasks.filter(t => t.column !== 'done' && t.blockedBy?.length > 0).length;
    const requeued   = tasks.filter(t => t.requeued).length;

    // Today's opens/closes from history.
    let openedToday = 0, closedToday = 0;
    tasks.forEach(t => {
      t.history?.forEach(h => {
        if (h.kind === 'opened' && isTodayAt(h.at)) openedToday++;
        if (h.kind === 'closed' && isTodayAt(h.at)) closedToday++;
      });
    });

    return { open, running, review, closed, ready, blocked, requeued, openedToday, closedToday };
  }, [tasks]);

  // Respect pinnedAgent if set
  function suggestForDispatch(task) {
    if (task.pinnedAgent) return task.pinnedAgent;
    return window.MUSTER_DATA.suggestAgent(task);
  }
  const readyList = useMemoL(() =>
    tasks.filter(bdReady).sort((a, b) => a.priority - b.priority).slice(0, 8),
  [tasks]);

  // Flat activity feed (event + originating task), newest first.
  const feed = useMemoL(() => {
    const all = [];
    tasks.forEach(t => t.history?.forEach((h, idx) => all.push({event: h, task: t, idx})));
    // Sort with a heuristic: events from later-indexed positions per task come later;
    // within same position, group by `at` token ordering using a coarse week-day order.
    const order = { 'Fri': 0, 'Sat': 1, 'Sun': 2, 'Mon': 3, 'today': 4, 'just now': 5, 'recent': 6 };
    const score = (at) => {
      for (const k of Object.keys(order)) if (at?.includes(k)) return order[k];
      return 3;
    };
    all.sort((a, b) => {
      const da = score(a.event.at);
      const db = score(b.event.at);
      if (da !== db) return db - da;
      // Same day: later idx wins (assume history is appended in order).
      return b.idx - a.idx;
    });
    const scoped = activityScope === 'today'
      ? all.filter(x => isTodayAt(x.event.at))
      : all;
    return scoped.slice(0, 60);
  }, [tasks, activityScope]);

  return (
    <div className="lifecycle">
      <header className="lc-head">
        <div className="lc-head-text">
          <h1 className="lc-title">Lifecycle</h1>
          <p className="lc-sub">Beads opened, dispatched, closed — and what wants your attention now.</p>
        </div>
      </header>

      <section className="pulse-strip">
        <PulseStat tone="amber"   glyph="↑" value={pulse.openedToday} label="opened today"     sub={`${pulse.open} open total`} />
        <PulseStat tone="accent"  glyph="↗" value={pulse.ready}       label="ready to dispatch" sub="bd ready" />
        <PulseStat tone="live"    glyph="●" value={pulse.running}     label="in flight"          sub="now playing" />
        <PulseStat tone="violet"  glyph="◐" value={pulse.review}      label="needs review"       sub="awaits you" />
        <PulseStat tone="green"   glyph="✓" value={pulse.closedToday} label="closed today"       sub={`${pulse.closed} all-time`} />
        <PulseStat tone="rose"    glyph="◊" value={pulse.blocked}     label="blocked"            sub="dep or gate" />
        <PulseStat tone="amber"   glyph="↻" value={pulse.requeued}    label="requeued"           sub="needs split or smaller scope" />
      </section>

      <div className="lc-grid">
        <section className="lc-panel lc-ready">
          <header className="lc-panel-head">
            <h3 className="lc-panel-title">Ready to dispatch</h3>
            <span className="lc-panel-meta">
              <code>bd ready</code> · sorted by priority · {readyList.length} shown
            </span>
          </header>
          {readyList.length === 0 && (
            <div className="lc-empty">Queue is clear — everything ready has been dispatched.</div>
          )}
          <div className="rd-list">
            {readyList.map(t => (
      <ReadyDispatchItem key={t.id} task={t} onDispatch={onDispatch} onOpen={onOpen} suggestAgentFn={suggestForDispatch} />
            ))}
          </div>
        </section>

        <section className="lc-panel lc-activity">
          <header className="lc-panel-head">
            <h3 className="lc-panel-title">Activity</h3>
            <div className="lc-activity-tabs">
              <button className={activityScope === 'all'   ? 'active' : ''} onClick={() => setActivityScope('all')}>All</button>
              <button className={activityScope === 'today' ? 'active' : ''} onClick={() => setActivityScope('today')}>Today</button>
            </div>
          </header>
          {feed.length === 0 && <div className="lc-empty">Nothing recent.</div>}
          <ul className="act-list">
            {feed.map((row, i) => (
              <ActivityRow key={i} event={row.event} task={row.task} onOpen={onOpen} />
            ))}
          </ul>
        </section>
      </div>
    </div>
  );
}

Object.assign(window, { LifecycleView, bdReady });
