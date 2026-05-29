// Kanban board v2 — Beads-native layout.
//
// Layout:
//   ┌──────────────┬──────────────┬─────────────────┬───┐
//   │ Scheduled    │ Running      │ Ready for review│ D │
//   │ (top, lean)  │              │                 │ o │
//   ├──────────────┤              │                 │ n │
//   │ Backlog      │              │                 │ e │
//   │ (bottom)     │              │                 │ ▶ │
//   └──────────────┴──────────────┴─────────────────┴───┘
//
//   - Left col stacks Scheduled + Backlog (vertical flex, scrolls within each).
//   - Center splits Running and Review equally.
//   - Done collapses to a 56px rail; click chevron to expand to ~280px.

const { useState, useRef, useEffect, useMemo } = React;

function fmtTokens(n) {
  if (n >= 1_000_000) return (n / 1_000_000).toFixed(n >= 10_000_000 ? 0 : 1) + 'M';
  if (n >= 1000) return (n / 1000).toFixed(n >= 10000 ? 0 : 1) + 'k';
  return String(n);
}

function fmtQuota(q) {
  if (!q || q.selfHosted) return '—';
  if (q.unit === '$') return '$' + q.used.toFixed(2) + ' / $' + q.limit.toFixed(2);
  if (q.unit === 'tok') return fmtTokens(q.used) + ' / ' + fmtTokens(q.limit) + ' tok';
  if (q.unit === 'msg') return fmtTokens(q.used) + ' / ' + fmtTokens(q.limit) + ' msg';
  return q.used + ' / ' + q.limit;
}

function fmtElapsed(secs) {
  if (secs < 60) return secs + 's';
  if (secs < 3600) return Math.round(secs / 60) + 'm';
  return Math.round(secs / 3600) + 'h';
}

function ModeDot({ mode, size = 8 }) {
  return <span className="mode-dot" style={{ width: size, height: size }} data-mode={mode}></span>;
}

function AgentChip({ agent, compact = false, mode = null }) {
  const A = window.MUSTER_DATA.AGENTS.find((a) => a.id === agent);
  if (!A) return null;
  const tooltip =
  <div className="agent-tooltip" role="tooltip">
      <div className="agent-tooltip-head">
        <span className="agent-mono" style={{ ['--agent-color']: A.color }}>{A.mono}</span>
        <div>
          <div className="agent-tooltip-name">{A.name}</div>
          <div className="agent-tooltip-plan">{A.plan}</div>
        </div>
      </div>
      <div className="agent-tooltip-rows">
        <div className="agent-tooltip-row">
          <span>{A.quota.window}</span>
          <span>{fmtQuota(A.quota)}</span>
        </div>
        {!A.quota.selfHosted &&
      <div className="agent-tooltip-bar">
            <div className="agent-tooltip-bar-fill" style={{ width: Math.min(100, A.quota.used / A.quota.limit * 100) + '%', background: A.color }}></div>
          </div>
      }
        <div className="agent-tooltip-row muted">
          <span>resets in</span><span>{A.quota.resetIn}</span>
        </div>
        <div className="agent-tooltip-row muted">
          <span>rate limit</span><span>{A.rateLimit}</span>
        </div>
        <div className="agent-tooltip-row muted">
          <span>parallel</span><span>{A.parallel} max</span>
        </div>
      </div>
    </div>;

  return (
    <span className="agent-chip" style={{ ['--agent-color']: A.color }} tabIndex={0}>
      <span className="agent-mono">{A.mono}</span>
      {!compact && <span className="agent-name">{A.name}</span>}
      {mode && <span className="agent-mode-suffix">· {mode}</span>}
      {tooltip}
    </span>);

}

// Bead-chain step rail. Each step is a literal "bead" strung on a hairline thread.
// Sub-beads can hang underneath as smaller secondary beads.
function StepRail({ steps, compact = false, withSubBeads = null }) {
  return (
    <div className={'step-rail ' + (compact ? 'compact' : '')} role="list">
      <span className="step-thread" aria-hidden="true"></span>
      {steps.map((s, i) => {
        const A = window.MUSTER_DATA.AGENTS.find((a) => a.id === s.agent);
        return (
          <span
            key={i}
            role="listitem"
            className={'step-bead status-' + s.status}
            data-mode={s.mode}
            style={{ ['--agent-color']: A?.color || 'var(--ink-3)' }}
            title={`${i + 1}. ${A?.name} · ${s.mode}${s.note ? ' — ' + s.note : ''} (${s.status})`}>
          </span>);

      })}
      {withSubBeads && withSubBeads.length > 0 &&
      <span className="step-rail-sub" aria-hidden="true">
          {withSubBeads.slice(0, 6).map((b, i) =>
        <span key={i} className={'step-subbead status-' + b.status}></span>
        )}
          {withSubBeads.length > 6 && <span className="step-subbead-more">+{withSubBeads.length - 6}</span>}
        </span>
      }
    </div>);

}

