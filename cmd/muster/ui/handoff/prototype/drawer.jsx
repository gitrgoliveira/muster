// Task Drawer — rich Beads ticket view.
// Side panel (right), no blocking scrim. Clicking cards on the board switches
// the open bead without closing first.
//
// Tabs: Overview · Deps · Steps · Activity · Run log · Files

const {useState: useStateD, useEffect: useEffectD, useRef: useRefD} = React;

// ─── SkillPicker (used in StepsTab) ─────────────────────────────────────────

function SkillPicker({selected, onToggle, compact=false}) {
  const {SKILLS, SKILL_CATEGORIES} = window.MUSTER_DATA;
  const [cat, setCat] = useStateD('all');
  const [q, setQ] = useStateD('');

  const visible = SKILLS.filter(s => {
    if (cat !== 'all' && s.cat !== cat) return false;
    if (q && !(s.name + ' ' + s.desc).toLowerCase().includes(q.toLowerCase())) return false;
    return true;
  });

  return (
    <div className="skill-picker">
      <div className="skill-toolbar">
        <input className="skill-search" placeholder="Filter skills…" value={q} onChange={(e) => setQ(e.target.value)} />
        <div className="skill-cats">
          <button className={cat === 'all' ? 'active' : ''} onClick={() => setCat('all')}>All</button>
          {SKILL_CATEGORIES.map(c => (
            <button key={c.id} className={cat === c.id ? 'active' : ''} onClick={() => setCat(c.id)}>{c.name}</button>
          ))}
        </div>
      </div>
      <div className="skill-grid">
        {visible.map(s => {
          const on = selected.includes(s.id);
          return (
            <button key={s.id} className={'skill-card ' + (on ? 'on' : '')} onClick={() => onToggle(s.id)}>
              <span className="skill-icon">{s.icon}</span>
              <span className="skill-text">
                <span className="skill-name">{s.name}</span>
                <span className="skill-desc">{s.desc}</span>
              </span>
              <span className={'skill-check ' + (on ? 'on' : '')}>{on ? '✓' : '+'}</span>
            </button>
          );
        })}
        {visible.length === 0 && <div className="skill-empty">No skills match.</div>}
      </div>
    </div>
  );
}

// ─── StepEditor (used in StepsTab) ───────────────────────────────────────────

function StepEditor({step, idx, allSteps, onChange, onRemove}) {
  const {AGENTS, SKILLS, SKILL_SOURCES} = window.MUSTER_DATA;
  const [expanded, setExpanded] = useStateD(false);

  const A = AGENTS.find(a => a.id === step.agent);
  const availableModes = A?.modes || [];
  const M = availableModes.find(m => m.id === step.mode);
  const stepSkills = step.skills || [];
  const specSkill = stepSkills.find(s => ['speckit','openspec'].includes(s));

  const defaultPromptsByMode = {
    plan:   'Decompose the spec into atomic, ordered, idempotent steps. Emit plan.md. No file writes.',
    build:  'Execute plan.md step by step. Halt on a failed test; do not invent new steps.',
    apply:  'Apply the approved patch set. Run the test suite after each chunk.',
    agent:  'Implement the task end to end. Run tests after each meaningful change.',
    yolo:   'Free-roaming. Auto-approve tool calls. Self-handoff at 80% budget.',
    review: 'Review the worktree diff against the spec. Inline comments + summary. No writes.',
  };
  const specOverrides = {
    speckit:  'Draft acceptance criteria using Speckit. Output Gherkin scenarios in spec.md. Run `speckit verify` before locking.',
    openspec: 'Validate the diff against OpenAPI contracts. Report breakages by route, severity.',
  };
  const defaultPrompt = (specSkill && specOverrides[specSkill]) || defaultPromptsByMode[step.mode] || '';
  const prompt = step.prompt ?? defaultPrompt;
  const loopBack = step.loopBackTo;
  const loopMax = step.loopMax ?? 3;

  const setAgent = (id) => {
    const next = AGENTS.find(a => a.id === id);
    const validMode = next?.modes.find(m => m.id === step.mode) ? step.mode : next?.modes[0]?.id;
    onChange({...step, agent: id, mode: validMode, prompt: undefined});
  };
  const setMode = (id) => onChange({...step, mode: id, prompt: undefined});
  const toggleSkill = (id) => {
    const has = stepSkills.includes(id);
    onChange({...step, skills: has ? stepSkills.filter(x => x !== id) : [...stepSkills, id]});
  };

  return (
    <div className={'step-editor status-' + step.status + (expanded ? ' is-expanded' : '')}>
      <div className="step-index">
        <span className="step-num">{idx + 1}</span>
        {idx < allSteps.length - 1 && <span className="step-connector"></span>}
      </div>
      <div className="step-body">
        <div className="step-summary">
          <button className="step-disclose" onClick={() => setExpanded(e => !e)}>
            <span className="disclose-arrow">{expanded ? '▾' : '▸'}</span>
          </button>
          <span className="agent-chip" style={{['--agent-color']: A?.color}}>
            <span className="agent-mono">{A?.mono}</span>
            <span className="agent-name">{A?.name}</span>
          </span>
          <span className="step-arrow">·</span>
          <span className="step-mode-tag" data-mode={step.mode}>
            <span className="step-mode-icon">{M?.icon || '◯'}</span>
            <span>{M?.name || step.mode}</span>
          </span>
          {stepSkills.length > 0 && (
            <span className="step-skills-inline">
              {stepSkills.map(id => {
                const s = SKILLS.find(x => x.id === id);
                return s ? <span key={id} className="step-skill-pill" title={s.desc}>{s.icon} {s.name}</span> : null;
              })}
            </span>
          )}
          <span className="step-summary-spacer"></span>
          {loopBack !== undefined && (
            <span className="loop-tag" title={`Loops back to step ${loopBack + 1}, up to ${loopMax}×`}>↻ {loopBack + 1} · {loopMax}×</span>
          )}
          <span className={'step-status-pill status-' + step.status}>{step.status}</span>
          <button className="step-remove" onClick={onRemove} title="Remove step">×</button>
        </div>

        {expanded && (
          <div className="step-detail">
            <div className="step-row">
              <label className="step-field">
                <span className="step-field-label">Agent</span>
                <select value={step.agent} onChange={(e) => setAgent(e.target.value)}>
                  {AGENTS.map(a => <option key={a.id} value={a.id}>{a.name}</option>)}
                </select>
              </label>
              <label className="step-field">
                <span className="step-field-label">Mode</span>
                <select value={step.mode} onChange={(e) => setMode(e.target.value)}>
                  {availableModes.map(m => <option key={m.id} value={m.id}>{m.icon} {m.name}</option>)}
                </select>
              </label>
              {M?.cli && (
                <div className="step-clihint">
                  <span className="step-field-label">CLI</span>
                  <code>{M.cli}</code>
                </div>
              )}
            </div>
            {M?.desc && <div className="step-mode-desc">{M.desc}</div>}
            <div className="step-skills-field">
              <span className="step-field-label">Skills for this step <span className="step-field-sub">· {stepSkills.length} selected</span></span>
              <div className="step-skill-row">
                {SKILLS.map(s => {
                  const on = stepSkills.includes(s.id);
                  const src = SKILL_SOURCES.find(x => x.id === s.source);
                  return (
                    <button key={s.id} className={'step-skill-chip ' + (on ? 'on' : '')} onClick={() => toggleSkill(s.id)} title={s.desc} data-cat={s.cat}>
                      <span className="ssc-icon">{s.icon}</span>
                      <span>{s.name}</span>
                      <span className="ssc-source" data-source={s.source}>{src?.label}</span>
                    </button>
                  );
                })}
              </div>
            </div>
            <div className="step-prompt-field">
              <span className="step-field-label">Prompt to {A?.name}</span>
              <textarea className="step-prompt" rows={3} value={prompt} placeholder="What should the agent do?" onChange={(e) => onChange({...step, prompt: e.target.value})} />
              {step.prompt !== undefined && (
                <button className="link-btn" onClick={() => onChange({...step, prompt: undefined})}>reset to default</button>
              )}
            </div>
            <div className="step-loop-field">
              <span className="step-field-label">Loop control</span>
              <div className="loop-controls">
                <span>If review fails, loop back to</span>
                <select value={loopBack ?? ''} onChange={(e) => onChange({...step, loopBackTo: e.target.value === '' ? undefined : Number(e.target.value)})}>
                  <option value="">— don't loop —</option>
                  {allSteps.map((_, i) => i < idx && <option key={i} value={i}>step {i + 1}</option>)}
                </select>
                {loopBack !== undefined && (
                  <>
                    <span>at most</span>
                    <input type="number" min="1" max="20" value={loopMax} onChange={(e) => onChange({...step, loopMax: Math.max(1, Number(e.target.value) || 1)})} className="loop-max-input" />
                    <span>times</span>
                  </>
                )}
              </div>
            </div>
          </div>
        )}
        {!expanded && step.note && <div className="step-note">{step.note}</div>}
      </div>
    </div>
  );
}

