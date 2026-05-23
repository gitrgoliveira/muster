package services_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gitrgoliveira/muster/internal/core"
	"github.com/gitrgoliveira/muster/internal/services"
	"github.com/gitrgoliveira/muster/internal/store"
	"github.com/gitrgoliveira/muster/internal/ws"
)

// mockStore is a minimal store.Store implementation for service tests.
type mockStore struct {
	beads map[string]*core.Bead
	// Injectable error for testing error paths.
	createErr   error
	patchErr    error
	moveErr     error
	dispatchErr error
	commentErr  error
}

func newMockStore(initial ...*core.Bead) *mockStore {
	m := &mockStore{beads: make(map[string]*core.Bead)}
	for _, b := range initial {
		cp := *b
		m.beads[b.ID] = &cp
	}
	return m
}

func (m *mockStore) List(_ context.Context, column string) ([]core.Bead, error) {
	var out []core.Bead
	for _, b := range m.beads {
		if column == "" || string(b.Column) == column {
			out = append(out, *b)
		}
	}
	return out, nil
}

func (m *mockStore) Get(_ context.Context, id string) (*core.Bead, error) {
	b, ok := m.beads[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	cp := *b
	return &cp, nil
}

func (m *mockStore) Create(_ context.Context, b core.Bead) (*core.Bead, error) {
	if m.createErr != nil {
		return nil, m.createErr
	}
	b.ID = "bd-t001"
	m.beads[b.ID] = &b
	cp := b
	return &cp, nil
}

func (m *mockStore) Patch(_ context.Context, id string, patch store.PatchBeadInput) (*core.Bead, error) {
	if m.patchErr != nil {
		return nil, m.patchErr
	}
	b, ok := m.beads[id]
	if !ok {
		return nil, store.ErrNotFound
	}
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
	cp := *b
	return &cp, nil
}

func (m *mockStore) Move(_ context.Context, id, toColumn, beforeID string) (*core.Bead, error) {
	if m.moveErr != nil {
		return nil, m.moveErr
	}
	b, ok := m.beads[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	b.Column = core.Column(toColumn)
	cp := *b
	return &cp, nil
}

func (m *mockStore) Dispatch(_ context.Context, id string, req store.DispatchRequest) (*core.Bead, error) {
	if m.dispatchErr != nil {
		return nil, m.dispatchErr
	}
	b, ok := m.beads[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	if b.Column != core.ColScheduled {
		return nil, store.ErrInvalidState
	}
	b.Column = core.ColRunning
	b.Steps = append(b.Steps, core.Step{
		Agent:  req.Agent,
		Mode:   req.Mode,
		Skills: []string{},
		Status: core.StepActive,
	})
	b.History = append(b.History,
		core.HistoryEvent{Kind: core.EvClaimed, Actor: "dispatcher", Agent: req.Agent},
		core.HistoryEvent{Kind: core.EvStarted, Actor: string(req.Agent)},
	)
	cp := *b
	return &cp, nil
}

func (m *mockStore) AddComment(_ context.Context, id string, req store.CommentRequest) (*core.Bead, error) {
	if m.commentErr != nil {
		return nil, m.commentErr
	}
	b, ok := m.beads[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	b.History = append(b.History, core.HistoryEvent{
		Kind:  core.EvComment,
		Actor: req.Actor,
		Note:  req.Note,
	})
	b.Comments++
	cp := *b
	return &cp, nil
}

// noopPublish discards all events.
func noopPublish(_ ws.Frame) {}

// svcError extracts a *services.ServiceError from err, failing the test if absent.
func svcError(t *testing.T, err error) *services.ServiceError {
	t.Helper()
	var se *services.ServiceError
	require.True(t, errors.As(err, &se), "expected *services.ServiceError, got %T: %v", err, err)
	return se
}

func ptr[T any](v T) *T { return &v }

// ── Create ────────────────────────────────────────────────────────────────────

func TestCreate_MissingTitle(t *testing.T) {
	svc := services.NewBeadService(newMockStore(), noopPublish)
	_, err := svc.Create(context.Background(), services.CreateBeadInput{Title: ""})
	se := svcError(t, err)
	assert.Equal(t, services.CodeInvalidRequest, se.Code)
}

func TestCreate_TitleTooLong(t *testing.T) {
	svc := services.NewBeadService(newMockStore(), noopPublish)
	_, err := svc.Create(context.Background(), services.CreateBeadInput{
		Title:    strings.Repeat("ü", 256), // 256 runes, each multibyte
		Priority: 2,
	})
	se := svcError(t, err)
	assert.Equal(t, services.CodeInvalidRequest, se.Code)
}

func TestCreate_InvalidType(t *testing.T) {
	svc := services.NewBeadService(newMockStore(), noopPublish)
	_, err := svc.Create(context.Background(), services.CreateBeadInput{
		Title:    "ok",
		Type:     core.BeadType("unknown"),
		Priority: 2,
	})
	se := svcError(t, err)
	assert.Equal(t, services.CodeInvalidRequest, se.Code)
}

func TestCreate_InvalidPriority(t *testing.T) {
	svc := services.NewBeadService(newMockStore(), noopPublish)
	_, err := svc.Create(context.Background(), services.CreateBeadInput{
		Title:    "ok",
		Priority: core.Priority(99),
	})
	se := svcError(t, err)
	assert.Equal(t, services.CodeInvalidRequest, se.Code)
}

func TestCreate_WhitespaceTitleTrimmed(t *testing.T) {
	svc := services.NewBeadService(newMockStore(), noopPublish)
	b, err := svc.Create(context.Background(), services.CreateBeadInput{
		Title:    "  hello world  ",
		Priority: 2,
	})
	require.NoError(t, err)
	assert.Equal(t, "hello world", b.Title)
}

// ── Patch ─────────────────────────────────────────────────────────────────────

func TestPatch_EmptyBodyReturnsInvalidRequest(t *testing.T) {
	ms := newMockStore(&core.Bead{ID: "bd-0001", Column: core.ColBacklog})
	svc := services.NewBeadService(ms, noopPublish)
	_, err := svc.Patch(context.Background(), "bd-0001", services.PatchBeadInput{})
	se := svcError(t, err)
	assert.Equal(t, services.CodeInvalidRequest, se.Code)
}

func TestPatch_AllPointerFields(t *testing.T) {
	type tc struct {
		name  string
		patch services.PatchBeadInput
		check func(t *testing.T, b *core.Bead)
	}
	tests := []tc{
		{
			name:  "title",
			patch: services.PatchBeadInput{Title: ptr("new title")},
			check: func(t *testing.T, b *core.Bead) { assert.Equal(t, "new title", b.Title) },
		},
		{
			name:  "desc",
			patch: services.PatchBeadInput{Desc: ptr("some desc")},
			check: func(t *testing.T, b *core.Bead) { assert.Equal(t, "some desc", b.Desc) },
		},
		{
			name:  "type",
			patch: services.PatchBeadInput{Type: ptr(core.TypeBug)},
			check: func(t *testing.T, b *core.Bead) { assert.Equal(t, core.TypeBug, b.Type) },
		},
		{
			name:  "priority",
			patch: services.PatchBeadInput{Priority: ptr(core.Priority(1))},
			check: func(t *testing.T, b *core.Bead) { assert.Equal(t, core.Priority(1), b.Priority) },
		},
		{
			name:  "labels",
			patch: services.PatchBeadInput{Labels: ptr([]string{"a", "b"})},
			check: func(t *testing.T, b *core.Bead) { assert.Equal(t, []string{"a", "b"}, b.Labels) },
		},
		{
			name:  "tokensBudget",
			patch: services.PatchBeadInput{TokensBudget: ptr(100_000)},
			check: func(t *testing.T, b *core.Bead) { assert.Equal(t, 100_000, b.TokensBudget) },
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ms := newMockStore(&core.Bead{
				ID:     "bd-0001",
				Title:  "original",
				Column: core.ColBacklog,
				Labels: []string{},
			})
			svc := services.NewBeadService(ms, noopPublish)
			b, err := svc.Patch(context.Background(), "bd-0001", tc.patch)
			require.NoError(t, err)
			tc.check(t, b)
		})
	}
}

// ── Move ──────────────────────────────────────────────────────────────────────

func TestMove_ColumnChange(t *testing.T) {
	ms := newMockStore(&core.Bead{
		ID:     "bd-0002",
		Column: core.ColBacklog,
	})
	svc := services.NewBeadService(ms, noopPublish)
	b, err := svc.Move(context.Background(), "bd-0002", services.MoveInput{ToColumn: core.ColScheduled})
	require.NoError(t, err)
	assert.Equal(t, core.ColScheduled, b.Column)
}

func TestMove_BeforeID_Reorder(t *testing.T) {
	ms := newMockStore(
		&core.Bead{ID: "bd-0003", Column: core.ColBacklog},
		&core.Bead{ID: "bd-0004", Column: core.ColBacklog},
	)
	svc := services.NewBeadService(ms, noopPublish)
	// Move 0003 before 0004 (same column, reorder).
	b, err := svc.Move(context.Background(), "bd-0003", services.MoveInput{
		ToColumn: core.ColBacklog,
		BeforeID: "bd-0004",
	})
	require.NoError(t, err)
	assert.Equal(t, core.ColBacklog, b.Column)
}

// ── Dispatch ──────────────────────────────────────────────────────────────────

func TestDispatch_Scheduled_Succeeds(t *testing.T) {
	ms := newMockStore(&core.Bead{
		ID:      "bd-0005",
		Column:  core.ColScheduled,
		Steps:   []core.Step{},
		History: []core.HistoryEvent{},
	})
	svc := services.NewBeadService(ms, noopPublish)
	b, err := svc.Dispatch(context.Background(), "bd-0005", services.DispatchInput{
		Agent: core.AgentClaude,
		Mode:  core.ModeBuild,
	})
	require.NoError(t, err)
	assert.Equal(t, core.ColRunning, b.Column)
}

func TestDispatch_NonScheduled_ReturnsInvalidState(t *testing.T) {
	ms := newMockStore(&core.Bead{
		ID:     "bd-0006",
		Column: core.ColRunning,
	})
	svc := services.NewBeadService(ms, noopPublish)
	_, err := svc.Dispatch(context.Background(), "bd-0006", services.DispatchInput{
		Agent: core.AgentClaude,
		Mode:  core.ModeBuild,
	})
	se := svcError(t, err)
	assert.Equal(t, services.CodeInvalidState, se.Code)
}

func TestDispatch_AppendsStepAndTwoHistoryEvents(t *testing.T) {
	ms := newMockStore(&core.Bead{
		ID:      "bd-0007",
		Column:  core.ColScheduled,
		Steps:   []core.Step{},
		History: []core.HistoryEvent{},
	})
	svc := services.NewBeadService(ms, noopPublish)
	b, err := svc.Dispatch(context.Background(), "bd-0007", services.DispatchInput{
		Agent: core.AgentGemini,
		Mode:  core.ModePlan,
	})
	require.NoError(t, err)
	require.Len(t, b.Steps, 1)
	assert.Equal(t, core.AgentGemini, b.Steps[0].Agent)
	assert.Equal(t, core.ModePlan, b.Steps[0].Mode)
	assert.Equal(t, core.StepActive, b.Steps[0].Status)

	require.Len(t, b.History, 2)
	assert.Equal(t, core.EvClaimed, b.History[0].Kind)
	assert.Equal(t, core.EvStarted, b.History[1].Kind)
}

// ── AddComment ────────────────────────────────────────────────────────────────

func TestAddComment_AppendsAndCounts(t *testing.T) {
	ms := newMockStore(&core.Bead{
		ID:       "bd-0008",
		Column:   core.ColRunning,
		History:  []core.HistoryEvent{},
		Comments: 0,
	})
	svc := services.NewBeadService(ms, noopPublish)
	b, err := svc.AddComment(context.Background(), "bd-0008", services.CommentInput{
		Actor: "claude",
		Note:  "looks good",
	})
	require.NoError(t, err)
	assert.Equal(t, 1, b.Comments)
	require.Len(t, b.History, 1)
	assert.Equal(t, core.EvComment, b.History[0].Kind)
	assert.Equal(t, "claude", b.History[0].Actor)
	assert.Equal(t, "looks good", b.History[0].Note)
}

// ── Error paths ───────────────────────────────────────────────────────────────

func TestServiceError_Error(t *testing.T) {
	se := &services.ServiceError{Code: "INVALID_REQUEST", Message: "title is required"}
	assert.Contains(t, se.Error(), "INVALID_REQUEST")
	assert.Contains(t, se.Error(), "title is required")
}

func TestPatch_NotFound(t *testing.T) {
	svc := services.NewBeadService(newMockStore(), noopPublish)
	_, err := svc.Patch(context.Background(), "bd-missing", services.PatchBeadInput{Title: ptr("x")})
	se := svcError(t, err)
	assert.Equal(t, services.CodeNotFound, se.Code)
}

func TestMove_InvalidColumn(t *testing.T) {
	ms := newMockStore(&core.Bead{ID: "bd-0020", Column: core.ColBacklog})
	svc := services.NewBeadService(ms, noopPublish)
	_, err := svc.Move(context.Background(), "bd-0020", services.MoveInput{ToColumn: "invalid"})
	se := svcError(t, err)
	assert.Equal(t, services.CodeInvalidRequest, se.Code)
}

func TestMove_NotFound(t *testing.T) {
	svc := services.NewBeadService(newMockStore(), noopPublish)
	_, err := svc.Move(context.Background(), "bd-missing", services.MoveInput{ToColumn: core.ColBacklog})
	se := svcError(t, err)
	assert.Equal(t, services.CodeNotFound, se.Code)
}

func TestMove_BeforeIDErrors(t *testing.T) {
	tests := []struct {
		name     string
		storeErr error
		wantCode string
	}{
		{"beforeID not found", store.ErrBeforeIDNotFound, services.CodeInvalidRequest},
		{"beforeID different column", store.ErrBeforeIDDifferentColumn, services.CodeInvalidRequest},
		{"beforeID same as moved", store.ErrBeforeIDSameAsMoved, services.CodeInvalidRequest},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ms := newMockStore(&core.Bead{ID: "bd-0021", Column: core.ColBacklog})
			ms.moveErr = tc.storeErr
			svc := services.NewBeadService(ms, noopPublish)
			_, err := svc.Move(context.Background(), "bd-0021", services.MoveInput{
				ToColumn: core.ColBacklog,
				BeforeID: "bd-other",
			})
			se := svcError(t, err)
			assert.Equal(t, tc.wantCode, se.Code)
		})
	}
}

func TestDispatch_InvalidAgent(t *testing.T) {
	ms := newMockStore(&core.Bead{ID: "bd-0022", Column: core.ColScheduled})
	svc := services.NewBeadService(ms, noopPublish)
	_, err := svc.Dispatch(context.Background(), "bd-0022", services.DispatchInput{
		Agent: core.AgentID("unknown"),
		Mode:  core.ModeBuild,
	})
	se := svcError(t, err)
	assert.Equal(t, services.CodeInvalidRequest, se.Code)
}

func TestAddComment_MissingActor(t *testing.T) {
	ms := newMockStore(&core.Bead{ID: "bd-0023", Column: core.ColRunning, History: []core.HistoryEvent{}})
	svc := services.NewBeadService(ms, noopPublish)
	_, err := svc.AddComment(context.Background(), "bd-0023", services.CommentInput{Actor: "", Note: "hi"})
	se := svcError(t, err)
	assert.Equal(t, services.CodeInvalidRequest, se.Code)
}

func TestAddComment_NotFound(t *testing.T) {
	svc := services.NewBeadService(newMockStore(), noopPublish)
	_, err := svc.AddComment(context.Background(), "bd-missing", services.CommentInput{Actor: "x", Note: "y"})
	se := svcError(t, err)
	assert.Equal(t, services.CodeNotFound, se.Code)
}
