package http

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	adsdao "github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/dao"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/domain"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/usecase"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/common/middleware"
)

func newDeleteAdServer(fn usecase.DeleteAdFunc) *httptest.Server {
	mux := http.NewServeMux()
	mux.Handle("DELETE /ads/{adID}", middleware.RequireAuth(handlerDeleteAd(fn)))

	return httptest.NewServer(mux)
}

func deleteRequest(t *testing.T, url string, authHeader string) *http.Response {
	t.Helper()

	req, err := http.NewRequest(http.MethodDelete, url, nil)
	require.NoError(t, err)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)

	return resp
}

func TestHandlerDeleteAd(t *testing.T) {
	t.Run("returns 204 on success", func(t *testing.T) {
		srv := newDeleteAdServer(func(_ context.Context, _ usecase.DeleteAdRequest) error {
			return nil
		})
		defer srv.Close()

		resp := deleteRequest(t, srv.URL+"/ads/42", "00000000-0000-0000-0000-000000000001")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	})

	t.Run("passes the ad ID and the authenticated user to the usecase", func(t *testing.T) {
		var received usecase.DeleteAdRequest
		srv := newDeleteAdServer(func(_ context.Context, req usecase.DeleteAdRequest) error {
			received = req
			return nil
		})
		defer srv.Close()

		resp := deleteRequest(t, srv.URL+"/ads/42", "00000000-0000-0000-0000-000000000042")
		defer resp.Body.Close()

		require.Equal(t, http.StatusNoContent, resp.StatusCode)
		assert.Equal(t, domain.AdID(42), received.ID)
		assert.Equal(t, domain.UserID("00000000-0000-0000-0000-000000000042"), received.OwnerID)
	})

	t.Run("returns 401 when Authorization header is missing", func(t *testing.T) {
		srv := newDeleteAdServer(func(_ context.Context, _ usecase.DeleteAdRequest) error {
			t.Fatal("usecase should not be called")
			return nil
		})
		defer srv.Close()

		resp := deleteRequest(t, srv.URL+"/ads/42", "")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("returns 400 when adID is not a number", func(t *testing.T) {
		srv := newDeleteAdServer(func(_ context.Context, _ usecase.DeleteAdRequest) error {
			t.Fatal("usecase should not be called")
			return nil
		})
		defer srv.Close()

		resp := deleteRequest(t, srv.URL+"/ads/not-a-number", "00000000-0000-0000-0000-000000000001")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("returns 404 when the ad does not exist or is not owned by the user", func(t *testing.T) {
		srv := newDeleteAdServer(func(_ context.Context, _ usecase.DeleteAdRequest) error {
			return adsdao.ErrNotFound
		})
		defer srv.Close()

		resp := deleteRequest(t, srv.URL+"/ads/99", "00000000-0000-0000-0000-000000000001")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("returns 500 on unexpected usecase error", func(t *testing.T) {
		srv := newDeleteAdServer(func(_ context.Context, _ usecase.DeleteAdRequest) error {
			return errors.New("db down")
		})
		defer srv.Close()

		resp := deleteRequest(t, srv.URL+"/ads/1", "00000000-0000-0000-0000-000000000001")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	})
}
