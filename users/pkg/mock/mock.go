package mock

import (
	"context"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/common/model"
	usersclient "github.mpi-internal.com/leboncoin/backend-hiring-assessment/users/pkg"
)

// Mock is a testify-based implementation of pkg.UserClient.
type Mock struct {
	mock.Mock
}

// New returns a mock bound to the given test. Every expectation set on it is verified when the test finishes.
func New(t *testing.T) *Mock {
	t.Helper()

	m := &Mock{}
	m.Test(t)
	t.Cleanup(func() { m.AssertExpectations(t) })

	return m
}

// ListUsers returns the users and the error set on the mock.
func (m *Mock) ListUsers(ctx context.Context) ([]usersclient.User, error) {
	args := m.Called(ctx)

	users, _ := args.Get(0).([]usersclient.User)

	return users, args.Error(1)
}

// GetUserByID returns the user and the error set on the mock.
func (m *Mock) GetUserByID(ctx context.Context, userID model.UserID) (usersclient.User, error) {
	args := m.Called(ctx, userID)

	user, _ := args.Get(0).(usersclient.User)

	return user, args.Error(1)
}
