// Add-provider modal — choose CLI tool or direct API, then configure.

const {useState: useStateP, useEffect: useEffectP} = React;

const CLI_CATALOG = [
  { id: 'claude',   name: 'Claude Code',   sub: 'Anthropic',  pkg: 'brew install anthropic-claude-code', detected: true,  installed: '/opt/homebrew/bin/claude' },
  { id: 'gemini',   name: 'Gemini CLI',    sub: 'Google',     pkg: 'npm i -g @google/gemini-cli',         detected: true,  installed: '/opt/homebrew/bin/gemini' },
  { id: 'opencode', name: 'OpenCode',      sub: 'open source',pkg: 'curl -fsSL opencode.ai/install | sh', detected: true,  installed: '/opt/homebrew/bin/opencode' },
  { id: 'codex',    name: 'Codex',         sub: 'OpenAI',     pkg: 'npm i -g @openai/codex',              detected: true,  installed: '/opt/homebrew/bin/codex' },
  { id: 'aider',    name: 'Aider',         sub: 'open source',pkg: 'pip install aider-chat',              detected: false },
  { id: 'goose',    name: 'Goose',         sub: 'Block',      pkg: 'brew install block-goose-cli',        detected: false },
  { id: 'amp',      name: 'Amp',           sub: 'Sourcegraph',pkg: 'brew install sourcegraph/amp/amp',    detected: false },
];

const API_CATALOG = [
  { id: 'anthropic',  name: 'Anthropic API',    sub: 'claude-sonnet-4.5, opus',    url: 'https://api.anthropic.com/v1',          keyPrefix: 'sk-ant-' },
  { id: 'openai',     name: 'OpenAI API',       sub: 'gpt-4.1, o3, o4-mini',       url: 'https://api.openai.com/v1',             keyPrefix: 'sk-proj-' },
  { id: 'openrouter', name: 'OpenRouter',       sub: 'multi-model gateway',         url: 'https://openrouter.ai/api/v1',          keyPrefix: 'sk-or-' },
  { id: 'mistral',    name: 'Mistral',          sub: 'mistral-large, codestral',   url: 'https://api.mistral.ai/v1',             keyPrefix: 'msk-' },
  { id: 'groq',       name: 'Groq',             sub: 'fast llama, mixtral',         url: 'https://api.groq.com/openai/v1',        keyPrefix: 'gsk_' },
  { id: 'together',   name: 'Together AI',      sub: 'open-weights catalogue',      url: 'https://api.together.xyz/v1',           keyPrefix: '' },
  { id: 'ollama',     name: 'Local Ollama',     sub: 'self-hosted, no auth',        url: 'http://localhost:11434/v1',             keyPrefix: '', local: true },
  { id: 'lmstudio',   name: 'LM Studio',        sub: 'self-hosted, GGUF models',    url: 'http://localhost:1234/v1',              keyPrefix: '', local: true },
];

