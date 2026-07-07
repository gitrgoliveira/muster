package services

import (
	"errors"

	"github.com/gitrgoliveira/muster/internal/skills"
)

// M6 skill error codes (match the render-layer codes so the handler can derive
// the HTTP status from the code).
const (
	CodeSkillReadonly   = "SKILL_READONLY"
	CodeSkillIDConflict = "SKILL_ID_CONFLICT"
	CodeSkillInvalidID  = "SKILL_INVALID_ID"
	// CodeSkillNotFound is the plain 404 code (distinct from bead NOT_FOUND).
	CodeSkillNotFound = "NOT_FOUND"
)

// SkillService is the thin business layer over the skill registry. It maps the
// registry's error sentinels to typed ServiceErrors for the handler.
type SkillService struct {
	reg *skills.Registry
}

// NewSkillService constructs the service over a loaded registry.
func NewSkillService(reg *skills.Registry) *SkillService {
	return &SkillService{reg: reg}
}

// List returns the full registry (built-in + imported).
func (s *SkillService) List() []skills.Skill { return s.reg.List() }

// Categories returns the distinct categories across the registry.
func (s *SkillService) Categories() []string { return s.reg.Categories() }

// Import fetches and registers a skill from a URL.
func (s *SkillService) Import(url string) (skills.Skill, error) {
	sk, err := s.reg.ImportFromURL(url)
	if err == nil {
		return sk, nil
	}
	if errors.Is(err, skills.ErrIDConflict) {
		return skills.Skill{}, &ServiceError{Code: CodeSkillIDConflict, Message: "a built-in skill already uses this id"}
	}
	// Blocked URL, transport failure, oversize, or a bad skill document — all a
	// client-side 400 with no partial registration.
	return skills.Skill{}, &ServiceError{Code: CodeInvalidRequest, Message: err.Error()}
}

// Delete removes an imported skill.
func (s *SkillService) Delete(id string) error {
	err := s.reg.Delete(id)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, skills.ErrReadonly):
		return &ServiceError{Code: CodeSkillReadonly, Message: "built-in skills are read-only"}
	case errors.Is(err, skills.ErrNotFound):
		return &ServiceError{Code: CodeSkillNotFound, Message: "skill not found"}
	default:
		// A ValidateID failure (e.g. a traversal id) reaches here.
		return &ServiceError{Code: CodeSkillInvalidID, Message: err.Error()}
	}
}
