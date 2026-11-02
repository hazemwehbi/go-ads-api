package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	adsdao "github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/dao"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/usecase"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/common/httpresponse"
)

type getAdResponse struct {
	ID         int64       `json:"id"`
	Title      string      `json:"title"`
	Price      json.Number `json:"price"`
	PriceCents int64       `json:"price_cents"` // Deprecated
	PhotoURL   string      `json:"photo_url"`
	UserID     string      `json:"user_id"`
	Username   *string     `json:"username"`
	CreatedAt  time.Time   `json:"created_at"`
}

func adToGetAdResponse(details usecase.AdDetails) getAdResponse {
	a := details.Ad

	return getAdResponse{
		ID:         int64(a.ID),
		Title:      a.Title,
		Price:      centsToEuros(a.Price),
		PriceCents: a.Price,
		PhotoURL:   a.PhotoURL,
		UserID:     a.UserID.String(),
		Username:   details.Username,
		CreatedAt:  a.CreatedAt.UTC(),
	}
}

func handlerGetAd(getAd usecase.GetAdByIDFunc) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		adID, err := adIDFromURL(r)
		if err != nil {
			httpresponse.WriteError(rw, http.StatusBadRequest, err.Error())

			return
		}

		details, err := getAd(r.Context(), adID)
		if handleRequestContext(rw, r, err) {
			return
		}
		if err != nil {
			if errors.Is(err, usecase.ErrOwnerLookup) {
				httpresponse.WriteError(rw, 502, "invalid user service response")
				return
			}
			if errors.Is(err, adsdao.ErrNotFound) {
				httpresponse.WriteError(rw, http.StatusNotFound, fmt.Sprintf("ad %d not found", adID))
			} else {
				slog.Error("unable to get ad", "ad_id", adID, "error", err)
				httpresponse.WriteError(rw, http.StatusInternalServerError, "internal error")
			}

			return
		}

		rw.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(rw).Encode(adToGetAdResponse(details))
	}
}
