package pkg

import (
	"context"

	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/common/model"
)

type UserClient interface {
	ListUsers(ctx context.Context) ([]User, error)
	GetUserByID(ctx context.Context, userID model.UserID) (User, error)
}

// User represents a user returned by the identity service.
type User struct {
	UserID   model.UserID `json:"user_id"`
	Nickname string       `json:"nickname"`
}
