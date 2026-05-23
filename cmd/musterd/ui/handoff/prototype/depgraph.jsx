// Dependency graph — visualizes blocks, discovered-from relationships between beads.

const { useState: useDG, useMemo: useMDG } = React;

const DG_NODE_W = 244;
const DG_NODE_H = 90;
const DG_LEVEL_GAP = 110;
const DG_NODE_GAP = 18;
const DG_PAD = 40;

function DepGraph({ tasks, onOpen }) {
  const { AGENTS, COLUMNS } = window.MUSTER_DATA;
  const [hovered, setHovered] = useDG(null);

  const { connected, independent, edges, positions, graphW, graphH } = useMDG(() => {
    const taskMap = Object.fromEntries(tasks.map(t => [t.id, t]));
    const edgeList = [];

    tasks.forEach(t => {
      (t.blocks || []).forEach(bid => {
        if (taskMap[bid]) edgeList.push({ from: t.id, to: bid, kind: 'blocks' });
      });
      if (t.discoveredFrom && taskMap[t.discoveredFrom]) {
        edgeList.push({ from: t.discoveredFrom, to: t.id, kind: 'discovered' });
      }
    });

    const connIds = new Set();
    edgeList.forEach(e => { connIds.add(e.from); connIds.add(e.to); });
    const conn = tasks.filter(t => connIds.has(t.id));
    const indep = tasks.filter(t => !connIds.has(t.id));

    // Topo levels via iterative propagation
    const levels = {};
    conn.forEach(t => { levels[t.id] = 0; });
    let changed = true;
    let safety = 0;
    while (changed && safety++ < 20) {
      changed = false;
      edgeList.forEach(e => {
        const nl = (levels[e.from] || 0) + 1;
        if (levels[e.to] !== undefined && nl > levels[e.to]) {
          levels[e.to] = nl;
          changed = true;
        }
      });
    }

    // Group by level, sort by priority within
    const byLevel = {};
    conn.forEach(t => {
      const l = levels[t.id] || 0;
      (byLevel[l] = byLevel[l] || []).push(t);
    });
    Object.values(byLevel).forEach(a => a.sort((x, y) => x.priority - y.priority));

    const maxLvl = Math.max(0, ...Object.keys(byLevel).map(Number));
    const maxInLvl = Math.max(1, ...Object.values(byLevel).map(a => a.length));
    const totalMaxH = maxInLvl * DG_NODE_H + (maxInLvl - 1) * DG_NODE_GAP;

    const pos = {};
    for (let l = 0; l <= maxLvl; l++) {
      const col = byLevel[l] || [];
      const colH = col.length * DG_NODE_H + (col.length - 1) * DG_NODE_GAP;
      const offY = (totalMaxH - colH) / 2;
      col.forEach((n, i) => {
        pos[n.id] = {
          x: DG_PAD + l * (DG_NODE_W + DG_LEVEL_GAP),
          y: DG_PAD + offY + i * (DG_NODE_H + DG_NODE_GAP),
        };
      });
    }

    return {
      connected: conn,
      independent: indep,
      edges: edgeList,
      positions: pos,
      graphW: Math.max(DG_PAD * 2 + (maxLvl + 1) * DG_NODE_W + maxLvl * DG_LEVEL_GAP, 500),
      graphH: Math.max(DG_PAD * 2 + totalMaxH, 260),
    };
  }, [tasks]);

  // Highlight connections on hover
  const { hlEdges, hlNodes } = useMDG(() => {
    if (!hovered) return { hlEdges: null, hlNodes: null };
    const es = new Set();
    const ns = new Set([hovered]);
    edges.forEach((e, i) => {
      if (e.from === hovered || e.to === hovered) { es.add(i); ns.add(e.from); ns.add(e.to); }
    });
    return { hlEdges: es, hlNodes: ns };
  }, [hovered, edges]);

  const colTint = { backlog: 'var(--ink-4)', scheduled: 'var(--amber)', running: 'var(--accent)', review: 'var(--violet)', done: 'var(--green)' };

  return (
    <div className="page-pad" style={{ maxWidth: 1280 }}>
      <div className="page-h">
        <h1 className="page-title">Dependencies</h1>
        <p className="page-sub">
          Bead dependency graph — <code style={{ fontFamily: 'var(--mono)', fontSize: 13, background: 'var(--paper-2)', padding: '1px 5px', borderRadius: 3 }}>blocks</code> and <code style={{ fontFamily: 'var(--mono)', fontSize: 13, background: 'var(--paper-2)', padding: '1px 5px', borderRadius: 3 }}>discovered-from</code> chains across the worktree.
        </p>
      </div>

      <div className="dg-legend">
        <span className="dg-legend-item">
          <svg width="24" height="8"><line x1="0" y1="4" x2="24" y2="4" stroke="var(--rose)" strokeWidth="2" /></svg>
          blocks
        </span>
        <span className="dg-legend-item">
          <svg width="24" height="8"><line x1="0" y1="4" x2="24" y2="4" stroke="var(--amber)" strokeWidth="2" strokeDasharray="5 3" /></svg>
          discovered from
        </span>
        <span className="dg-legend-sep">·</span>
        {COLUMNS.map(c =>
          <span key={c.id} className="dg-legend-col">
            <span className="dg-legend-dot" style={{ background: colTint[c.id] }}></span>
            {c.name}
          </span>
        )}
      </div>

      {connected.length > 0 && (
        <div className="dg-canvas" style={{ minHeight: graphH }}>
          <div className="dg-canvas-inner" style={{ width: graphW, height: graphH, position: 'relative' }}>
            <svg width={graphW} height={graphH} style={{ position: 'absolute', top: 0, left: 0, pointerEvents: 'none' }}>
              <defs>
                <marker id="ah-blocks" viewBox="0 0 10 8" refX="9" refY="4" markerWidth="7" markerHeight="5" orient="auto">
                  <path d="M 0 0 L 10 4 L 0 8 Z" fill="var(--rose)" />
                </marker>
                <marker id="ah-disc" viewBox="0 0 10 8" refX="9" refY="4" markerWidth="7" markerHeight="5" orient="auto">
                  <path d="M 0 0 L 10 4 L 0 8 Z" fill="var(--amber)" />
                </marker>
              </defs>
              {edges.map((e, i) => {
                const fp = positions[e.from], tp = positions[e.to];
                if (!fp || !tp) return null;
                const x1 = fp.x + DG_NODE_W, y1 = fp.y + DG_NODE_H / 2;
                const x2 = tp.x, y2 = tp.y + DG_NODE_H / 2;
                const dx = x2 - x1;
                const cp = Math.max(50, Math.abs(dx) * 0.35);
                const d = `M ${x1} ${y1} C ${x1 + cp} ${y1}, ${x2 - cp} ${y2}, ${x2} ${y2}`;
                const hl = hlEdges?.has(i);
                const dim = hlEdges && !hl;
                return <path key={i} d={d} fill="none"
                  stroke={e.kind === 'blocks' ? 'var(--rose)' : 'var(--amber)'}
                  strokeWidth={hl ? 2.5 : 1.5}
                  strokeDasharray={e.kind === 'discovered' ? '6 4' : 'none'}
                  opacity={dim ? 0.1 : hl ? 1 : 0.5}
                  markerEnd={`url(#ah-${e.kind === 'blocks' ? 'blocks' : 'disc'})`}
                  style={{ transition: 'opacity .15s' }}
                />;
              })}
            </svg>

            {connected.map(n => {
              const p = positions[n.id];
              if (!p) return null;
              const A = AGENTS.find(a => a.id === n.assignee);
              const dim = hlNodes && !hlNodes.has(n.id);
              return (
                <div key={n.id}
                  className={'dg-node type-edge-' + n.type + (dim ? ' is-dimmed' : '') + (hovered === n.id ? ' is-hovered' : '')}
                  style={{ left: p.x, top: p.y, width: DG_NODE_W, ['--col-tint']: colTint[n.column] }}
                  onClick={() => onOpen(n)}
                  onMouseEnter={() => setHovered(n.id)}
                  onMouseLeave={() => setHovered(null)}>
                  <div className="dg-node-head">
                    <PriBadge n={n.priority} />
                    <span className="dg-node-id">{n.id}</span>
                    <span className="dg-node-col" style={{ background: colTint[n.column] }}>{n.column}</span>
                  </div>
                  <div className="dg-node-title">{n.title}</div>
                  <div className="dg-node-foot">
                    {A && <span className="agent-mono" style={{ ['--agent-color']: A.color }}>{A.mono}</span>}
                    <StepRail steps={n.steps} compact />
                  </div>
                </div>
              );
            })}
          </div>
        </div>
      )}

      {independent.length > 0 && (
        <div className="dg-independent">
          <div className="dg-ind-head">Independent — no dependency edges <span className="dg-ind-count">{independent.length}</span></div>
          <div className="dg-ind-grid">
            {independent.map(n => (
              <div key={n.id} className={'dg-ind-card type-edge-' + n.type} onClick={() => onOpen(n)}>
                <div className="dg-ind-head-row">
                  <PriBadge n={n.priority} />
                  <span className="dg-node-id">{n.id}</span>
                  <span className="dg-node-col" style={{ background: colTint[n.column] }}>{n.column}</span>
                </div>
                <div className="dg-ind-title">{n.title}</div>
              </div>
            ))}
          </div>
        </div>
      )}

      {connected.length === 0 && independent.length === 0 && (
        <div className="empty-state">No beads to graph.</div>
      )}
    </div>
  );
}

Object.assign(window, { DepGraph });
