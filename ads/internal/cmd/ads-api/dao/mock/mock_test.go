package mock_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	adsdao "github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/dao"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/dao/mock"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/domain"
)

var ctx = context.Background()

func TestMock_SearchAds(t *testing.T) {
	t.Run("returns configured ads", func(t *testing.T) {
		m := mock.New(t)
		req := adsdao.SearchAdsRequest{Title: "bike"}
		want := []domain.Ad{{ID: 1, Title: "bike", Price: 5000}}

		m.On("SearchAds", ctx, req).Return(want, nil)

		got, err := m.SearchAds(ctx, req)

		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("returns nil slice on error", func(t *testing.T) {
		m := mock.New(t)
		req := adsdao.SearchAdsRequest{Title: "bike"}
		boom := errors.New("db down")

		m.On("SearchAds", ctx, req).Return(nil, boom)

		got, err := m.SearchAds(ctx, req)

		assert.ErrorIs(t, err, boom)
		assert.Nil(t, got)
	})
}

func TestMock_GetAdByID(t *testing.T) {
	t.Run("returns configured ad", func(t *testing.T) {
		m := mock.New(t)
		want := domain.Ad{ID: 42, Title: "surfboard", Price: 15000}

		m.On("GetAdByID", ctx, domain.AdID(42)).Return(want, nil)

		got, err := m.GetAdByID(ctx, 42)

		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("returns ErrNotFound", func(t *testing.T) {
		m := mock.New(t)

		m.On("GetAdByID", ctx, domain.AdID(99)).Return(domain.Ad{}, adsdao.ErrNotFound)

		_, err := m.GetAdByID(ctx, 99)

		assert.ErrorIs(t, err, adsdao.ErrNotFound)
	})
}

func TestMock_CreateAd(t *testing.T) {
	t.Run("returns generated ID", func(t *testing.T) {
		m := mock.New(t)
		req := adsdao.CreateAdRequest{
			Title:      "bike",
			PriceCents: 5000,
			PhotoURL:   "https://example.com/bike.jpg",
			OwnerID:    "00000000-0000-0000-0000-000000000001",
		}

		m.On("CreateAd", ctx, req).Return(domain.AdID(7), nil)

		got, err := m.CreateAd(ctx, req)

		require.NoError(t, err)
		assert.Equal(t, domain.AdID(7), got)
	})

	t.Run("returns storage errors", func(t *testing.T) {
		storageErr := errors.New("storage failed")
		m := mock.New(t)
		req := adsdao.CreateAdRequest{
			Title:      "bike",
			PriceCents: 5000,
			PhotoURL:   "https://example.com/bike.jpg",
			OwnerID:    "ghost",
		}

		m.On("CreateAd", ctx, req).Return(domain.AdID(0), storageErr)

		_, err := m.CreateAd(ctx, req)

		assert.ErrorIs(t, err, storageErr)
	})
}

func TestMock_DeleteAdByID(t *testing.T) {
	t.Run("returns nil on success", func(t *testing.T) {
		m := mock.New(t)

		m.On("DeleteAdByID", ctx, domain.AdID(42), domain.UserID("00000000-0000-0000-0000-000000000001")).Return(nil)

		require.NoError(t, m.DeleteAdByID(ctx, 42, "00000000-0000-0000-0000-000000000001"))
	})

	t.Run("returns ErrNotFound", func(t *testing.T) {
		m := mock.New(t)

		m.On("DeleteAdByID", ctx, domain.AdID(99), domain.UserID("00000000-0000-0000-0000-000000000001")).Return(adsdao.ErrNotFound)

		assert.ErrorIs(t, m.DeleteAdByID(ctx, 99, "00000000-0000-0000-0000-000000000001"), adsdao.ErrNotFound)
	})
}

func TestMock_Begin(t *testing.T) {
	t.Run("returns the mock itself as transaction", func(t *testing.T) {
		m := mock.New(t)

		m.On("Begin", ctx).Return(nil)

		tx, err := m.Begin(ctx)

		require.NoError(t, err)
		assert.Same(t, m, tx)
	})

	t.Run("returns configured error", func(t *testing.T) {
		m := mock.New(t)
		boom := errors.New("db down")

		m.On("Begin", ctx).Return(boom)

		tx, err := m.Begin(ctx)

		assert.ErrorIs(t, err, boom)
		assert.Nil(t, tx)
	})
}

func TestMock_Commit(t *testing.T) {
	t.Run("returns nil on success", func(t *testing.T) {
		m := mock.New(t)

		m.On("Commit").Return(nil)

		require.NoError(t, m.Commit())
	})

	t.Run("returns configured error", func(t *testing.T) {
		m := mock.New(t)
		boom := errors.New("commit failed")

		m.On("Commit").Return(boom)

		assert.ErrorIs(t, m.Commit(), boom)
	})
}

func TestMock_Rollback(t *testing.T) {
	t.Run("returns nil on success", func(t *testing.T) {
		m := mock.New(t)

		m.On("Rollback").Return(nil)

		require.NoError(t, m.Rollback())
	})

	t.Run("returns configured error", func(t *testing.T) {
		m := mock.New(t)
		boom := errors.New("rollback failed")

		m.On("Rollback").Return(boom)

		assert.ErrorIs(t, m.Rollback(), boom)
	})
}

func TestMock_ClaimRequest(t *testing.T) {
	key := adsdao.RequestKey{OwnerID: "00000000-0000-0000-0000-000000000001", IdempotencyKey: "key-1"}

	t.Run("returns configured report result", func(t *testing.T) {
		m := mock.New(t)

		m.On("ClaimRequest", ctx, key, "hash-1").Return(true, nil)

		reported, err := m.ClaimRequest(ctx, key, "hash-1")

		require.NoError(t, err)
		assert.True(t, reported)
	})

	t.Run("returns configured error", func(t *testing.T) {
		m := mock.New(t)
		boom := errors.New("db down")

		m.On("ClaimRequest", ctx, key, "hash-1").Return(false, boom)

		_, err := m.ClaimRequest(ctx, key, "hash-1")

		assert.ErrorIs(t, err, boom)
	})
}

func TestMock_GetRequestReceipt(t *testing.T) {
	key := adsdao.RequestKey{OwnerID: "00000000-0000-0000-0000-000000000001", IdempotencyKey: "key-1"}

	t.Run("returns configured import", func(t *testing.T) {
		m := mock.New(t)
		want := adsdao.RequestReceipt{RequestHash: "hash-1", AdIDs: []domain.AdID{10, 11}}

		m.On("GetRequestReceipt", ctx, key).Return(want, nil)

		got, err := m.GetRequestReceipt(ctx, key)

		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("returns zero import on error", func(t *testing.T) {
		m := mock.New(t)

		m.On("GetRequestReceipt", ctx, key).Return(nil, adsdao.ErrNotFound)

		got, err := m.GetRequestReceipt(ctx, key)

		assert.ErrorIs(t, err, adsdao.ErrNotFound)
		assert.Equal(t, adsdao.RequestReceipt{}, got)
	})
}

func TestMock_CompleteRequest(t *testing.T) {
	key := adsdao.RequestKey{OwnerID: "00000000-0000-0000-0000-000000000001", IdempotencyKey: "key-1"}

	t.Run("returns nil on success", func(t *testing.T) {
		m := mock.New(t)

		m.On("CompleteRequest", ctx, key, []domain.AdID{10, 11}).Return(nil)

		require.NoError(t, m.CompleteRequest(ctx, key, []domain.AdID{10, 11}))
	})

	t.Run("returns configured error", func(t *testing.T) {
		m := mock.New(t)

		m.On("CompleteRequest", ctx, key, []domain.AdID{10}).Return(adsdao.ErrNotFound)

		assert.ErrorIs(t, m.CompleteRequest(ctx, key, []domain.AdID{10}), adsdao.ErrNotFound)
	})
}