function AcceptanceCriteria({task, onUpdate}) {
  const items = task.acceptance || [];
  const done = items.filter(i => i.done).length;
  const toggle = (idx) => {
    const next = items.map((it, i) => i === idx ? {...it, done: !it.done} : it);
    onUpdate({...task, acceptance: next});
  };
  if (!items.length) return (
    <div className="ac-empty">No acceptance criteria yet — <code>bd update {task.id} --ac "..."</code></div>
  );
  return (
    <div className="ac-list">
      <div className="ac-progress-row">
        <div className="ac-bar"><div className="ac-bar-fill" style={{width: items.length ? (done/items.length*100)+'%' : '0%'}}></div></div>
        <span className="ac-tally">{done}/{items.length}</span>
      </div>
      {items.map((it, i) => (
        <label key={i} className={'ac-item ' + (it.done ? 'done' : '')}>
          <input type="checkbox" checked={it.done} onChange={() => toggle(i)} />
          <span className="ac-text">{it.text}</span>
        </label>
      ))}
    </div>
  );
}

function CommentThread({task, onUpdate}) {
  const {AGENTS} = window.MUSTER_DATA;
  const comments = (task.history || []).filter(h => h.kind === 'comment');
  const [draft, setDraft] = useStateD('');
  const submit = () => {
    const text = draft.trim();
    if (!text) return;
    const now = 'just now';
    const next = {
      ...task,
      history: [...(task.history || []), { at: now, kind: 'comment', actor: 'you@yours.dev', note: text }],
      comments: (task.comments || 0) + 1,
      lastActivity: now,
    };
    onUpdate?.(next);
    setDraft('');
  };
  if (!comments.length && !draft) return (
    <div className="comments-empty">
      No comments yet — <code>bd comment {task.id} "..."</code>
      <div className="comment-compose">
        <textarea className="comment-draft" placeholder="Add a comment…" rows={2} value={draft} onChange={e=>setDraft(e.target.value)} onKeyDown={e => { if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) submit(); }} />
        {draft && <button className="btn btn-primary comment-submit" style={{fontSize:'12px',padding:'5px 12px'}} onClick={submit}>Post</button>}
      </div>
    </div>
  );
  return (
    <div className="comment-thread">
      {comments.map((c, i) => {
        const isAgent = ['claude','gemini','opencode','codex'].includes(c.actor);
        const A = isAgent ? AGENTS.find(a => a.id === c.actor) : null;
        return (
          <div key={i} className="comment-row">
            <div className="comment-avatar">
              {A
                ? <span className="agent-mono" style={{['--agent-color']: A.color}}>{A.mono}</span>
                : <span className="comment-human-avatar">{c.actor.slice(0,2).toUpperCase()}</span>}
            </div>
            <div className="comment-body">
              <div className="comment-meta">
                <span className="comment-author">{A?.name || c.actor}</span>
                <span className="comment-when">{c.at}</span>
              </div>
              <div className="comment-text">{c.note}</div>
            </div>
          </div>
        );
      })}
      <div className="comment-compose">
        <textarea className="comment-draft" placeholder="Reply or add a comment… (⌘+Enter to post)" rows={2} value={draft} onChange={e=>setDraft(e.target.value)} onKeyDown={e => { if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) submit(); }} />
        {draft && <button className="btn btn-primary comment-submit" style={{fontSize:'12px',padding:'5px 12px'}} onClick={submit}>Post</button>}
      </div>
    </div>
  );
}

