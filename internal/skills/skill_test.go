package skills

import "testing"

const validSkill = `---
id: repo-grep
name: Repo Grep
desc: Search fast
category: code
icon: 🔎
mcpServers: [ctx7]
---
Use ripgrep before editing.
Second line.
`

func TestParseSkill_Valid(t *testing.T) {
	s, err := ParseSkill([]byte(validSkill), false)
	if err != nil {
		t.Fatal(err)
	}
	if s.ID != "repo-grep" || s.Name != "Repo Grep" || s.Category != "code" || s.Icon != "🔎" {
		t.Fatalf("front-matter parse wrong: %+v", s)
	}
	if len(s.MCPServers) != 1 || s.MCPServers[0] != "ctx7" {
		t.Fatalf("mcpServers wrong: %v", s.MCPServers)
	}
	if s.PromptStub != "Use ripgrep before editing.\nSecond line." {
		t.Fatalf("body/PromptStub wrong: %q", s.PromptStub)
	}
	if s.BuiltIn {
		t.Fatal("imported skill should not be BuiltIn")
	}
	if PromptStubFirstLine(s) != "Use ripgrep before editing." {
		t.Fatalf("firstline = %q", PromptStubFirstLine(s))
	}
}

func TestParseSkill_Errors(t *testing.T) {
	cases := map[string]string{
		"no front-matter": "just a body",
		"unterminated":    "---\nid: x\nname: X\n",
		"missing name":    "---\nid: x\n---\nbody",
		"invalid id":      "---\nid: ../evil\nname: X\n---\nbody",
		"empty id":        "---\nname: X\n---\nbody",
	}
	for name, data := range cases {
		if _, err := ParseSkill([]byte(data), false); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestParseSkill_EmptyStubOK(t *testing.T) {
	s, err := ParseSkill([]byte("---\nid: x\nname: X\n---\n"), false)
	if err != nil {
		t.Fatal(err)
	}
	if s.PromptStub != "" {
		t.Fatalf("expected empty stub, got %q", s.PromptStub)
	}
}

func TestBuiltins_LoadAndParse(t *testing.T) {
	bs, err := loadBuiltins()
	if err != nil {
		t.Fatal(err)
	}
	if len(bs) == 0 {
		t.Fatal("expected a non-empty built-in catalog")
	}
	for _, b := range bs {
		if !b.BuiltIn || b.ID == "" || b.Name == "" {
			t.Errorf("bad built-in: %+v", b)
		}
	}
}
