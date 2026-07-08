package skills

import "strings"

// Skill is a single registry entry (handoff §3.3). A skill is muster's own
// operating config, either built-in (embedded, read-only) or imported
// (persisted under <musterDir>/skills/<id>.md, mutable via CRUD).
//
// On disk a skill is a markdown file with a YAML front-matter header carrying
// the metadata fields and a markdown body that becomes PromptStub. The front-
// matter parser is added with the registry (US3); this type is the shared shape
// the assembly path (US1/US4) and the registry consume.
type Skill struct {
	ID         string   `json:"id" yaml:"id"`
	Name       string   `json:"name" yaml:"name"`
	Desc       string   `json:"desc" yaml:"desc"`
	Category   string   `json:"category" yaml:"category"`
	Icon       string   `json:"icon" yaml:"icon"`
	PromptStub string   `json:"promptStub" yaml:"-"` // markdown body, not front-matter
	MCPServers []string `json:"mcpServers" yaml:"mcpServers"`
	// BuiltIn marks a read-only embedded skill; DELETE on a built-in id fails
	// with SKILL_READONLY.
	BuiltIn bool `json:"builtIn" yaml:"-"`
}

// PromptStubFirstLine returns the first non-blank line of the skill's PromptStub
// (used in the assembled prompt's "Skills loaded" section per handoff §9). An
// empty stub yields an empty string — the skill is still listed by name.
func PromptStubFirstLine(s Skill) string {
	for line := range strings.Lines(s.PromptStub) {
		if t := strings.TrimSpace(line); t != "" {
			return t
		}
	}
	return ""
}
