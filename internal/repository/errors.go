package repository

import "errors"

var (
	// ErrNotFound marks missing repository entities.
	ErrNotFound = errors.New("storage: not found")

	// ErrConflict marks duplicate or conflicting repository entities.
	ErrConflict = errors.New("storage: conflict")
)
