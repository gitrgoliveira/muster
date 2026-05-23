package core

import (
	"strings"

	"github.com/google/uuid"
)

// NewBeadID generates "bd-XXXX" where XXXX is the first 4 hex chars (lowercase)
// of a UUIDv4. Callers must check store-level uniqueness and retry on collision.
func NewBeadID() string {
	return "bd-" + strings.ToLower(uuid.NewString()[:4])
}