function TokenBar({ used, budget }) {
  const pct = Math.min(100, used / budget * 100);
  const danger = pct >= 85;
  const warn = pct >= 70 && !danger;
  return (
    <div className="token-bar" title={`${fmtTokens(used)} / ${fmtTokens(budget)} tokens`}>
      <div className="token-bar-fill" style={{ width: pct + '%' }} data-danger={danger} data-warn={warn}></div>
    </div>);

}

function PriBadge({ n }) {
  const P = window.MUSTER_DATA.priMeta(n);
  return (
    <span className={'pri-digit pri-' + P.tone} title={`Priority ${n} · ${P.label}`}>{n}</span>);

}

function ReadyDot({ task }) {
  // Beads `bd ready`: open + no blocking deps + no waiting gates.
  if (task.column === 'done' || task.column === 'running' || task.column === 'review') return null;
  if (task.blockedBy?.length || task.gates?.some((g) => g.status === 'waiting')) {
    return <span className="ready-pill blocked" title={'Blocked — ' + (task.blockedBy?.length ? 'deps' : 'gate')}>blocked</span>;
  }
  if (task.ready) return <span className="ready-pill ready" title="bd ready · dispatchable">ready</span>;
  return null;
}

function GateChip({ gate }) {
  const icons = { human: '☻', timer: '◷', github: '⎇' };
  return (
    <span className={'gate-chip gate-' + gate.kind + ' status-' + gate.status} title={`${gate.kind} gate · ${gate.status}`}>
      <span className="gate-icon">{icons[gate.kind] || '◇'}</span>
      <span className="gate-label">{gate.label}</span>
    </span>);

}

// -----------------------------------------------------------------------------
// Cards — three flavours, each tuned to its column's job-to-be-done.
// -----------------------------------------------------------------------------

// Default card (Running + Review + Scheduled).
function Card({ task, onOpen, onDragStart, isDragging, onCardDragOver, dropIndicator, variant = 'full' }) {
  const activeStep = task.steps.find((s) => s.status === 'active') ||
  [...task.steps].reverse().find((s) => s.status === 'done');
  const activeAgent = activeStep?.agent;
  const isRunning = task.column === 'running';
  const isReview = task.column === 'review';

  return (
    <article
      className={'card v-' + variant + ' type-edge-' + task.type + ' ' + (
      isDragging ? 'is-dragging ' : '') + (
      task.requeued ? 'is-requeued ' : '') + (
      dropIndicator === 'above' ? 'drop-above ' : '') + (
      dropIndicator === 'below' ? 'drop-below ' : '')}
      draggable
      onDragStart={(e) => onDragStart(e, task)}
      onDragOver={(e) => onCardDragOver?.(e, task)}
      onClick={() => onOpen(task)}>
      
      <header className="card-head">
        <span className="card-id-row">
          <PriBadge n={task.priority} />
          <span className="card-id">{task.id}</span>
          <ReadyDot task={task} />
        </span>
        {task.discoveredFrom &&
        <span className="discovered-tag" title={`Discovered while working on ${task.discoveredFrom}`}>
            ↪ {task.discoveredFrom}
          </span>
        }
        {task.requeued && <span className="requeued-tag">requeued</span>}
      </header>

      <h3 className="card-title">{task.title}</h3>

      {task.labels?.length > 0 && variant !== 'lean' &&
      <div className="card-labels">
          {task.labels.slice(0, 3).map((l) =>
        <span key={l} className="label-chip">{l}</span>
        )}
          {task.labels.length > 3 && <span className="label-more">+{task.labels.length - 3}</span>}
        </div>
      }

      <StepRail steps={task.steps} withSubBeads={task.subBeads} />

      {isReview && task.reviewer &&
      <div className="review-line">
          {task.reviewer.comments > 0 ?
        <span className="review-comments">{task.reviewer.comments} {task.reviewer.comments === 1 ? 'comment' : 'comments'}</span> :
        <span className="review-clean">no comments · autoclose pending</span>}
          <span className="review-by">via</span>
          <AgentChip agent={task.reviewer.agent} compact />
        </div>
      }

      {!isRunning && !isReview && activeAgent &&
      <footer className="card-foot">
          <AgentChip agent={activeAgent} compact />
        </footer>
      }

      {task.gates?.some((g) => g.status === 'waiting') && variant === 'full' &&
      <div className="card-gates">
          {task.gates.filter((g) => g.status === 'waiting').map((g, i) =>
        <GateChip key={i} gate={g} />
        )}
        </div>
      }
    </article>);

}

