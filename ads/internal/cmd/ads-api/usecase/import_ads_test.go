package usecase_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	adsdao "github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/dao"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/dao/mock"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/domain"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/usecase"
)

func TestImportAds(t *testing.T) {
	ctx := context.Background()

	req := usecase.ImportAdsRequest{
		IdempotencyKey: "key-1",
		OwnerID:        "00000000-0000-0000-0000-000000000001",
		Ads: []usecase.ImportedAd{
			{Title: "bike", PriceCents: 5000, PhotoURL: "https://example.com/bike.jpg"},
			{Title: "scooter", PriceCents: 8000, PhotoURL: "https://example.com/scooter.jpg"},
		},
	}
	bike := adsdao.CreateAdRequest{Title: "bike", PriceCents: 5000, PhotoURL: "https://example.com/bike.jpg", OwnerID: "00000000-0000-0000-0000-000000000001"}
	scooter := adsdao.CreateAdRequest{Title: "scooter", PriceCents: 8000, PhotoURL: "https://example.com/scooter.jpg", OwnerID: "00000000-0000-0000-0000-000000000001"}

	t.Run("nominal", func(t *testing.T) {
		m := mock.New(t)
		m.On("Begin", ctx).Return(nil).Once()
		m.On("ClaimRequest", ctx, adsdao.RequestKey{OwnerID: req.OwnerID, IdempotencyKey: "key-1"}, "4c68d866a3e18f6e579c8959d7f0f82bc192e90c63ec9a89a70bd5b5f8358fc7").Return(true, nil).Once()
		m.On("CreateAd", ctx, bike).Return(domain.AdID(10), nil).Once()
		m.On("CreateAd", ctx, scooter).Return(domain.AdID(11), nil).Once()
		m.On("CompleteRequest", ctx, adsdao.RequestKey{OwnerID: req.OwnerID, IdempotencyKey: "key-1"}, []domain.AdID{10, 11}).Return(nil).Once()
		m.On("Commit").Return(nil).Once()
		m.On("Rollback").Return(nil).Once()

		got, err := usecase.NewImportAdsFunc(m)(ctx, req)

		require.NoError(t, err)
		assert.Equal(t, []domain.AdID{10, 11}, got)
	})

	t.Run("error when the transaction cannot be opened", func(t *testing.T) {
		m := mock.New(t)
		dbErr := errors.New("db down")
		m.On("Begin", ctx).Return(dbErr).Once()

		_, err := usecase.NewImportAdsFunc(m)(ctx, req)

		require.Error(t, err)
		assert.ErrorIs(t, err, dbErr)
	})

	t.Run("rolls back and stops on first create error", func(t *testing.T) {
		m := mock.New(t)
		dbErr := errors.New("connection reset")
		m.On("Begin", ctx).Return(nil).Once()
		m.On("ClaimRequest", ctx, adsdao.RequestKey{OwnerID: req.OwnerID, IdempotencyKey: "key-1"}, "4c68d866a3e18f6e579c8959d7f0f82bc192e90c63ec9a89a70bd5b5f8358fc7").Return(true, nil).Once()
		m.On("CreateAd", ctx, bike).Return(domain.AdID(0), dbErr).Once()
		m.On("Rollback").Return(nil).Once()

		_, err := usecase.NewImportAdsFunc(m)(ctx, req)

		require.Error(t, err)
		assert.ErrorIs(t, err, dbErr)
	})

	t.Run("error when the commit fails", func(t *testing.T) {
		m := mock.New(t)
		dbErr := errors.New("commit failed")
		m.On("Begin", ctx).Return(nil).Once()
		m.On("ClaimRequest", ctx, adsdao.RequestKey{OwnerID: req.OwnerID, IdempotencyKey: "key-1"}, "4c68d866a3e18f6e579c8959d7f0f82bc192e90c63ec9a89a70bd5b5f8358fc7").Return(true, nil).Once()
		m.On("CreateAd", ctx, bike).Return(domain.AdID(10), nil).Once()
		m.On("CreateAd", ctx, scooter).Return(domain.AdID(11), nil).Once()
		m.On("CompleteRequest", ctx, adsdao.RequestKey{OwnerID: req.OwnerID, IdempotencyKey: "key-1"}, []domain.AdID{10, 11}).Return(nil).Once()
		m.On("Commit").Return(dbErr).Once()
		m.On("Rollback").Return(nil).Once()

		_, err := usecase.NewImportAdsFunc(m)(ctx, req)

		require.Error(t, err)
		assert.ErrorIs(t, err, dbErr)
	})
}

