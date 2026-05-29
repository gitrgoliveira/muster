// Repos view — register working trees with Muster. Each repo is a directory
// that contains a `.beads/` folder. Muster probes that folder to figure out
// whether Beads is running in **embedded** mode (direct read of
// `.beads/beads.db`) or **server** mode (Dolt SQL server on a local port).
// Either way, existing issues are pulled in and surfaced as beads on the
// board.

const {useState: useStateR, useEffect: useEffectR, useRef: useRefR} = React;

// Single-glyph icons drawn inline as SVG so they stay crisp at any scale and
// don't depend on an icon font. Each returns an SVG node sized to fit a 14px
// run of text.
function RepoGlyph({kind, size=14}) {
  const stroke = 'currentColor';
  const sw = 1.4;
  if (kind === 'folder') return (
    <svg viewBox="0 0 16 16" width={size} height={size} fill="none" stroke={stroke} strokeWidth={sw}>
      <path d="M2 4.5a1 1 0 0 1 1-1h3l1.5 1.5H13a1 1 0 0 1 1 1V12a1 1 0 0 1-1 1H3a1 1 0 0 1-1-1V4.5Z"/>
    </svg>
  );
  if (kind === 'db') return (
    <svg viewBox="0 0 16 16" width={size} height={size} fill="none" stroke={stroke} strokeWidth={sw}>
      <ellipse cx="8" cy="4" rx="5" ry="1.8"/>
      <path d="M3 4v4c0 1 2.2 1.8 5 1.8s5-.8 5-1.8V4"/>
      <path d="M3 8v4c0 1 2.2 1.8 5 1.8s5-.8 5-1.8V8"/>
    </svg>
  );
  if (kind === 'server') return (
    <svg viewBox="0 0 16 16" width={size} height={size} fill="none" stroke={stroke} strokeWidth={sw}>
      <rect x="2.2" y="3" width="11.6" height="4" rx="1"/>
      <rect x="2.2" y="9" width="11.6" height="4" rx="1"/>
      <circle cx="4.5" cy="5" r=".55" fill={stroke} stroke="none"/>
      <circle cx="4.5" cy="11" r=".55" fill={stroke} stroke="none"/>
    </svg>
  );
  if (kind === 'sync') return (
    <svg viewBox="0 0 16 16" width={size} height={size} fill="none" stroke={stroke} strokeWidth={sw}>
      <path d="M3 8a5 5 0 0 1 9-3"/>
      <path d="M13 8a5 5 0 0 1-9 3"/>
      <path d="M12 2v3h-3"/>
      <path d="M4 14v-3h3"/>
    </svg>
  );
  if (kind === 'detach') return (
    <svg viewBox="0 0 16 16" width={size} height={size} fill="none" stroke={stroke} strokeWidth={sw}>
      <path d="M6 10 3.5 12.5a2 2 0 0 1-2.8-2.8L3.2 7.2"/>
      <path d="M10 6l2.5-2.5a2 2 0 0 1 2.8 2.8L12.8 8.8"/>
      <path d="M6.5 9.5l3-3"/>
    </svg>
  );
  if (kind === 'plus') return (
    <svg viewBox="0 0 16 16" width={size} height={size} fill="none" stroke={stroke} strokeWidth={sw}>
      <path d="M8 3v10M3 8h10"/>
    </svg>
  );
  return null;
}

// ─── List page ──────────────────────────────────────────────────────────────

