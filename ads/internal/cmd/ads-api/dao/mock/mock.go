package mock

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	adsdao "github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/dao"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/domain"
)

// Mock is a testify-based implementation of dao.DAO.
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

// SearchAds returns the ads and the error set on the mock.
func (m *Mock) SearchAds(ctx context.Context, req adsdao.SearchAdsRequest) ([]domain.Ad, error) {
	args := m.Called(ctx, req)

	ads, _ := args.Get(0).([]domain.Ad)

	return ads, args.Error(1)
}

// GetAdByID returns the ad and the error set on the mock.
func (m *Mock) GetAdByID(ctx context.Context, id domain.AdID) (domain.Ad, error) {
	args := m.Called(ctx, id)

	ad, _ := args.Get(0).(domain.Ad)

	return ad, args.Error(1)
}

// CreateAd returns the ad and the error set on the mock.
func (m *Mock) CreateAd(ctx context.Context, req adsdao.CreateAdRequest) (domain.AdID, error) {
	args := m.Called(ctx, req)

	adID, _ := args.Get(0).(domain.AdID)

	return adID, args.Error(1)
}

// DeleteAd returns the error set on the mock.
func (m *Mock) DeleteAdByID(ctx context.Context, id domain.AdID, owner domain.UserID) error {
	return m.Called(ctx, id, owner).Error(0)
}

// ClaimRequest returns the claim result and the error set on the mock.
func (m *Mock) ClaimRequest(ctx context.Context, key adsdao.RequestKey, requestHash string) (bool, error) {
	args := m.Called(ctx, key, requestHash)

	return args.Bool(0), args.Error(1)
}

// GetRequestReceipt returns the receipt and the error set on the mock.
func (m *Mock) GetRequestReceipt(ctx context.Context, key adsdao.RequestKey) (adsdao.RequestReceipt, error) {
	args := m.Called(ctx, key)

	receipt, _ := args.Get(0).(adsdao.RequestReceipt)

	return receipt, args.Error(1)
}

// CompleteRequest returns the error set on the mock.
func (m *Mock) CompleteRequest(ctx context.Context, key adsdao.RequestKey, adIDs []domain.AdID) error {
	return m.Called(ctx, key, adIDs).Error(0)
}

// Begin returns the mock itself as the transaction, so expectations stay set on a single object, and the error set on the mock.
func (m *Mock) Begin(ctx context.Context) (adsdao.Tx, error) {
	args := m.Called(ctx)

	if err := args.Error(0); err != nil {
		return nil, err
	}

	return m, nil
}

// Commit returns the error set on the mock.
func (m *Mock) Commit() error {
	return m.Called().Error(0)
}

// Rollback returns the error set on the mock.
func (m *Mock) Rollback() error {
	return m.Called().Error(0)
}

func (m *Mock) LockOwnerPublications(ctx context.Context, owner domain.UserID) error {
	return m.Called(ctx, owner).Error(0)
}
func (m *Mock) CountRecentPublications(ctx context.Context, owner domain.UserID, window time.Duration) (int, error) {
	args := m.Called(ctx, owner, window)
	return args.Int(0), args.Error(1)
}
func (m *Mock) RecordPublication(ctx context.Context, owner domain.UserID, id domain.AdID) error {
	return m.Called(ctx, owner, id).Error(0)
}