function GatesSection({task}) {
  const gates = task.gates || [];
  const icons = { human: '☻', timer: '◷', github: '⎇' };
  if (!gates.length) return null;
  return (
    <div className="drawer-block">
      <h4 className="drawer-h">Gates <span className="drawer-h-sub">— bd gate {task.id}</span></h4>
      <div className="gates-list">
        {gates.map((g, i) => (
          <div key={i} className={'gate-row status-' + g.status}>
            <span className="gate-kind-icon">{icons[g.kind] || '◇'}</span>
            <span className="gate-kind-label">{g.kind}</span>
            <span className="gate-row-label">{g.label}</span>
            <span className={'gate-status-pill status-' + g.status}>{g.status}</span>
          </div>
        ))}
      </div>
    </div>
  );
}

// ─── Tabs ────────────────────────────────────────────────────────────────────

function OverviewTab({task, onUpdate, constitution, onEditConstitution}) {
  const [showConst, setShowConst] = useStateD(false);
  const [newAC, setNewAC] = useStateD('');
  const titleRef = useRefD(null);
  const descRef = useRefD(null);
  const constWords = constitution ? constitution.trim().split(/\s+/).filter(Boolean).length : 0;

  // Sync contenteditable title when task changes
  useEffectD(() => {
    if (titleRef.current && titleRef.current.textContent !== task.title) {
      titleRef.current.textContent = task.title;
    }
  }, [task.id]);

  const onTitleBlur = () => {
    const text = titleRef.current?.textContent?.trim();
    if (text && text !== task.title) onUpdate({...task, title: text});
  };
  const onTitleKey = (e) => {
    if (e.key === 'Enter') { e.preventDefault(); titleRef.current?.blur(); }
  };
  const onDescBlur = () => {
    const text = descRef.current?.value;
    if (text !== task.desc) onUpdate({...task, desc: text});
  };

  // AC helpers
  const toggleAC = (idx) => {
    const next = (task.acceptance||[]).map((it, i) => i === idx ? {...it, done: !it.done} : it);
    onUpdate({...task, acceptance: next});
  };
  const removeAC = (idx) => {
    onUpdate({...task, acceptance: (task.acceptance||[]).filter((_, i) => i !== idx)});
  };
  const addAC = (text) => {
    if (!text.trim()) return;
    onUpdate({...task, acceptance: [...(task.acceptance||[]), {text: text.trim(), done: false}]});
    setNewAC('');
  };

  // Label helpers
  const removeLabel = (l) => onUpdate({...task, labels: (task.labels||[]).filter(x => x !== l)});

  const acItems = task.acceptance || [];
  const acDone = acItems.filter(a => a.done).length;

  return (
    <div className="tab-overview">
      {/* ── Editable title ─────────────────────────────────────────────── */}
      <div className="ov-title-wrap">
        <h1
          ref={titleRef}
          className="ov-title"
          contentEditable
          suppressContentEditableWarning
          onBlur={onTitleBlur}
          onKeyDown={onTitleKey}
          data-placeholder="Bead title…"
        >
          {task.title}
        </h1>
        <div className="ov-title-meta">
          <PriBadge n={task.priority} />
          <span className={'ov-type-chip type-' + task.type}>{task.type}</span>
          {task.estimate && <span className="ov-est">{task.estimate}</span>}
          {task.vcs && <span className={'vcs-badge vcs-' + task.vcs}>{task.vcs === 'jj' ? '⌥ jj' : '⎇ git'}</span>}
          {task.assignee && (() => {
            const A = window.MUSTER_DATA.AGENTS.find(a => a.id === task.assignee);
            return A ? <span className="agent-mono sm" style={{['--agent-color']: A.color}}>{A.mono}</span> : null;
          })()}
          {task.pinnedAgent && <span className="pinned-badge">⊣ {task.pinnedAgent}</span>}
          {task.requeued && <span className="requeued-tag">requeued</span>}
          {task.formula && <span className="formula-badge">ƒ {task.formula}</span>}
          <span className="ov-bead-id">{task.id}</span>
          <span className="ov-date">{task.createdAt}</span>
        </div>
      </div>

      {/* ── Acceptance criteria ─────────────────────────────────────────── */}
      <div className="ov-section">
        <div className="ov-section-head">
          <span className="ov-section-label">Acceptance criteria</span>
          {acItems.length > 0 && (
            <span className="ov-ac-progress">
              <span className="ov-ac-bar"><span className="ov-ac-bar-fill" style={{width: (acDone/acItems.length*100)+'%'}}></span></span>
              <span className="ov-ac-tally">{acDone}/{acItems.length}</span>
            </span>
          )}
        </div>
        <div className="ov-ac-list">
          {acItems.map((it, i) => (
            <div key={i} className={'ov-ac-row ' + (it.done ? 'done' : '')}>
              <input type="checkbox" checked={it.done} onChange={() => toggleAC(i)} />
              <span className="ov-ac-text">{it.text}</span>
              <button className="ov-ac-remove" onClick={() => removeAC(i)} title="Remove">×</button>
            </div>
          ))}
          <div className="ov-ac-add-row">
            <span className="ov-ac-add-icon">+</span>
            <input
              className="ov-ac-input"
              placeholder="Add criterion, press Enter…"
              value={newAC}
              onChange={e => setNewAC(e.target.value)}
              onKeyDown={e => { if (e.key === 'Enter') addAC(newAC); }}
            />
          </div>
        </div>
      </div>

      {/* ── Description ─────────────────────────────────────────────────── */}
      <div className="ov-section">
        <div className="ov-section-head">
          <span className="ov-section-label">Description</span>
          <span className="ov-section-hint">bd edit {task.id}</span>
        </div>
        <textarea
          ref={descRef}
          className="ov-desc"
          defaultValue={task.desc}
          rows={5}
          onBlur={onDescBlur}
          placeholder="What should the agent do? Add context, constraints, links…"
        />
      </div>

      {/* ── Labels ──────────────────────────────────────────────────────── */}
      <div className="ov-section">
        <div className="ov-section-head">
          <span className="ov-section-label">Labels</span>
          <span className="ov-section-hint">bd label {task.id}</span>
        </div>
        <div className="ov-labels-row">
          {(task.labels||[]).map(l => (
            <span key={l} className="ov-label-chip">
              <span className="ov-label-text">#{l}</span>
              <button className="ov-label-remove" onClick={() => removeLabel(l)}>×</button>
            </span>
          ))}
          <LabelAddInput task={task} onUpdate={onUpdate} />
        </div>
      </div>

      {/* ── Gates ───────────────────────────────────────────────────────── */}
      <GatesSection task={task} />

      {/* ── Comments ────────────────────────────────────────────────────── */}
      <div className="ov-section">
        <div className="ov-section-head">
          <span className="ov-section-label">
            Comments
            {task.comments > 0 && <span className="tab-count" style={{marginLeft:6}}>{task.comments}</span>}
          </span>
          <span className="ov-section-hint">bd comments {task.id}</span>
        </div>
        <CommentThread task={task} onUpdate={onUpdate} />
      </div>

      {/* ── Constitution ────────────────────────────────────────────────── */}
      {constitution && (
        <div className="ov-section">
          <div className="constitution-banner">
            <div className="constitution-banner-row">
              <span className="constitution-glyph">§</span>
              <div className="constitution-banner-text">
                <strong>Constitution applies</strong>
                <span className="constitution-banner-meta">{constWords} words prepended to every prompt</span>
              </div>
              <button className="link-btn" onClick={() => setShowConst(s => !s)}>{showConst ? 'hide' : 'preview'}</button>
              <button className="link-btn" onClick={onEditConstitution}>edit →</button>
            </div>
            {showConst && <pre className="constitution-preview">{constitution}</pre>}
          </div>
        </div>
      )}
    </div>
  );
}