function ReposView({onAddRepo, onPickRepo, repoFilter}) {
  const {REPOS, TASKS} = window.MUSTER_DATA;

  return (
    <div className="repos-view">
      <header className="repos-toolbar">
        <div className="repos-toolbar-meta">
          <span className="repos-meta-count">{REPOS.length} repos</span>
          <span className="meta-sep">·</span>
          <span className="repos-meta-sum">
            {REPOS.reduce((n, r) => n + Object.values(r.counts).reduce((a, b) => a + b, 0), 0)} beads discovered
          </span>
          <span className="meta-sep">·</span>
          <span className="repos-meta-mode">
            {REPOS.filter(r => r.beadsMode === 'embedded').length} embedded ·{' '}
            {REPOS.filter(r => r.beadsMode === 'server').length} server
          </span>
        </div>
        <button className="btn btn-primary" onClick={onAddRepo}>
          <RepoGlyph kind="plus" size={12} /> Add repo
        </button>
      </header>

      <div className="repo-grid">
        {REPOS.map(r => (
          <RepoCard key={r.id} repo={r} tasks={TASKS} onPick={() => onPickRepo?.(r.id)} active={repoFilter === r.id} />
        ))}
        <button className="repo-card repo-card-add" onClick={onAddRepo}>
          <div className="repo-add-mark"><RepoGlyph kind="plus" size={18} /></div>
          <div className="repo-add-title">Add a repo</div>
          <div className="repo-add-sub">point at a directory · embedded or server-mode beads will be auto-detected</div>
        </button>
      </div>

      <footer className="repos-foot">
        <h4>How discovery works</h4>
        <ol>
          <li>
            <strong>Probe.</strong> Muster scans the directory for a <code>.beads/</code> folder
            and reads <code>.beads/config.toml</code> to determine the operating mode.
          </li>
          <li>
            <strong>Embedded.</strong> If no SQL server is configured, Muster opens
            <code>.beads/beads.db</code> directly (Dolt is single-writer, so dispatches from this
            repo coordinate through a file lock). No external process needed.
          </li>
          <li>
            <strong>Server.</strong> If a <code>[dolt.server]</code> stanza is present, Muster
            connects to that host:port instead. Multiple Muster instances can attach to the
            same server.
          </li>
          <li>
            <strong>Hydrate.</strong> Every issue, sub-bead, comment, and run-log line is
            streamed into the board. Schema migrations from older Beads versions are
            offered as <code>bd migrate</code>, not run automatically.
          </li>
        </ol>
      </footer>
    </div>
  );
}

function RepoCard({repo, tasks, onPick, active}) {
  const total = Object.values(repo.counts).reduce((a, b) => a + b, 0);
  const live  = repo.counts.running + repo.counts.review;

  return (
    <article className={'repo-card ' + (active ? 'is-active' : '') + ' status-' + repo.status}>
      <header className="repo-card-head">
        <div className="repo-card-titleblock">
          <h3 className="repo-card-name">
            <RepoGlyph kind="folder" />
            {repo.name}
            {repo.isDefault && <span className="repo-default-tag">default</span>}
          </h3>
          <code className="repo-card-path">{repo.path}</code>
        </div>
        <div className="repo-card-status">
          <span className={'repo-status-dot status-' + repo.status} aria-hidden="true"></span>
          <span className="repo-status-text">{repo.status}</span>
        </div>
      </header>

      <div className="repo-card-stripe">
        <div className="repo-stripe-item">
          <span className="repo-stripe-label">VCS</span>
          <span className="repo-stripe-value">
            <span className="repo-vcs-tag" data-vcs={repo.vcs}>{repo.vcs}</span>
            <span className="repo-stripe-sub">{repo.vcsBranch}</span>
          </span>
        </div>
        <div className="repo-stripe-item">
          <span className="repo-stripe-label">Beads</span>
          <span className="repo-stripe-value">
            {repo.beadsMode === 'embedded'
              ? <><RepoGlyph kind="db" /> embedded</>
              : <><RepoGlyph kind="server" /> server</>}
            <span className="repo-stripe-sub">v{repo.detected.beadsVersion}</span>
          </span>
        </div>
        <div className="repo-stripe-item">
          <span className="repo-stripe-label">Location</span>
          <code className="repo-stripe-loc">
            {repo.beadsMode === 'embedded'
              ? `${repo.dbPath} · ${repo.dbSize}`
              : `${repo.dbHost}:${repo.dbPort}/${repo.dbName}`}
          </code>
        </div>
        <div className="repo-stripe-item">
          <span className="repo-stripe-label">Synced</span>
          <span className="repo-stripe-value"><RepoGlyph kind="sync" /> {repo.lastSync}</span>
        </div>
      </div>

      <div className="repo-card-counts">
        <BeadCount label="backlog"   n={repo.counts.backlog}   tone="neutral" />
        <BeadCount label="scheduled" n={repo.counts.scheduled} tone="amber"   />
        <BeadCount label="running"   n={repo.counts.running}   tone="live"    pulse={repo.counts.running > 0} />
        <BeadCount label="review"    n={repo.counts.review}    tone="violet"  />
        <BeadCount label="done"      n={repo.counts.done}      tone="green"   />
        <div className="repo-count-total"><strong>{total}</strong> total · <em>{live} live</em></div>
      </div>

      {repo.note && (
        <div className="repo-card-note" role="note">⚠ {repo.note}</div>
      )}

      <footer className="repo-card-foot">
        <button className="btn btn-ghost btn-tiny" onClick={(e) => { e.stopPropagation(); onPick(); }}>
          {active ? '✓ filtering board' : 'Filter board'}
        </button>
        <button className="btn btn-ghost btn-tiny" disabled={repo.status === 'scanning'}>
          <RepoGlyph kind="sync" size={11} /> Re-scan
        </button>
        <button className="btn btn-ghost btn-tiny">
          Reveal in shell
        </button>
        <span className="repo-card-spacer"></span>
        <button className="btn btn-ghost btn-tiny repo-detach">
          <RepoGlyph kind="detach" size={11} /> Detach
        </button>
      </footer>
    </article>
  );
}

