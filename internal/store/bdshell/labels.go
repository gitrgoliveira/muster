package bdshell

// SPIKE T003 (verified against real bd 1.1.0, 2026-07-07) — label read contract.
// Implemented in US4 (T042); this block pins the shape the read verb builds to.
//
// IMPORTANT: `bd show <id> --json` does NOT include labels (its keys are
// id/title/description/status/priority/issue_type/owner/spec_id/created_at/
// updated_at/created_by/comment_count/dependency_count/dependent_count). So the
// label read MUST use the dedicated label command, not show-enrichment (this is
// exactly the review's F4 concern, now confirmed):
//
//	bd label list <id> --json     -> a JSON ARRAY of strings, e.g. ["skill:repo-grep"];
//	                                 `[]` when the bead has no labels.
//	bd label add <id> <label>     -> `✓ Added label '<label>' to <id>`   (write; muster reads only)
//	bd label remove <id> <label>  -> `✓ Removed label '<label>' from <id>`
//
// The read verb `Labels(ctx, id) ([]string, error)` runs `bd label list <id>
// --json` via the exec pattern and unmarshals the string array. IssueToBead then
// splits `skill:<id>` entries (each id passing skills.ValidateID) into
// core.Bead.Skills and the rest into core.Bead.Labels. A dispatch-time read (not
// per List row) avoids an N+1 on the beads list.

import "context"

// Labels reads a bead's labels via `bd label list <id> --json`, which returns a
// JSON array of label strings ([] when none).
func (c *CLI) Labels(ctx context.Context, id string) ([]string, error) {
	var labels []string
	if err := c.RunJSON(ctx, &labels, "label", "list", "--json", "--", id); err != nil {
		return nil, err
	}
	return labels, nil
}
