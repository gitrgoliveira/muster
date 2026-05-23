package services_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gitrgoliveira/muster/internal/core"
	"github.com/gitrgoliveira/muster/internal/services"
	"github.com/gitrgoliveira/muster/internal/ws"
)

// capturePublish returns a Publisher that records the last frame published.
func capturePublish(captured *ws.Frame) services.Publisher {
	return func(f ws.Frame) { *captured = f }
}

func TestService_PublishesCreatedEvent(t *testing.T) {
	var got ws.Frame
	svc := services.NewBeadService(newMockStore(), capturePublish(&got))
	b, err := svc.Create(context.Background(), services.CreateBeadInput{Title: "test", Priority: 2})
	require.NoError(t, err)
	assert.Equal(t, ws.EventBeadCreated, got.Type)
	require.NotNil(t, got.Bead)
	assert.Equal(t, b.ID, got.Bead.ID)
}

func TestService_PublishesUpdatedEvent_OnPatch(t *testing.T) {
	var got ws.Frame
	ms := newMockStore(&core.Bead{ID: "bd-0010", Column: core.ColBacklog, Labels: []string{}})
	svc := services.NewBeadService(ms, capturePublish(&got))
	_, err := svc.Patch(context.Background(), "bd-0010", services.PatchBeadInput{Title: ptr("patched")})
	require.NoError(t, err)
	assert.Equal(t, ws.EventBeadUpdated, got.Type)
	require.NotNil(t, got.Bead)
	assert.Equal(t, "bd-0010", got.Bead.ID)
}

func TestService_PublishesUpdatedEvent_OnDispatch(t *testing.T) {
	var got ws.Frame
	ms := newMockStore(&core.Bead{
		ID:      "bd-0011",
		Column:  core.ColScheduled,
		Steps:   []core.Step{},
		History: []core.HistoryEvent{},
	})
	svc := services.NewBeadService(ms, capturePublish(&got))
	_, err := svc.Dispatch(context.Background(), "bd-0011", services.DispatchInput{
		Agent: core.AgentClaude,
		Mode:  core.ModeBuild,
	})
	require.NoError(t, err)
	assert.Equal(t, ws.EventBeadUpdated, got.Type)
	require.NotNil(t, got.Bead)
}

func TestService_PublishesMovedEvent(t *testing.T) {
	var got ws.Frame
	ms := newMockStore(&core.Bead{ID: "bd-0012", Column: core.ColBacklog})
	svc := services.NewBeadService(ms, capturePublish(&got))
	_, err := svc.Move(context.Background(), "bd-0012", services.MoveInput{
		ToColumn: core.ColScheduled,
		BeforeID: "",
	})
	require.NoError(t, err)
	assert.Equal(t, ws.EventBeadMoved, got.Type)
	assert.Equal(t, core.ColBacklog, got.FromColumn)
	assert.Equal(t, core.ColScheduled, got.ToColumn)
	assert.Equal(t, "", got.BeforeID)
}

func TestService_PublishesCommentAddedEvent(t *testing.T) {
	var got ws.Frame
	ms := newMockStore(&core.Bead{
		ID:      "bd-0013",
		Column:  core.ColRunning,
		History: []core.HistoryEvent{},
	})
	svc := services.NewBeadService(ms, capturePublish(&got))
	_, err := svc.AddComment(context.Background(), "bd-0013", services.CommentInput{
		Actor: "gemini",
		Note:  "LGTM",
	})
	require.NoError(t, err)
	assert.Equal(t, ws.EventCommentAdded, got.Type)
	assert.Equal(t, "bd-0013", got.ID)
	require.NotNil(t, got.Bead)
	require.NotNil(t, got.Event)
	assert.Equal(t, core.EvComment, got.Event.Kind)
	assert.Equal(t, "gemini", got.Event.Actor)
	assert.Equal(t, "LGTM", got.Event.Note)
}