function BeadCount({label, n, tone, pulse}) {
  return (
    <div className={'bead-count tone-' + tone + (pulse ? ' pulse' : '')}>
      <span className="bead-count-n">{n}</span>
      <span className="bead-count-label">{label}</span>
    </div>
  );
}

// ─── Add-repo modal ─────────────────────────────────────────────────────────

// Mock probe — in a real build this hits POST /api/v1/repos/probe and gets
// back the parsed config + schema info. Here we synthesise plausible output
// based on the path string so the UI can be demoed.
function mockProbe(path) {
  // Force one specific path to demo server-mode detection.
  const isServer = /api|backend|server/i.test(path);
  const isLegacy = /legacy|old/i.test(path);
  return new Promise((resolve, reject) => {
    setTimeout(() => {
      if (path.trim().length < 2) return reject(new Error('Path looks empty.'));
      if (/nope|missing/i.test(path)) return reject(new Error('No .beads/ directory found at this path. Run `bd init` first.'));
      resolve({
        path,
        name: path.split('/').filter(Boolean).pop() || 'repo',
        vcs: /jj|jujutsu/i.test(path) ? 'jj' : 'git',
        vcsBranch: 'main',
        beadsMode: isServer ? 'server' : 'embedded',
        dbPath: '.beads/beads.db',
        dbSize: isServer ? null : (Math.random() * 4 + 0.3).toFixed(1) + ' MB',
        dbHost: isServer ? '127.0.0.1' : null,
        dbPort: isServer ? 3306 : null,
        dbName: isServer ? (path.split('/').filter(Boolean).pop() || 'repo').replace(/-/g, '_') + '_beads' : null,
        detected: {
          beadsVersion: isLegacy ? '0.8.4' : '0.9.1',
          schemaVersion: isLegacy ? 3 : 4,
          lastWrite: ['just now', '3m ago', '12m ago', '1h ago'][Math.floor(Math.random() * 4)],
        },
        beadCount: Math.floor(Math.random() * 80) + 4,
        beadsByStatus: {
          open: Math.floor(Math.random() * 30) + 2,
          inProgress: Math.floor(Math.random() * 4),
          review: Math.floor(Math.random() * 3),
          closed: Math.floor(Math.random() * 50) + 1,
        },
        needsMigrate: isLegacy,
      });
    }, 700);
  });
}

const SUGGESTED_PATHS = [
  '~/code/octane-mobile',
  '~/code/octane-billing-api',
  '~/work/internal-tools',
  '~/oss/legacy-monolith',
];

