// Orchestrator config, Providers (quota + keys), and Modes (per-agent operation modes).

const {useState: useStateO} = React;

function CapacityRow({agent, capacity, onChange}) {
  const A = window.MUSTER_DATA.AGENTS.find(a => a.id === agent.id);
  const cap = capacity.find(c => c.agent === agent.id);
  const pct = cap ? Math.min(100, (cap.running / cap.limit) * 100) : 0;
  return (
    <div className="cap-row" style={{['--agent-color']: A.color}}>
      <div className="cap-agent">
        <span className="agent-mono lg">{A.mono}</span>
        <div className="cap-agent-text">
          <div className="cap-agent-name">{A.name}</div>
          <div className="cap-agent-sub">{cap.running} running · {cap.queued} queued</div>
        </div>
      </div>
      <div className="cap-bar-wrap">
        <div className="cap-bar">
          <div className="cap-bar-fill" style={{width: pct + '%'}}></div>
          {Array.from({length: cap.limit}).map((_, i) => (
            <div key={i} className={'cap-slot ' + (i < cap.running ? 'filled' : '')}></div>
          ))}
        </div>
      </div>
      <div className="cap-limit">
        <button className="step-btn" onClick={() => onChange(agent.id, Math.max(1, cap.limit - 1))}>−</button>
        <span className="cap-limit-num">{cap.limit}</span>
        <button className="step-btn" onClick={() => onChange(agent.id, cap.limit + 1)}>+</button>
        <span className="cap-limit-label">parallel max</span>
      </div>
    </div>
  );
}

function OrchestratorView({capacity, setCapacity, constitution, setConstitution}) {
  const {AGENTS} = window.MUSTER_DATA;
  const setLimit = (agentId, limit) => {
    setCapacity(capacity.map(c => c.agent === agentId ? {...c, limit} : c));
  };

  const lineCount = constitution.split('\n').length;
  const wordCount = constitution.trim().split(/\s+/).filter(Boolean).length;

  return (
    <div className="orch">
      <section className="orch-section">
        <header className="orch-section-head">
          <h3>Constitution</h3>
          <p>Rules and conventions prepended to every bead's prompt. Treated as <em>system-level instructions</em> by all agents. Edit here and the next dispatch picks it up.</p>
        </header>
        <div className="constitution-wrap">
          <textarea
            className="constitution"
            value={constitution}
            onChange={(e) => setConstitution(e.target.value)}
            spellCheck={false}
            rows={14}
          />
          <div className="constitution-meta">
            <span>{lineCount} lines · {wordCount} words · ~{Math.ceil(wordCount * 1.3)} tokens</span>
            <span className="constitution-applied">applied to all {window.MUSTER_DATA.TASKS.length} beads</span>
          </div>
        </div>
      </section>

      <section className="orch-section">
        <header className="orch-section-head">
          <h3>Agent capacity</h3>
          <p>How many beads each provider may run in parallel. The dispatcher holds tasks in <em>Scheduled</em> until a slot frees up.</p>
        </header>
        <div className="cap-table">
          {AGENTS.map(a => <CapacityRow key={a.id} agent={a} capacity={capacity} onChange={setLimit} />)}
        </div>
      </section>

      <section className="orch-section">
        <header className="orch-section-head">
          <h3>Failure policy</h3>
          <p>What happens when a bead trips on token budget, tool failure, or hits a wall.</p>
        </header>
        <div className="policy-table">
          <label className="policy-row">
            <input type="checkbox" defaultChecked />
            <div>
              <div className="policy-title">Requeue on token exhaustion</div>
              <div className="policy-desc">Return the bead to <em>Scheduled</em> at the front of its priority bucket. Carry forward run log + worktree.</div>
            </div>
          </label>
          <label className="policy-row">
            <input type="checkbox" defaultChecked />
            <div>
              <div className="policy-title">Auto-split if budget &gt; 80% used and progress &lt; 50%</div>
              <div className="policy-desc">Spawn child beads via Beads, link with <code>blocks</code>, halt parent.</div>
            </div>
          </label>
          <label className="policy-row">
            <input type="checkbox" />
            <div>
              <div className="policy-title">Escalate to a larger model on second failure</div>
              <div className="policy-desc">e.g. OpenCode → Claude Code if a step fails twice.</div>
            </div>
          </label>
          <label className="policy-row">
            <input type="checkbox" defaultChecked />
            <div>
              <div className="policy-title">Always run a VCS step before review</div>
              <div className="policy-desc">Runs <code>jj describe</code> (or <code>git commit</code>) on the bead's worktree before moving to review. Guarantees reviewers see a clean, named change.</div>
            </div>
          </label>
        </div>
      </section>

      <RoutesSection />
    </div>
  );
}

