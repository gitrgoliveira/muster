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
	// The opening fence must be a standalone `---` line (LF or CRLF), not a
	// prefix like `----` or `---not-a-fence`.
	rest, ok := strings.CutPrefix(s, "---\n")
	if !ok {
		rest, ok = strings.CutPrefix(s, "---\r\n")
	}
	if !ok {
		return Skill{}, fmt.Errorf("skill: missing YAML front-matter (expected a leading '---' line)")
	}
	// The closing fence must likewise be a standalone `---` line, so a `---`
	// inside a YAML scalar or the body is not mistaken for the terminator.
	front, body, ok := splitAtClosingFence(rest)
	if !ok {
		return Skill{}, fmt.Errorf("skill: unterminated front-matter (missing closing '---' line)")
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

// splitAtClosingFence scans rest line by line for the first standalone `---`
// line (LF or CRLF, EOF-safe) and returns the front-matter before it and the
// body after it. A line like `----` or `---not-a-fence` is NOT a fence. ok is
// false when no closing fence line exists.
func splitAtClosingFence(rest string) (front, body string, ok bool) {
	for pos := 0; pos <= len(rest); {
		nl := strings.IndexByte(rest[pos:], '\n')
		line := rest[pos:]
		lineEnd := len(rest) // index just past this line (past its '\n', or EOF)
		if nl >= 0 {
			line = rest[pos : pos+nl]
			lineEnd = pos + nl + 1
		}
		if strings.TrimSuffix(line, "\r") == "---" {
			return rest[:pos], rest[lineEnd:], true
		}
		if nl < 0 {
			break // last line, no fence
		}
		pos = lineEnd
	}
	return "", "", false
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
