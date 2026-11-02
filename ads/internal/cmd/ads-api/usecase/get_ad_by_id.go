package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	adsdao "github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/dao"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/domain"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/userlookup"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/common/model"
	usersclient "github.mpi-internal.com/leboncoin/backend-hiring-assessment/users/pkg"
)

// ErrOwnerLookup marks owner data failures that prevent returning ad details.
// HTTP maps this application error without depending on a lookup implementation.
var ErrOwnerLookup = errors.New("owner lookup failed")

type (
	// GetAdByIDFunc returns a single ad with its owner's username. It fails with dao.ErrNotFound when no ad matches the identifier.
	GetAdByIDFunc func(ctx context.Context, id domain.AdID) (AdDetails, error)

	// AdDetails is an ad enriched with its owner's username. Username is nil when the user service cannot provide it.
	AdDetails struct {
		Ad       domain.Ad
		Username *string
	}

	// UserClient is the part of the user service GetAdByID needs.
	UserClient interface {
		GetUserByID(ctx context.Context, userID model.UserID) (usersclient.User, error)
	}

	getAdByID struct {
		adDAO      adsdao.DAO
		userClient UserClient
	}
)

// NewGetAdByIDFunc returns a GetAdByIDFunc backed by the given DAO and user
func NewGetAdByIDFunc(adDAO adsdao.DAO, userClient UserClient) GetAdByIDFunc {
	return getAdByID{adDAO: adDAO, userClient: userClient}.getAdByID
}

func (uc getAdByID) getAdByID(ctx context.Context, id domain.AdID) (AdDetails, error) {
	if err := ctx.Err(); err != nil {
		return AdDetails{}, err
	}
	found, err := uc.adDAO.GetAdByID(ctx, id)
	if err != nil {
		return AdDetails{}, fmt.Errorf("unable to get ad %s: %w", id, err)
	}

	// A missing adapter is a wiring error, not a transient dependency outage.
	if uc.userClient == nil {
		return AdDetails{}, fmt.Errorf("%w: %w", ErrOwnerLookup, &userlookup.Failure{Reason: "unexpected"})
	}

	// The username is secondary: the ad is still returned when the user service fails
	user, err := uc.userClient.GetUserByID(ctx, found.UserID)
	if err != nil {
		if ctx.Err() != nil {
			return AdDetails{}, ctx.Err()
		}
		if err == context.Canceled || err == context.DeadlineExceeded {
			return AdDetails{}, err
		}
		slog.Warn("unable to get ad owner username", "reason", userlookup.Reason(err), "ad_id", id, "user_id", found.UserID, "error", err)

		// Availability is an application decision, not a transport policy.
		switch userlookup.Reason(err) {
		case "timeout", "unavailable", "not_found", "circuit_open", "capacity":
			return AdDetails{Ad: found}, nil
		case "invalid_response", "unexpected":
			return AdDetails{}, fmt.Errorf("%w: %w", ErrOwnerLookup, err)
		default:
			return AdDetails{}, fmt.Errorf("owner enrichment failed: %w", err)
		}
	}
	if ctx.Err() != nil {
		return AdDetails{}, ctx.Err()
	}
	return AdDetails{Ad: found, Username: &user.Nickname}, nil
}
