// New bead composer — opens from the "+ New bead" button.

const {useState: useStateC, useEffect: useEffectC} = React;

const TEMPLATES = [
  {
    id: 'speckit-flow',
    name: 'Speckit-driven',
    desc: 'Spec → plan → build → review → commit',
    steps: [
      { agent: 'claude', mode: 'plan',   skills: ['speckit'] },
      { agent: 'claude', mode: 'plan',   skills: [] },
      { agent: 'claude', mode: 'build',  skills: [] },
      { agent: 'gemini', mode: 'review', skills: [] },
      { agent: 'claude', mode: 'agent',  skills: ['jujutsu'] },
    ],
  },
  {
    id: 'plan-build',
    name: 'Plan + Build',
    desc: 'Skip Speckit; plan, build, review',
    steps: [
      { agent: 'claude', mode: 'plan',   skills: [] },
      { agent: 'claude', mode: 'build',  skills: [] },
      { agent: 'gemini', mode: 'review', skills: [] },
    ],
  },
  {
    id: 'autonomous',
    name: 'Autonomous',
    desc: 'Single agent, free-roaming',
    steps: [
      { agent: 'claude', mode: 'agent',  skills: [] },
    ],
  },
  {
    id: 'review-only',
    name: 'Review-only',
    desc: 'No writes — diff critique',
    steps: [
      { agent: 'claude', mode: 'review', skills: [] },
    ],
  },
];

function NewBeadComposer({onClose, onCreate}) {
  const {AGENTS, COLUMNS} = window.MUSTER_DATA;

  const [title, setTitle] = useStateC('');
  const [desc, setDesc] = useStateC('');
  const [priority, setPriority] = useStateC(2);
  const [type, setType] = useStateC('task');
  const [destination, setDestination] = useStateC('backlog');
  const [templateId, setTemplateId] = useStateC('speckit-flow');

  useEffectC(() => {
    const esc = (e) => { if (e.key === 'Escape') onClose(); };
    window.addEventListener('keydown', esc);
    return () => window.removeEventListener('keydown', esc);
  }, []);

  const template = TEMPLATES.find(t => t.id === templateId);

  const submit = () => {
    if (!title.trim()) return;
    const id = 'bd-' + Math.random().toString(16).slice(2, 6);
    onCreate({
      id,
      title: title.trim(),
      desc: desc.trim() || '—',
      type,
      labels: [],
      column: destination,
      priority,
      ready: true,
      branch: null,
      blocks: [],
      blockedBy: [],
      tokensUsed: 0,
      tokensBudget: 250_000,
      createdAt: 'just now',
      steps: template.steps.map(s => ({...s, status: 'pending'})),
    });
    onClose();
  };

  return (
    <div className="composer-scrim" onClick={onClose}>
      <div className="composer" onClick={(e) => e.stopPropagation()}>
        <header className="composer-head">
          <h2>New bead</h2>
          <button className="icon-btn" onClick={onClose}>×</button>
        </header>
        <div className="composer-body">
          <div className="composer-row">
            <label className="composer-label">Title</label>
            <input
              className="composer-input"
              placeholder="What should the agent do?"
              autoFocus
              value={title}
              onChange={(e) => setTitle(e.target.value)}
            />
          </div>

          <div className="composer-row">
            <label className="composer-label">Instructions</label>
            <textarea
              className="composer-textarea"
              rows={4}
              placeholder="Acceptance criteria, constraints, links to context…"
              value={desc}
              onChange={(e) => setDesc(e.target.value)}
            />
          </div>

          <div className="composer-row">
            <label className="composer-label">Chain template <span style={{color:'var(--ink-4)'}}>· edit individual steps from the drawer after creating</span></label>
            <div className="template-grid">
              {TEMPLATES.map(t => (
                <button
                  key={t.id}
                  className={'template-card ' + (templateId === t.id ? 'on' : '')}
                  onClick={() => setTemplateId(t.id)}
                >
                  <div className="template-name">{t.name}</div>
                  <div className="template-desc">{t.desc}</div>
                  <div className="template-rail">
                    {t.steps.map((s, i) => {
                      const A = AGENTS.find(a => a.id === s.agent);
                      return (
                        <span key={i} className="template-step" style={{['--agent-color']: A?.color}}>
                          <span className="template-step-agent">{A?.mono}</span>
                          <span className="template-step-mode">{s.mode}</span>
                        </span>
                      );
                    })}
                  </div>
                </button>
              ))}
            </div>
          </div>

          <div className="composer-row-pair">
            <div className="composer-row">
              <label className="composer-label">Type</label>
              <select className="composer-select" value={type} onChange={(e) => setType(e.target.value)}>
                <option value="feature">✦ feature</option>
                <option value="bug">✕ bug</option>
                <option value="task">☐ task</option>
                <option value="epic">⌘ epic</option>
                <option value="chore">⊹ chore</option>
              </select>
            </div>
            <div className="composer-row">
              <label className="composer-label">Priority <span style={{color:'var(--ink-4)'}}>· 0=crit, 4=icebox</span></label>
              <select className="composer-select" value={priority} onChange={(e) => setPriority(Number(e.target.value))}>
                <option value={0}>0 — critical, drop everything</option>
                <option value={1}>1 — high</option>
                <option value={2}>2 — normal</option>
                <option value={3}>3 — low</option>
                <option value={4}>4 — icebox</option>
              </select>
            </div>
            <div className="composer-row">
              <label className="composer-label">Send to</label>
              <select className="composer-select" value={destination} onChange={(e) => setDestination(e.target.value)}>
                {COLUMNS.filter(c => c.id !== 'done' && c.id !== 'review').map(c => (
                  <option key={c.id} value={c.id}>{c.name}</option>
                ))}
              </select>
            </div>
          </div>
        </div>
        <footer className="composer-foot">
          <div className="composer-foot-left">
            <span>{template.steps.length} steps · agents from chain template</span>
          </div>
          <div className="composer-foot-right">
            <button className="btn btn-ghost" onClick={onClose}>Cancel</button>
            <button className="btn btn-primary" onClick={submit} disabled={!title.trim()}>
              Create bead →
            </button>
          </div>
        </footer>
      </div>
    </div>
  );
}

Object.assign(window, { NewBeadComposer });
