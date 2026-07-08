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
	Agent          core.AgentID        `json:"agent"`
	Mode           core.Mode           `json:"mode"`
	PermissionMode core.PermissionMode `json:"permissionMode,omitempty"`
	// Chain is an optional per-dispatch step-chain override (contract
	// http-endpoints.md: "Request body unchanged, plus optional additive
	// field `chain`"). Omitted/empty means the M2 single-step default.
	Chain []ChainStepRequest `json:"chain,omitempty"`
	// Skills is the optional per-dispatch step-level skill override (FR-018):
	// unioned additively on top of the bead's skill:<id> labels, never
	// subtractive. Omitted/empty means the loadout is the bead-level set alone.
	Skills []string `json:"skills,omitempty"`
}

// ChainStepRequest is the wire shape of a single step in DispatchRequest.Chain.
type ChainStepRequest struct {
	Name           string              `json:"name"`
	PermissionMode core.PermissionMode `json:"permissionMode,omitempty"`
	PromptRef      string              `json:"promptRef,omitempty"`
}

type CommentRequest struct {
	Actor string `json:"actor"`
	Note  string `json:"note"`
}
