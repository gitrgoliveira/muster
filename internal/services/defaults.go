package services

import (
	"time"

	"github.com/gitrgoliveira/muster/internal/core"
)

// applyCreateDefaults fills in all zero-value fields of b with defaults.
// Called before a new bead is passed to the store.
func applyCreateDefaults(b *core.Bead) {
	now := time.Now().UTC().Format(time.RFC3339)

	if b.Column == "" {
		b.Column = core.ColBacklog
	}
	if b.Type == "" {
		b.Type = core.TypeTask
	}
	// Priority is NOT defaulted here: 0 = "critical" (valid), so callers must
	// translate DTO nil-pointer → 2 before invoking applyCreateDefaults.
	if b.VCS == "" {
		b.VCS = core.VCSGit
	}
	if b.Repo == "" {
		b.Repo = "main"
	}

	b.TokensUsed = 0
	b.CreatedAt = now
	b.OpenedAt = now
	b.LastActivity = now

	if b.Labels == nil {
		b.Labels = []string{}
	}
	if b.Skills == nil {
		b.Skills = []string{}
	}
	if b.Steps == nil {
		b.Steps = []core.Step{}
	}
	if b.SubBeads == nil {
		b.SubBeads = []core.SubBead{}
	}
	b.History = []core.HistoryEvent{
		{Kind: core.EvOpened, Actor: "user", At: now},
	}
	if b.Acceptance == nil {
		b.Acceptance = []core.Acceptance{}
	}
	if b.Log == nil {
		b.Log = []core.LogEntry{}
	}
	if b.Files == nil {
		b.Files = []core.FileChange{}
	}
	if b.Blocks == nil {
		b.Blocks = []string{}
	}
	if b.BlockedBy == nil {
		b.BlockedBy = []string{}
	}
	b.Comments = 0
}
