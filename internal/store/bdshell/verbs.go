package bdshell

import (
	"context"
	"fmt"

	"github.com/gitrgoliveira/muster/internal/store"
)

// CreateInput holds the fields needed to create a new issue via bd.
type CreateInput struct {
	Title       string
	Description string
	Type        string
	Priority    *int
}

// UpdatePatch holds the optional fields to update on an existing issue.
type UpdatePatch struct {
	Title       *string
	Description *string
	Status      *string
	Priority    *int
	Type        *string
	AppendNotes *string
	Claim       bool
}

// Create runs `bd create --json` and returns the created issue.
func (c *CLI) Create(ctx context.Context, in CreateInput) (store.Issue, error) {
	args := []string{"create", "--json", "--dolt-auto-commit=on"}
	if in.Title != "" {
		args = append(args, "--title="+in.Title)
	}
	if in.Description != "" {
		args = append(args, "--description="+in.Description)
	}
	if in.Type != "" {
		args = append(args, "--type="+in.Type)
	}
	if in.Priority != nil {
		args = append(args, fmt.Sprintf("--priority=%d", *in.Priority))
	}
	var iss store.Issue
	if err := c.RunJSON(ctx, &iss, args...); err != nil {
		return store.Issue{}, err
	}
	return iss, nil
}

// Update runs `bd update <id>` with the given patch and returns the updated issue.
// bd update --json returns a JSON array; we unmarshal and return the first element.
func (c *CLI) Update(ctx context.Context, id string, p UpdatePatch) (store.Issue, error) {
	args := []string{"update", id, "--json", "--dolt-auto-commit=on"}
	if p.Claim {
		args = append(args, "--claim")
	}
	if p.Title != nil {
		args = append(args, "--title="+*p.Title)
	}
	if p.Description != nil {
		args = append(args, "--description="+*p.Description)
	}
	if p.Status != nil {
		args = append(args, "--status="+*p.Status)
	}
	if p.Priority != nil {
		args = append(args, fmt.Sprintf("--priority=%d", *p.Priority))
	}
	if p.Type != nil {
		args = append(args, "--type="+*p.Type)
	}
	if p.AppendNotes != nil {
		args = append(args, "--append-notes="+*p.AppendNotes)
	}
	var issues []store.Issue
	if err := c.RunJSON(ctx, &issues, args...); err != nil {
		return store.Issue{}, err
	}
	if len(issues) == 0 {
		return store.Issue{}, fmt.Errorf("bd update returned empty array")
	}
	return issues[0], nil
}

// Close runs `bd close <id> --dolt-auto-commit=on`.
func (c *CLI) Close(ctx context.Context, id string) error {
	return c.RunVoid(ctx, "close", id, "--dolt-auto-commit=on")
}

// Dispatch claims a bead (bd update <id> --claim --json).
func (c *CLI) Dispatch(ctx context.Context, id string) (store.Issue, error) {
	return c.Update(ctx, id, UpdatePatch{Claim: true})
}

// AppendNote appends text to a bead's notes (bd update <id> --append-notes=<text> --json).
func (c *CLI) AppendNote(ctx context.Context, id, text string) (store.Issue, error) {
	return c.Update(ctx, id, UpdatePatch{AppendNotes: &text})
}

// DoltStart runs `bd dolt start` (idempotent server startup).
func (c *CLI) DoltStart(ctx context.Context) error {
	return c.RunVoid(ctx, "dolt", "start")
}
