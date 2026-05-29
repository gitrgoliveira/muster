package store

import "errors"

var (
	ErrNotFound         = errors.New("issue not found")
	ErrStoreUnavailable = errors.New("store unavailable")
	ErrStoreReadOnly    = errors.New("store is read-only")
	ErrSchemaMismatch   = errors.New("schema version mismatch")
)