function RoutesSection() {
  const {ROUTES, HYDRATE_REPOS} = window.MUSTER_DATA;
  const [testInput, setTestInput] = useStateO('');
  const [addPattern, setAddPattern] = useStateO('');
  const [addTarget, setAddTarget] = useStateO('');
  const [hydrating, setHydrating] = useStateO(null);

  function testRoute(title) {
    if (!title.trim()) return null;
    const sorted = [...ROUTES].sort((a, b) => b.priority - a.priority);
    for (const r of sorted) {
      if (r.pattern === '*') return r;
      const pat = r.pattern.replace('/**', '').replace('/*', '');
      if (title.toLowerCase().includes(pat)) return r;
    }
    return ROUTES[ROUTES.length - 1];
  }
  const matched = testInput ? testRoute(testInput) : null;

  return (
    <section className="orch-section">
      <header className="orch-section-head">
        <h3>Multi-repo routing</h3>
        <p>Rules in <code>.beads/routes.jsonl</code> that auto-route new beads to the correct repository. Checked in priority order — first match wins.</p>
      </header>

      <div className="routes-block">
        <div className="routes-table">
          <div className="routes-header">
            <span>Pattern</span>
            <span>Target repo</span>
            <span>Priority</span>
            <span></span>
          </div>
          {ROUTES.map((r, i) => (
            <div key={i} className={'routes-row ' + (r.pattern === '*' ? 'is-default' : '')}>
              <code className="route-pattern">{r.pattern}</code>
              <span className="route-target">{r.target}</span>
              <span className="route-priority">{r.priority}</span>
              <button className="route-remove" title="Remove route">×</button>
            </div>
          ))}
          <div className="routes-add-row">
            <input
              className="route-add-input"
              placeholder="frontend/**"
              value={addPattern}
              onChange={e => setAddPattern(e.target.value)}
            />
            <input
              className="route-add-input"
              placeholder="frontend-repo"
              value={addTarget}
              onChange={e => setAddTarget(e.target.value)}
            />
            <button
              className="btn btn-ghost route-add-btn"
              disabled={!addPattern.trim() || !addTarget.trim()}
              onClick={() => { setAddPattern(''); setAddTarget(''); }}
            >
              Add route
            </button>
          </div>
        </div>

        <div className="routes-test">
          <div className="routes-test-label">Test routing — <code>bd routes test</code></div>
          <div className="routes-test-row">
            <input
              className="routes-test-input"
              placeholder='e.g. "Fix frontend button alignment"'
              value={testInput}
              onChange={e => setTestInput(e.target.value)}
            />
            {matched && (
              <div className="routes-test-result">
                <span className="routes-test-arrow">→</span>
                <code className="routes-test-target">{matched.target}</code>
                <span className="routes-test-via">via <code>{matched.pattern}</code></span>
              </div>
            )}
          </div>
        </div>

        <div className="hydrate-block">
          <div className="hydrate-label">
            Hydration
            <span className="hydrate-sub">bd hydrate — pull related issues from sibling repos</span>
          </div>
          <div className="hydrate-repos">
            {HYDRATE_REPOS.map(r => (
              <div key={r.id} className="hydrate-repo">
                <span className="hydrate-repo-name">{r.id}</span>
                <span className="hydrate-repo-meta">
                  {r.ahead > 0 ? <strong>{r.ahead} new</strong> : 'up to date'} · {r.lastSync}
                </span>
                <button
                  className={'btn btn-ghost hydrate-btn ' + (hydrating === r.id ? 'is-syncing' : '')}
                  onClick={() => {
                    setHydrating(r.id);
                    setTimeout(() => setHydrating(null), 1800);
                  }}
                >
                  {hydrating === r.id ? 'pulling…' : 'bd hydrate'}
                </button>
              </div>
            ))}
          </div>
        </div>
      </div>
    </section>
  );
}

