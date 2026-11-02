package http

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/domain"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/usecase"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/common/httpresponse"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/common/middleware"
)

type createAdBody struct {
	Title      string          `json:"title"`
	Price      json.RawMessage `json:"price,omitempty"`
	PriceCents json.RawMessage `json:"price_cents,omitempty"` // Deprecated: kept until web and mobile clients send price
	PhotoURL   string          `json:"photo_url"`
}

// Price body validator
func (b createAdBody) priceInCents() (int64, error) {
	if (len(b.Price) == 0) == (len(b.PriceCents) == 0) {
		return 0, errors.New("supply exactly one of price or price_cents")
	}
	if len(b.Price) > 0 {
		return eurosToCents(json.Number(b.Price))
	}
	value, err := strconv.ParseInt(string(b.PriceCents), 10, 64)
	if err != nil || value < 0 || value > domain.MaxPriceCents {
		return 0, errors.New("invalid price_cents")
	}
	return value, nil
}

type createAdResponse struct {
	ID int64 `json:"id"`
}

// handlerCreateAd serves POST /ads.
func handlerCreateAd(createAd usecase.CreateAdFunc) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		key, err := requestKey(r, false)
		if err != nil {
			httpresponse.WriteError(rw, http.StatusBadRequest, err.Error())
			return
		}
		var body createAdBody
		if err := decodeBody(r, &body); err != nil {
			if handleRequestContext(rw, r, err) {
				return
			}
			httpresponse.WriteError(rw, bodyErrorStatus(err), err.Error())
			return
		}

		priceCents, err := body.priceInCents()
		if err != nil {
			httpresponse.WriteError(rw, http.StatusBadRequest, err.Error())
			return
		}

		ownerID := middleware.UserIDFromRequest(r)

		adID, err := createAd(r.Context(), usecase.CreateAdRequest{
			IdempotencyKey: key,
			Title:          body.Title,
			PriceCents:     priceCents,
			PhotoURL:       body.PhotoURL,
			OwnerID:        ownerID,
		})
		if handleRequestContext(rw, r, err) {
			return
		}
		if err != nil {
			if errors.Is(err, usecase.ErrIdempotencyKeyReused) {
				httpresponse.WriteError(rw, http.StatusConflict, "Idempotency-Key already used with different content")
				return
			}
			if errors.Is(err, usecase.ErrQuotaExceeded) {
				httpresponse.WriteError(rw, http.StatusTooManyRequests, "at most 5 publications per rolling 24 hours")
				return
			}
			if errors.Is(err, domain.ErrInvalid) {
				httpresponse.WriteError(rw, 400, err.Error())
				return
			}
			slog.Error("unable to create ad", "user_id", ownerID, "error", err)
			httpresponse.WriteError(rw, http.StatusInternalServerError, "internal error")
			return
		}

		rw.Header().Set("Content-Type", "application/json")
		rw.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(rw).Encode(createAdResponse{ID: int64(adID)})
	}
}
