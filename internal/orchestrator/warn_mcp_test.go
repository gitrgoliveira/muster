package orchestrator

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gitrgoliveira/muster/internal/core"
	"github.com/gitrgoliveira/muster/internal/skills"
)

func writeClaudeConfig(t *testing.T, servers ...string) string {
	t.Helper()
	mcp := map[string]any{}
	for _, s := range servers {
		mcp[s] = map[string]any{}
	}
	data, err := json.Marshal(map[string]any{"mcpServers": mcp})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), ".claude.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func assembleWithMCP(t *testing.T, configPath string, skill skills.Skill) []string {
	t.Helper()
	o := &Orchestrator{
		skills:           fakeSkillProvider{m: map[string]skills.Skill{skill.ID: skill}},
		claudeConfigPath: configPath,
	}
	frames := captureWarnings(o)
	req := DispatchRequest{BeadID: "b-1", BeadTitle: "T", BeadDesc: "D", Agent: core.AgentID("claude")}
	if got := o.assemblePrompt(nil, req, core.ModeAgent, 0, []string{skill.ID}); got == "" {
		t.Fatal("assembly blocked by MCP check")
	}
	var reasons []string
	for _, f := range *frames {
		reasons = append(reasons, f.Reason)
	}
	return reasons
}

func TestMCP_MissingServerWarnsNotBlocks(t *testing.T) {
	cfg := writeClaudeConfig(t, "present-server")
	skill := skills.Skill{ID: "needs-mcp", Name: "Needs MCP", MCPServers: []string{"absent-server"}}
	reasons := assembleWithMCP(t, cfg, skill)
	if !anyContains(reasons, "absent-server") {
		t.Fatalf("expected a warning for the absent MCP server, got %v", reasons)
	}
}

func TestMCP_PresentServerNoWarn(t *testing.T) {
	cfg := writeClaudeConfig(t, "present-server")
	skill := skills.Skill{ID: "needs-mcp", Name: "Needs MCP", MCPServers: []string{"present-server"}}
	if reasons := assembleWithMCP(t, cfg, skill); anyContains(reasons, "present-server") {
		t.Fatalf("present server should not warn, got %v", reasons)
	}
}

func TestMCP_UnreadableConfigTreatedAsNotFound(t *testing.T) {
	// A non-existent config path => every named server warns (not a block).
	skill := skills.Skill{ID: "needs-mcp", Name: "Needs MCP", MCPServers: []string{"any-server"}}
	reasons := assembleWithMCP(t, filepath.Join(t.TempDir(), "nope.json"), skill)
	if !anyContains(reasons, "any-server") {
		t.Fatalf("unreadable config should warn for the named server, got %v", reasons)
	}
}

func anyContains(ss []string, sub string) bool {
	for _, s := range ss {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
