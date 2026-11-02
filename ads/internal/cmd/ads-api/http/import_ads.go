package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/domain"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/usecase"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/common/httpresponse"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/common/middleware"
)

type importAdsBody struct {
	Ads []createAdBody `json:"ads"`
}

type importAdsResponse struct {
	IDs []int64 `json:"ids"`
}

// Import ads handler serves POST /ads/import.
func handlerImportAds(importAds usecase.ImportAdsFunc) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		idempotencyKey, err := requestKey(r, true)
		if err != nil {
			httpresponse.WriteError(rw, 400, err.Error())
			return
		}
		var body importAdsBody
		if err := decodeBody(r, &body); err != nil {
			if handleRequestContext(rw, r, err) {
				return
			}
			httpresponse.WriteError(rw, bodyErrorStatus(err), err.Error())
			return
		}

		ownerID := domain.UserID(middleware.UserIDFromRequest(r))
		ads := make([]usecase.ImportedAd, 0, len(body.Ads))

		for i, item := range body.Ads {
			priceCents, err := item.priceInCents()
			if err != nil {
				httpresponse.WriteError(rw, http.StatusBadRequest, fmt.Sprintf("ad %d: %s", i, err))
				return
			}

			ads = append(ads, usecase.ImportedAd{
				Title:      item.Title,
				PriceCents: priceCents,
				PhotoURL:   item.PhotoURL,
			})
		}

		adIDs, err := importAds(r.Context(), usecase.ImportAdsRequest{
			OwnerID:        ownerID,
			IdempotencyKey: idempotencyKey,
			Ads:            ads,
		})
		if handleRequestContext(rw, r, err) {
			return
		}
		if errors.Is(err, usecase.ErrIdempotencyKeyReused) {
			httpresponse.WriteError(rw, http.StatusConflict, fmt.Sprintf("%s already used with other ads", idempotencyKeyHeader))
			return
		}

		if err != nil {
			if errors.Is(err, domain.ErrInvalid) {
				httpresponse.WriteError(rw, 400, err.Error())
				return
			}
			slog.Error("unable to import ads", "user_id", ownerID, "idempotency_key", idempotencyKey, "ads_count", len(ads), "error", err)
			httpresponse.WriteError(rw, http.StatusInternalServerError, "internal error")
			return
		}

		ids := make([]int64, 0, len(adIDs))
		for _, id := range adIDs {
			ids = append(ids, int64(id))
		}

		rw.Header().Set("Content-Type", "application/json")
		rw.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(rw).Encode(importAdsResponse{IDs: ids})
	}
}
