package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/domain"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/usecase"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/common/httpresponse"
)

type listAdsItem struct {
	ID         int64       `json:"id"`
	Title      string      `json:"title"`
	Price      json.Number `json:"price"`
	PriceCents int64       `json:"price_cents"` // Deprecated: retained for existing clients
	PhotoURL   string      `json:"photo_url"`
	UserID     string      `json:"user_id"`
	CreatedAt  time.Time   `json:"created_at"`
}

type listAdsResponse struct {
	Ads          []listAdsItem `json:"ads"`
	NextBeforeID *int64        `json:"next_before_id"`
}

func adToListAdsItem(a domain.Ad) listAdsItem {
	return listAdsItem{
		ID:         int64(a.ID),
		Title:      a.Title,
		Price:      centsToEuros(a.Price),
		PriceCents: a.Price,
		PhotoURL:   a.PhotoURL,
		UserID:     a.UserID.String(),
		CreatedAt:  a.CreatedAt.UTC(),
	}
}

func handlerListAds(listAds usecase.ListAdsFunc) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		q, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil {
			httpresponse.WriteError(rw, http.StatusBadRequest, "invalid query encoding")
			return
		}
		for key, values := range q {
			if !utf8.ValidString(key) || strings.ContainsRune(key, 0) {
				httpresponse.WriteError(rw, http.StatusBadRequest, "query must be valid UTF-8 without NUL")
				return
			}
			for _, value := range values {
				if !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
					httpresponse.WriteError(rw, http.StatusBadRequest, "query must be valid UTF-8 without NUL")
					return
				}
			}
		}

		req := usecase.ListAdsRequest{
			Title: q.Get("title"),
			Limit: 50,
		}

		if raw := q.Get("user_id"); raw != "" {
			owner, err := domain.CanonicalOwner(domain.UserID(raw))
			if err != nil {
				httpresponse.WriteError(rw, http.StatusBadRequest, fmt.Sprintf("user_id %q is not a valid UUID", raw))
				return
			}

			req.OwnerID = owner
		}

		if raw := q.Get("limit"); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil || n < 1 || n > 100 {
				httpresponse.WriteError(rw, 400, "limit must be 1..100")
				return
			}
			req.Limit = n
		}
		if raw := q.Get("before_id"); raw != "" {
			n, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || n <= 0 {
				httpresponse.WriteError(rw, 400, "invalid before_id")
				return
			}
			req.BeforeID = domain.AdID(n)
		}
		minPriceCents, maxPriceCents, err := priceFiltersInCents(q)
		if err != nil {
			httpresponse.WriteError(rw, http.StatusBadRequest, err.Error())
			return
		}

		req.MinPriceCents = minPriceCents
		req.MaxPriceCents = maxPriceCents
		req.MaxPriceSet = q.Has("max_price") || q.Has("max_price_cents")

		ads, err := listAds(r.Context(), req)
		if handleRequestContext(rw, r, err) {
			return
		}
		if err != nil {
			slog.Error("unable to list ads", "user_id", req.OwnerID, "error", err)
			httpresponse.WriteError(rw, http.StatusInternalServerError, "internal error")
			return
		}

		var next *int64
		if len(ads) > req.Limit {
			n := int64(ads[req.Limit-1].ID)
			next = &n
			ads = ads[:req.Limit]
		}
		items := make([]listAdsItem, 0, len(ads))
		for _, a := range ads {
			items = append(items, adToListAdsItem(a))
		}

		rw.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(rw).Encode(listAdsResponse{Ads: items, NextBeforeID: next})
	}
}

// Price filters validator: either min_price and max_price in euros, or min_price_cents and max_price_cents, never both
func priceFiltersInCents(q url.Values) (int64, int64, error) {
	for _, key := range []string{"min_price", "max_price", "min_price_cents", "max_price_cents"} {
		if q.Has(key) && q.Get(key) == "" {
			return 0, 0, fmt.Errorf("%s cannot be empty", key)
		}
	}
	inEuros := q.Get("min_price") != "" || q.Get("max_price") != ""
	inCents := q.Get("min_price_cents") != "" || q.Get("max_price_cents") != ""
	if inEuros && inCents {
		return 0, 0, errors.New("send either min_price and max_price or min_price_cents and max_price_cents, not both")
	}

	minCents, err := priceFilter(q, "min_price", "min_price_cents")
	if err != nil {
		return 0, 0, err
	}

	maxCents, err := priceFilter(q, "max_price", "max_price_cents")
	if err != nil {
		return 0, 0, err
	}

	if (q.Has("max_price") || q.Has("max_price_cents")) && minCents > maxCents {
		return 0, 0, errors.New("min price exceeds max price")
	}
	return minCents, maxCents, nil
}

func priceFilter(q url.Values, eurosParam, centsParam string) (int64, error) {
	// Parse string euros into cents
	if raw := q.Get(eurosParam); raw != "" {
		euros := json.Number(raw)

		cents, err := eurosToCents(euros)
		if err != nil {
			return 0, fmt.Errorf("%s: %w", eurosParam, err)
		}

		if cents < 0 || cents > domain.MaxPriceCents {
			return 0, fmt.Errorf("invalid %s", centsParam)
		}
		return cents, nil
	}

	// Parse string cents
	if raw := q.Get(centsParam); raw != "" {
		cents, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("%s %q is not a number", centsParam, raw)
		}

		if cents < 0 || cents > domain.MaxPriceCents {
			return 0, fmt.Errorf("invalid %s", centsParam)
		}
		return cents, nil
	}

	return 0, nil
}
