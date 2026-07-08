package skills

import (
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

//go:embed builtin
var builtinFS embed.FS

// loadBuiltins parses every embedded built-in skill. Built-ins are read-only.
// A malformed embedded skill is a programming error (the catalog ships in the
// binary), so it is returned as an error to fail fast in tests/startup.
func loadBuiltins() ([]Skill, error) {
	entries, err := fs.ReadDir(builtinFS, "builtin")
	if err != nil {
		return nil, err
	}
	var out []Skill
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := builtinFS.ReadFile("builtin/" + e.Name())
		if err != nil {
			return nil, err
		}
		s, err := ParseSkill(data, true)
		if err != nil {
			return nil, fmt.Errorf("built-in skill %s: %w", e.Name(), err)
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
