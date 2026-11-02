package usecase_test

import (
	"context"
	"errors"
	"github.com/stretchr/testify/mock"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	adsdao "github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/dao"
	mockdao "github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/dao/mock"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/domain"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/usecase"
)

func TestCreateAd(t *testing.T) {
	ctx := context.Background()
	owner := domain.UserID("00000000-0000-0000-0000-000000000001")
	req := usecase.CreateAdRequest{Title: "bike", PriceCents: 5000, PhotoURL: "https://example.com/bike.jpg", OwnerID: owner}
	daoReq := adsdao.CreateAdRequest{Title: "bike", PriceCents: 5000, PhotoURL: "https://example.com/bike.jpg", OwnerID: owner}

	// expectQuotaCheck sets the expectations shared by every case that reaches the quota check.
	expectQuotaCheck := func(m *mockdao.Mock, published int) {
		m.On("Begin", ctx).Return(nil)
		m.On("LockOwnerPublications", ctx, owner).Return(nil)
		m.On("CountRecentPublications", ctx, owner, usecase.PublicationWindow).Return(published, nil)
		m.On("Rollback").Return(nil)
	}

	t.Run("creates the ad and records the publication below the quota", func(t *testing.T) {
		m := mockdao.New(t)
		expectQuotaCheck(m, usecase.MaxPublicationsPerWindow-1)
		m.On("CreateAd", ctx, daoReq).Return(domain.AdID(42), nil)
		m.On("RecordPublication", ctx, owner, domain.AdID(42)).Return(nil)
		m.On("Commit").Return(nil)

		got, err := usecase.NewCreateAdFunc(m)(ctx, req)

		require.NoError(t, err)
		assert.Equal(t, domain.AdID(42), got)
	})

	t.Run("rejects the ad when the quota is reached", func(t *testing.T) {
		m := mockdao.New(t)
		expectQuotaCheck(m, usecase.MaxPublicationsPerWindow) // no CreateAd expectation: creating would fail the test

		_, err := usecase.NewCreateAdFunc(m)(ctx, req)

		require.Error(t, err)
		assert.ErrorIs(t, err, usecase.ErrQuotaExceeded)
	})

	t.Run("does not commit when the creation fails", func(t *testing.T) {
		m := mockdao.New(t)
		dbErr := errors.New("db down")
		expectQuotaCheck(m, 0)
		m.On("CreateAd", ctx, daoReq).Return(domain.AdID(0), dbErr)

		_, err := usecase.NewCreateAdFunc(m)(ctx, req)

		assert.ErrorIs(t, err, dbErr)
	})

	t.Run("does not commit when the publication cannot be recorded", func(t *testing.T) {
		m := mockdao.New(t)
		dbErr := errors.New("db down")
		expectQuotaCheck(m, 0)
		m.On("CreateAd", ctx, daoReq).Return(domain.AdID(42), nil)
		m.On("RecordPublication", ctx, owner, domain.AdID(42)).Return(dbErr)

		_, err := usecase.NewCreateAdFunc(m)(ctx, req)

		assert.ErrorIs(t, err, dbErr)
	})

	t.Run("returns error when the owner lock fails", func(t *testing.T) {
		m := mockdao.New(t)
		dbErr := errors.New("lock timeout")
		m.On("Begin", ctx).Return(nil)
		m.On("LockOwnerPublications", ctx, owner).Return(dbErr)
		m.On("Rollback").Return(nil)

		_, err := usecase.NewCreateAdFunc(m)(ctx, req)

		assert.ErrorIs(t, err, dbErr)
	})
}

func TestCreateTransactionFailures(t *testing.T) {
	ctx := context.Background()
	owner := domain.UserID("00000000-0000-0000-0000-000000000001")
	req := usecase.CreateAdRequest{OwnerID: owner, Title: "bike", PriceCents: 100, PhotoURL: "https://example.com/b.jpg"}
	for _, stage := range []string{"begin", "count", "commit"} {
		t.Run(stage, func(t *testing.T) {
			m := mockdao.New(t)
			failure := errors.New("injected " + stage)
			if stage == "begin" {
				m.On("Begin", ctx).Return(failure).Once()
			} else {
				m.On("Begin", ctx).Return(nil).Once()
				m.On("Rollback").Return(nil).Once()
				m.On("LockOwnerPublications", ctx, owner).Return(nil).Once()
				if stage == "count" {
					m.On("CountRecentPublications", ctx, owner, usecase.PublicationWindow).Return(0, failure).Once()
				} else {
					m.On("CountRecentPublications", ctx, owner, usecase.PublicationWindow).Return(0, nil).Once()
					m.On("CreateAd", ctx, adsdao.CreateAdRequest{OwnerID: owner, Title: "bike", PriceCents: 100, PhotoURL: req.PhotoURL}).Return(domain.AdID(1), nil).Once()
					m.On("RecordPublication", ctx, owner, domain.AdID(1)).Return(nil).Once()
					m.On("Commit").Return(failure).Once()
				}
			}
			id, err := usecase.NewCreateAdFunc(m)(ctx, req)
			require.ErrorIs(t, err, failure)
			require.Zero(t, id)
		})
	}
}

func TestCreateIdempotencyFailures(t *testing.T) {
	ctx := context.Background()
	req := usecase.CreateAdRequest{OwnerID: "00000000-0000-0000-0000-000000000001", Title: "bike", PhotoURL: "https://example.com/b", IdempotencyKey: "retry"}
	key := adsdao.RequestKey{OwnerID: req.OwnerID, IdempotencyKey: req.IdempotencyKey, Operation: "create"}
	for _, stage := range []string{"claim", "read", "complete", "commit"} {
		t.Run(stage, func(t *testing.T) {
			m := mockdao.New(t)
			failure := errors.New("injected failure")
			m.On("Begin", ctx).Return(nil).Once()
			m.On("Rollback").Return(nil).Once()
			switch stage {
			case "claim":
				m.On("ClaimRequest", ctx, key, mock.Anything).Return(false, failure).Once()
			case "read":
				m.On("ClaimRequest", ctx, key, mock.Anything).Return(false, nil).Once()
				m.On("GetRequestReceipt", ctx, key).Return(adsdao.RequestReceipt{}, failure).Once()
			default:
				m.On("ClaimRequest", ctx, key, mock.Anything).Return(true, nil).Once()
				m.On("LockOwnerPublications", ctx, req.OwnerID).Return(nil).Once()
				m.On("CountRecentPublications", ctx, req.OwnerID, usecase.PublicationWindow).Return(0, nil).Once()
				m.On("CreateAd", ctx, mock.Anything).Return(domain.AdID(42), nil).Once()
				m.On("RecordPublication", ctx, req.OwnerID, domain.AdID(42)).Return(nil).Once()
				if stage == "complete" {
					m.On("CompleteRequest", ctx, key, []domain.AdID{42}).Return(failure).Once()
				} else {
					m.On("CompleteRequest", ctx, key, []domain.AdID{42}).Return(nil).Once()
					m.On("Commit").Return(failure).Once()
				}
			}
			id, err := usecase.NewCreateAdFunc(m)(ctx, req)
			require.Zero(t, id)
			require.ErrorIs(t, err, failure)
		})
	}
	for _, key := range []string{"has space", strings.Repeat("x", 129), "\u00e9"} {
		t.Run(key, func(t *testing.T) {
			m := mockdao.New(t)
			invalid := req
			invalid.IdempotencyKey = key
			_, err := usecase.NewCreateAdFunc(m)(ctx, invalid)
			require.ErrorIs(t, err, domain.ErrInvalid)
		})
	}
}
