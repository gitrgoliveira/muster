package skills

import (
	"net/http"
	"path/filepath"
	"sort"
)

// Registry is the full skill registry: embedded read-only built-ins plus
// user/URL-imported skills persisted under <musterDir>/skills/. It implements
// the orchestrator's SkillProvider (Resolve).
type Registry struct {
	builtins map[string]Skill
	store    *fileStore
	client   *http.Client // import client (nil => a default bounded client)
}

// NewRegistry loads the built-in catalog and the imported-skill store.
func NewRegistry(musterDir string) (*Registry, error) {
	bs, err := loadBuiltins()
	if err != nil {
		return nil, err
	}
	bm := make(map[string]Skill, len(bs))
	for _, b := range bs {
		bm[b.ID] = b
	}
	store, err := newFileStore(filepath.Join(musterDir, "skills"))
	if err != nil {
		return nil, err
	}
	return &Registry{builtins: bm, store: store}, nil
}

// List returns the full registry (built-in + imported), sorted by id. When an
// imported id somehow duplicates a built-in id, the built-in wins (imports of a
// built-in id are rejected, so this is defensive).
func (r *Registry) List() []Skill {
	seen := make(map[string]bool)
	var out []Skill
	for _, b := range r.builtins {
		out = append(out, b)
		seen[b.ID] = true
	}
	for _, s := range r.store.list() {
		if !seen[s.ID] {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Categories returns the distinct categories present across the registry.
func (r *Registry) Categories() []string {
	set := make(map[string]bool)
	for _, s := range r.List() {
		if s.Category != "" {
			set[s.Category] = true
		}
	}
	out := make([]string, 0, len(set))
	for c := range set {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}

// Get returns a skill by id (built-in takes precedence).
func (r *Registry) Get(id string) (Skill, bool) {
	if b, ok := r.builtins[id]; ok {
		return b, true
	}
	return r.store.get(id)
}

// ImportFromURL fetches, validates, and persists a skill from a URL. Importing
// an id that collides with a built-in is rejected (ErrIDConflict); importing an
// id equal to an existing imported skill is an upsert (overwrite in place).
func (r *Registry) ImportFromURL(rawURL string) (Skill, error) {
	sk, err := fetchSkill(r.client, rawURL)
	if err != nil {
		return Skill{}, err
	}
	if _, ok := r.builtins[sk.ID]; ok {
		return Skill{}, ErrIDConflict
	}
	if err := r.store.put(sk); err != nil {
		return Skill{}, err
	}
	return sk, nil
}

// Delete removes an imported skill. Deleting a built-in fails with ErrReadonly;
// deleting an unknown id fails with ErrNotFound.
func (r *Registry) Delete(id string) error {
	if _, ok := r.builtins[id]; ok {
		return ErrReadonly
	}
	return r.store.delete(id)
}

// Resolve implements orchestrator.SkillProvider: it resolves a set of skill ids
// into concrete skills (built-in + imported), de-duplicated and in a stable
// order, and returns the ids it could not resolve so assembly can warn (never a
// silent drop). An id that fails ValidateID is treated as unresolvable.
func (r *Registry) Resolve(ids []string) (resolved []Skill, unresolved []string) {
	seen := make(map[string]bool)
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		if err := ValidateID(id); err != nil {
			unresolved = append(unresolved, id)
			continue
		}
		if sk, ok := r.Get(id); ok {
			resolved = append(resolved, sk)
		} else {
			unresolved = append(unresolved, id)
		}
	}
	sort.Slice(resolved, func(i, j int) bool { return resolved[i].ID < resolved[j].ID })
	return resolved, unresolved
}
