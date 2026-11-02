package mock_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/common/model"
	usersclient "github.mpi-internal.com/leboncoin/backend-hiring-assessment/users/pkg"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/users/pkg/mock"
)

func TestMockListUsers(t *testing.T) {
	ctx := context.Background()

	t.Run("returns configured users", func(t *testing.T) {
		m := mock.New(t)
		want := []usersclient.User{
			{UserID: "user-1", Nickname: "alice"},
			{UserID: "user-2", Nickname: "bob"},
		}
		m.On("ListUsers", ctx).Return(want, nil)

		got, err := m.ListUsers(ctx)

		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("returns configured error", func(t *testing.T) {
		m := mock.New(t)
		boom := errors.New("connection reset")
		m.On("ListUsers", ctx).Return(nil, boom)

		_, err := m.ListUsers(ctx)

		require.Error(t, err)
		assert.ErrorIs(t, err, boom)
	})
}

func TestMockGetUserByID(t *testing.T) {
	ctx := context.Background()

	t.Run("returns configured user", func(t *testing.T) {
		m := mock.New(t)
		want := usersclient.User{UserID: "user-1", Nickname: "alice"}
		m.On("GetUserByID", ctx, model.UserID("user-1")).Return(want, nil)

		got, err := m.GetUserByID(ctx, "user-1")

		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("returns configured error", func(t *testing.T) {
		m := mock.New(t)
		boom := errors.New("user not found")
		m.On("GetUserByID", ctx, model.UserID("unknown")).Return(usersclient.User{}, boom)

		_, err := m.GetUserByID(ctx, "unknown")

		require.Error(t, err)
		assert.ErrorIs(t, err, boom)
	})
}
