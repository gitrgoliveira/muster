package beads

import "github.com/gitrgoliveira/muster/internal/core"

type ListResponse struct {
	Items      []core.Bead `json:"items"`
	NextCursor *string     `json:"nextCursor"`
	Total      int         `json:"total"`
}

type CreateRequest struct {
	Title        string         `json:"title"`
	Desc         string         `json:"desc,omitempty"`
	Type         core.BeadType  `json:"type,omitempty"`
	Column       core.Column    `json:"column,omitempty"`
	Priority     *core.Priority `json:"priority,omitempty"`
	Assignee     string         `json:"assignee,omitempty"`
	Labels       []string       `json:"labels,omitempty"`
	VCS          core.VCS       `json:"vcs,omitempty"`
	TokensBudget int            `json:"tokensBudget,omitempty"`
}

type PatchRequest struct {
	Title *string `json:"title,omitempty"`
	// Desc and Description are aliases; the bd-cli-bridge contract accepts both.
	Desc         *string        `json:"desc,omitempty"`
	Description  *string        `json:"description,omitempty"`
	Type         *core.BeadType `json:"type,omitempty"`
	Column       *core.Column   `json:"column,omitempty"`
	Priority     *core.Priority `json:"priority,omitempty"`
	Assignee     *string        `json:"assignee,omitempty"`
	Labels       *[]string      `json:"labels,omitempty"`
	Ready        *bool          `json:"ready,omitempty"`
	TokensBudget *int           `json:"tokensBudget,omitempty"`
}

type MoveRequest struct {
	ToColumn core.Column `json:"toColumn"`
	BeforeID string      `json:"beforeID,omitempty"`
}

type DispatchRequest struct {
	Agent          core.AgentID       `json:"agent"`
	Mode           core.Mode          `json:"mode"`
	PermissionMode core.PermissionMode `json:"permissionMode,omitempty"`
}

type CommentRequest struct {
	Actor string `json:"actor"`
	Note  string `json:"note"`
}
