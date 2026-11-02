package usecase

import (
	"context"
	"fmt"
	"strings"

	adsdao "github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/dao"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/domain"
)

type (
	// ImportAdsFunc creates every ad of the request for the calling user, or none of them.
	// A retry with the same idempotency key returns the ads created the first time instead of creating them again.
	ImportAdsFunc func(ctx context.Context, req ImportAdsRequest) ([]domain.AdID, error)

	// ImportAdsRequest describes a bulk import. OwnerID is the authenticated user and owns every imported ad.
	// IdempotencyKey is required and must be reused after an ambiguous response.
	ImportAdsRequest struct {
		OwnerID        domain.UserID
		IdempotencyKey string
		Ads            []ImportedAd
	}

	// ImportedAd is a single ad of a bulk import.
	ImportedAd struct {
		Title      string
		PriceCents int64
		PhotoURL   string
	}

	importAds struct {
		adDAO adsdao.DAO
	}
)

// NewImportAdsFunc returns an ImportAdsFunc backed by the given DAO.
func NewImportAdsFunc(adDAO adsdao.DAO) ImportAdsFunc {
	return importAds{adDAO: adDAO}.importAds
}

func (uc importAds) importAds(ctx context.Context, req ImportAdsRequest) ([]domain.AdID, error) {
	owner, err := domain.CanonicalOwner(req.OwnerID)
	if err != nil {
		return nil, err
	}
	req.OwnerID = owner
	if len(req.Ads) < 1 || len(req.Ads) > 100 {
		return nil, fmt.Errorf("%w: import must contain 1..100 ads", domain.ErrInvalid)
	}
	if err = ValidateRequestKey(req.IdempotencyKey); err != nil {
		return nil, err
	}
	req.Ads = append([]ImportedAd(nil), req.Ads...)
	for i := range req.Ads {
		req.Ads[i].Title = strings.TrimSpace(req.Ads[i].Title)
		item := req.Ads[i]
		if err = domain.ValidateAd(item.Title, item.PriceCents, item.PhotoURL); err != nil {
			return nil, fmt.Errorf("item %d: %w", i, err)
		}
	}
	tx, err := uc.adDAO.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to import ads: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	key := adsdao.RequestKey{OwnerID: req.OwnerID, IdempotencyKey: req.IdempotencyKey}

	hash, err := hashAdPayload(req.Ads)
	if err != nil {
		return nil, fmt.Errorf("unable to import ads: %w", err)
	}

	reported, err := tx.ClaimRequest(ctx, key, hash)
	if err != nil {
		return nil, fmt.Errorf("unable to import ads: %w", err)
	}

	// The key was already used: replay the first import instead of creating the ads again.
	if !reported {
		return replayRequest(ctx, tx, key, hash)
	}

	ids := make([]domain.AdID, 0, len(req.Ads))

	for i, ad := range req.Ads {
		id, err := tx.CreateAd(ctx, adsdao.CreateAdRequest{
			Title:      ad.Title,
			PriceCents: ad.PriceCents,
			PhotoURL:   ad.PhotoURL,
			OwnerID:    req.OwnerID,
		})
		if err != nil {
			return nil, fmt.Errorf("unable to import ad %d: %w", i, err)
		}

		ids = append(ids, id)
	}

	if err := tx.CompleteRequest(ctx, key, ids); err != nil {
		return nil, fmt.Errorf("unable to import ads: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("unable to import ads: %w", err)
	}

	return ids, nil
}
