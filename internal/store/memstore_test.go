package store_test

import (
	"context"
	"testing"

	"github.com/gitrgoliveira/muster/internal/core"
	"github.com/gitrgoliveira/muster/internal/store"
)

// ptr is a helper to take the address of any value.
func ptr[T any](v T) *T { return &v }

// seedBeads are convenient test fixtures.
var (
	beadA = core.Bead{
		ID:      "bd-aaaa",
		Title:   "Alpha",
		Column:  core.ColBacklog,
		Type:    core.TypeFeature,
		Labels:  []string{},
		Skills:  []string{},
		Steps:   []core.Step{},
		History: []core.HistoryEvent{},
	}
	beadB = core.Bead{
		ID:      "bd-bbbb",
		Title:   "Beta",
		Column:  core.ColScheduled,
		Type:    core.TypeBug,
		Labels:  []string{},
		Skills:  []string{},
		Steps:   []core.Step{},
		History: []core.HistoryEvent{},
	}
	beadC = core.Bead{
		ID:      "bd-cccc",
		Title:   "Gamma",
		Column:  core.ColBacklog,
		Type:    core.TypeTask,
		Labels:  []string{},
		Skills:  []string{},
		Steps:   []core.Step{},
		History: []core.HistoryEvent{},
	}
)

// ---- List ----

func TestList_NoFilter(t *testing.T) {
	ms := store.NewMemStore([]core.Bead{beadA, beadB})

	got, err := ms.List(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 beads, got %d", len(got))
	}
}

func TestList_FilterByColumn(t *testing.T) {
	ms := store.NewMemStore([]core.Bead{beadA, beadB, beadC})

	got, err := ms.List(context.Background(), "backlog")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 backlog beads, got %d", len(got))
	}
	for _, b := range got {
		if b.Column != core.ColBacklog {
			t.Errorf("expected column backlog, got %q", b.Column)
		}
	}
}

