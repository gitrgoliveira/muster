package services

import (
	"context"
	"errors"
	"testing"

	"github.com/gitrgoliveira/muster/internal/store/bdshell"
)

type fakeMemStore struct {
	m        map[string]string
	failList bool
}

func newFakeMemStore() *fakeMemStore { return &fakeMemStore{m: map[string]string{}} }

func (f *fakeMemStore) Remember(_ context.Context, key, value string) (string, error) {
	if key == "" {
		key = "derived-" + value
	}
	f.m[key] = value
	return key, nil
}
func (f *fakeMemStore) Recall(_ context.Context, key string) (string, error) { return f.m[key], nil }
func (f *fakeMemStore) Forget(_ context.Context, key string) error {
	if _, ok := f.m[key]; !ok {
		return bdshell.ErrMemoryNotFound
	}
	delete(f.m, key)
	return nil
}
func (f *fakeMemStore) Memories(_ context.Context, _ string) (map[string]string, error) {
	if f.failList {
		return nil, errors.New("bd boom")
	}
	return f.m, nil
}

func codeOf(t *testing.T, err error) string {
	t.Helper()
	var se *ServiceError
	if !errors.As(err, &se) {
		t.Fatalf("want *ServiceError, got %T: %v", err, err)
	}
	return se.Code
}

func TestMemories_UpsertListDelete(t *testing.T) {
	svc := NewMemoriesService(newFakeMemStore(), t.TempDir())
	ctx := context.Background()

	m, err := svc.Upsert(ctx, "", "run tests with -race")
	if err != nil {
		t.Fatal(err)
	}
	if m.Key == "" || m.Value != "run tests with -race" {
		t.Fatalf("upsert = %+v", m)
	}

	list, err := svc.List(ctx, "")
	if err != nil || len(list) != 1 || list[0].Key != m.Key {
		t.Fatalf("list = %v err=%v", list, err)
	}

	if err := svc.Delete(ctx, m.Key); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, m.Key); codeOf(t, err) != CodeMemoryNotFound {
		t.Fatalf("delete missing code = %v, want NOT_FOUND", err)
	}
}

func TestMemories_EmptyValueRejected(t *testing.T) {
	svc := NewMemoriesService(newFakeMemStore(), t.TempDir())
	if _, err := svc.Upsert(context.Background(), "", ""); codeOf(t, err) != CodeInvalidRequest {
		t.Fatalf("empty value code = %v, want INVALID_REQUEST", err)
	}
}

func TestMemories_BDFailureSurfacedNotEmptyList(t *testing.T) {
	store := newFakeMemStore()
	store.failList = true
	svc := NewMemoriesService(store, t.TempDir())
	_, err := svc.List(context.Background(), "")
	if codeOf(t, err) != CodeBDUnavailable {
		t.Fatalf("bd failure code = %v, want BD_UNAVAILABLE (never empty-list success)", err)
	}
}

func TestMemories_NilStoreUnavailable(t *testing.T) {
	svc := NewMemoriesService(nil, t.TempDir())
	if _, err := svc.List(context.Background(), ""); codeOf(t, err) != CodeBDUnavailable {
		t.Fatalf("nil store code = %v, want BD_UNAVAILABLE", err)
	}
}

func TestMemories_PrimePersistsAndReadsBack(t *testing.T) {
	store := newFakeMemStore()
	store.m["k1"] = "v1"
	store.m["k2"] = "v2"
	dir := t.TempDir()
	svc := NewMemoriesService(store, dir)

	n, err := svc.Prime(context.Background(), "muster-ep0")
	if err != nil || n != 2 {
		t.Fatalf("prime = %d err=%v", n, err)
	}

	// A fresh service over the same dir (a restart) still reads the snapshot.
	svc2 := NewMemoriesService(store, dir)
	primed := svc2.PrimedMemories("muster-ep0")
	if primed["k1"] != "v1" || primed["k2"] != "v2" {
		t.Fatalf("primed snapshot did not survive restart: %v", primed)
	}
	if svc2.PrimedMemories("never-primed") != nil {
		t.Fatal("un-primed bead should return nil")
	}
}

func TestMemories_PrimeInvalidBeadID(t *testing.T) {
	svc := NewMemoriesService(newFakeMemStore(), t.TempDir())
	if _, err := svc.Prime(context.Background(), "../escape"); codeOf(t, err) != CodeInvalidRequest {
		t.Fatalf("invalid beadID code = %v, want INVALID_REQUEST", err)
	}
}
