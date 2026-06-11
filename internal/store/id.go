package store

import "github.com/google/uuid"

// newID returns a random unique identifier for a new row.
func newID() string { return uuid.NewString() }