func TestImportAdsWithIdempotencyKey(t *testing.T) {
	ctx := context.Background()

	req := usecase.ImportAdsRequest{
		OwnerID:        "00000000-0000-0000-0000-000000000001",
		IdempotencyKey: "key-1",
		Ads: []usecase.ImportedAd{
			{Title: "bike", PriceCents: 5000, PhotoURL: "https://example.com/bike.jpg"},
			{Title: "scooter", PriceCents: 8000, PhotoURL: "https://example.com/scooter.jpg"},
		},
	}
	key := adsdao.RequestKey{OwnerID: "00000000-0000-0000-0000-000000000001", IdempotencyKey: "key-1"}
	// Hard-coded on purpose: a change of the hash would turn the retries of imports made before a deploy into ErrIdempotencyKeyReused.
	hash := "4c68d866a3e18f6e579c8959d7f0f82bc192e90c63ec9a89a70bd5b5f8358fc7"
	bike := adsdao.CreateAdRequest{Title: "bike", PriceCents: 5000, PhotoURL: "https://example.com/bike.jpg", OwnerID: "00000000-0000-0000-0000-000000000001"}
	scooter := adsdao.CreateAdRequest{Title: "scooter", PriceCents: 8000, PhotoURL: "https://example.com/scooter.jpg", OwnerID: "00000000-0000-0000-0000-000000000001"}

	t.Run("creates the ads and records them under the key", func(t *testing.T) {
		m := mock.New(t)
		m.On("Begin", ctx).Return(nil).Once()
		m.On("ClaimRequest", ctx, key, hash).Return(true, nil).Once()
		m.On("CreateAd", ctx, bike).Return(domain.AdID(10), nil).Once()
		m.On("CreateAd", ctx, scooter).Return(domain.AdID(11), nil).Once()
		m.On("CompleteRequest", ctx, key, []domain.AdID{10, 11}).Return(nil).Once()
		m.On("Commit").Return(nil).Once()
		m.On("Rollback").Return(nil).Once()

		got, err := usecase.NewImportAdsFunc(m)(ctx, req)

		require.NoError(t, err)
		assert.Equal(t, []domain.AdID{10, 11}, got)
	})

	t.Run("replays the first import without creating the ads again", func(t *testing.T) {
		m := mock.New(t)
		m.On("Begin", ctx).Return(nil).Once()
		m.On("ClaimRequest", ctx, key, hash).Return(false, nil).Once()
		m.On("GetRequestReceipt", ctx, key).Return(adsdao.RequestReceipt{RequestHash: hash, AdIDs: []domain.AdID{10, 11}}, nil).Once()
		m.On("Rollback").Return(nil).Once()

		got, err := usecase.NewImportAdsFunc(m)(ctx, req)

		require.NoError(t, err)
		assert.Equal(t, []domain.AdID{10, 11}, got)
	})

	t.Run("error when the key was used with other ads", func(t *testing.T) {
		m := mock.New(t)
		m.On("Begin", ctx).Return(nil).Once()
		m.On("ClaimRequest", ctx, key, hash).Return(false, nil).Once()
		m.On("GetRequestReceipt", ctx, key).Return(adsdao.RequestReceipt{RequestHash: "other-hash", AdIDs: []domain.AdID{7}}, nil).Once()
		m.On("Rollback").Return(nil).Once()

		_, err := usecase.NewImportAdsFunc(m)(ctx, req)

		require.Error(t, err)
		assert.ErrorIs(t, err, usecase.ErrIdempotencyKeyReused)
	})

	t.Run("error when the key cannot be reported", func(t *testing.T) {
		m := mock.New(t)
		dbErr := errors.New("connection reset")
		m.On("Begin", ctx).Return(nil).Once()
		m.On("ClaimRequest", ctx, key, hash).Return(false, dbErr).Once()
		m.On("Rollback").Return(nil).Once()

		_, err := usecase.NewImportAdsFunc(m)(ctx, req)

		require.Error(t, err)
		assert.ErrorIs(t, err, dbErr)
	})

	t.Run("error when the first import cannot be read", func(t *testing.T) {
		m := mock.New(t)
		dbErr := errors.New("connection reset")
		m.On("Begin", ctx).Return(nil).Once()
		m.On("ClaimRequest", ctx, key, hash).Return(false, nil).Once()
		m.On("GetRequestReceipt", ctx, key).Return(adsdao.RequestReceipt{}, dbErr).Once()
		m.On("Rollback").Return(nil).Once()

		_, err := usecase.NewImportAdsFunc(m)(ctx, req)

		require.Error(t, err)
		assert.ErrorIs(t, err, dbErr)
	})

	t.Run("rolls back when the ads cannot be recorded under the key", func(t *testing.T) {
		m := mock.New(t)
		dbErr := errors.New("connection reset")
		m.On("Begin", ctx).Return(nil).Once()
		m.On("ClaimRequest", ctx, key, hash).Return(true, nil).Once()
		m.On("CreateAd", ctx, bike).Return(domain.AdID(10), nil).Once()
		m.On("CreateAd", ctx, scooter).Return(domain.AdID(11), nil).Once()
		m.On("CompleteRequest", ctx, key, []domain.AdID{10, 11}).Return(dbErr).Once()
		m.On("Rollback").Return(nil).Once()

		_, err := usecase.NewImportAdsFunc(m)(ctx, req)

		require.Error(t, err)
		assert.ErrorIs(t, err, dbErr)
	})
}

func TestImportValidationBeforeTransaction(t *testing.T) {
	valid := usecase.ImportAdsRequest{OwnerID: "00000000-0000-0000-0000-000000000001", IdempotencyKey: "key", Ads: []usecase.ImportedAd{{Title: "bike", PriceCents: 1, PhotoURL: "https://example.com/b.jpg"}}}
	for _, name := range []string{"empty", "too many", "missing key", "space in key", "invalid owner", "invalid later ad"} {
		t.Run(name, func(t *testing.T) {
			req := valid
			req.Ads = append([]usecase.ImportedAd(nil), valid.Ads...)
			switch name {
			case "empty":
				req.Ads = nil
			case "too many":
				req.Ads = make([]usecase.ImportedAd, 101)
			case "missing key":
				req.IdempotencyKey = ""
			case "space in key":
				req.IdempotencyKey = "bad key"
			case "invalid owner":
				req.OwnerID = "alice"
			case "invalid later ad":
				req.Ads = append(req.Ads, usecase.ImportedAd{Title: "invalid", PriceCents: -1, PhotoURL: "https://example.com"})
			}
			m := mock.New(t)
			_, err := usecase.NewImportAdsFunc(m)(context.Background(), req)
			require.ErrorIs(t, err, domain.ErrInvalid)
		})
	}
}