function AddRepoModal({onClose, onAdd}) {
  const [path, setPath] = useStateR('');
  const [probing, setProbing] = useStateR(false);
  const [probe, setProbe] = useStateR(null);
  const [error, setError] = useStateR(null);
  const inputRef = useRefR(null);

  useEffectR(() => {
    inputRef.current?.focus();
    const esc = (e) => { if (e.key === 'Escape') onClose(); };
    window.addEventListener('keydown', esc);
    return () => window.removeEventListener('keydown', esc);
  }, []);

  const doProbe = async () => {
    setProbing(true); setError(null); setProbe(null);
    try {
      const r = await mockProbe(path);
      setProbe(r);
    } catch (e) {
      setError(e.message);
    } finally {
      setProbing(false);
    }
  };

  const submit = () => {
    if (!probe) return;
    onAdd({
      id: probe.name.replace(/[^a-z0-9]+/gi, '-').toLowerCase() + '-' + Math.random().toString(36).slice(2, 5),
      name: probe.name,
      path: probe.path,
      vcs: probe.vcs,
      vcsBranch: probe.vcsBranch,
      beadsMode: probe.beadsMode,
      dbPath: probe.dbPath,
      dbSize: probe.dbSize,
      dbHost: probe.dbHost,
      dbPort: probe.dbPort,
      dbName: probe.dbName,
      detected: probe.detected,
      status: 'connected',
      lastSync: 'just now',
      counts: {
        backlog:   probe.beadsByStatus.open - probe.beadsByStatus.inProgress - probe.beadsByStatus.review,
        scheduled: 0,
        running:   probe.beadsByStatus.inProgress,
        review:    probe.beadsByStatus.review,
        done:      probe.beadsByStatus.closed,
      },
      note: probe.needsMigrate ? `schema v${probe.detected.schemaVersion} → v4 — run "bd migrate" to upgrade` : null,
    });
    onClose();
  };

  return (
    <div className="composer-scrim" onClick={onClose}>
      <div className="composer addrepo" onClick={(e) => e.stopPropagation()}>
        <header className="composer-head">
          <h2>Add a repo</h2>
          <button className="icon-btn" onClick={onClose}>×</button>
        </header>

        <div className="composer-body">
          <p className="addprov-help">
            Point at any directory containing a <code>.beads/</code> store. Muster reads it directly —
            no need to launch a Dolt server, and existing beads stay where they are.
          </p>

          <div className="addrepo-pathrow">
            <label className="composer-label">Path</label>
            <div className="addrepo-pathwrap">
              <input
                ref={inputRef}
                className="addrepo-input"
                placeholder="~/code/my-repo"
                value={path}
                onChange={(e) => { setPath(e.target.value); setProbe(null); setError(null); }}
                onKeyDown={(e) => { if (e.key === 'Enter' && path) doProbe(); }}
              />
              <button className="btn btn-ghost addrepo-browse" type="button">Browse…</button>
              <button
                className="btn btn-primary addrepo-probe"
                onClick={doProbe}
                disabled={!path || probing}
              >
                {probing ? 'Probing…' : 'Probe'}
              </button>
            </div>
            <div className="addrepo-suggests">
              <span className="addrepo-suggests-label">try:</span>
              {SUGGESTED_PATHS.map(p => (
                <button key={p} className="addrepo-suggest" onClick={() => { setPath(p); setProbe(null); setError(null); }}>
                  {p}
                </button>
              ))}
            </div>
          </div>

          <div className="addrepo-result">
            {probing && (
              <div className="addrepo-probing">
                <div className="addrepo-probing-rows">
                  <div className="addrepo-probing-row">
                    <span className="addrepo-probing-glyph">▸</span>
                    <code>stat {path}/.beads/</code>
                    <span className="addrepo-probing-tag">ok</span>
                  </div>
                  <div className="addrepo-probing-row">
                    <span className="addrepo-probing-glyph">▸</span>
                    <code>cat {path}/.beads/config.toml</code>
                    <span className="addrepo-probing-tag">parsing…</span>
                  </div>
                  <div className="addrepo-probing-row dim">
                    <span className="addrepo-probing-glyph">·</span>
                    <code>SELECT count(*) FROM issues</code>
                    <span className="addrepo-probing-tag">pending</span>
                  </div>
                </div>
              </div>
            )}

            {error && (
              <div className="addrepo-error">
                <strong>Probe failed.</strong>
                <p>{error}</p>
              </div>
            )}

            {probe && !probing && (
              <div className="addrepo-found">
                <header className="addrepo-found-head">
                  <span className="addrepo-found-name">
                    <RepoGlyph kind="folder" /> {probe.name}
                  </span>
                  <span className="addrepo-found-mode" data-mode={probe.beadsMode}>
                    {probe.beadsMode === 'embedded'
                      ? <><RepoGlyph kind="db" size={12} /> embedded mode</>
                      : <><RepoGlyph kind="server" size={12} /> server mode</>}
                  </span>
                </header>

                <div className="addrepo-detect-grid">
                  <div><span>VCS</span><strong>{probe.vcs} · {probe.vcsBranch}</strong></div>
                  <div><span>Beads</span><strong>v{probe.detected.beadsVersion} · schema v{probe.detected.schemaVersion}</strong></div>
                  <div><span>Last write</span><strong>{probe.detected.lastWrite}</strong></div>
                  {probe.beadsMode === 'embedded' ? (
                    <div><span>Database</span><strong>{probe.dbPath} · {probe.dbSize}</strong></div>
                  ) : (
                    <div><span>Server</span><strong>{probe.dbHost}:{probe.dbPort} / {probe.dbName}</strong></div>
                  )}
                  <div className="addrepo-detect-wide">
                    <span>Issues</span>
                    <strong>
                      {probe.beadCount} total
                      <span className="addrepo-detect-breakdown">
                        — {probe.beadsByStatus.open} open · {probe.beadsByStatus.inProgress} in-progress · {probe.beadsByStatus.review} review · {probe.beadsByStatus.closed} closed
                      </span>
                    </strong>
                  </div>
                </div>

                {probe.beadsMode === 'embedded' && (
                  <p className="addrepo-mode-note">
                    Muster will open <code>{probe.dbPath}</code> in single-writer mode.
                    Dispatches from this repo coordinate through a file lock — no Dolt server required.
                  </p>
                )}
                {probe.beadsMode === 'server' && (
                  <p className="addrepo-mode-note">
                    Muster will connect to the existing Dolt SQL server at
                    <code> {probe.dbHost}:{probe.dbPort}</code>. Multiple Muster instances can attach to the
                    same server safely.
                  </p>
                )}
                {probe.needsMigrate && (
                  <div className="addrepo-warning">
                    Schema v{probe.detected.schemaVersion} predates the current Beads release (v4).
                    Muster will surface beads in read-only mode until you run
                    <code> bd migrate</code> in this repo.
                  </div>
                )}
              </div>
            )}
          </div>
        </div>

        <footer className="composer-foot">
          <div className="composer-foot-left">
            {probe
              ? <span>Ready to attach <strong>{probe.name}</strong> · {probe.beadCount} beads will be discovered</span>
              : <span>Paste a path or pick one above</span>}
          </div>
          <div className="composer-foot-right">
            <button className="btn btn-ghost" onClick={onClose}>Cancel</button>
            <button className="btn btn-primary" onClick={submit} disabled={!probe}>
              Attach repo →
            </button>
          </div>
        </footer>
      </div>
    </div>
  );
}

