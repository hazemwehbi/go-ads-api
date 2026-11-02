package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	adsdao "github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/dao"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/domain"
)

const (
	// MaxPublicationsPerWindow is the number of ads a user can publish through POST /ads within any rolling PublicationWindow.
	MaxPublicationsPerWindow = 5
	// PublicationWindow is the rolling window of the publication quota.
	PublicationWindow = 24 * time.Hour
)

// ErrQuotaExceeded is returned when the user already published MaxPublicationsPerWindow ads within the rolling PublicationWindow.
var ErrQuotaExceeded = errors.New("ad publication quota exceeded")

type (
	// CreateAdFunc creates an ad owned by the calling user.
	CreateAdFunc func(ctx context.Context, req CreateAdRequest) (domain.AdID, error)

	// CreateAdRequest describes the ad to create. OwnerID is supplied by the identity context, not by the JSON body.
	CreateAdRequest struct {
		IdempotencyKey string // Optional; reuse for retries of the same logical publication.
		Title          string
		PriceCents     int64
		PhotoURL       string
		OwnerID        domain.UserID
	}

	createAd struct {
		adDAO adsdao.DAO
	}
)

// NewCreateAdFunc returns a CreateAdFunc backed by the given DAO.
func NewCreateAdFunc(adDAO adsdao.DAO) CreateAdFunc {
	return createAd{adDAO: adDAO}.createAd
}

func (uc createAd) createAd(ctx context.Context, req CreateAdRequest) (domain.AdID, error) {
	owner, err := domain.CanonicalOwner(req.OwnerID)
	if err != nil {
		return 0, err
	}
	req.OwnerID = owner
	req.Title = strings.TrimSpace(req.Title)
	if err = domain.ValidateAd(req.Title, req.PriceCents, req.PhotoURL); err != nil {
		return 0, err
	}
	if req.IdempotencyKey != "" {
		if err := ValidateRequestKey(req.IdempotencyKey); err != nil {
			return 0, err
		}
	}
	tx, err := uc.adDAO.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("unable to create ad: %w", err)
	}

	// Discards every change unless Commit succeeded; a no-op after a successful Commit.
	defer func() { _ = tx.Rollback() }()

	// Claim or replay before checking quota. A replay is not another publication.
	key := adsdao.RequestKey{OwnerID: req.OwnerID, IdempotencyKey: req.IdempotencyKey, Operation: "create"}
	if req.IdempotencyKey != "" {
		hash, err := hashAdPayload([]ImportedAd{{Title: req.Title, PriceCents: req.PriceCents, PhotoURL: req.PhotoURL}})
		if err != nil {
			return 0, err
		}
		claimed, err := tx.ClaimRequest(ctx, key, hash)
		if err != nil {
			return 0, fmt.Errorf("unable to claim creation: %w", err)
		}
		if !claimed {
			ids, err := replayRequest(ctx, tx, key, hash)
			if err != nil {
				return 0, err
			}
			if len(ids) != 1 || ids[0] <= 0 {
				return 0, errors.New("invalid creation receipt")
			}
			return ids[0], nil
		}
	}

	// Held until the transaction ends: a concurrent request of the same owner waits here, so it counts this publication too.
	if err := tx.LockOwnerPublications(ctx, req.OwnerID); err != nil {
		return 0, fmt.Errorf("unable to create ad: %w", err)
	}

	published, err := tx.CountRecentPublications(ctx, req.OwnerID, PublicationWindow)
	if err != nil {
		return 0, fmt.Errorf("unable to create ad: %w", err)
	}

	if published >= MaxPublicationsPerWindow {
		return 0, fmt.Errorf("unable to create ad: %w", ErrQuotaExceeded)
	}

	adID, err := tx.CreateAd(ctx, adsdao.CreateAdRequest{
		Title:      req.Title,
		PriceCents: req.PriceCents,
		PhotoURL:   req.PhotoURL,
		OwnerID:    req.OwnerID,
	})
	if err != nil {
		return 0, fmt.Errorf("unable to create ad: %w", err)
	}

	if err := tx.RecordPublication(ctx, req.OwnerID, adID); err != nil {
		return 0, fmt.Errorf("unable to create ad: %w", err)
	}

	if req.IdempotencyKey != "" {
		if err := tx.CompleteRequest(ctx, key, []domain.AdID{adID}); err != nil {
			return 0, fmt.Errorf("unable to complete creation: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("unable to create ad: %w", err)
	}

	return adID, nil
}