// ─── Providers ────────────────────────────────────────────────────────────
function ProvidersView({onAddProvider}) {
  const {AGENTS, TASKS} = window.MUSTER_DATA;

  const usagePerAgent = AGENTS.map(a => {
    const running = TASKS.filter(t => t.column === 'running' && t.steps.some(s => s.agent === a.id && s.status === 'active')).length;
    const total = TASKS.reduce((n, t) => n + t.steps.filter(s => s.agent === a.id).length, 0);
    return { id: a.id, running, total };
  });

  return (
    <div className="providers">
      {AGENTS.map(a => {
        const u = usagePerAgent.find(x => x.id === a.id);
        const dailyPct = a.quota.selfHosted ? 0 : Math.min(100, (a.quota.used / a.quota.limit) * 100);
        const monthlyPct = a.monthly.selfHosted ? 0 : Math.min(100, (a.monthly.used / a.monthly.limit) * 100);
        const danger = dailyPct >= 85;

        return (
          <article key={a.id} className="provider-card" style={{['--agent-color']: a.color}}>
            <header className="provider-head">
              <div className="provider-id">
                <span className="agent-mono lg">{a.mono}</span>
                <div>
                  <div className="provider-name">{a.name}</div>
                  <div className="provider-plan">{a.plan}</div>
                </div>
              </div>
              <div className="provider-status">
                <span className={'provider-status-dot ' + (a.quota.selfHosted ? 'local' : 'live')}></span>
                <span>{a.quota.selfHosted ? 'local' : 'connected'}</span>
              </div>
            </header>

            <div className="provider-quota-block">
              <div className="provider-quota-head">
                <span className="provider-quota-label">{a.quota.window}</span>
                <span className={'provider-quota-val' + (danger ? ' danger' : '')}>
                  {a.quota.selfHosted ? 'unmetered' : fmtQuota(a.quota)}
                </span>
              </div>
              {!a.quota.selfHosted && (
                <div className="provider-quota-bar">
                  <div className="provider-quota-bar-fill" style={{width: dailyPct + '%'}} data-danger={danger}></div>
                </div>
              )}
              <div className="provider-quota-meta">
                {!a.quota.selfHosted && <span>resets in {a.quota.resetIn}</span>}
                <span>rate limit · {a.rateLimit}</span>
              </div>
            </div>

            <div className="provider-quota-block">
              <div className="provider-quota-head">
                <span className="provider-quota-label">this month</span>
                <span className="provider-quota-val">
                  {a.monthly.selfHosted ? '—' : fmtQuota(a.monthly)}
                </span>
              </div>
              {!a.monthly.selfHosted && (
                <div className="provider-quota-bar small">
                  <div className="provider-quota-bar-fill" style={{width: monthlyPct + '%'}}></div>
                </div>
              )}
            </div>

            <div className="provider-grid">
              <div className="provider-grid-cell">
                <span className="provider-grid-label">parallel</span>
                <span className="provider-grid-val">{a.parallel} max</span>
              </div>
              <div className="provider-grid-cell">
                <span className="provider-grid-label">running now</span>
                <span className="provider-grid-val">{u.running}</span>
              </div>
              <div className="provider-grid-cell">
                <span className="provider-grid-label">steps assigned</span>
                <span className="provider-grid-val">{u.total}</span>
              </div>
              <div className="provider-grid-cell">
                <span className="provider-grid-label">modes</span>
                <span className="provider-grid-val">{a.modes.length}</span>
              </div>
            </div>

            <div className="provider-cli-block">
              {a.kind === 'cli' && (
                <div className="provider-cli-row">
                  <span className="provider-grid-label">CLI binary</span>
                  <code className="provider-key">{a.binary}</code>
                  <span className="provider-version">v{a.version}</span>
                </div>
              )}
              {a.kind === 'sdk' && (
                <div className="provider-cli-row">
                  <span className="provider-grid-label">SDK package</span>
                  <code className="provider-key">{a.sdkPackage}</code>
                  <span className={'provider-version ' + (a.sdkLinked ? 'sdk-linked' : 'sdk-missing')}>
                    {a.sdkLinked ? 'linked · v' + a.sdkVersion : 'not linked'}
                  </span>
                </div>
              )}
              {a.kind === 'api' && (
                <div className="provider-cli-row">
                  <span className="provider-grid-label">base URL</span>
                  <code className="provider-key">{a.baseURL}</code>
                </div>
              )}
              <div className="provider-cli-row">
                <span className="provider-grid-label">auth</span>
                <span className={'provider-auth-status status-' + a.auth.status}>
                  {a.auth.status === 'logged-in'      && <>● logged in as <strong>{a.auth.as}</strong></>}
                  {a.auth.status === 'logged-out'     && <>○ logged out</>}
                  {a.auth.status === 'expired'        && <>⚠ token expired</>}
                  {a.auth.status === 'no-auth-needed' && (a.kind === 'sdk' ? <>— in-process, no auth</> : <>— no auth required</>)}
                </span>
                {a.auth.status === 'logged-in'  && <button className="link-btn">re-auth</button>}
                {a.auth.status === 'logged-out' && <button className="link-btn">login →</button>}
              </div>
              {a.kind === 'cli' && (
                <div className="provider-cli-row">
                  <span className="provider-grid-label">tmux</span>
                  <span className="provider-tmux-note">sessions at <code>muster/{a.id}/…</code></span>
                  <button className="link-btn">attach →</button>
                </div>
              )}
            </div>

            <div className="provider-modes-row">
              <span className="provider-grid-label">native modes</span>
              <div className="provider-mode-pills">
                {a.modes.map(m => (
                  <span key={m.id} className="provider-mode-pill" data-mode={m.id} title={m.desc + ' · ' + m.cli}>
                    <span>{m.icon}</span> {m.name}
                  </span>
                ))}
              </div>
            </div>

            <footer className="provider-foot">
              <button className="btn btn-ghost">Adjust parallel</button>
              <button className="btn btn-ghost">Test invocation</button>
              <button className="btn btn-ghost">View activity log</button>
            </footer>
          </article>
        );
      })}

      <article className="provider-card provider-add" onClick={onAddProvider}>
        <span className="provider-add-plus">＋</span>
        <strong>Add another provider</strong>
        <span className="provider-add-sub">Install a CLI, or wire a direct API (OpenRouter, Anthropic API, Mistral, Groq, local Ollama…)</span>
      </article>
    </div>
  );
}

