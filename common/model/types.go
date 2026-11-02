package model

// UserID uniquely identifies a user.
type UserID string

// String returns the raw identifier.
func (id UserID) String() string { return string(id) }
