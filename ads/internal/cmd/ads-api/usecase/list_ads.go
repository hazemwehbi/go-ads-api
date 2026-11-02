package usecase

import (
	"context"
	"fmt"

	adsdao "github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/dao"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/domain"
)

type (
	// ListAdsFunc returns all ads matching the filters.
	ListAdsFunc func(ctx context.Context, req ListAdsRequest) ([]domain.Ad, error)

	// ListAdsRequest describes which ads to return.
	ListAdsRequest struct {
		OwnerID       domain.UserID
		MinPriceCents int64
		MaxPriceCents int64
		MaxPriceSet   bool
		Title         string
		Limit         int
		BeforeID      domain.AdID
	}

	listAds struct {
		adDAO adsdao.DAO
	}
)

// NewListAdsFunc returns a new ListAdsFunc.
func NewListAdsFunc(adDAO adsdao.DAO) ListAdsFunc {
	return listAds{adDAO: adDAO}.listAds
}

func (uc listAds) listAds(ctx context.Context, req ListAdsRequest) ([]domain.Ad, error) {
	ads, err := uc.adDAO.SearchAds(ctx, adsdao.SearchAdsRequest{
		OwnerID:       req.OwnerID,
		MinPriceCents: req.MinPriceCents,
		MaxPriceCents: req.MaxPriceCents,
		MaxPriceSet:   req.MaxPriceSet,
		Title:         req.Title,
		Limit:         req.Limit,
		BeforeID:      req.BeforeID,
	})
	if err != nil {
		return nil, fmt.Errorf("unable to search ads: %w", err)
	}

	return ads, nil
}