func TestList_PreservesInsertionOrder(t *testing.T) {
	seeds := []core.Bead{beadA, beadB, beadC}
	ms := store.NewMemStore(seeds)

	got, err := ms.List(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for i, b := range got {
		if b.ID != seeds[i].ID {
			t.Errorf("position %d: expected ID %q, got %q", i, seeds[i].ID, b.ID)
		}
	}
}

// ---- Get ----

func TestGet_Found(t *testing.T) {
	ms := store.NewMemStore([]core.Bead{beadA, beadB})

	got, err := ms.Get(context.Background(), "bd-aaaa")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != "bd-aaaa" {
		t.Errorf("expected ID bd-aaaa, got %q", got.ID)
	}
	if got.Title != "Alpha" {
		t.Errorf("expected title Alpha, got %q", got.Title)
	}
}

func TestGet_Missing(t *testing.T) {
	ms := store.NewMemStore([]core.Bead{beadA})

	_, err := ms.Get(context.Background(), "bd-zzzz")
	if err != store.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// ---- Create ----

func TestCreate(t *testing.T) {
	ms := store.NewMemStore(nil)

	b := core.Bead{
		Title:  "New bead",
		Column: core.ColBacklog,
		Type:   core.TypeTask,
	}
	got, err := ms.Create(context.Background(), b)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID == "" {
		t.Error("expected non-empty ID")
	}
	if got.Title != "New bead" {
		t.Errorf("expected title %q, got %q", "New bead", got.Title)
	}

	// Verify stored in the list.
	list, _ := ms.List(context.Background(), "")
	if len(list) != 1 {
		t.Fatalf("expected 1 bead in store, got %d", len(list))
	}
}

func TestCreate_IDCollisionRetry(t *testing.T) {
	// Seed with one bead whose ID is "bd-dupe".
	seed := core.Bead{
		ID:      "bd-dupe",
		Title:   "Existing",
		Column:  core.ColBacklog,
		Type:    core.TypeTask,
		Labels:  []string{},
		Skills:  []string{},
		Steps:   []core.Step{},
		History: []core.HistoryEvent{},
	}
	ms := store.NewMemStore([]core.Bead{seed})

	// Override generateID: return duplicate twice, then unique.
	callCount := 0
	ms.GenerateID = func() string {
		callCount++
		if callCount < 3 {
			return "bd-dupe"
		}
		return "bd-uniq"
	}

	got, err := ms.Create(context.Background(), core.Bead{Title: "Retry", Column: core.ColBacklog, Type: core.TypeTask})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != "bd-uniq" {
		t.Errorf("expected ID bd-uniq, got %q", got.ID)
	}
	if callCount != 3 {
		t.Errorf("expected 3 calls to generateID, got %d", callCount)
	}
}

func TestCreate_IDExhausted(t *testing.T) {
	seed := core.Bead{
		ID:      "bd-dupe",
		Title:   "Existing",
		Column:  core.ColBacklog,
		Type:    core.TypeTask,
		Labels:  []string{},
		Skills:  []string{},
		Steps:   []core.Step{},
		History: []core.HistoryEvent{},
	}
	ms := store.NewMemStore([]core.Bead{seed})

	ms.GenerateID = func() string { return "bd-dupe" }

	_, err := ms.Create(context.Background(), core.Bead{Title: "Exhausted", Column: core.ColBacklog, Type: core.TypeTask})
	if err != store.ErrIDExhausted {
		t.Fatalf("expected ErrIDExhausted, got %v", err)
	}
}

// ---- Patch ----

func TestPatch_PartialFields(t *testing.T) {
	seed := core.Bead{
		ID:      "bd-p001",
		Title:   "Original",
		Desc:    "keep me",
		Column:  core.ColBacklog,
		Type:    core.TypeFeature,
		Labels:  []string{},
		Skills:  []string{},
		Steps:   []core.Step{},
		History: []core.HistoryEvent{},
	}
	ms := store.NewMemStore([]core.Bead{seed})

	got, err := ms.Patch(context.Background(), "bd-p001", store.PatchBeadInput{
		Title: ptr("Updated"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Title != "Updated" {
		t.Errorf("expected title Updated, got %q", got.Title)
	}
	if got.Desc != "keep me" {
		t.Errorf("expected desc to be untouched, got %q", got.Desc)
	}
	if got.Column != core.ColBacklog {
		t.Errorf("expected column to be untouched, got %q", got.Column)
	}
}

func TestPatch_NotFound(t *testing.T) {
	ms := store.NewMemStore(nil)

	_, err := ms.Patch(context.Background(), "bd-zzzz", store.PatchBeadInput{})
	if err != store.ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// ---- Move ----

func TestMove_AppendsToColumn(t *testing.T) {
	seeds := []core.Bead{
		{ID: "bd-m001", Title: "M1", Column: core.ColBacklog, Labels: []string{}, Skills: []string{}, Steps: []core.Step{}, History: []core.HistoryEvent{}},
		{ID: "bd-m002", Title: "M2", Column: core.ColScheduled, Labels: []string{}, Skills: []string{}, Steps: []core.Step{}, History: []core.HistoryEvent{}},
	}
	ms := store.NewMemStore(seeds)

	got, err := ms.Move(context.Background(), "bd-m001", "scheduled", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Column != core.ColScheduled {
		t.Errorf("expected column scheduled, got %q", got.Column)
	}
}

func TestMove_BeforeID_Reorders(t *testing.T) {
	seeds := []core.Bead{
		{ID: "bd-r001", Title: "R1", Column: core.ColBacklog, Labels: []string{}, Skills: []string{}, Steps: []core.Step{}, History: []core.HistoryEvent{}},
		{ID: "bd-r002", Title: "R2", Column: core.ColBacklog, Labels: []string{}, Skills: []string{}, Steps: []core.Step{}, History: []core.HistoryEvent{}},
		{ID: "bd-r003", Title: "R3", Column: core.ColBacklog, Labels: []string{}, Skills: []string{}, Steps: []core.Step{}, History: []core.HistoryEvent{}},
	}
	ms := store.NewMemStore(seeds)

	// Move R3 before R1 in backlog.
	got, err := ms.Move(context.Background(), "bd-r003", "backlog", "bd-r001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Column != core.ColBacklog {
		t.Errorf("expected column backlog, got %q", got.Column)
	}

	list, _ := ms.List(context.Background(), "backlog")
	order := make([]string, len(list))
	for i, b := range list {
		order[i] = b.ID
	}
	expected := []string{"bd-r003", "bd-r001", "bd-r002"}
	for i, id := range expected {
		if i >= len(order) || order[i] != id {
			t.Errorf("expected order %v, got %v", expected, order)
			break
		}
	}
}

func TestMove_BeforeID_UnknownReturnsError(t *testing.T) {
	seeds := []core.Bead{
		{ID: "bd-u001", Title: "U1", Column: core.ColBacklog, Labels: []string{}, Skills: []string{}, Steps: []core.Step{}, History: []core.HistoryEvent{}},
	}
	ms := store.NewMemStore(seeds)

	_, err := ms.Move(context.Background(), "bd-u001", "backlog", "bd-zzzz")
	if err != store.ErrBeforeIDNotFound {
		t.Fatalf("expected ErrBeforeIDNotFound, got %v", err)
	}
}

func TestMove_BeforeID_DifferentColumnReturnsError(t *testing.T) {
	seeds := []core.Bead{
		{ID: "bd-d001", Title: "D1", Column: core.ColBacklog, Labels: []string{}, Skills: []string{}, Steps: []core.Step{}, History: []core.HistoryEvent{}},
		{ID: "bd-d002", Title: "D2", Column: core.ColScheduled, Labels: []string{}, Skills: []string{}, Steps: []core.Step{}, History: []core.HistoryEvent{}},
	}
	ms := store.NewMemStore(seeds)

	// Try to move D1 before D2 into backlog, but D2 is in scheduled.
	_, err := ms.Move(context.Background(), "bd-d001", "backlog", "bd-d002")
	if err != store.ErrBeforeIDDifferentColumn {
		t.Fatalf("expected ErrBeforeIDDifferentColumn, got %v", err)
	}
}

func TestMove_BeforeID_SameAsMovedReturnsError(t *testing.T) {
	seeds := []core.Bead{
		{ID: "bd-s001", Title: "S1", Column: core.ColBacklog, Labels: []string{}, Skills: []string{}, Steps: []core.Step{}, History: []core.HistoryEvent{}},
	}
	ms := store.NewMemStore(seeds)

	_, err := ms.Move(context.Background(), "bd-s001", "backlog", "bd-s001")
	if err != store.ErrBeforeIDSameAsMoved {
		t.Fatalf("expected ErrBeforeIDSameAsMoved, got %v", err)
	}
}

// ---- Dispatch ----

func TestDispatch_AppendsStep_ChangesColumn_AppendsTwoHistoryEvents(t *testing.T) {
	seed := core.Bead{
		ID:      "bd-disp",
		Title:   "Dispatch me",
		Column:  core.ColScheduled,
		Type:    core.TypeFeature,
		Labels:  []string{},
		Skills:  []string{},
		Steps:   []core.Step{},
		History: []core.HistoryEvent{},
	}
	ms := store.NewMemStore([]core.Bead{seed})

	req := store.DispatchRequest{
		Agent: core.AgentClaude,
		Mode:  core.ModeBuild,
	}
	got, err := ms.Dispatch(context.Background(), "bd-disp", req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.Column != core.ColRunning {
		t.Errorf("expected column running, got %q", got.Column)
	}
	if len(got.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(got.Steps))
	}
	step := got.Steps[0]
	if step.Agent != core.AgentClaude {
		t.Errorf("expected agent claude, got %q", step.Agent)
	}
	if step.Mode != core.ModeBuild {
		t.Errorf("expected mode build, got %q", step.Mode)
	}
	if step.Skills == nil {
		t.Error("expected non-nil Skills slice")
	}
	if len(step.Skills) != 0 {
		t.Errorf("expected empty Skills, got %v", step.Skills)
	}
	if step.Status != core.StepActive {
		t.Errorf("expected status active, got %q", step.Status)
	}

	if len(got.History) != 2 {
		t.Fatalf("expected 2 history events, got %d", len(got.History))
	}
	claimed := got.History[0]
	started := got.History[1]

	if claimed.Kind != core.EvClaimed {
		t.Errorf("expected first event kind claimed, got %q", claimed.Kind)
	}
	if claimed.Actor != "dispatcher" {
		t.Errorf("expected actor dispatcher, got %q", claimed.Actor)
	}
	if claimed.Agent != core.AgentClaude {
		t.Errorf("expected agent claude in claimed event, got %q", claimed.Agent)
	}
	if claimed.At == "" {
		t.Error("expected non-empty At on claimed event")
	}

	if started.Kind != core.EvStarted {
		t.Errorf("expected second event kind started, got %q", started.Kind)
	}
	if started.Actor != string(core.AgentClaude) {
		t.Errorf("expected actor claude in started event, got %q", started.Actor)
	}
	if started.At == "" {
		t.Error("expected non-empty At on started event")
	}
}

func TestDispatch_FromNonScheduledReturnsErrInvalidState(t *testing.T) {
	seed := core.Bead{
		ID:      "bd-bad",
		Title:   "Not scheduled",
		Column:  core.ColBacklog,
		Type:    core.TypeTask,
		Labels:  []string{},
		Skills:  []string{},
		Steps:   []core.Step{},
		History: []core.HistoryEvent{},
	}
	ms := store.NewMemStore([]core.Bead{seed})

	_, err := ms.Dispatch(context.Background(), "bd-bad", store.DispatchRequest{
		Agent: core.AgentClaude,
		Mode:  core.ModeBuild,
	})
	if err != store.ErrInvalidState {
		t.Fatalf("expected ErrInvalidState, got %v", err)
	}
}

// ---- AddComment ----

func TestAddComment_AppendsHistoryAndIncrementsCount(t *testing.T) {
	seed := core.Bead{
		ID:       "bd-cmt",
		Title:    "Comment target",
		Column:   core.ColBacklog,
		Type:     core.TypeFeature,
		Labels:   []string{},
		Skills:   []string{},
		Steps:    []core.Step{},
		History:  []core.HistoryEvent{},
		Comments: 0,
	}
	ms := store.NewMemStore([]core.Bead{seed})

	req := store.CommentRequest{
		Actor: "alice",
		Note:  "great work!",
	}
	got, err := ms.AddComment(context.Background(), "bd-cmt", req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Comments != 1 {
		t.Errorf("expected Comments=1, got %d", got.Comments)
	}
	if len(got.History) != 1 {
		t.Fatalf("expected 1 history event, got %d", len(got.History))
	}
	ev := got.History[0]
	if ev.Kind != core.EvComment {
		t.Errorf("expected kind comment, got %q", ev.Kind)
	}
	if ev.Actor != "alice" {
		t.Errorf("expected actor alice, got %q", ev.Actor)
	}
	if ev.Note != "great work!" {
		t.Errorf("expected note 'great work!', got %q", ev.Note)
	}
	if ev.At == "" {
		t.Error("expected non-empty At timestamp")
	}
}