// ─── Top-bar repo filter ────────────────────────────────────────────────────

function RepoFilterChip({value, onChange, onManage}) {
  const {REPOS} = window.MUSTER_DATA;
  const [open, setOpen] = useStateR(false);
  const ref = useRefR(null);

  useEffectR(() => {
    if (!open) return;
    const click = (e) => { if (!ref.current?.contains(e.target)) setOpen(false); };
    window.addEventListener('mousedown', click);
    return () => window.removeEventListener('mousedown', click);
  }, [open]);

  const current = value === 'all' ? null : REPOS.find(r => r.id === value);
  const label = current ? current.name : 'All repos';

  return (
    <div className="repo-chip-wrap" ref={ref}>
      <button className={'repo-chip ' + (open ? 'is-open' : '')} onClick={() => setOpen(v => !v)}>
        <RepoGlyph kind="folder" size={12} />
        <span className="repo-chip-label">{label}</span>
        <span className="repo-chip-count">{REPOS.length}</span>
        <span className="repo-chip-caret">▾</span>
      </button>
      {open && (
        <div className="repo-chip-menu" role="menu">
          <button
            className={'repo-chip-item ' + (value === 'all' ? 'on' : '')}
            onClick={() => { onChange('all'); setOpen(false); }}
          >
            <span className="repo-chip-item-name">All repos</span>
            <span className="repo-chip-item-sub">no filter</span>
          </button>
          <div className="repo-chip-sep"></div>
          {REPOS.map(r => {
            const tot = Object.values(r.counts).reduce((a, b) => a + b, 0);
            return (
              <button
                key={r.id}
                className={'repo-chip-item ' + (value === r.id ? 'on' : '')}
                onClick={() => { onChange(r.id); setOpen(false); }}
              >
                <span className="repo-chip-item-name">
                  <RepoGlyph kind="folder" size={11} /> {r.name}
                  <span className="repo-chip-item-mode" data-mode={r.beadsMode}>{r.beadsMode}</span>
                </span>
                <span className="repo-chip-item-sub">{r.path} · {tot} beads</span>
              </button>
            );
          })}
          <div className="repo-chip-sep"></div>
          <button className="repo-chip-item repo-chip-item-add" onClick={() => { onManage(); setOpen(false); }}>
            <RepoGlyph kind="plus" size={11} /> Manage / add repos…
          </button>
        </div>
      )}
    </div>
  );
}

Object.assign(window, { ReposView, AddRepoModal, RepoFilterChip, RepoGlyph });
