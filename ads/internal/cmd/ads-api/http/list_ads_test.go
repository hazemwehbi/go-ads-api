package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/domain"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/usecase"
)

func newListAdsServer(fn usecase.ListAdsFunc) *httptest.Server {
	mux := http.NewServeMux()
	mux.Handle("GET /ads", handlerListAds(fn))
	return httptest.NewServer(mux)
}

func TestHandlerListAds(t *testing.T) {
	createdAt := time.Date(2024, 3, 1, 12, 0, 0, 0, time.UTC)

	t.Run("returns 200 with ads JSON on success", func(t *testing.T) {
		ads := []domain.Ad{
			{ID: 1, Title: "bike", Price: 5000, PhotoURL: "https://example.com/bike.jpg", UserID: "00000000-0000-0000-0000-000000000001", CreatedAt: createdAt},
			{ID: 2, Title: "scooter", Price: 12000, PhotoURL: "https://example.com/scooter.jpg", UserID: "00000000-0000-0000-0000-000000000002", CreatedAt: createdAt},
		}
		srv := newListAdsServer(func(_ context.Context, _ usecase.ListAdsRequest) ([]domain.Ad, error) {
			return ads, nil
		})
		defer srv.Close()

		resp, err := http.Get(srv.URL + "/ads")
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

		var got listAdsResponse
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
		require.Len(t, got.Ads, 2)
		assert.Equal(t, int64(1), got.Ads[0].ID)
		assert.Equal(t, "bike", got.Ads[0].Title)
		assert.Equal(t, json.Number("50.00"), got.Ads[0].Price)
		assert.Equal(t, int64(5000), got.Ads[0].PriceCents)
		assert.Equal(t, int64(2), got.Ads[1].ID)
	})

	t.Run("returns 200 with empty ads array when no results", func(t *testing.T) {
		srv := newListAdsServer(func(_ context.Context, _ usecase.ListAdsRequest) ([]domain.Ad, error) {
			return nil, nil
		})
		defer srv.Close()

		resp, err := http.Get(srv.URL + "/ads")
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var got listAdsResponse
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
		assert.Empty(t, got.Ads)
	})

	t.Run("forwards query params to the usecase", func(t *testing.T) {
		var received usecase.ListAdsRequest
		srv := newListAdsServer(func(_ context.Context, req usecase.ListAdsRequest) ([]domain.Ad, error) {
			received = req
			return nil, nil
		})
		defer srv.Close()

		resp, err := http.Get(srv.URL + "/ads?title=bike&user_id=00000000-0000-0000-0000-000000000003&min_price_cents=1000&max_price_cents=50000")
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "bike", received.Title)
		assert.Equal(t, domain.UserID("00000000-0000-0000-0000-000000000003"), received.OwnerID)
		assert.Equal(t, int64(1000), received.MinPriceCents)
		assert.Equal(t, int64(50000), received.MaxPriceCents)
	})

	t.Run("returns 400 when user_id is not a valid UUID", func(t *testing.T) {
		srv := newListAdsServer(func(_ context.Context, _ usecase.ListAdsRequest) ([]domain.Ad, error) {
			t.Fatal("usecase should not be called")
			return nil, nil
		})
		defer srv.Close()

		resp, err := http.Get(srv.URL + "/ads?user_id=user-3")
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("converts price filters in euros to cents", func(t *testing.T) {
		var received usecase.ListAdsRequest
		srv := newListAdsServer(func(_ context.Context, req usecase.ListAdsRequest) ([]domain.Ad, error) {
			received = req
			return nil, nil
		})
		defer srv.Close()

		resp, err := http.Get(srv.URL + "/ads?min_price=10.5&max_price=500")
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, int64(1050), received.MinPriceCents)
		assert.Equal(t, int64(50000), received.MaxPriceCents)
	})

	t.Run("returns 400 when price filters are invalid", func(t *testing.T) {
		for _, query := range []string{
			"min_price=abc",
			"max_price=abc",
			"min_price=1.234",
			"max_price=-1",
			"min_price=10&min_price_cents=1000",
			"min_price=10&max_price_cents=5000",
			"min_price_cents=1000&max_price=50",
		} {
			t.Run(query, func(t *testing.T) {
				srv := newListAdsServer(func(_ context.Context, _ usecase.ListAdsRequest) ([]domain.Ad, error) {
					t.Fatal("usecase should not be called")
					return nil, nil
				})
				defer srv.Close()

				resp, err := http.Get(srv.URL + "/ads?" + query)
				require.NoError(t, err)
				defer func() { _ = resp.Body.Close() }()

				assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
			})
		}
	})

	t.Run("returns 400 when min_price_cents is not a number", func(t *testing.T) {
		srv := newListAdsServer(func(_ context.Context, _ usecase.ListAdsRequest) ([]domain.Ad, error) {
			t.Fatal("usecase should not be called")
			return nil, nil
		})
		defer srv.Close()

		resp, err := http.Get(srv.URL + "/ads?min_price_cents=not-a-number")
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("returns 400 when max_price_cents is not a number", func(t *testing.T) {
		srv := newListAdsServer(func(_ context.Context, _ usecase.ListAdsRequest) ([]domain.Ad, error) {
			t.Fatal("usecase should not be called")
			return nil, nil
		})
		defer srv.Close()

		resp, err := http.Get(srv.URL + "/ads?max_price_cents=abc")
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("returns 500 when the usecase errors", func(t *testing.T) {
		srv := newListAdsServer(func(_ context.Context, _ usecase.ListAdsRequest) ([]domain.Ad, error) {
			return nil, errors.New("db down")
		})
		defer srv.Close()

		resp, err := http.Get(srv.URL + "/ads")
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	})
}
