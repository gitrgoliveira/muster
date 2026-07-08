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

// Import fetches and registers a skill from a URL. Error classification is
// explicit so a server-side fault is never misreported as a client 400 and no
// raw internal error text leaks to the client:
//   - id collides with a built-in            -> SKILL_ID_CONFLICT (409)
//   - imported id fails validation           -> SKILL_INVALID_ID  (400)
//   - filesystem persistence failure         -> INTERNAL          (500)
//   - blocked URL / transport / oversize /
//     bad skill document (fetch+parse)       -> INVALID_REQUEST   (400)
func (s *SkillService) Import(url string) (skills.Skill, error) {
	sk, err := s.reg.ImportFromURL(url)
	switch {
	case err == nil:
		return sk, nil
	case errors.Is(err, skills.ErrIDConflict):
		return skills.Skill{}, &ServiceError{Code: CodeSkillIDConflict, Message: "a built-in skill already uses this id"}
	case errors.Is(err, skills.ErrInvalidID):
		return skills.Skill{}, &ServiceError{Code: CodeSkillInvalidID, Message: err.Error()}
	case errors.Is(err, skills.ErrPersist):
		return skills.Skill{}, &ServiceError{Code: CodeInternal, Message: "failed to persist skill"}
	default:
		// Blocked URL, transport failure, oversize, or a bad skill document — a
		// client-side 400 with no partial registration.
		return skills.Skill{}, &ServiceError{Code: CodeInvalidRequest, Message: err.Error()}
	}
}

// Delete removes an imported skill. The id is validated up front so an invalid
// id is a clean 400 (SKILL_INVALID_ID); any unexpected error from the store
// (e.g. a filesystem failure) is a 500 rather than a misclassified 400 with raw
// error text.
func (s *SkillService) Delete(id string) error {
	if err := skills.ValidateID(id); err != nil {
		return &ServiceError{Code: CodeSkillInvalidID, Message: err.Error()}
	}
	err := s.reg.Delete(id)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, skills.ErrReadonly):
		return &ServiceError{Code: CodeSkillReadonly, Message: "built-in skills are read-only"}
	case errors.Is(err, skills.ErrNotFound):
		return &ServiceError{Code: CodeSkillNotFound, Message: "skill not found"}
	default:
		// Unexpected (e.g. filesystem) fault — internal, don't leak raw text.
		return &ServiceError{Code: CodeInternal, Message: "failed to delete skill"}
	}
}
