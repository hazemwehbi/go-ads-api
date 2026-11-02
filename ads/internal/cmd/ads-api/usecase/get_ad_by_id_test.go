package usecase_test

import (
	"context"
	"errors"
	"fmt"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/userlookup"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	adsdao "github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/dao"
	mockdao "github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/dao/mock"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/domain"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/usecase"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/common/model"
	usersclient "github.mpi-internal.com/leboncoin/backend-hiring-assessment/users/pkg"
	mockusers "github.mpi-internal.com/leboncoin/backend-hiring-assessment/users/pkg/mock"
)

func TestGetAdByID(t *testing.T) {
	ctx := context.Background()
	ad := domain.Ad{ID: 42, Title: "bike", Price: 5000, UserID: "00000000-0000-0000-0000-000000000001"}

	t.Run("nominal", func(t *testing.T) {
		m := mockdao.New(t)
		users := mockusers.New(t)
		m.On("GetAdByID", ctx, domain.AdID(42)).Return(ad, nil)
		users.On("GetUserByID", ctx, model.UserID("00000000-0000-0000-0000-000000000001")).
			Return(usersclient.User{UserID: "00000000-0000-0000-0000-000000000001", Nickname: "alice"}, nil)

		got, err := usecase.NewGetAdByIDFunc(m, users)(ctx, 42)

		require.NoError(t, err)
		assert.Equal(t, usecase.AdDetails{Ad: ad, Username: usernamePtr("alice")}, got)
	})

	t.Run("null username when user service fails", func(t *testing.T) {
		m := mockdao.New(t)
		users := mockusers.New(t)
		m.On("GetAdByID", ctx, domain.AdID(42)).Return(ad, nil)
		users.On("GetUserByID", ctx, model.UserID("00000000-0000-0000-0000-000000000001")).
			Return(usersclient.User{}, &userlookup.Failure{Reason: "unavailable"})

		got, err := usecase.NewGetAdByIDFunc(m, users)(ctx, 42)

		require.NoError(t, err)
		assert.Equal(t, usecase.AdDetails{Ad: ad}, got)
	})

	t.Run("missing adapter is a configuration error", func(t *testing.T) {
		m := mockdao.New(t)
		m.On("GetAdByID", ctx, domain.AdID(42)).Return(ad, nil)

		got, err := usecase.NewGetAdByIDFunc(m, nil)(ctx, 42)

		require.Error(t, err)
		require.Equal(t, "unexpected", userlookup.Reason(err))
		require.ErrorIs(t, err, usecase.ErrOwnerLookup)
		require.Nil(t, got.Username)
	})

	t.Run("error no ad found", func(t *testing.T) {
		m := mockdao.New(t)
		m.On("GetAdByID", ctx, domain.AdID(99)).Return(domain.Ad{}, adsdao.ErrNotFound)

		_, err := usecase.NewGetAdByIDFunc(m, mockusers.New(t))(ctx, 99)

		require.Error(t, err)
		assert.ErrorIs(t, err, adsdao.ErrNotFound)
	})

	t.Run("error from dao", func(t *testing.T) {
		m := mockdao.New(t)
		dbErr := errors.New("connection reset")
		m.On("GetAdByID", ctx, domain.AdID(1)).Return(domain.Ad{}, dbErr)

		_, err := usecase.NewGetAdByIDFunc(m, mockusers.New(t))(ctx, 1)

		require.Error(t, err)
		assert.ErrorIs(t, err, dbErr)
	})
}

func usernamePtr(s string) *string { return &s }

func TestGetAdEnrichmentPolicy(t *testing.T) {
	for _, reason := range []string{"timeout", "unavailable", "not_found", "circuit_open", "capacity", "invalid_response", "unexpected", "future_reason"} {
		t.Run(reason, func(t *testing.T) {
			ctx := context.Background()
			m := mockdao.New(t)
			u := mockusers.New(t)
			ad := domain.Ad{ID: 1, UserID: "00000000-0000-0000-0000-000000000001"}
			failure := &userlookup.Failure{Reason: reason}
			if reason == "timeout" {
				failure.Cause = context.DeadlineExceeded
			}
			m.On("GetAdByID", ctx, domain.AdID(1)).Return(ad, nil)
			u.On("GetUserByID", ctx, ad.UserID).Return(usersclient.User{}, fmt.Errorf("lookup adapter: %w", failure))
			result, err := usecase.NewGetAdByIDFunc(m, u)(ctx, 1)
			if reason == "timeout" || reason == "unavailable" || reason == "not_found" || reason == "circuit_open" || reason == "capacity" {
				require.NoError(t, err)
				require.Equal(t, ad, result.Ad)
				require.Nil(t, result.Username)
			} else {
				require.ErrorIs(t, err, failure)
				require.Equal(t, reason == "invalid_response" || reason == "unexpected", errors.Is(err, usecase.ErrOwnerLookup))
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	m := mockdao.New(t)
	_, err := usecase.NewGetAdByIDFunc(m, nil)(ctx, 1)
	require.ErrorIs(t, err, context.Canceled)
}
