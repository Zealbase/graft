package store

import "errors"

// DeleteWorkspace removes a workspace and all rows that cascade from it, in
// FK-safe order within a transaction (v0.0.3 task 1 / destroy). Frozen at s-0;
// the db agent implements the cascade in phase t-db.
func (s *sqlStore) DeleteWorkspace(wsID string) error {
	return errors.New("store: DeleteWorkspace not implemented yet (s-0 contract stub)")
}
