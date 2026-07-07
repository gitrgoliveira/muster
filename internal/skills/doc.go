// Package skills is muster's skill registry: embedded read-only built-in skills
// plus user/URL-imported skills persisted as local markdown files under
// <musterDir>/skills/. Skills are muster's own operating config (not beads issue
// state): a skill contributes its PromptStub to an assembled dispatch prompt and
// may name MCP servers the agent is expected to already have configured.
//
// Skill selection is carried per-bead as reserved `skill:<id>` bd labels
// (resolved into core.Bead.Skills) and per-dispatch via Step.Skills; assembly
// resolves the de-duplicated union against this registry.
package skills
