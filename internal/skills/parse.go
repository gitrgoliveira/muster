package skills

import (
	"bytes"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// frontMatter is the YAML header of a skill file. PromptStub is the markdown
// body, not a front-matter field.
type frontMatter struct {
	ID         string   `yaml:"id"`
	Name       string   `yaml:"name"`
	Desc       string   `yaml:"desc"`
	Category   string   `yaml:"category"`
	Icon       string   `yaml:"icon"`
	MCPServers []string `yaml:"mcpServers"`
}

// ParseSkill parses a skill markdown file: a YAML front-matter block delimited
// by `---` fences, followed by a markdown body that becomes PromptStub. The id
// must pass ValidateID. builtIn marks the result read-only.
func ParseSkill(data []byte, builtIn bool) (Skill, error) {
	s := string(data)
	if !strings.HasPrefix(s, "---") {
		return Skill{}, fmt.Errorf("skill: missing YAML front-matter (expected leading '---')")
	}
	// Strip the opening fence line.
	rest := strings.TrimPrefix(s, "---")
	rest = strings.TrimPrefix(rest, "\r")
	rest = strings.TrimPrefix(rest, "\n")

	// Find the closing fence at the start of a line.
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return Skill{}, fmt.Errorf("skill: unterminated front-matter (missing closing '---')")
	}
	front := rest[:end]
	body := rest[end+len("\n---"):]
	// Drop the remainder of the closing-fence line, then one separating newline.
	if nl := strings.IndexByte(body, '\n'); nl >= 0 {
		body = body[nl+1:]
	} else {
		body = ""
	}

	var fm frontMatter
	if err := yaml.Unmarshal([]byte(front), &fm); err != nil {
		return Skill{}, fmt.Errorf("skill: bad front-matter: %w", err)
	}
	if err := ValidateID(fm.ID); err != nil {
		return Skill{}, err
	}
	if fm.Name == "" {
		return Skill{}, fmt.Errorf("skill %q: name is required", fm.ID)
	}
	return Skill{
		ID:         fm.ID,
		Name:       fm.Name,
		Desc:       fm.Desc,
		Category:   fm.Category,
		Icon:       fm.Icon,
		PromptStub: strings.TrimRight(body, "\n"),
		MCPServers: fm.MCPServers,
		BuiltIn:    builtIn,
	}, nil
}

// formatSkill serializes a skill back to the on-disk front-matter + body form
// (used when persisting an imported skill).
func formatSkill(s Skill) ([]byte, error) {
	front, err := yaml.Marshal(frontMatter{
		ID: s.ID, Name: s.Name, Desc: s.Desc,
		Category: s.Category, Icon: s.Icon, MCPServers: s.MCPServers,
	})
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	b.WriteString("---\n")
	b.Write(front)
	b.WriteString("---\n")
	b.WriteString(s.PromptStub)
	b.WriteString("\n")
	return b.Bytes(), nil
}
