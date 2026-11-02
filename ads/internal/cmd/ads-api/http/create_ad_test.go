package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/domain"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/usecase"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/common/middleware"
)

func newCreateAdServer(fn usecase.CreateAdFunc) *httptest.Server {
	mux := http.NewServeMux()
	mux.Handle("POST /ads", middleware.RequireAuth(handlerCreateAd(fn)))
	return httptest.NewServer(mux)
}

func postJSON(t *testing.T, url string, body any, authHeader string) *http.Response {
	t.Helper()
	b, err := json.Marshal(body)
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

func TestHandlerCreateAd(t *testing.T) {
	validBody := createAdBody{
		Title:      "mountain bike",
		PriceCents: json.RawMessage("15000"),
		PhotoURL:   "https://example.com/bike.jpg",
	}

	t.Run("returns 201 with ID on success", func(t *testing.T) {
		srv := newCreateAdServer(func(_ context.Context, req usecase.CreateAdRequest) (domain.AdID, error) {
			return domain.AdID(7), nil
		})
		defer srv.Close()

		resp := postJSON(t, srv.URL+"/ads", validBody, "00000000-0000-0000-0000-000000000001")
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusCreated, resp.StatusCode)
		assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

		var got createAdResponse
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
		assert.Equal(t, int64(7), got.ID)
	})

	t.Run("passes authenticated user ID as owner to the usecase", func(t *testing.T) {
		var received usecase.CreateAdRequest
		srv := newCreateAdServer(func(_ context.Context, req usecase.CreateAdRequest) (domain.AdID, error) {
			received = req
			return domain.AdID(1), nil
		})
		defer srv.Close()

		resp := postJSON(t, srv.URL+"/ads", validBody, "00000000-0000-0000-0000-000000000042")
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusCreated, resp.StatusCode)
		assert.Equal(t, domain.UserID("00000000-0000-0000-0000-000000000042"), received.OwnerID)
		assert.Equal(t, "mountain bike", received.Title)
		assert.Equal(t, int64(15000), received.PriceCents)
		assert.Equal(t, "https://example.com/bike.jpg", received.PhotoURL)
	})

	t.Run("converts price in euros to cents", func(t *testing.T) {
		var received usecase.CreateAdRequest
		srv := newCreateAdServer(func(_ context.Context, req usecase.CreateAdRequest) (domain.AdID, error) {
			received = req
			return domain.AdID(1), nil
		})
		defer srv.Close()

		price := json.RawMessage("150.5")
		body := createAdBody{Title: "mountain bike", Price: price, PhotoURL: "https://example.com/bike.jpg"}
		resp := postJSON(t, srv.URL+"/ads", body, "00000000-0000-0000-0000-000000000001")
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusCreated, resp.StatusCode)
		assert.Equal(t, int64(15050), received.PriceCents)
	})

	t.Run("accepts price_cents 0", func(t *testing.T) {
		var received usecase.CreateAdRequest
		srv := newCreateAdServer(func(_ context.Context, req usecase.CreateAdRequest) (domain.AdID, error) {
			received = req
			return domain.AdID(1), nil
		})
		defer srv.Close()

		body := createAdBody{Title: "free chair", PriceCents: json.RawMessage("0"), PhotoURL: "https://example.com/chair.jpg"}
		resp := postJSON(t, srv.URL+"/ads", body, "00000000-0000-0000-0000-000000000001")
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusCreated, resp.StatusCode)
		assert.Equal(t, int64(0), received.PriceCents)
	})

	t.Run("returns 400 when price is invalid", func(t *testing.T) {
		for name, raw := range map[string]string{
			"more than 2 decimals":            `{"title":"bike","price":12.345,"photo_url":"https://example.com/bike.jpg"}`,
			"negative":                        `{"title":"bike","price":-1,"photo_url":"https://example.com/bike.jpg"}`,
			"price and price_cents missing":   `{"title":"bike","photo_url":"https://example.com/bike.jpg"}`,
			"price and price_cents both sent": `{"title":"bike","price":150,"price_cents":15000,"photo_url":"https://example.com/bike.jpg"}`,
		} {
			t.Run(name, func(t *testing.T) {
				srv := newCreateAdServer(func(_ context.Context, _ usecase.CreateAdRequest) (domain.AdID, error) {
					t.Fatal("usecase should not be called")
					return 0, nil
				})
				defer srv.Close()

				resp := postJSON(t, srv.URL+"/ads", json.RawMessage(raw), "00000000-0000-0000-0000-000000000001")
				defer func() { _ = resp.Body.Close() }()

				assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
			})
		}
	})

	t.Run("returns 401 when Authorization header is missing", func(t *testing.T) {
		srv := newCreateAdServer(func(_ context.Context, _ usecase.CreateAdRequest) (domain.AdID, error) {
			t.Fatal("usecase should not be called")
			return 0, nil
		})
		defer srv.Close()

		resp := postJSON(t, srv.URL+"/ads", validBody, "")
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("returns 400 when body is invalid JSON", func(t *testing.T) {
		srv := newCreateAdServer(func(_ context.Context, _ usecase.CreateAdRequest) (domain.AdID, error) {
			t.Fatal("usecase should not be called")
			return 0, nil
		})
		defer srv.Close()

		req, err := http.NewRequest(http.MethodPost, srv.URL+"/ads", bytes.NewBufferString("not json"))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "00000000-0000-0000-0000-000000000001")

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("returns 500 when the usecase errors", func(t *testing.T) {
		srv := newCreateAdServer(func(_ context.Context, _ usecase.CreateAdRequest) (domain.AdID, error) {
			return 0, errors.New("db down")
		})
		defer srv.Close()

		resp := postJSON(t, srv.URL+"/ads", validBody, "00000000-0000-0000-0000-000000000001")
		defer func() { _ = resp.Body.Close() }()

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	})
}

func TestCreateIdempotencyHeader(t *testing.T) {
	for _, tc := range []struct {
		name    string
		keys    []string
		failure error
		status  int
	}{
		{"omitted", nil, nil, 201}, {"valid", []string{"retry-1"}, nil, 201},
		{"empty", []string{""}, nil, 400}, {"duplicate", []string{"a", "b"}, nil, 400},
		{"invalid", []string{"has space"}, nil, 400},
		{"conflict", []string{"retry-1"}, usecase.ErrIdempotencyKeyReused, 409},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			h := middleware.RequireAuth(handlerCreateAd(func(_ context.Context, req usecase.CreateAdRequest) (domain.AdID, error) {
				called = true
				expected := ""
				if len(tc.keys) > 0 {
					expected = tc.keys[0]
				}
				require.Equal(t, expected, req.IdempotencyKey)
				return 7, tc.failure
			}))
			r := httptest.NewRequest("POST", "/ads", bytes.NewBufferString(`{"title":"bike","price":10,"photo_url":"https://example.com/b"}`))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Authorization", "00000000-0000-0000-0000-000000000001")
			if tc.keys != nil {
				r.Header["Idempotency-Key"] = tc.keys
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			require.Equal(t, tc.status, w.Code)
			require.Equal(t, tc.status != 400, called)
			require.Equal(t, "application/json", w.Header().Get("Content-Type"))
		})
	}
}
