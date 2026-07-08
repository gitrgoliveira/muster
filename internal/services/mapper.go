package services

import (
	"strings"
	"time"

	"github.com/gitrgoliveira/muster/internal/core"
	"github.com/gitrgoliveira/muster/internal/store"
)

// skillLabelPrefix marks a bead label that selects a skill (M6 US4, option b).
const skillLabelPrefix = "skill:"

// SplitSkillLabels partitions a bead's labels into skill ids (from the reserved
// `skill:<id>` prefix, stripped) and the remaining plain labels. It does not
// validate the ids — an empty or malformed id is returned as-is so the assembly
// layer can warn on it (never a silent drop, FR-020).
func SplitSkillLabels(labels []string) (skillIDs, plain []string) {
	for _, l := range labels {
		if id, ok := strings.CutPrefix(l, skillLabelPrefix); ok {
			skillIDs = append(skillIDs, id)
		} else {
			plain = append(plain, l)
		}
	}
	return skillIDs, plain
}

// IssueToBead converts a store.Issue to a core.Bead.
func IssueToBead(issue *store.Issue, repo string) core.Bead {
	col := statusToColumn(issue.Status)

	assigneeStr := issue.Assignee
	if assigneeStr == "" {
		assigneeStr = issue.Owner
	}

	bead := core.Bead{
		ID:       issue.ID,
		Title:    issue.Title,
		Desc:     issue.Description,
		Column:   col,
		Priority: core.Priority(issue.Priority),
		Type:     issueTypeToBeadType(issue.IssueType),
		Ready:    issue.DependencyCount == 0 && issue.Status == "open",
		Repo:     repo,
		Assignee: core.AgentID(assigneeStr),
		Labels:   []string{},
		Skills:   []string{},
		// (Labels/Skills split below from issue.Labels when present.)
		Steps:      []core.Step{},
		SubBeads:   []core.SubBead{},
		Gates:      []core.Gate{},
		History:    buildHistory(issue),
		Acceptance: []core.Acceptance{},
		Log:        []core.LogEntry{},
		Files:      []core.FileChange{},
		Blocks:     []string{},
		BlockedBy:  []string{},
		Comments:   issue.CommentCount,
	}

	if len(issue.Labels) > 0 {
		skillIDs, plain := SplitSkillLabels(issue.Labels)
		if len(plain) > 0 {
			bead.Labels = plain
		}
		if len(skillIDs) > 0 {
			bead.Skills = skillIDs
		}
	}

	bead.CreatedAt = issue.CreatedAt.UTC().Format(time.RFC3339)
	bead.OpenedAt = issue.CreatedAt.UTC().Format(time.RFC3339)
	bead.LastActivity = issue.UpdatedAt.UTC().Format(time.RFC3339)
	if issue.ClosedAt != nil {
		bead.ClosedAt = issue.ClosedAt.UTC().Format(time.RFC3339)
	}

	return bead
}

// IssueToBeads converts a slice of store.Issue to []core.Bead.
func IssueToBeads(issues []store.Issue, repo string) []core.Bead {
	beads := make([]core.Bead, len(issues))
	for i := range issues {
		beads[i] = IssueToBead(&issues[i], repo)
	}
	return beads
}

// statusToColumn maps a beads status value to a Kanban column.
// Per bd-cli-bridge.md (M1 simplification): beads does not model a review
// status, so review is folded into running. ColReview remains in the enum
// for client-side UI use but no server-side bead is ever rendered into it.
func statusToColumn(status string) core.Column {
	switch status {
	case "in_progress":
		return core.ColRunning
	case "closed", "cancelled", "superseded":
		return core.ColDone
	default: // "open", "scheduled", "blocked", "deferred", unknown
		return core.ColBacklog
	}
}

// columnToStatuses returns the status values for a given column name.
// Used to translate ?column= query params into store filter values.
// Per bd-cli-bridge.md, review maps to in_progress (M1 simplification).
func columnToStatuses(column string) []string {
	switch column {
	case "running", "review":
		return []string{"in_progress"}
	case "done":
		return []string{"closed", "cancelled", "superseded"}
	default: // "backlog" or any other — per data-model.md
		return []string{"open", "scheduled", "blocked", "deferred"}
	}
}

func issueTypeToBeadType(issueType string) core.BeadType {
	switch issueType {
	case "bug":
		return core.TypeBug
	case "feature":
		return core.TypeFeature
	case "epic":
		return core.TypeEpic
	case "chore":
		return core.TypeChore
	default:
		return core.TypeTask
	}
}

func buildHistory(issue *store.Issue) []core.HistoryEvent {
	history := []core.HistoryEvent{
		{
			Kind: core.EvOpened,
			At:   issue.CreatedAt.UTC().Format(time.RFC3339),
		},
	}
	if issue.StartedAt != nil {
		history = append(history, core.HistoryEvent{
			Kind: core.EvStarted,
			At:   issue.StartedAt.UTC().Format(time.RFC3339),
		})
	}
	if issue.ClosedAt != nil {
		history = append(history, core.HistoryEvent{
			Kind: core.EvClosed,
			At:   issue.ClosedAt.UTC().Format(time.RFC3339),
		})
	}
	return history
}