// Mini-card for the Backlog — title + pri + type, no rail.
function MiniCard({ task, onOpen, onDragStart, isDragging, onCardDragOver, dropIndicator }) {
  return (
    <article
      className={'mini-card type-edge-' + task.type + ' ' + (
      isDragging ? 'is-dragging ' : '') + (
      dropIndicator === 'above' ? 'drop-above ' : '') + (
      dropIndicator === 'below' ? 'drop-below ' : '')}
      draggable
      onDragStart={(e) => onDragStart(e, task)}
      onDragOver={(e) => onCardDragOver?.(e, task)}
      onClick={() => onOpen(task)}>
      
      <div className="mini-head">
        <PriBadge n={task.priority} />
        <span className="mini-id">{task.id}</span>
        <ReadyDot task={task} />
        {task.discoveredFrom && <span className="mini-disc" title={`Discovered from ${task.discoveredFrom}`}>↪</span>}
      </div>
      <h4 className="mini-title">{task.title}</h4>
    </article>);

}

// Done — one-line tombstone in the collapsed rail, two lines when expanded.
function DoneRow({ task, onOpen, expanded }) {
  const lastStep = [...task.steps].reverse().find((s) => s.status === 'done');
  return (
    <div className="done-row" onClick={() => onOpen(task)} title={task.title}>
      <span className="done-check">✓</span>
      {expanded ?
      <>
          <div className="done-text">
            <div className="done-title">{task.title}</div>
            <div className="done-meta">
              <span className="done-id">{task.id}</span>
              <span className="done-sep">·</span>
              {lastStep && <AgentChip agent={lastStep.agent} compact />}
              <span className="done-sep">·</span>
              <span className="done-when">{task.closedAt || task.createdAt}</span>
            </div>
          </div>
        </> :

      <span className="done-id-only">{task.id.replace('bd-', '')}</span>
      }
    </div>);

}

// -----------------------------------------------------------------------------
// Columns
// -----------------------------------------------------------------------------

