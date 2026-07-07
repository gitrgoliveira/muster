package services

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gitrgoliveira/muster/internal/ws"
)

// Constitution is muster's single versioned operating-rules document, merged
// into every dispatched prompt (M6 US2). It is muster's own config (local file),
// not beads issue state.
type Constitution struct {
	Markdown  string    `json:"markdown"`
	Version   int       `json:"version"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// constMeta is the sidecar that persists the version/updatedAt next to the
// human-editable constitution.md so a restart reports the same version (FR-010).
type constMeta struct {
	Version   int       `json:"version"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// ConstitutionService owns the constitution file under <musterDir>. It is safe
// for concurrent use: Snapshot/Get take a read lock and copy markdown+version
// together (no torn read under a concurrent Set — edge case), and Set persists
// atomically before swapping the in-memory value.
type ConstitutionService struct {
	mu       sync.RWMutex
	mdPath   string
	metaPath string
	cur      Constitution
	publish  Publisher
}

// NewConstitutionService constructs the service and loads any existing
// constitution from disk. A missing file is not an error — it resolves to the
// empty default at version 0 (FR-011).
func NewConstitutionService(musterDir string, publish Publisher) *ConstitutionService {
	s := &ConstitutionService{
		mdPath:   filepath.Join(musterDir, "constitution.md"),
		metaPath: filepath.Join(musterDir, "constitution.meta.json"),
		publish:  publish,
	}
	s.load()
	return s
}

// load reads the markdown + meta sidecar (best-effort). A missing or unreadable
// markdown file yields an empty document; a missing or corrupt meta yields
// version 0. Never returns an error — assembly and startup must not fail on a
// bad constitution (FR-011, corrupt-file edge case).
func (s *ConstitutionService) load() {
	md, err := os.ReadFile(s.mdPath)
	if err != nil {
		md = nil
	}
	var meta constMeta
	if b, err := os.ReadFile(s.metaPath); err == nil {
		_ = json.Unmarshal(b, &meta) // corrupt meta -> zero value (v0)
	}
	s.mu.Lock()
	s.cur = Constitution{Markdown: string(md), Version: meta.Version, UpdatedAt: meta.UpdatedAt}
	s.mu.Unlock()
}

// Get returns the current constitution snapshot (atomic copy).
func (s *ConstitutionService) Get() Constitution {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cur
}

// Snapshot implements the orchestrator's ConstitutionProvider: it returns the
// markdown and version together under a single read lock so assembly never sees
// a torn combination.
func (s *ConstitutionService) Snapshot() (string, int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cur.Markdown, s.cur.Version
}

// Set overwrites the constitution, bumps the version monotonically, persists
// atomically, and emits a constitution.changed event. The change takes effect
// for the NEXT assembly; an already-written prompt file is untouched (FR-009).
func (s *ConstitutionService) Set(markdown string) (Constitution, error) {
	return s.setAt(markdown, time.Now())
}

// setAt is Set with an injectable timestamp (tests).
func (s *ConstitutionService) setAt(markdown string, now time.Time) (Constitution, error) {
	s.mu.Lock()
	next := Constitution{Markdown: markdown, Version: s.cur.Version + 1, UpdatedAt: now}
	if err := s.persist(next); err != nil {
		s.mu.Unlock()
		return Constitution{}, err // cur unchanged on failure
	}
	s.cur = next
	pub := s.publish
	s.mu.Unlock()

	if pub != nil {
		v := next.Version
		pub(ws.Frame{Type: ws.EventConstitutionChanged, Version: &v})
	}
	return next, nil
}

// persist writes the markdown and meta sidecar atomically (temp file + rename),
// so a crash mid-write never leaves a torn constitution file.
func (s *ConstitutionService) persist(c Constitution) error {
	if err := os.MkdirAll(filepath.Dir(s.mdPath), 0o700); err != nil {
		return err
	}
	if err := atomicWriteFile(s.mdPath, []byte(c.Markdown), 0o600); err != nil {
		return err
	}
	meta, err := json.Marshal(constMeta{Version: c.Version, UpdatedAt: c.UpdatedAt})
	if err != nil {
		return err
	}
	return atomicWriteFile(s.metaPath, meta, 0o600)
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
	defer func() { _ = os.Remove(tmp) }() // no-op after a successful rename
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
