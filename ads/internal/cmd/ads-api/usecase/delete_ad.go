package usecase

import (
	"context"
	"fmt"

	adsdao "github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/dao"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/domain"
)

type (
	// DeleteAdFunc deletes an ad owned by the calling user. It fails with dao.ErrNotFound when the ad does not exist or is owned by someone else.
	DeleteAdFunc func(ctx context.Context, req DeleteAdRequest) error

	// DeleteAdRequest describes the ad to delete. OwnerID is the authenticated user: a caller can never delete an ad owned by someone else.
	DeleteAdRequest struct {
		ID      domain.AdID
		OwnerID domain.UserID
	}

	deleteAd struct {
		adDAO adsdao.DAO
	}
)

// NewDeleteAdFunc returns a DeleteAdFunc backed by the given DAO.
func NewDeleteAdFunc(adDAO adsdao.DAO) DeleteAdFunc {
	return deleteAd{adDAO: adDAO}.deleteAd
}

func (uc deleteAd) deleteAd(ctx context.Context, req DeleteAdRequest) error {
	if req.ID <= 0 {
		return fmt.Errorf("%w: positive ad ID required", domain.ErrInvalid)
	}
	owner, err := domain.CanonicalOwner(req.OwnerID)
	if err != nil {
		return err
	}
	if err := uc.adDAO.DeleteAdByID(ctx, req.ID, owner); err != nil {
		return fmt.Errorf("unable to delete ad %s: %w", req.ID, err)
	}

	return nil
}
