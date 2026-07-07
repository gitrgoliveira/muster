package orchestrator

// SPIKE T004 (verified on this host, 2026-07-07) — claude MCP config discovery.
// Implemented in US4 (T054); this block pins where the best-effort check reads.
//
// `claude mcp list` prints human-readable lines ("<name>: <url> - <status>") and
// has NO --json form, so parsing it is fragile. The authoritative source is the
// claude CLI's own config file:
//
//	~/.claude.json    (mode 0600; a large JSON doc that includes MCP server config)
//
// So the best-effort MCP check (FR-021) reads ~/.claude.json (path overridable
// via a --claude-config-path flag / MUSTER_CLAUDE_CONFIG env for non-default
// installs), extracts the configured MCP server names, and warns for any skill
// MCPServers entry not present. Per FR-021/FR-022 the check is READ-ONLY and
// NON-BLOCKING: an unreadable/absent config is treated as "server not found" — a
// runlog.warning, never a dispatch failure — and muster never spawns or manages
// an MCP server.
