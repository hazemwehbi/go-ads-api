package http

import (
	"context"
	"github.com/stretchr/testify/require"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/domain"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/usecase"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/common/middleware"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCreateRequestRejection(t *testing.T) {
	for _, tc := range []struct {
		name, body, auth, media string
		want                    int
	}{
		{"duplicate", `{"price":1,"PRICE":2}`, "valid", "application/json", 400},
		{"unknown field", `{"price":1,"owner":"another"}`, "valid", "application/json", 400},
		{"trailing object", `{"price":1}{}`, "valid", "application/json", 400},
		{"array", `[]`, "valid", "application/json", 400},
		{"null", `null`, "valid", "application/json", 400},
		{"missing price", `{}`, "valid", "application/json", 400},
		{"null price", `{"price":null}`, "valid", "application/json", 400},
		{"quoted price", `{"price":"1"}`, "valid", "application/json", 400},
		{"both zero", `{"price":0,"price_cents":0}`, "valid", "application/json", 400},
		{"null legacy", `{"price":0,"price_cents":null}`, "valid", "application/json", 400},
		{"fractional cents", `{"price_cents":1.2}`, "valid", "application/json", 400},
		{"huge exponent", `{"price":1e99999999}`, "valid", "application/json", 400},
		{"body size", strings.Repeat(" ", 1<<20) + "{}", "valid", "application/json", 413},
		{"utf8", "{\"title\":\"\xff\"}", "valid", "application/json", 400},
		{"depth", `{"price":` + strings.Repeat("[", 20) + "0" + strings.Repeat("]", 20) + "}", "valid", "application/json", 400},
		{"media", `{}`, "valid", "text/plain", 400},
		{"missing auth", `{}`, "", "application/json", 401},
		{"invalid auth", `{}`, "alice", "application/json", 401},
		{"nil uuid", `{}`, "00000000-0000-0000-0000-000000000000", "application/json", 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			handler := middleware.RequireAuth(handlerCreateAd(func(context.Context, usecase.CreateAdRequest) (domain.AdID, error) {
				t.Fatal("must reject before usecase")
				return 0, nil
			}))
			req := httptest.NewRequest("POST", "/ads", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", tc.media)
			auth := tc.auth
			if auth == "valid" {
				auth = "aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa"
			}
			req.Header.Set("Authorization", auth)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			require.Equal(t, tc.want, w.Code)
		})
	}
}

func TestCreateQuotaHTTP(t *testing.T) {
	h := middleware.RequireAuth(handlerCreateAd(func(context.Context, usecase.CreateAdRequest) (domain.AdID, error) {
		return 0, usecase.ErrQuotaExceeded
	}))
	req := httptest.NewRequest("POST", "/ads", strings.NewReader(`{"title":"bike","price":1,"photo_url":"https://example.com/p.jpg"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	require.Equal(t, http.StatusTooManyRequests, w.Code)
}

func TestListPagination(t *testing.T) {
	for _, query := range []string{"limit=0", "limit=101", "limit=abc", "before_id=0", "before_id=-1", "min_price=1&max_price=0", "max_price_cents=-1", "max_price=", "max_price_cents="} {
		h := handlerListAds(func(context.Context, usecase.ListAdsRequest) ([]domain.Ad, error) {
			t.Fatal("invalid query reached usecase")
			return nil, nil
		})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/ads?"+query, nil))
		require.Equal(t, 400, w.Code, query)
	}
	h := handlerListAds(func(_ context.Context, req usecase.ListAdsRequest) ([]domain.Ad, error) {
		require.True(t, req.MaxPriceSet)
		require.Zero(t, req.MaxPriceCents)
		require.Equal(t, domain.AdID(10), req.BeforeID)
		return []domain.Ad{{ID: 9}, {ID: 8}, {ID: 7}}, nil
	})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/ads?limit=2&before_id=10&max_price=0", nil))
	require.Equal(t, 200, w.Code)
	require.Contains(t, w.Body.String(), `"next_before_id":8`)
	require.NotContains(t, w.Body.String(), `"id":7`)
}

func TestListCanonicalOwner(t *testing.T) {
	h := handlerListAds(func(_ context.Context, req usecase.ListAdsRequest) ([]domain.Ad, error) {
		require.Equal(t, domain.UserID("aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa"), req.OwnerID)
		return nil, nil
	})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/ads?user_id=urn:uuid:AAAAAAAA-AAAA-4AAA-AAAA-AAAAAAAAAAAA", nil))
	require.Equal(t, 200, w.Code)
}
