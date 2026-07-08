package orchestrator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gitrgoliveira/muster/internal/skills"
	"github.com/gitrgoliveira/muster/internal/ws"
)

// SPIKE T004 (verified on this host, 2026-07-07) — claude MCP config discovery.
//
// `claude mcp list` prints human-readable lines and has NO --json form, so the
// authoritative source is the claude CLI's own config file: ~/.claude.json
// (mode 0600; a JSON doc whose top-level and per-project `mcpServers` objects
// name the configured servers). The best-effort MCP check (FR-021) reads that
// file (path overridable via --claude-config-path / MUSTER_CLAUDE_CONFIG),
// collects the configured server names, and warns for any skill MCPServers
// entry that is absent. The check is READ-ONLY and NON-BLOCKING: an unreadable
// or absent config is treated as "server not found" — a runlog.warning, never a
// dispatch failure — and muster never spawns or manages an MCP server (FR-022).

// warn emits a non-blocking runlog.warning tied to a bead/step. Nil-safe: with
// no publisher (e.g. in tests) it is a no-op, and it never affects the assembled
// prompt string.
func (o *Orchestrator) warn(beadID string, stepIdx int, msg string) {
	if o.publish == nil {
		return
	}
	idx := stepIdx
	o.publish(ws.Frame{
		Type:    ws.EventRunlogWarning,
		BeadID:  beadID,
		StepIdx: &idx,
		Reason:  msg,
	})
}

// claudeConfigFile returns the resolved path to the claude CLI config, honoring
// the configured override then falling back to ~/.claude.json.
func (o *Orchestrator) claudeConfigFile() string {
	if o.claudeConfigPath != "" {
		return o.claudeConfigPath
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude.json")
}

// claudeMCPConfig models the subset of ~/.claude.json we read: the top-level
// and per-project mcpServers maps (values are ignored — we only need the names).
type claudeMCPConfig struct {
	MCPServers map[string]json.RawMessage `json:"mcpServers"`
	Projects   map[string]struct {
		MCPServers map[string]json.RawMessage `json:"mcpServers"`
	} `json:"projects"`
}

// configuredMCPServers reads the set of MCP server names from the agent's own
// config. The bool is false when the config could not be read (absent/unreadable
// /unparseable) — treated as "no servers found", so every named server warns.
func (o *Orchestrator) configuredMCPServers() (map[string]bool, bool) {
	path := o.claudeConfigFile()
	if path == "" {
		return nil, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var cfg claudeMCPConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, false
	}
	set := make(map[string]bool)
	for name := range cfg.MCPServers {
		set[name] = true
	}
	for _, p := range cfg.Projects {
		for name := range p.MCPServers {
			set[name] = true
		}
	}
	return set, true
}

// verifyMCPServers performs the best-effort, non-blocking check (FR-021/FR-022):
// for each MCP server a resolved skill names, warn if it is absent from the
// agent's own config. Never blocks; never manages a server.
func (o *Orchestrator) verifyMCPServers(beadID string, stepIdx int, loadout []skills.Skill) {
	// Collect the distinct servers the loadout expects.
	var wanted []string
	seen := make(map[string]bool)
	for _, s := range loadout {
		for _, srv := range s.MCPServers {
			if srv != "" && !seen[srv] {
				seen[srv] = true
				wanted = append(wanted, srv)
			}
		}
	}
	if len(wanted) == 0 {
		return
	}
	configured, _ := o.configuredMCPServers() // ok=false => every server is "not found"
	for _, srv := range wanted {
		if !configured[srv] {
			o.warn(beadID, stepIdx, fmt.Sprintf("MCP server %q is not in the agent's config; the skill may not work", srv))
		}
	}
}
