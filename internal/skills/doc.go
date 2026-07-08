// Package skills is muster's skill registry: embedded read-only built-in skills
// plus user/URL-imported skills persisted as local markdown files under
// <musterDir>/skills/. Skills are muster's own operating config (not beads issue
// state): a selected skill appears in an assembled dispatch prompt's "Skills
// loaded" section as its name plus the FIRST non-blank line of its PromptStub
// (via skills.PromptStubFirstLine — the full body is not injected), and may name
// MCP servers the agent is expected to already have configured.
//
// Skill selection is carried per-bead as reserved `skill:<id>` bd labels
// (resolved into core.Bead.Skills) and per-dispatch via Step.Skills; assembly
// resolves the de-duplicated union against this registry.
package skills