// ─── Modes (per-provider native operation modes) ──────────────────────────
function ModesView() {
  const {AGENTS} = window.MUSTER_DATA;
  const [selected, setSel] = useStateO(AGENTS[0].id);
  const A = AGENTS.find(a => a.id === selected);

  return (
    <div className="modes-view">
      <aside className="modes-sidebar">
        <div className="se-side-h">Providers</div>
        {AGENTS.map(a => (
          <button
            key={a.id}
            className={'modes-side-item ' + (selected === a.id ? 'active' : '')}
            onClick={() => setSel(a.id)}
            style={{['--agent-color']: a.color}}
          >
            <span className="agent-mono lg">{a.mono}</span>
            <div className="modes-side-text">
              <div className="modes-side-name">{a.name}</div>
              <div className="modes-side-sub">{a.modes.length} modes</div>
            </div>
          </button>
        ))}
      </aside>

      <main className="modes-main" style={{['--agent-color']: A.color}}>
        <header className="modes-main-head">
          <span className="agent-mono xl">{A.mono}</span>
          <div>
            <h3 className="modes-main-title">{A.name}</h3>
            <p className="modes-main-sub">{A.plan} · {A.rateLimit}</p>
          </div>
        </header>

        <div className="mode-cards">
          {A.modes.map(m => (
            <article key={m.id} className="mode-card" data-mode={m.id}>
              <header className="mode-card-head">
                <span className="mode-card-icon">{m.icon}</span>
                <h4 className="mode-card-name">{m.name}</h4>
              </header>
              <p className="mode-card-desc">{m.desc}</p>
              <code className="mode-card-cli">{m.cli}</code>
            </article>
          ))}
        </div>

        <section className="modes-howto">
          <h4 className="se-block-h">When the dispatcher picks {A.name}</h4>
          <ol className="modes-howto-list">
            <li>The bead's step specifies <code>agent: '{A.id}'</code> and one of the modes above.</li>
            <li>The dispatcher checks {A.name}'s capacity ({A.parallel} parallel max) and quota.</li>
            <li>The step's prompt is wrapped with the active constitution and skill loadout, then sent.</li>
            <li>Output is captured to the bead's run log; worktree changes are diffed and exposed.</li>
          </ol>
        </section>
      </main>
    </div>
  );
}

Object.assign(window, { OrchestratorView, ProvidersView, ModesView });
