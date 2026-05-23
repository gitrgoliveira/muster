package store

import (
	"context"
	"errors"

	"github.com/gitrgoliveira/muster/internal/core"
)

var (
	ErrNotFound                = errors.New("bead not found")
	ErrInvalidState            = errors.New("invalid state")
	ErrIDExhausted             = errors.New("bead ID exhausted after 3 retries")
	ErrBeforeIDNotFound        = errors.New("beforeID not found")
	ErrBeforeIDDifferentColumn = errors.New("beforeID must be in toColumn")
	ErrBeforeIDSameAsMoved     = errors.New("bead cannot insert before itself")
)

// PatchBeadInput carries optional updates for a bead. Nil means no change;
// non-nil (including zero value) means "set to this value".
type PatchBeadInput struct {
	Title        *string
	Desc         *string
	Type         *core.BeadType
	Column       *core.Column
	Priority     *core.Priority
	Ready        *bool
	Labels       *[]string
	TokensBudget *int
}

// DispatchRequest carries the agent and mode for a dispatch operation.
type DispatchRequest struct {
	Agent core.AgentID
	Mode  core.Mode
}

// CommentRequest carries the actor and note for a comment operation.
type CommentRequest struct {
	Actor string
	Note  string
}

// Store is the persistence interface for beads. All implementations must be
// safe for concurrent use by multiple goroutines.
type Store interface {
	// List returns all beads, optionally filtered by column (empty = all).
	List(ctx context.Context, column string) ([]core.Bead, error)

	// Get returns the bead with the given ID, or ErrNotFound.
	Get(ctx context.Context, id string) (*core.Bead, error)

	// Create stores the bead and returns the stored copy.
	// It generates an ID via core.NewBeadID, retrying up to 3 times on
	// collision. Returns ErrIDExhausted if all 3 attempts collide.
	Create(ctx context.Context, b core.Bead) (*core.Bead, error)

	// Patch applies only the non-nil fields of patch to the bead identified
	// by id. Returns the updated bead, or ErrNotFound.
	Patch(ctx context.Context, id string, patch PatchBeadInput) (*core.Bead, error)

	// Move places the bead in toColumn. If beforeID is empty, the bead is
	// appended at the end. If beforeID is set, the bead is inserted immediately
	// before it. Returns ErrBeforeIDNotFound, ErrBeforeIDDifferentColumn, or
	// ErrBeforeIDSameAsMoved on invalid beforeID values.
	Move(ctx context.Context, id, toColumn, beforeID string) (*core.Bead, error)

	// Dispatch transitions a scheduled bead to running, appending a new step
	// and two history events (claimed + started). Returns ErrInvalidState if
	// the bead is not in the scheduled column.
	Dispatch(ctx context.Context, id string, req DispatchRequest) (*core.Bead, error)

	// AddComment appends a comment history event and increments the comment count.
	AddComment(ctx context.Context, id string, req CommentRequest) (*core.Bead, error)
}

// NewMemStore is the constructor for the in-memory store implementation.
// Defined here so that callers can reference it without importing memstore.go
// — the implementation lives in memstore.go within the same package.
// Seeds are stored in insertion order and define the initial bead set.
func NewMemStore(seeds []core.Bead) *MemStore {
	ms := &MemStore{
		beads:      make([]core.Bead, len(seeds)),
		GenerateID: core.NewBeadID,
	}
	copy(ms.beads, seeds)
	return ms
}