function Column({ col, tasks, onOpen, onDrop, draggingId, onDragStart, onDragOver, isHot, onCardDragOver, indicators, variant = 'cards', actionHint, collapsed = false, onToggleCollapsed, collapseAxis = 'v', repoFilter }) {
  const totalRunning = tasks.length;

  // Collapsed renderings — one per axis. Both still accept drops so a user
  // can drag a card onto a collapsed column and watch it expand.
  if (collapsed && collapseAxis === 'v') {
    return (
      <section
        className={'col col-collapsed-v tone-' + col.tone + (isHot ? ' is-hot' : '')}
        onDragOver={(e) => {e.preventDefault();onDragOver(col.id);}}
        onDrop={(e) => {e.preventDefault();onDrop(col.id);}}
        onClick={onToggleCollapsed}
        title={`Expand ${col.name}`}>
        
        <button className="col-vrail-toggle" aria-label={`Expand ${col.name}`}>
          <span className="col-vrail-arrow">‹</span>
          <span className="col-vrail-label">
            <span className="col-vrail-name">{col.name}</span>
            <span className="col-vrail-count">{totalRunning}</span>
          </span>
        </button>
        <div className="col-vrail-stack">
          {tasks.slice(0, 18).map((t) =>
          <span key={t.id} className={'col-vrail-pip type-edge-' + t.type} title={t.id + ' — ' + t.title}></span>
          )}
          {tasks.length > 18 && <span className="col-vrail-more">+{tasks.length - 18}</span>}
        </div>
      </section>);

  }
  if (collapsed && collapseAxis === 'h') {
    return (
      <section
        className={'col col-collapsed-h tone-' + col.tone + (isHot ? ' is-hot' : '')}
        onDragOver={(e) => {e.preventDefault();onDragOver(col.id);}}
        onDrop={(e) => {e.preventDefault();onDrop(col.id);}}
        onClick={onToggleCollapsed}
        title={`Expand ${col.name}`}>
        
        <button className="col-hbar" aria-label={`Expand ${col.name}`}>
          <span className="col-hbar-arrow">▾</span>
          <span className="col-hbar-name">{col.name}</span>
          <span className="col-hbar-hint">{actionHint}</span>
          <span className="col-hbar-stack">
            {tasks.slice(0, 12).map((t) =>
            <span key={t.id} className={'col-hbar-pip type-edge-' + t.type} title={t.id + ' — ' + t.title}></span>
            )}
            {tasks.length > 12 && <span className="col-hbar-more">+{tasks.length - 12}</span>}
          </span>
          <span className="col-hbar-count">{totalRunning}</span>
        </button>
      </section>);

  }

  return (
    <section
      className={'col tone-' + col.tone + (isHot ? ' is-hot' : '') + ' col-variant-' + variant}
      onDragOver={(e) => {e.preventDefault();onDragOver(col.id);}}
      onDrop={(e) => {e.preventDefault();onDrop(col.id);}}>
      
      <header className="col-head">
        <h2 className="col-name">{col.name}</h2>
        <span className="col-meta">
          <span className="col-count">{totalRunning}</span>
          {onToggleCollapsed &&
          <button
            className="col-collapse-btn"
            onClick={(e) => {e.stopPropagation();onToggleCollapsed();}}
            aria-label={`Collapse ${col.name}`}
            title={`Collapse ${col.name}`}>
            
              {collapseAxis === 'h' ? '▴' : '›'}
            </button>
          }
        </span>
      </header>
      <div className="col-body">
        {tasks.length === 0 && <div className="col-empty">—</div>}
        {tasks.map((t) => variant === 'mini' ?
        <MiniCard
          key={t.id}
          task={t}
          onOpen={onOpen}
          onDragStart={onDragStart}
          isDragging={t.id === draggingId}
          onCardDragOver={onCardDragOver}
          dropIndicator={indicators?.[t.id]} /> :


        <Card
          key={t.id}
          task={t}
          onOpen={onOpen}
          onDragStart={onDragStart}
          isDragging={t.id === draggingId}
          onCardDragOver={onCardDragOver}
          dropIndicator={indicators?.[t.id]}
          variant={variant === 'compact' ? 'compact' : 'full'} />

        )}
      </div>
    </section>);

}

// The collapsible Done rail.
// Collapsed state now matches the col-collapsed-v pattern used by Running & Review.
function DoneRail({ col, tasks, onOpen, expanded, onToggle, onDrop, onDragOver, isHot }) {
  if (!expanded) {
    return (
      <section
        className={'col col-collapsed-v tone-' + col.tone + (isHot ? ' is-hot' : '')}
        onDragOver={(e) => {e.preventDefault();onDragOver(col.id);}}
        onDrop={(e) => {e.preventDefault();onDrop(col.id);}}
        onClick={onToggle}
        title={`Expand ${col.name}`}>
        
        <button className="col-vrail-toggle" aria-label={`Expand ${col.name}`} data-comment-anchor="98a7a85eaf-button-402-7">
          <span className="col-vrail-arrow">‹</span>
          <span className="col-vrail-label">
            <span className="col-vrail-name">{col.name}</span>
            <span className="col-vrail-count">{tasks.length}</span>
          </span>
        </button>
        <div className="col-vrail-stack">
          {tasks.slice(0, 18).map((t) =>
          <span key={t.id} className={'col-vrail-pip type-edge-' + t.type} title={t.id + ' — ' + t.title}></span>
          )}
          {tasks.length > 18 && <span className="col-vrail-more">+{tasks.length - 18}</span>}
        </div>
      </section>);
  }

  return (
    <section
      className={'col-done tone-' + col.tone + ' expanded' + (isHot ? ' is-hot' : '')}
      onDragOver={(e) => {e.preventDefault();onDragOver(col.id);}}
      onDrop={(e) => {e.preventDefault();onDrop(col.id);}}>
      
      <button className="done-toggle" onClick={onToggle} aria-label="Collapse Done" data-comment-anchor="98a7a85eaf-button-402-7">
        <span className="done-toggle-arrow">›</span>
      </button>
      <header className="col-head">
        <h2 className="col-name">{col.name}</h2>
        <span className="col-meta">
          <span className="col-count">{tasks.length}</span>
        </span>
      </header>
      <div className="col-body done-body">
        {tasks.length === 0 && <div className="col-empty">archive is quiet</div>}
        {tasks.map((t) => <DoneRow key={t.id} task={t} onOpen={onOpen} expanded />)}
      </div>
    </section>);

}

