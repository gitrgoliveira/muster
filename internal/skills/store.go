package skills

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Registry error sentinels — mapped to typed API errors by the service layer.
var (
	// ErrReadonly: attempt to delete/modify a read-only built-in skill.
	ErrReadonly = errors.New("skill is a read-only built-in")
	// ErrIDConflict: an imported skill id collides with a built-in id.
	ErrIDConflict = errors.New("skill id collides with a built-in")
	// ErrNotFound: no such skill.
	ErrNotFound = errors.New("skill not found")
)

// fileStore persists imported skills as one markdown file per id under dir. It
// is the sole writer of that directory; every mutation holds mu (per-id
// serialization) and writes atomically, so concurrent CRUD cannot leave a
// half-written or orphaned file.
type fileStore struct {
	mu  sync.Mutex
	dir string
	m   map[string]Skill
}

func newFileStore(dir string) (*fileStore, error) {
	s := &fileStore{dir: dir, m: map[string]Skill{}}
	if err := s.reload(); err != nil {
		return nil, err
	}
	return s, nil
}

// reload re-scans the directory. A missing directory is an empty store (not an
// error); an unreadable or malformed on-disk file is skipped rather than
// failing the whole load.
func (s *fileStore) reload() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := map[string]Skill{}
	entries, err := os.ReadDir(s.dir)
	if errors.Is(err, os.ErrNotExist) {
		s.m = m
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.dir, e.Name()))
		if err != nil {
			continue
		}
		sk, err := ParseSkill(data, false)
		if err != nil {
			continue
		}
		m[sk.ID] = sk
	}
	s.m = m
	return nil
}

func (s *fileStore) list() []Skill {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Skill, 0, len(s.m))
	for _, sk := range s.m {
		out = append(out, sk)
	}
	return out
}

func (s *fileStore) get(id string) (Skill, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sk, ok := s.m[id]
	return sk, ok
}

// put upserts an imported skill (create or overwrite in place). The id is
// gated by ValidateID before any filesystem path is formed, so a traversal id
// can never escape dir.
func (s *fileStore) put(sk Skill) error {
	if err := ValidateID(sk.ID); err != nil {
		return err
	}
	data, err := formatSkill(sk)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return err
	}
	if err := atomicWriteFile(s.pathFor(sk.ID), data, 0o600); err != nil {
		return err
	}
	s.m[sk.ID] = sk
	return nil
}

func (s *fileStore) delete(id string) error {
	if err := ValidateID(id); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.m[id]; !ok {
		return ErrNotFound
	}
	if err := os.Remove(s.pathFor(id)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	delete(s.m, id)
	return nil
}

func (s *fileStore) pathFor(id string) string {
	return filepath.Join(s.dir, id+".md")
}

// atomicWriteFile writes data to a temp file in the same directory then renames
// it over path (atomic on a single filesystem).
func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() { _ = os.Remove(tmp) }()
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Chmod(perm); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