function AddProviderModal({onClose, onAdd}) {
  const [kind, setKind] = useStateP('cli');  // cli | api
  const [selected, setSelected] = useStateP(null);
  const [apiKey, setApiKey] = useStateP('');
  const [model, setModel] = useStateP('');
  const [name, setName] = useStateP('');

  useEffectP(() => {
    const esc = (e) => { if (e.key === 'Escape') onClose(); };
    window.addEventListener('keydown', esc);
    return () => window.removeEventListener('keydown', esc);
  }, []);

  const canSubmit = kind === 'cli'
    ? selected
    : selected && (selected.local || apiKey.length > 8) && model.length > 0;

  const submit = () => {
    if (!canSubmit) return;
    onAdd({ kind, selected, apiKey, model, name });
    onClose();
  };

  return (
    <div className="composer-scrim" onClick={onClose}>
      <div className="composer addprov" onClick={(e) => e.stopPropagation()}>
        <header className="composer-head">
          <h2>Add a provider</h2>
          <button className="icon-btn" onClick={onClose}>×</button>
        </header>

        <nav className="addprov-tabs">
          <button className={kind === 'cli' ? 'active' : ''} onClick={() => { setKind('cli'); setSelected(null); }}>
            <strong>CLI tool</strong>
            <span>local binary · uses its own auth</span>
          </button>
          <button className={kind === 'api' ? 'active' : ''} onClick={() => { setKind('api'); setSelected(null); }}>
            <strong>Direct API</strong>
            <span>HTTP endpoint · API key</span>
          </button>
        </nav>

        <div className="composer-body">
          {kind === 'cli' ? (
            <>
              <p className="addprov-help">
                Muster invokes the CLI on each step. The CLI handles its own subscription, auth and rate limits.
                Detected binaries are ready to use; the rest show install instructions.
              </p>
              <div className="addprov-grid">
                {CLI_CATALOG.map(c => (
                  <button
                    key={c.id}
                    className={'addprov-card ' + (selected?.id === c.id ? 'on' : '')}
                    onClick={() => setSelected(c)}
                  >
                    <div className="addprov-card-head">
                      <span className="addprov-name">{c.name}</span>
                      {c.detected
                        ? <span className="addprov-tag detected">✓ detected</span>
                        : <span className="addprov-tag missing">not installed</span>
                      }
                    </div>
                    <div className="addprov-card-sub">{c.sub}</div>
                    <code className="addprov-card-code">{c.detected ? c.installed : c.pkg}</code>
                  </button>
                ))}
              </div>

              {selected && (
                <div className="addprov-config">
                  <span className="composer-label">Step name <span style={{color:'var(--ink-4)'}}>(optional)</span></span>
                  <input
                    className="composer-input"
                    placeholder={selected.name + ' #2'}
                    value={name}
                    onChange={(e) => setName(e.target.value)}
                    style={{fontSize: 15}}
                  />
                  <p className="addprov-postnote">
                    {selected.detected
                      ? <>Will run <code>{selected.installed} --help</code> to verify and read its native modes.</>
                      : <>Run <code>{selected.pkg}</code> first, then re-add.</>
                    }
                  </p>
                </div>
              )}
            </>
          ) : (
            <>
              <p className="addprov-help">
                Hits the LLM provider's HTTP API directly. You're responsible for your own auth and rate-limit handling.
                These won't have native CLI modes — Muster builds the plan/build/agent/review loop in process.
              </p>
              <div className="addprov-grid">
                {API_CATALOG.map(c => (
                  <button
                    key={c.id}
                    className={'addprov-card ' + (selected?.id === c.id ? 'on' : '')}
                    onClick={() => setSelected(c)}
                  >
                    <div className="addprov-card-head">
                      <span className="addprov-name">{c.name}</span>
                      {c.local && <span className="addprov-tag local">local</span>}
                    </div>
                    <div className="addprov-card-sub">{c.sub}</div>
                    <code className="addprov-card-code">{c.url}</code>
                  </button>
                ))}
              </div>

              {selected && (
                <div className="addprov-config">
                  <div className="addprov-config-row">
                    <label className="composer-label">Base URL</label>
                    <code className="addprov-readonly">{selected.url}</code>
                  </div>

                  {!selected.local && (
                    <div className="addprov-config-row">
                      <label className="composer-label">API key</label>
                      <input
                        className="addprov-input"
                        type="password"
                        placeholder={selected.keyPrefix + '…'}
                        value={apiKey}
                        onChange={(e) => setApiKey(e.target.value)}
                        autoComplete="off"
                      />
                      <span className="addprov-key-hint">stored in macOS Keychain · never written to disk</span>
                    </div>
                  )}

                  <div className="addprov-config-row">
                    <label className="composer-label">Default model</label>
                    <input
                      className="addprov-input"
                      placeholder={selected.id === 'anthropic' ? 'claude-sonnet-4-5' : selected.id === 'openai' ? 'gpt-4.1' : selected.id === 'ollama' ? 'qwen2.5-coder:32b' : 'pick a model'}
                      value={model}
                      onChange={(e) => setModel(e.target.value)}
                    />
                  </div>

                  <div className="addprov-config-row">
                    <label className="composer-label">Display name <span style={{color:'var(--ink-4)'}}>(optional)</span></label>
                    <input
                      className="addprov-input"
                      placeholder={selected.name}
                      value={name}
                      onChange={(e) => setName(e.target.value)}
                    />
                  </div>

                  <div className="addprov-modes-note">
                    <strong>Synthetic modes:</strong> Plan · Build · Agent · Review will be emulated via system prompt wrappers. No native YOLO/Apply distinctions.
                  </div>
                </div>
              )}
            </>
          )}
        </div>

        <footer className="composer-foot">
          <div className="composer-foot-left">
            {selected
              ? <span>Ready to add <strong>{selected.name}</strong> as a {kind === 'cli' ? 'CLI' : 'direct API'} provider</span>
              : <span>Pick a provider above</span>
            }
          </div>
          <div className="composer-foot-right">
            <button className="btn btn-ghost" onClick={onClose}>Cancel</button>
            <button className="btn btn-primary" onClick={submit} disabled={!canSubmit}>
              Add provider →
            </button>
          </div>
        </footer>
      </div>
    </div>
  );
}

Object.assign(window, { AddProviderModal });