function LabelAddInput({task, onUpdate}) {
  const [adding, setAdding] = useStateD(false);
  const [val, setVal] = useStateD('');
  const addLabel = () => {
    const trimmed = val.trim().toLowerCase().replace(/\s+/g, '-').replace(/^#+/, '');
    if (trimmed && !(task.labels||[]).includes(trimmed)) {
      onUpdate({...task, labels: [...(task.labels||[]), trimmed]});
    }
    setVal(''); setAdding(false);
  };
  if (!adding) return (
    <button className="ov-label-add" onClick={() => setAdding(true)}>+ label</button>
  );
  return (
    <input
      className="ov-label-input"
      placeholder="label-name"
      value={val}
      autoFocus
      onChange={e => setVal(e.target.value)}
      onKeyDown={e => { if (e.key === 'Enter') addLabel(); if (e.key === 'Escape') { setAdding(false); setVal(''); } }}
      onBlur={addLabel}
    />
  );
}


function DepsTab({task, onOpenBead}) {
  // Dependency types per Beads docs:
  // - blocks / blocked-by (hard dep, affects ready queue)
  // - parent-child (epic/subtask)
  // - discovered-from (track issues found during work)
  // - related (soft)
  // - external (cross-repo)
  const {AGENTS} = window.MUSTER_DATA;
  const hasAny = task.blockedBy?.length || task.blocks?.length ||
    task.externalDeps?.length || task.discoveredFrom ||
    task.subBeads?.length;

  return (
    <div className="tab-deps">
      {/* Graph header */}
      <div className="deps-commands">
        <code>bd dep {task.id}</code>
        <code>bd graph {task.id}</code>
        <code>bd blocked {task.id}</code>
        <code>bd children {task.id}</code>
      </div>

      {/* Blocking deps */}
      {(task.blockedBy?.length > 0 || task.blocks?.length > 0) && (
        <div className="deps-section">
          <div className="deps-section-head">
            <span className="dep-kind-badge dep-blocks">blocks</span>
            <span className="deps-section-note">hard dep · affects bd ready queue</span>
          </div>
          {task.blockedBy?.length > 0 && (
            <div className="dep-group">
              <span className="dep-group-label">blocked by</span>
              <div className="dep-pills">
                {task.blockedBy.map(id => (
                  <button key={id} className="dep-pill dep-pill-blocker" onClick={() => onOpenBead?.(id)}>
                    <span className="dep-arrow">→</span>{id}
                  </button>
                ))}
              </div>
            </div>
          )}
          <div className="dep-self-node">
            <span className="dep-self-id">{task.id}</span>
            <span className="dep-self-title">{task.title}</span>
          </div>
          {task.blocks?.length > 0 && (
            <div className="dep-group">
              <span className="dep-group-label">blocks</span>
              <div className="dep-pills">
                {task.blocks.map(id => (
                  <button key={id} className="dep-pill dep-pill-blocked" onClick={() => onOpenBead?.(id)}>
                    {id}<span className="dep-arrow">→</span>
                  </button>
                ))}
              </div>
            </div>
          )}
        </div>
      )}

      {/* External cross-repo deps */}
      {task.externalDeps?.length > 0 && (
        <div className="deps-section">
          <div className="deps-section-head">
            <span className="dep-kind-badge dep-external">external</span>
            <span className="deps-section-note">cross-repo · bd dep add {task.id} external:repo/bd-xxx</span>
          </div>
          <div className="dep-pills">
            {task.externalDeps.map(ref => {
              const parts = ref.replace('external:', '').split('/');
              return (
                <span key={ref} className="dep-pill dep-pill-ext" title={ref}>
                  <span className="dep-ext-repo">{parts[0]}/</span>{parts[1]}
                </span>
              );
            })}
          </div>
        </div>
      )}

      {/* Discovered-from */}
      {task.discoveredFrom && (
        <div className="deps-section">
          <div className="deps-section-head">
            <span className="dep-kind-badge dep-discovered">discovered-from</span>
            <span className="deps-section-note">found while working on another bead</span>
          </div>
          <button className="dep-pill dep-pill-discovered" onClick={() => onOpenBead?.(task.discoveredFrom)}>
            ↪ {task.discoveredFrom}
          </button>
        </div>
      )}

      {/* Sub-beads (parent-child) */}
      {task.subBeads?.length > 0 && (
        <div className="deps-section">
          <div className="deps-section-head">
            <span className="dep-kind-badge dep-children">parent-child</span>
            <span className="deps-section-note">sub-beads · bd children {task.id}</span>
          </div>
          <div className="subbead-list">
            {task.subBeads.map(b => {
              const A = AGENTS.find(a => a.id === b.agent);
              return (
                <div key={b.id} className={'subbead-row status-' + b.status}>
                  <span className="subbead-bullet">
                    {b.status === 'done' ? '✓' : b.status === 'active' ? '◉' : b.status === 'failed' ? '×' : '○'}
                  </span>
                  <button className="subbead-id bead-link" onClick={() => onOpenBead?.(b.id)}>{b.id}</button>
                  <span className="subbead-title">{b.title}</span>
                  {b.autoSplit && <span className="subbead-auto">auto</span>}
                  {A && <span className="agent-mono" style={{['--agent-color']: A.color}}>{A.mono}</span>}
                </div>
              );
            })}
          </div>
          <button className="subbead-add">＋ Create sub-bead</button>
        </div>
      )}

      {!hasAny && (
        <div className="deps-empty">
          No dependencies yet.
          <div className="deps-empty-cmds">
            <code>bd dep add {task.id} other-bd-id</code>
            <code>bd dep add {task.id} external:repo/bd-xxx</code>
          </div>
        </div>
      )}
    </div>
  );
}

function SkillChipRow({skills, onToggle}) {
  // Compact skill chips — always visible, click to remove, + to add more.
  const {SKILLS} = window.MUSTER_DATA;
  const [picking, setPicking] = useStateD(false);
  const [q, setQ] = useStateD('');
  const active = skills.map(id => SKILLS.find(s => s.id === id)).filter(Boolean);
  const available = SKILLS.filter(s => !skills.includes(s.id) &&
    (!q || (s.name+' '+s.desc).toLowerCase().includes(q.toLowerCase())));

  return (
    <div className="skill-chip-row">
      {active.map(s => (
        <button
          key={s.id}
          className={'skill-inline-chip active' + (s.cat === 'spec' ? ' spec' : '')}
          onClick={() => onToggle(s.id)}
          title={'Click to remove · ' + s.desc}
        >
          <span className="sic-icon">{s.icon}</span>
          <span className="sic-name">{s.name}</span>
          <span className="sic-remove">×</span>
        </button>
      ))}
      {picking ? (
        <div className="skill-picker-popover">
          <input
            className="spp-search"
            placeholder="Filter skills…"
            value={q}
            autoFocus
            onChange={e => setQ(e.target.value)}
            onKeyDown={e => e.key === 'Escape' && setPicking(false)}
          />
          <div className="spp-list">
            {available.slice(0, 12).map(s => (
              <button
                key={s.id}
                className={'spp-item' + (s.cat === 'spec' ? ' spec' : '')}
                onClick={() => { onToggle(s.id); setQ(''); }}
                title={s.desc}
              >
                <span className="spp-icon">{s.icon}</span>
                <span className="spp-name">{s.name}</span>
                <span className="spp-cat">{s.cat}</span>
              </button>
            ))}
            {available.length === 0 && <div className="spp-empty">No skills match</div>}
          </div>
          <button className="spp-close" onClick={() => { setPicking(false); setQ(''); }}>done</button>
        </div>
      ) : (
        <button className="skill-inline-add" onClick={() => setPicking(true)} title="Add skill">+ skill</button>
      )}
    </div>
  );
}

function StepCard({step, idx, allSteps, onChange, onRemove}) {
  const {AGENTS} = window.MUSTER_DATA;
  const A = AGENTS.find(a => a.id === step.agent);
  const availableModes = A?.modes || [];
  const M = availableModes.find(m => m.id === step.mode);
  const stepSkills = step.skills || [];

  const defaultPromptsByMode = {
    plan:   'Decompose the spec into atomic, ordered, idempotent steps. Emit plan.md. No file writes.',
    build:  'Execute plan.md step by step. Halt on a failed test; do not invent new steps.',
    apply:  'Apply the approved patch set. Run the test suite after each chunk.',
    agent:  'Implement the task end to end. Run tests after each meaningful change.',
    yolo:   'Free-roaming. Auto-approve tool calls. Self-handoff at 80% budget.',
    review: 'Review the worktree diff against the spec. Inline comments + summary. No writes.',
  };
  const prompt = step.prompt ?? defaultPromptsByMode[step.mode] ?? '';
  const loopBack = step.loopBackTo;
  const loopMax = step.loopMax ?? 3;

  const setAgent = (id) => {
    const next = AGENTS.find(a => a.id === id);
    const validMode = next?.modes.find(m => m.id === step.mode) ? step.mode : next?.modes[0]?.id;
    onChange({...step, agent: id, mode: validMode, prompt: undefined});
  };
  const toggleSkill = (id) => {
    const has = stepSkills.includes(id);
    onChange({...step, skills: has ? stepSkills.filter(x => x !== id) : [...stepSkills, id]});
  };

  return (
    <div className={'step-card status-' + step.status}>
      {/* Step number + connector */}
      <div className="step-card-index">
        <span className={'step-card-num status-' + step.status}>{idx + 1}</span>
        {idx < allSteps.length - 1 && <span className="step-card-connector"></span>}
      </div>

      {/* Card body */}
      <div className="step-card-body">
        {/* Agent + mode row */}
        <div className="step-card-head">
          <select
            className="step-agent-select"
            value={step.agent}
            style={{['--agent-color']: A?.color}}
            onChange={e => setAgent(e.target.value)}
          >
            {AGENTS.map(a => <option key={a.id} value={a.id}>{a.mono} {a.name}</option>)}
          </select>
          <select
            className="step-mode-select"
            value={step.mode}
            data-mode={step.mode}
            onChange={e => onChange({...step, mode: e.target.value, prompt: undefined})}
          >
            {availableModes.map(m => <option key={m.id} value={m.id}>{m.icon} {m.name}</option>)}
          </select>
          {M?.cli && <code className="step-cli-hint">{M.cli}</code>}
          <span className={'step-card-status status-' + step.status}>{step.status}</span>
          <span className="step-card-spacer"></span>
          {loopBack !== undefined && (
            <span className="loop-tag" title={`Loops back to step ${loopBack + 1}, up to ${loopMax}×`}>
              ↻{loopBack + 1} · {loopMax}×
            </span>
          )}
          <button className="step-remove" onClick={onRemove} title="Remove step">×</button>
        </div>

        {/* Skills — per-step, compact chips */}
        <SkillChipRow skills={stepSkills} onToggle={toggleSkill} />

        {/* Prompt */}
        <textarea
          className="step-prompt"
          rows={2}
          value={prompt}
          placeholder="Prompt sent to the agent for this step…"
          onChange={e => onChange({...step, prompt: e.target.value})}
          onFocus={e => { e.target.rows = 4; }}
          onBlur={e => { e.target.rows = 2; if (e.target.value === defaultPromptsByMode[step.mode]) onChange({...step, prompt: undefined}); }}
        />
        {step.note && <div className="step-note">{step.note}</div>}

        {/* Loop control */}
        <div className="step-loop-inline">
          <span className="step-loop-label">on fail, loop to</span>
          <select
            className="step-loop-select"
            value={loopBack ?? ''}
            onChange={e => onChange({...step, loopBackTo: e.target.value === '' ? undefined : Number(e.target.value)})}
          >
            <option value="">— no loop —</option>
            {allSteps.map((_, i) => i < idx && <option key={i} value={i}>step {i+1}</option>)}
          </select>
          {loopBack !== undefined && (
            <>
              <span className="step-loop-label">max</span>
              <input
                type="number" min="1" max="20" value={loopMax} className="loop-max-input"
                onChange={e => onChange({...step, loopMax: Math.max(1, Number(e.target.value)||1)})}
              />
              <span className="step-loop-label">×</span>
            </>
          )}
        </div>
      </div>
    </div>
  );
}

function StepsTab({task, onUpdate}) {
  const {VCS_OPTIONS} = window.MUSTER_DATA;

  const updateStep = (i, s) => {
    const steps = [...task.steps]; steps[i] = s;
    onUpdate({...task, steps});
  };
  const removeStep = (i) => onUpdate({...task, steps: task.steps.filter((_, j) => j !== i)});
  const addStep = (template) => {
    const presets = {
      speckit: { agent: 'claude',   mode: 'plan',   skills: ['speckit'] },
      plan:    { agent: 'claude',   mode: 'plan',   skills: [] },
      build:   { agent: 'claude',   mode: 'build',  skills: [] },
      agent:   { agent: 'claude',   mode: 'agent',  skills: [] },
      review:  { agent: 'gemini',   mode: 'review', skills: [] },
      openspec:{ agent: 'gemini',   mode: 'plan',   skills: ['openspec'] },
    };
    const p = presets[template] || presets.agent;
    onUpdate({...task, steps: [...task.steps, {...p, status: 'pending'}]});
  };

  return (
    <div className="tab-steps">
      {/* VCS selector — brief, at the top */}
      <div className="steps-vcs-row">
        <span className="steps-vcs-label">Worktree</span>
        {VCS_OPTIONS.map(v => (
          <button
            key={v.id}
            className={'steps-vcs-btn ' + (task.vcs === v.id ? 'active' : '')}
            onClick={() => onUpdate({...task, vcs: v.id})}
            disabled={!!task.branch}
            title={task.branch ? 'VCS locked — worktree exists' : v.desc}
          >
            {v.icon} {v.label}
          </button>
        ))}
        {task.branch && <code className="steps-branch">{task.branch}</code>}
      </div>

      {/* Step cards */}
      <div className="step-cards-list">
        {task.steps.map((s, i) => (
          <StepCard
            key={i}
            step={s}
            idx={i}
            allSteps={task.steps}
            onChange={ns => updateStep(i, ns)}
            onRemove={() => removeStep(i)}
          />
        ))}
      </div>

      {/* Add step */}
      <div className="add-step">
        <span className="add-step-label">Add step:</span>
        <button className="add-step-btn" onClick={() => addStep('speckit')}>◆ Speckit</button>
        <button className="add-step-btn" onClick={() => addStep('plan')}>▤ Plan</button>
        <button className="add-step-btn" onClick={() => addStep('build')}>▣ Build</button>
        <button className="add-step-btn" onClick={() => addStep('agent')}>◉ Agent</button>
        <button className="add-step-btn" onClick={() => addStep('review')}>◐ Review</button>
        <button className="add-step-btn" onClick={() => addStep('openspec')}>◇ OpenSpec</button>
      </div>
    </div>
  );
}

  const updateStep = (i, s) => {
    const steps = [...task.steps];
    steps[i] = s;
    onUpdate({...task, steps});
  };
  const removeStep = (i) => {
    const steps = task.steps.filter((_, j) => j !== i);
    onUpdate({...task, steps});
  };
  const addStepTemplate = (template) => {
    const presets = {
      speckit:  { agent: 'claude',   mode: 'plan',   skills: ['speckit'] },
      plan:     { agent: 'claude',   mode: 'plan',   skills: [] },
      build:    { agent: 'claude',   mode: 'build',  skills: [] },
      agent:    { agent: 'claude',   mode: 'agent',  skills: [] },
      review:   { agent: 'gemini',   mode: 'review', skills: [] },
      openspec: { agent: 'gemini',   mode: 'plan',   skills: ['openspec'] },
    };
    const p = presets[template] || presets.agent;
    onUpdate({...task, steps: [...task.steps, {...p, status: 'pending'}]});
  };

function ActivityTab({task}) {
  const {EVENT_KINDS, AGENTS} = window.MUSTER_DATA;
  const history = task.history || [];
  const events = [...history].reverse();
  return (
    <div className="tab-activity">
      {events.length === 0 && <div className="empty-state">No lifecycle events yet.</div>}
      <ul className="drawer-act-list">
        {events.map((h, i) => {
          const K = EVENT_KINDS[h.kind] || { glyph: '·', label: h.kind, tone: 'neutral' };
          const isAgent = ['claude','gemini','opencode','codex'].includes(h.actor);
          const A = isAgent ? AGENTS.find(a => a.id === h.actor) : null;
          const Ag = h.agent ? AGENTS.find(a => a.id === h.agent) : null;
          return (
            <li key={i} className={'drawer-act-row kind-' + h.kind + ' tone-' + K.tone}>
              <span className="drawer-act-glyph">{K.glyph}</span>
              <span className="drawer-act-body">
                <span className="drawer-act-kind">{K.label}</span>
                {Ag && (
                  <>
                    <span className="drawer-act-arrow">→</span>
                    <span className="agent-mono" style={{['--agent-color']: Ag.color}}>{Ag.mono}</span>
                  </>
                )}
                {!Ag && A && <span className="agent-mono" style={{['--agent-color']: A.color}}>{A.mono}</span>}
                {!Ag && !A && <span className="drawer-act-actor">{h.actor}</span>}
                {h.note && <span className="drawer-act-note">{h.note}</span>}
              </span>
              <span className="drawer-act-when">{h.at}</span>
            </li>
          );
        })}
      </ul>
    </div>
  );
}

function RunLogTab({task}) {
  const log = task.log || [];
  const endRef = useRefD(null);
  const [extra, setExtra] = useStateD([]);

  useEffectD(() => {
    if (task.column !== 'running') return;
    const lines = [
      { kind: 'tool', msg: 'shell: pnpm test auth --watch' },
      { kind: 'output', msg: '  ✓ session revocation cascades (211ms)' },
      { kind: 'thought', msg: 'All 14 tests passing — preparing diff for review.' },
      { kind: 'tool', msg: 'shell: jj diff --summary' },
    ];
    let i = 0;
    const id = setInterval(() => {
      if (i >= lines.length) { clearInterval(id); return; }
      const now = new Date();
      const t = `${String(now.getHours()).padStart(2,'0')}:${String(now.getMinutes()).padStart(2,'0')}:${String(now.getSeconds()).padStart(2,'0')}`;
      setExtra(prev => [...prev, {...lines[i], t}]);
      i++;
    }, 2400);
    return () => clearInterval(id);
  }, [task.id, task.column]);

  useEffectD(() => {
    endRef.current?.parentElement?.scrollTo({top: 1e9, behavior: 'smooth'});
  }, [extra.length]);

  const all = [...log, ...extra];
  return (
    <div className="tab-runlog">
      <div className="runlog-meta">
        <span>worktree <code>wt-{task.id.replace('bd-','')}</code></span>
        <span>·</span>
        <span>branch <code>{task.branch || '—'}</code></span>
        <span>·</span>
        <span>{fmtTokens(task.tokensUsed)} / {fmtTokens(task.tokensBudget)} tokens</span>
        {task.column === 'running' && (
          <>
            <span className="live-tag">● live</span>
            <button className="btn btn-ghost" style={{fontSize:'11px',padding:'3px 10px',marginLeft:'auto'}}>
              bd attach {task.id}
            </button>
          </>
        )}
      </div>
      <div className="runlog">
        {all.map((l, i) => (
          <div key={i} className={'log-line kind-' + l.kind}>
            <span className="log-time">{l.t}</span>
            <span className="log-kind">{l.kind}</span>
            <span className="log-msg">{l.msg}</span>
          </div>
        ))}
        <div ref={endRef}></div>
      </div>
    </div>
  );
}

function FilesTab({task}) {
  const [view, setView] = useStateD('files');
  const files = task.files || [];
  const lines = task.diffPreview?.split('\n') || [];

  return (
    <div className="tab-files">
      <div className="files-tabs">
        <button className={view === 'files' ? 'active' : ''} onClick={() => setView('files')}>
          Worktree {files.length > 0 && <span className="tab-count">{files.length}</span>}
        </button>
        <button className={view === 'diff' ? 'active' : ''} onClick={() => setView('diff')}>Diff</button>
      </div>
      {view === 'files' && (
        <>
          {files.length === 0
            ? <div className="empty-state">No worktree yet — task hasn't started running.</div>
            : (
              <>
                <div className="worktree-meta">
                  <span>{files.length} files</span>
                  <span>·</span>
                  <span className="adds">+{files.reduce((s,f)=>s+f.adds,0)}</span>
                  <span className="dels">−{files.reduce((s,f)=>s+f.dels,0)}</span>
                  <code className="wt-branch">{task.branch || 'no branch'}</code>
                </div>
                <div className="file-list">
                  {files.map(f => (
                    <div key={f.path} className="file-row">
                      <span className={'file-status fs-' + f.status}>{f.status}</span>
                      <span className="file-path">{f.path}</span>
                      <span className="file-stats">
                        <span className="adds">+{f.adds}</span>
                        <span className="dels">−{f.dels}</span>
                      </span>
                    </div>
                  ))}
                </div>
              </>
            )}
        </>
      )}
      {view === 'diff' && (
        !task.diffPreview
          ? <div className="empty-state">No diff preview available yet.</div>
          : (
            <pre className="diff">
              {lines.map((l, i) => {
                let cls = 'diff-ctx';
                if (l.startsWith('+')) cls = 'diff-add';
                else if (l.startsWith('-')) cls = 'diff-del';
                else if (l.startsWith('@@')) cls = 'diff-hunk';
                return <div key={i} className={'diff-line ' + cls}>{l || ' '}</div>;
              })}
            </pre>
          )
      )}
    </div>
  );
}

// ─── Main drawer ─────────────────────────────────────────────────────────────

function TaskDrawer({task, onClose, onUpdate, onMove, onOpenBead, constitution, onEditConstitution}) {
  const [tab, setTab] = useStateD('overview');
  const isRunning = task?.column === 'running';
  const isReview  = task?.column === 'review';

  // Reset tab to overview whenever a new bead is opened
  useEffectD(() => { if (task) setTab('overview'); }, [task?.id]);

  useEffectD(() => {
    if (!task) return;
    const esc = (e) => { if (e.key === 'Escape') onClose(); };
    window.addEventListener('keydown', esc);
    return () => window.removeEventListener('keydown', esc);
  }, [task]);

  if (!task) return null;
  const {COLUMNS} = window.MUSTER_DATA;

  // Stale check: no activity for "a long time" (heuristic on lastActivity string)
  const isStale = task.lastActivity && ['Fri','Thu','Wed'].some(d => task.lastActivity.includes(d));

  return (
    // Non-blocking backdrop (pointer-events: none) + centered drawer.
    // Cards behind remain clickable to switch the active bead.
    <>
      <div className="drawer-backdrop" aria-hidden="true"></div>
      <aside className="drawer">
      <header className="drawer-head">
        <div className="drawer-head-row1">
          <span className="drawer-bead-id">{task.id}</span>
          <PriBadge n={task.priority} />
          {task.vcs && <span className={'vcs-badge vcs-' + task.vcs}>{task.vcs === 'jj' ? '⌥ jj' : '⎇ git'}</span>}
          {task.pinnedAgent && (() => {
            const A = window.MUSTER_DATA.AGENTS.find(a => a.id === task.pinnedAgent);
            return <span className="pinned-badge" title={`bd pin ${task.id} --for ${task.pinnedAgent}`}>⊣ {A?.mono || task.pinnedAgent}</span>;
          })()}
          {task.requeued && <span className="requeued-tag">requeued</span>}
          {isStale && <span className="stale-badge" title="bd stale — no activity for several days">stale</span>}
          <span className="drawer-spacer"></span>
          <button className="icon-btn" onClick={onClose} aria-label="Close">×</button>
        </div>
        <h2 className="drawer-title">{task.title}</h2>
        <div className="drawer-meta-row">
          <select
            className="col-select"
            value={task.column}
            onChange={(e) => onMove(task.id, e.target.value)}
          >
            {COLUMNS.map(c => <option key={c.id} value={c.id}>{c.name}</option>)}
          </select>
          <span className="drawer-created">{task.createdAt}</span>
          {isRunning && <span className="live-tag"><span className="live-dot"></span>running</span>}
          {isReview && task.reviewer?.comments > 0 && (
            <span className="review-comments">{task.reviewer.comments} comment{task.reviewer.comments > 1 ? 's' : ''}</span>
          )}
          {task.acceptance?.length > 0 && (() => {
            const done = task.acceptance.filter(a=>a.done).length;
            const total = task.acceptance.length;
            return <span className="ac-mini-progress" title={`${done}/${total} acceptance criteria done`}>{done}/{total} AC</span>;
          })()}
        </div>
      </header>

      <nav className="drawer-tabs">
        <button className={tab === 'overview'  ? 'active' : ''} onClick={() => setTab('overview')}>Overview</button>
        <button className={tab === 'deps'      ? 'active' : ''} onClick={() => setTab('deps')}>
          Deps
          {(task.blockedBy?.length || task.blocks?.length || task.externalDeps?.length || task.subBeads?.length) ? (
            <span className="tab-count">
              {(task.blockedBy?.length||0) + (task.blocks?.length||0) + (task.externalDeps?.length||0) + (task.subBeads?.length||0)}
            </span>
          ) : null}
        </button>
        <button className={tab === 'steps'     ? 'active' : ''} onClick={() => setTab('steps')}>
          Steps <span className="tab-count">{task.steps.length}</span>
        </button>
        <button className={tab === 'activity'  ? 'active' : ''} onClick={() => setTab('activity')}>
          Activity{task.history?.length > 0 && <span className="tab-count">{task.history.length}</span>}
        </button>
        <button className={tab === 'log'       ? 'active' : ''} onClick={() => setTab('log')}>
          Log{isRunning && <span className="tab-dot"></span>}
        </button>
        <button className={tab === 'files'     ? 'active' : ''} onClick={() => setTab('files')}>
          Files{task.files?.length ? <span className="tab-count">{task.files.length}</span> : null}
        </button>
      </nav>

      <div className="drawer-body">
        {tab === 'overview' && <OverviewTab task={task} onUpdate={onUpdate} constitution={constitution} onEditConstitution={onEditConstitution} />}
        {tab === 'deps'     && <DepsTab task={task} onOpenBead={onOpenBead} />}
        {tab === 'steps'    && <StepsTab task={task} onUpdate={onUpdate} />}
        {tab === 'activity' && <ActivityTab task={task} />}
        {tab === 'log'      && <RunLogTab task={task} />}
        {tab === 'files'    && <FilesTab task={task} />}
      </div>

      <footer className="drawer-foot">
        {task.column === 'backlog' && (
          <button className="btn btn-primary" onClick={() => onMove(task.id, 'scheduled')}>Schedule →</button>
        )}
        {task.column === 'scheduled' && (
          <button className="btn btn-primary" onClick={() => onMove(task.id, 'running')}>Dispatch now →</button>
        )}
        {task.column === 'running' && (
          <>
            <button className="btn btn-ghost">Pause</button>
            <button className="btn btn-primary" onClick={() => onMove(task.id, 'review')}>Send to review →</button>
          </>
        )}
        {task.column === 'review' && (
          <>
            <button className="btn btn-ghost" onClick={() => onMove(task.id, 'running')}>← Back to running</button>
            <button className="btn btn-primary" onClick={() => onMove(task.id, 'done')}>Approve &amp; close →</button>
          </>
        )}
        {task.column === 'done' && (
          <button className="btn btn-ghost" onClick={() => onMove(task.id, 'review')}>← Reopen</button>
        )}
      </footer>
      </aside>
    </>
  );
}

Object.assign(window, { TaskDrawer, SkillPicker });
