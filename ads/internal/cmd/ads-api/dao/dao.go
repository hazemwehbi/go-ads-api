package dao

import (
	"context"
	"time"

	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/domain"
)

// Store reads and writes ads. It is implemented both by the DAO, where every call is its own transaction, and by a Tx opened from it.
type Store interface {
	// CreateAd inserts an ad and returns its generated identifier.
	CreateAd(ctx context.Context, req CreateAdRequest) (domain.AdID, error)
	// GetAdByID returns a single ad, or ErrNotFound.
	GetAdByID(ctx context.Context, id domain.AdID) (domain.Ad, error)
	// SearchAds returns all ads matching the filters.
	SearchAds(ctx context.Context, req SearchAdsRequest) ([]domain.Ad, error)
	// DeleteAd removes a single ad, or returns ErrNotFound.
	DeleteAdByID(ctx context.Context, id domain.AdID, ownerID domain.UserID) error
}

// DAO stores and retrieves ads.
type DAO interface {
	Store
	// Begin opens a transaction. Every call made on the returned Tx is part of it, and the caller must end it with Commit or Rollback.
	Begin(ctx context.Context) (Tx, error)
}

// Tx is a Store scoped to an opened transaction.
type Tx interface {
	Store
	LockOwnerPublications(context.Context, domain.UserID) error
	CountRecentPublications(context.Context, domain.UserID, time.Duration) (int, error)
	RecordPublication(context.Context, domain.UserID, domain.AdID) error
	// ClaimRequest claims a request with its operation-scoped idempotency key. It returns false when the key is already recorded. A concurrent claim of the same key waits until the first transaction ends.
	ClaimRequest(ctx context.Context, key RequestKey, requestHash string) (bool, error)
	// GetRequestReceipt returns a recorded request receipt, or ErrNotFound.
	GetRequestReceipt(ctx context.Context, key RequestKey) (RequestReceipt, error)
	// CompleteRequest stores the ids of the ads created by a claimed request, or returns ErrNotFound.
	CompleteRequest(ctx context.Context, key RequestKey, adIDs []domain.AdID) error
	// Commit applies every change made through the transaction.
	Commit() error
	// Rollback discards every change made through the transaction. Calling it on an already committed transaction does nothing, so `defer func() { _ = tx.Rollback() }()` is a safe guard.
	Rollback() error
}

// SearchAdsRequest describes an ad search. Zero means no price bound unless MaxPriceSet is true.
type SearchAdsRequest struct {
	OwnerID       domain.UserID
	MinPriceCents int64
	MaxPriceCents int64
	MaxPriceSet   bool
	Title         string
	Limit         int
	BeforeID      domain.AdID
}

// RequestKey identifies a request by owner, operation and client key.
type RequestKey struct {
	Operation      string // Empty means bulk import for compatibility with existing callers.
	OwnerID        domain.UserID
	IdempotencyKey string
}

// RequestReceipt is the durable result of a create or import request.
type RequestReceipt struct {
	// RequestHash identifies the request content, to detect a key reused with other ads.
	RequestHash string
	AdIDs       []domain.AdID
}

// CreateAdRequest carries the data needed to create an ad.
type CreateAdRequest struct {
	Title      string
	PriceCents int64
	PhotoURL   string
	OwnerID    domain.UserID
}

const (
	// ErrNotFound is returned when no ad matches the requested identifier.
	ErrNotFound sentinelError = "ad not found"
)

type sentinelError string

// Error implements the error interface.
func (e sentinelError) Error() string { return string(e) }
