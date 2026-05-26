package services

import (
	"time"

	"github.com/gitrgoliveira/muster/internal/core"
	"github.com/gitrgoliveira/muster/internal/store"
)

// IssueToBead converts a store.Issue to a core.Bead.
func IssueToBead(issue *store.Issue, repo string) core.Bead {
	col := statusToColumn(issue.Status)

	assigneeStr := issue.Assignee
	if assigneeStr == "" {
		assigneeStr = issue.Owner
	}

	bead := core.Bead{
		ID:         issue.ID,
		Title:      issue.Title,
		Desc:       issue.Description,
		Column:     col,
		Priority:   core.Priority(issue.Priority),
		Type:       issueTypeToBeadType(issue.IssueType),
		Repo:       repo,
		Assignee:   core.AgentID(assigneeStr),
		Labels:     []string{},
		Skills:     []string{},
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
func statusToColumn(status string) core.Column {
	switch status {
	case "in_progress":
		return core.ColRunning
	case "in_review":
		return core.ColReview
	case "closed", "cancelled", "superseded":
		return core.ColDone
	default: // "open", "scheduled", unknown
		return core.ColBacklog
	}
}

// columnToStatuses returns the status values for a given column name.
// Used to translate ?column= query params into store filter values.
func columnToStatuses(column string) []string {
	switch column {
	case "running":
		return []string{"in_progress"}
	case "review":
		return []string{"in_review"}
	case "done":
		return []string{"closed", "cancelled", "superseded"}
	default: // "backlog" or any other
		return []string{"open", "scheduled"}
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
