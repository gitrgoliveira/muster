package services

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"

	"github.com/gitrgoliveira/muster/internal/core"
	"github.com/gitrgoliveira/muster/internal/store/bdshell"
)

// CodeMemoryNotFound is the 404 code for a missing memory key.
const CodeMemoryNotFound = "NOT_FOUND"

// CodeBDUnavailable is the code for an underlying bd failure on a /memories
// route (mapped to 502 by the render layer). Never surfaced as an empty-list
// success (FR-025).
const CodeBDUnavailable = "BD_UNAVAILABLE"

// Memory is the wire shape of a memory (a thin view over bd's key/value store).
type Memory struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// MemoryStore is the bd-backed capability the memories service needs (satisfied
// by *bdshell.CLI). Kept narrow so it can be faked in tests.
type MemoryStore interface {
	Remember(ctx context.Context, key, value string) (string, error)
	Recall(ctx context.Context, key string) (string, error)
	Forget(ctx context.Context, key string) error
	Memories(ctx context.Context, query string) (map[string]string, error)
}

// MemoriesService is a thin facade over bd's memory primitives, plus per-bead
// prime snapshots persisted under <musterDir>/primed/ (survive restart). No
// muster-owned memory store — bd is the single writer (Constitution II).
type MemoriesService struct {
	store     MemoryStore
	primedDir string
}

// NewMemoriesService constructs the service. store may be nil (bd unavailable),
// in which case every call returns a typed BD_UNAVAILABLE error rather than a
// misleading empty result.
func NewMemoriesService(store MemoryStore, musterDir string) *MemoriesService {
	return &MemoriesService{store: store, primedDir: filepath.Join(musterDir, "primed")}
}

func bdUnavailable(err error) *ServiceError {
	return &ServiceError{Code: CodeBDUnavailable, Message: "bd memory backend unavailable: " + err.Error()}
}

// List returns memories, optionally filtered by query.
func (s *MemoriesService) List(ctx context.Context, query string) ([]Memory, error) {
	if s.store == nil {
		return nil, &ServiceError{Code: CodeBDUnavailable, Message: "bd is not available"}
	}
	m, err := s.store.Memories(ctx, query)
	if err != nil {
		return nil, bdUnavailable(err)
	}
	return sortedMemories(m), nil
}

// Upsert creates or updates a memory. An empty key is auto-derived by bd.
func (s *MemoriesService) Upsert(ctx context.Context, key, value string) (Memory, error) {
	if s.store == nil {
		return Memory{}, &ServiceError{Code: CodeBDUnavailable, Message: "bd is not available"}
	}
	if value == "" {
		return Memory{}, &ServiceError{Code: CodeInvalidRequest, Message: "value is required"}
	}
	k, err := s.store.Remember(ctx, key, value)
	if err != nil {
		return Memory{}, bdUnavailable(err)
	}
	return Memory{Key: k, Value: value}, nil
}

// Delete removes a memory. A missing key is a typed not-found (never a false
// success), distinct from a bd backend failure.
func (s *MemoriesService) Delete(ctx context.Context, key string) error {
	if s.store == nil {
		return &ServiceError{Code: CodeBDUnavailable, Message: "bd is not available"}
	}
	err := s.store.Forget(ctx, key)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, bdshell.ErrMemoryNotFound):
		return &ServiceError{Code: CodeMemoryNotFound, Message: "memory not found"}
	default:
		return bdUnavailable(err)
	}
}

// Prime snapshots the current memory set for a bead so its NEXT dispatch's
// assembled prompt includes them. The snapshot is persisted (survives restart)
// under <musterDir>/primed/<beadID>.json. Returns the number of memories primed.
func (s *MemoriesService) Prime(ctx context.Context, beadID string) (int, error) {
	if s.store == nil {
		return 0, &ServiceError{Code: CodeBDUnavailable, Message: "bd is not available"}
	}
	if !core.ValidBeadID(beadID) {
		return 0, &ServiceError{Code: CodeInvalidRequest, Message: "invalid beadID"}
	}
	m, err := s.store.Memories(ctx, "")
	if err != nil {
		return 0, bdUnavailable(err)
	}
	data, err := json.Marshal(m)
	if err != nil {
		return 0, &ServiceError{Code: CodeInternal, Message: err.Error()}
	}
	if err := os.MkdirAll(s.primedDir, 0o700); err != nil {
		return 0, &ServiceError{Code: CodeInternal, Message: err.Error()}
	}
	if err := atomicWriteFile(filepath.Join(s.primedDir, beadID+".json"), data, 0o600); err != nil {
		return 0, &ServiceError{Code: CodeInternal, Message: err.Error()}
	}
	return len(m), nil
}

// PrimedMemories implements the orchestrator's provider: it returns the primed
// snapshot for a bead (empty if none / unreadable). beadID is validated so it
// cannot escape primedDir.
func (s *MemoriesService) PrimedMemories(beadID string) map[string]string {
	if !core.ValidBeadID(beadID) {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(s.primedDir, beadID+".json"))
	if err != nil {
		return nil
	}
	var m map[string]string
	if err := json.Unmarshal(data, &m); err != nil {
		return nil
	}
	return m
}

func sortedMemories(m map[string]string) []Memory {
	out := make([]Memory, 0, len(m))
	for k, v := range m {
		out = append(out, Memory{Key: k, Value: v})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}
