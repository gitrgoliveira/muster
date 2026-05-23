package store

import (
	"context"
	"sync"
	"time"

	"github.com/gitrgoliveira/muster/internal/core"
)

// MemStore is the in-memory implementation of Store. All methods are safe for
// concurrent use by multiple goroutines.
//
// GenerateID is exported so tests in package store_test can inject a custom ID
// generator (e.g., to simulate collisions). It defaults to core.NewBeadID.
type MemStore struct {
	mu         sync.RWMutex
	beads      []core.Bead
	GenerateID func() string // defaults to core.NewBeadID; injectable for tests
}

// List returns all beads, optionally filtered by column (empty = all).
// The returned slice is a copy of the internal state.
func (ms *MemStore) List(_ context.Context, column string) ([]core.Bead, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	result := make([]core.Bead, 0, len(ms.beads))
	for _, b := range ms.beads {
		if column == "" || string(b.Column) == column {
			result = append(result, b)
		}
	}
	return result, nil
}

// Get returns the bead with the given ID, or ErrNotFound.
func (ms *MemStore) Get(_ context.Context, id string) (*core.Bead, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	for i := range ms.beads {
		if ms.beads[i].ID == id {
			cp := ms.beads[i]
			return &cp, nil
		}
	}
	return nil, ErrNotFound
}

// Create stores a new bead, generating an ID via GenerateID(). It retries up to
// 3 times on collision. Returns ErrIDExhausted if all 3 attempts collide.
func (ms *MemStore) Create(_ context.Context, b core.Bead) (*core.Bead, error) {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	const maxAttempts = 3
	for attempt := 0; attempt < maxAttempts; attempt++ {
		id := ms.GenerateID()
		if !ms.idExists(id) {
			b.ID = id
			// Ensure slice fields are non-nil.
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
			if b.Gates == nil {
				b.Gates = []core.Gate{}
			}
			if b.History == nil {
				b.History = []core.HistoryEvent{}
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
			ms.beads = append(ms.beads, b)
			cp := b
			return &cp, nil
		}
	}
	return nil, ErrIDExhausted
}

// idExists checks if an ID already exists in the store. Caller must hold lock.
func (ms *MemStore) idExists(id string) bool {
	for i := range ms.beads {
		if ms.beads[i].ID == id {
			return true
		}
	}
	return false
}

// Patch applies only the non-nil fields of patch to the identified bead.
// Derived fields (Estimate, Assignee, Comments, LastActivity) are recalculated.
// Returns the updated bead or ErrNotFound.
func (ms *MemStore) Patch(_ context.Context, id string, patch PatchBeadInput) (*core.Bead, error) {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	idx := ms.findIndex(id)
	if idx < 0 {
		return nil, ErrNotFound
	}

	b := &ms.beads[idx]

	if patch.Title != nil {
		b.Title = *patch.Title
	}
	if patch.Desc != nil {
		b.Desc = *patch.Desc
	}
	if patch.Type != nil {
		b.Type = *patch.Type
	}
	if patch.Column != nil {
		b.Column = *patch.Column
	}
	if patch.Priority != nil {
		b.Priority = *patch.Priority
	}
	if patch.Ready != nil {
		b.Ready = *patch.Ready
	}
	if patch.Labels != nil {
		b.Labels = *patch.Labels
	}
	if patch.TokensBudget != nil {
		b.TokensBudget = *patch.TokensBudget
	}

	// Recalculate derived fields.
	b.Estimate = core.DeriveEstimate(b.TokensBudget)
	b.Assignee = core.DeriveAssignee(b.Steps)
	b.Comments = core.DeriveCommentCount(b.History, b.Reviewer)
	if len(b.History) > 0 {
		b.LastActivity = b.History[len(b.History)-1].At
	} else {
		b.LastActivity = b.OpenedAt
	}

	cp := *b
	return &cp, nil
}

// Move places the bead in toColumn. If beforeID is empty, the bead is appended
// at the end. If beforeID is set, the bead is inserted immediately before it.
func (ms *MemStore) Move(_ context.Context, id, toColumn, beforeID string) (*core.Bead, error) {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	idx := ms.findIndex(id)
	if idx < 0 {
		return nil, ErrNotFound
	}

	if beforeID != "" {
		if beforeID == id {
			return nil, ErrBeforeIDSameAsMoved
		}

		targetIdx := ms.findIndex(beforeID)
		if targetIdx < 0 {
			return nil, ErrBeforeIDNotFound
		}
		if string(ms.beads[targetIdx].Column) != toColumn {
			return nil, ErrBeforeIDDifferentColumn
		}

		// Extract the bead from its current position.
		bead := ms.beads[idx]
		bead.Column = core.Column(toColumn)

		// Remove bead from slice.
		ms.beads = append(ms.beads[:idx], ms.beads[idx+1:]...)

		// Recalculate target index after removal.
		targetIdx = ms.findIndex(beforeID)

		// Insert before targetIdx.
		ms.beads = append(ms.beads, core.Bead{}) // grow by one
		copy(ms.beads[targetIdx+1:], ms.beads[targetIdx:])
		ms.beads[targetIdx] = bead

		cp := bead
		return &cp, nil
	}

	// No beforeID: just change the column (bead stays at its current position).
	ms.beads[idx].Column = core.Column(toColumn)
	cp := ms.beads[idx]
	return &cp, nil
}

// Dispatch transitions a scheduled bead to running, appending a new step and
// two history events (claimed + started). Returns ErrInvalidState if the bead
// is not in the scheduled column.
func (ms *MemStore) Dispatch(_ context.Context, id string, req DispatchRequest) (*core.Bead, error) {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	idx := ms.findIndex(id)
	if idx < 0 {
		return nil, ErrNotFound
	}

	b := &ms.beads[idx]
	if b.Column != core.ColScheduled {
		return nil, ErrInvalidState
	}

	b.Column = core.ColRunning

	newStep := core.Step{
		Agent:  req.Agent,
		Mode:   req.Mode,
		Skills: []string{},
		Status: core.StepActive,
	}
	b.Steps = append(b.Steps, newStep)

	now := time.Now().UTC().Format(time.RFC3339)

	claimedEvent := core.HistoryEvent{
		Kind:  core.EvClaimed,
		Actor: "dispatcher",
		Agent: req.Agent,
		At:    now,
	}
	startedEvent := core.HistoryEvent{
		Kind:  core.EvStarted,
		Actor: string(req.Agent),
		At:    now,
	}
	b.History = append(b.History, claimedEvent, startedEvent)
	b.LastActivity = now

	cp := *b
	return &cp, nil
}

// AddComment appends a comment history event and increments the comment count.
func (ms *MemStore) AddComment(_ context.Context, id string, req CommentRequest) (*core.Bead, error) {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	idx := ms.findIndex(id)
	if idx < 0 {
		return nil, ErrNotFound
	}

	b := &ms.beads[idx]

	now := time.Now().UTC().Format(time.RFC3339)
	event := core.HistoryEvent{
		Kind:  core.EvComment,
		Actor: req.Actor,
		Note:  req.Note,
		At:    now,
	}
	b.History = append(b.History, event)
	b.Comments++
	b.LastActivity = now

	cp := *b
	return &cp, nil
}

// findIndex returns the index of the bead with the given ID, or -1.
// Caller must hold at least a read lock.
func (ms *MemStore) findIndex(id string) int {
	for i := range ms.beads {
		if ms.beads[i].ID == id {
			return i
		}
	}
	return -1
}