// -----------------------------------------------------------------------------
// Board
// -----------------------------------------------------------------------------

function KanbanBoard({ tasks, onOpen, onMove, centerLayout = 'split' }) {
  const { COLUMNS } = window.MUSTER_DATA;
  const [draggingId, setDraggingId] = useState(null);
  const [hotColumn, setHotColumn] = useState(null);
  const [dropTarget, setDropTarget] = useState(null);
  const [doneExpanded, setDoneExpanded] = useState(false);
  const [centerTab, setCenterTab] = useState('running');
  // Per-column collapsed flags. Done is tracked separately via `doneExpanded`
  // to keep its existing rail behavior, but all other columns live here.
  const [collapsedCols, setCollapsedCols] = useState({});
  const toggleCol = (id) => setCollapsedCols((c) => ({ ...c, [id]: !c[id] }));

  const onDragStart = (e, task) => {
    setDraggingId(task.id);
    e.dataTransfer.effectAllowed = 'move';
    e.dataTransfer.setData('text/plain', task.id);
  };

  const onCardDragOver = (e, target) => {
    if (!draggingId || draggingId === target.id) return;
    e.preventDefault();
    e.stopPropagation();
    const rect = e.currentTarget.getBoundingClientRect();
    const before = e.clientY < rect.top + rect.height / 2;
    setDropTarget({ columnId: target.column, beforeId: target.id, position: before ? 'above' : 'below' });
    setHotColumn(target.column);
  };

  const onDrop = (colId) => {
    if (!draggingId) return;
    if (dropTarget && dropTarget.columnId === colId) {
      onMove(draggingId, colId, dropTarget.beforeId, dropTarget.position);
    } else {
      onMove(draggingId, colId, null, 'append');
    }
    setDraggingId(null);
    setHotColumn(null);
    setDropTarget(null);
  };

  const onDragOver = (colId) => {
    setHotColumn(colId);
    if (!dropTarget || dropTarget.columnId !== colId) setDropTarget(null);
  };

  useEffect(() => {
    const end = () => {setDraggingId(null);setHotColumn(null);setDropTarget(null);};
    window.addEventListener('dragend', end);
    return () => window.removeEventListener('dragend', end);
  }, []);

  const byColumn = useMemo(() => {
    const m = Object.fromEntries(COLUMNS.map((c) => [c.id, []]));
    tasks.forEach((t) => {if (m[t.column]) m[t.column].push(t);});
    return m;
  }, [tasks]);

  const indicators = {};
  if (dropTarget) indicators[dropTarget.beforeId] = dropTarget.position;

  const colMap = Object.fromEntries(COLUMNS.map((c) => [c.id, c]));

  const colCommon = (id) => ({
    onOpen,
    onDrop,
    onDragStart,
    draggingId,
    isHot: hotColumn === id && draggingId,
    onDragOver,
    onCardDragOver,
    indicators
  });

  // Center layout renderer.
  function renderCenter() {
    if (centerLayout === 'stack') {
      return (
        <div className={'col-center-stack' + (collapsedCols.running ? ' s-collapsed-running' : '') + (collapsedCols.review ? ' s-collapsed-review' : '')}>
          <Column col={colMap.running} tasks={byColumn.running} {...colCommon('running')} variant="cards" actionHint="cockpit" collapsed={collapsedCols.running} onToggleCollapsed={() => toggleCol('running')} collapseAxis="h" />
          <Column col={colMap.review} tasks={byColumn.review} {...colCommon('review')} variant="cards" actionHint="needs you" collapsed={collapsedCols.review} onToggleCollapsed={() => toggleCol('review')} collapseAxis="h" />
        </div>);

    }
    if (centerLayout === 'tabs') {
      const showCol = centerTab === 'running' ? colMap.running : colMap.review;
      const showTasks = centerTab === 'running' ? byColumn.running : byColumn.review;
      return (
        <div className="col-center-tabs">
          <div className="center-tabbar">
            <button
              className={'ctab tone-live ' + (centerTab === 'running' ? 'active' : '')}
              onClick={() => setCenterTab('running')}>
              
              <span className="ctab-dot ctab-dot-live"></span>
              Running
              <span className="ctab-count">{byColumn.running.length}</span>
            </button>
            <button
              className={'ctab tone-violet ' + (centerTab === 'review' ? 'active' : '')}
              onClick={() => setCenterTab('review')}>
              
              <span className="ctab-dot ctab-dot-violet"></span>
              Needs review
              <span className="ctab-count">{byColumn.review.length}</span>
            </button>
          </div>
          <Column col={showCol} tasks={showTasks} {...colCommon(showCol.id)} variant="cards" actionHint={null} />
        </div>);

    }
    // 'split' and 'dominant' both render two columns side-by-side; grid width is what differs.
    return (
      <>
        <Column col={colMap.running} tasks={byColumn.running} {...colCommon('running')} variant="cards" actionHint="cockpit" collapsed={collapsedCols.running} onToggleCollapsed={() => toggleCol('running')} collapseAxis="v" />
        <Column col={colMap.review} tasks={byColumn.review} {...colCommon('review')} variant="cards" actionHint="needs you" collapsed={collapsedCols.review} onToggleCollapsed={() => toggleCol('review')} collapseAxis="v" />
      </>);

  }

  // Dynamic grid template so collapsed columns visually shrink without
  // disturbing the layout primitives.
  const railSize = '44px';
  const leftWidth = collapsedCols.scheduled && collapsedCols.backlog ? railSize : '280px';
  const cR = collapsedCols.running ? railSize : 'minmax(0, 1fr)';
  const cRev = collapsedCols.review ? railSize : 'minmax(0, 1fr)';
  const cDom = collapsedCols.running ? railSize : 'minmax(0, 2fr)';
  const cDoneOpen = '300px';
  const cDoneShut = railSize;
  let gridCols;
  if (centerLayout === 'split') {
    gridCols = `${leftWidth} ${cR} ${cRev} ${doneExpanded ? cDoneOpen : cDoneShut}`;
  } else if (centerLayout === 'dominant') {
    gridCols = `${leftWidth} ${cDom} ${cRev} ${doneExpanded ? cDoneOpen : cDoneShut}`;
  } else {
    // stack and tabs collapse the center into a single grid cell
    gridCols = `${leftWidth} minmax(0, 1fr) ${doneExpanded ? cDoneOpen : cDoneShut}`;
  }

  const stackClass = 'col-stack' + (
  collapsedCols.scheduled ? ' s-collapsed-1' : '') + (
  collapsedCols.backlog ? ' s-collapsed-2' : '');

  return (
    <div
      className={'kanban ' + (doneExpanded ? 'done-open ' : 'done-shut ') + 'center-' + centerLayout}
      style={{ gridTemplateColumns: gridCols }}>
      
      {/* Left: Scheduled (top) + Backlog (bottom) stacked */}
      <div className={stackClass}>
        <Column
          col={colMap.scheduled}
          tasks={byColumn.scheduled}
          onOpen={onOpen}
          onDrop={onDrop}
          onDragStart={onDragStart}
          draggingId={draggingId}
          isHot={hotColumn === 'scheduled' && draggingId}
          onDragOver={onDragOver}
          onCardDragOver={onCardDragOver}
          indicators={indicators}
          variant="compact"
          actionHint="next to dispatch"
          collapsed={collapsedCols.scheduled}
          onToggleCollapsed={() => toggleCol('scheduled')}
          collapseAxis="h" />
        
        <Column
          col={colMap.backlog}
          tasks={byColumn.backlog}
          onOpen={onOpen}
          onDrop={onDrop}
          onDragStart={onDragStart}
          draggingId={draggingId}
          isHot={hotColumn === 'backlog' && draggingId}
          onDragOver={onDragOver}
          onCardDragOver={onCardDragOver}
          indicators={indicators}
          variant="mini"
          actionHint="grub-pile"
          collapsed={collapsedCols.backlog}
          onToggleCollapsed={() => toggleCol('backlog')}
          collapseAxis="h" />
        
      </div>

      {/* Center: Running + Review (layout depends on centerLayout) */}
      {renderCenter()}

      {/* Right: Done — collapsible rail */}
      <DoneRail
        col={colMap.done}
        tasks={byColumn.done}
        onOpen={onOpen}
        expanded={doneExpanded}
        onToggle={() => setDoneExpanded((v) => !v)}
        onDrop={onDrop}
        onDragOver={onDragOver}
        isHot={hotColumn === 'done' && draggingId} />
      
    </div>);

}

Object.assign(window, { KanbanBoard, AgentChip, ModeDot, StepRail, TokenBar, PriBadge, ReadyDot, GateChip, MiniCard, fmtTokens, fmtQuota, fmtElapsed });