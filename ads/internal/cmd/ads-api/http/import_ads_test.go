package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/domain"

	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/usecase"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/common/middleware"
)

func newImportAdsServer(fn usecase.ImportAdsFunc) *httptest.Server {
	mux := http.NewServeMux()
	mux.Handle("POST /ads/import", middleware.RequireAuth(handlerImportAds(fn)))
	return httptest.NewServer(mux)
}

func failImportAds(t *testing.T) usecase.ImportAdsFunc {
	t.Helper()

	return func(_ context.Context, _ usecase.ImportAdsRequest) ([]domain.AdID, error) {
		t.Fatal("usecase should not be called")
		return nil, nil
	}
}

func postImport(t *testing.T, url string, body any, idempotencyKey string) *http.Response {
	t.Helper()
	b, err := json.Marshal(body)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "00000000-0000-0000-0000-000000000001")
	req.Header.Set("Idempotency-Key", idempotencyKey)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

func TestHandlerImportAds(t *testing.T) {
	validBody := json.RawMessage(`{"ads":[{"title":"bike","price":50,"photo_url":"https://example.com/bike.jpg"}]}`)
	t.Run("passes key owner and normalized price", func(t *testing.T) {
		srv := newImportAdsServer(func(_ context.Context, req usecase.ImportAdsRequest) ([]domain.AdID, error) {
			assert.Equal(t, "key-1", req.IdempotencyKey)
			assert.Equal(t, domain.UserID("00000000-0000-0000-0000-000000000001"), req.OwnerID)
			require.Len(t, req.Ads, 1)
			assert.Equal(t, int64(5000), req.Ads[0].PriceCents)
			return []domain.AdID{10}, nil
		})
		defer srv.Close()
		resp := postImport(t, srv.URL+"/ads/import", validBody, "key-1")
		defer func() { require.NoError(t, resp.Body.Close()) }()
		require.Equal(t, 201, resp.StatusCode)
		var got importAdsResponse
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
		assert.Equal(t, []int64{10}, got.IDs)
	})
	for _, key := range []string{"", "bad key", string(bytes.Repeat([]byte("x"), 129))} {
		t.Run("invalid key "+key, func(t *testing.T) {
			srv := newImportAdsServer(failImportAds(t))
			defer srv.Close()
			resp := postImport(t, srv.URL+"/ads/import", validBody, key)
			defer func() { require.NoError(t, resp.Body.Close()) }()
			assert.Equal(t, 400, resp.StatusCode)
		})
	}
	for _, tc := range []struct {
		name   string
		err    error
		status int
	}{
		{"conflict", fmt.Errorf("wrapped: %w", usecase.ErrIdempotencyKeyReused), 409},
		{"invalid batch", domain.ErrInvalid, 400}, {"database", errors.New("private db details"), 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := newImportAdsServer(func(context.Context, usecase.ImportAdsRequest) ([]domain.AdID, error) { return nil, tc.err })
			defer srv.Close()
			resp := postImport(t, srv.URL+"/ads/import", validBody, "key-1")
			defer func() { require.NoError(t, resp.Body.Close()) }()
			assert.Equal(t, tc.status, resp.StatusCode)
		})
	}
	t.Run("invalid later price", func(t *testing.T) {
		srv := newImportAdsServer(failImportAds(t))
		defer srv.Close()
		resp := postImport(t, srv.URL+"/ads/import", json.RawMessage(`{"ads":[{"price":1},{"price":1.234}]}`), "key-1")
		defer func() { require.NoError(t, resp.Body.Close()) }()
		assert.Equal(t, 400, resp.StatusCode)
	})
}
