package store_test

import (
	"context"
	"math/rand"
	"sync"
	"testing"

	"github.com/gitrgoliveira/muster/internal/core"
	"github.com/gitrgoliveira/muster/internal/store"
)

func TestStore_ConcurrentReadsWrites(t *testing.T) {
	seeds := []core.Bead{
		{
			ID: "bd-cc01", Title: "Concurrent A", Column: core.ColBacklog,
			Type: core.TypeTask, Priority: 2, VCS: core.VCSGit, Repo: "main",
			Labels: []string{}, Skills: []string{}, Steps: []core.Step{},
			SubBeads: []core.SubBead{}, History: []core.HistoryEvent{},
			Acceptance: []core.Acceptance{}, Log: []core.LogEntry{},
			Files: []core.FileChange{}, Blocks: []string{}, BlockedBy: []string{},
		},
		{
			ID: "bd-cc02", Title: "Concurrent B", Column: core.ColScheduled,
			Type: core.TypeTask, Priority: 2, VCS: core.VCSGit, Repo: "main",
			Labels: []string{}, Skills: []string{}, Steps: []core.Step{},
			SubBeads: []core.SubBead{}, History: []core.HistoryEvent{},
			Acceptance: []core.Acceptance{}, Log: []core.LogEntry{},
			Files: []core.FileChange{}, Blocks: []string{}, BlockedBy: []string{},
		},
	}
	ms := store.NewMemStore(seeds)
	ctx := context.Background()

	const goroutines = 20
	const iterations = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := range goroutines {
		go func(n int) {
			defer wg.Done()
			rng := rand.New(rand.NewSource(int64(n)))
			for range iterations {
				switch rng.Intn(3) {
				case 0:
					ms.List(ctx, "")
				case 1:
					ms.Patch(ctx, "bd-cc01", store.PatchBeadInput{
						Title: ptr("updated " + string(rune('A'+n))),
					})
				case 2:
					ms.Move(ctx, "bd-cc01", string(core.ColBacklog), "")
				}
			}
		}(i)
	}

	wg.Wait()
	// If the race detector detects a data race, the test fails automatically.
}
