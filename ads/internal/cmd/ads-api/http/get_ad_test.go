package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	adsdao "github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/dao"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/domain"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/usecase"
)

func newGetAdServer(fn usecase.GetAdByIDFunc) *httptest.Server {
	mux := http.NewServeMux()
	mux.Handle("GET /ads/{adID}", handlerGetAd(fn))

	return httptest.NewServer(mux)
}

func TestHandlerGetAd(t *testing.T) {
	createdAt := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)

	t.Run("returns 200 with ad JSON on success", func(t *testing.T) {
		ad := domain.Ad{
			ID:        42,
			Title:     "mountain bike",
			Price:     15000,
			PhotoURL:  "https://example.com/bike.jpg",
			UserID:    "00000000-0000-0000-0000-000000000001",
			CreatedAt: createdAt,
		}
		srv := newGetAdServer(func(_ context.Context, id domain.AdID) (usecase.AdDetails, error) {
			return usecase.AdDetails{Ad: ad, Username: usernamePtr("alice")}, nil
		})
		defer srv.Close()

		resp, err := http.Get(srv.URL + "/ads/42")
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

		var got getAdResponse
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
		assert.Equal(t, int64(42), got.ID)
		assert.Equal(t, "mountain bike", got.Title)
		assert.Equal(t, json.Number("150.00"), got.Price)
		assert.Equal(t, int64(15000), got.PriceCents)
		assert.Equal(t, "https://example.com/bike.jpg", got.PhotoURL)
		assert.Equal(t, "00000000-0000-0000-0000-000000000001", got.UserID)
		assert.Equal(t, "alice", *got.Username)
		assert.Equal(t, createdAt, got.CreatedAt)
	})

	t.Run("returns 400 when adID is not a number", func(t *testing.T) {
		srv := newGetAdServer(func(_ context.Context, _ domain.AdID) (usecase.AdDetails, error) {
			t.Fatal("usecase should not be called")
			return usecase.AdDetails{}, nil
		})
		defer srv.Close()

		resp, err := http.Get(srv.URL + "/ads/not-a-number")
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("returns 404 when ad does not exist", func(t *testing.T) {
		srv := newGetAdServer(func(_ context.Context, _ domain.AdID) (usecase.AdDetails, error) {
			return usecase.AdDetails{}, adsdao.ErrNotFound
		})
		defer srv.Close()

		resp, err := http.Get(srv.URL + "/ads/99")
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("returns 500 on unexpected usecase error", func(t *testing.T) {
		srv := newGetAdServer(func(_ context.Context, _ domain.AdID) (usecase.AdDetails, error) {
			return usecase.AdDetails{}, errors.New("db down")
		})
		defer srv.Close()

		resp, err := http.Get(srv.URL + "/ads/1")
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	})

	t.Run("passes the correct ad ID to the usecase", func(t *testing.T) {
		var receivedID domain.AdID
		srv := newGetAdServer(func(_ context.Context, id domain.AdID) (usecase.AdDetails, error) {
			receivedID = id
			return usecase.AdDetails{Ad: domain.Ad{ID: id}}, nil
		})
		defer srv.Close()

		resp, err := http.Get(srv.URL + "/ads/7")
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, domain.AdID(7), receivedID)
	})
}

func usernamePtr(s string) *string { return &s }

func TestGetAdNullableContractAndDependencyErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		status int
	}{
		{"null", nil, 200}, {"bad upstream", usecase.ErrOwnerLookup, 502},
		{"opaque upstream", fmt.Errorf("wrapped: %w", usecase.ErrOwnerLookup), 502}, {"deadline", context.DeadlineExceeded, 504},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := NewRouter(Usecases{GetAd: func(context.Context, domain.AdID) (usecase.AdDetails, error) {
				return usecase.AdDetails{Ad: domain.Ad{ID: 1}}, tc.err
			}})
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest("GET", "/ads/1", nil))
			require.Equal(t, tc.status, w.Code)
			var body map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			if tc.err == nil {
				value, present := body["username"]
				require.True(t, present)
				require.Equal(t, "null", string(value))
			} else {
				require.NotEmpty(t, body["error"])
			}
		})
	}
}
