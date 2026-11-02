package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/domain"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/usecase"
)

func TestListRejectsMalformedQuery(t *testing.T) {
	for _, query := range []string{"title=%00", "title=%FF", "min_price=%ZZ", "title=bike&max_price=%", "%FF=value", "%00=value", "title=bike;limit=1"} {
		t.Run(query, func(t *testing.T) {
			h := handlerListAds(func(context.Context, usecase.ListAdsRequest) ([]domain.Ad, error) {
				t.Fatal("invalid query reached usecase")
				return nil, nil
			})
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest("GET", "/ads?"+query, nil))
			require.Equal(t, 400, w.Code)
			require.Equal(t, "application/json", w.Header().Get("Content-Type"))
			require.NotContains(t, w.Body.String(), "SQLSTATE")
		})
	}
	t.Run("valid unicode and literal wildcard", func(t *testing.T) {
		h := handlerListAds(func(_ context.Context, req usecase.ListAdsRequest) ([]domain.Ad, error) {
			require.Equal(t, "caf\u00e9%_", req.Title)
			return nil, nil
		})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/ads?title=caf%C3%A9%25_", nil))
		require.Equal(t, 200, w.Code)
	})
}

func TestImportIdempotencyHeader(t *testing.T) {
	for _, tc := range []struct {
		name   string
		keys   []string
		status int
	}{
		{"missing", nil, 400}, {"empty", []string{""}, 400}, {"repeated", []string{"a", "b"}, 400},
		{"repeated same", []string{"a", "a"}, 400}, {"invalid", []string{"has space"}, 400}, {"valid", []string{"batch-1"}, 201},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			router := NewRouter(Usecases{ImportAds: func(_ context.Context, req usecase.ImportAdsRequest) ([]domain.AdID, error) {
				called = true
				require.Equal(t, "batch-1", req.IdempotencyKey)
				return []domain.AdID{1}, nil
			}})
			r := httptest.NewRequest("POST", "/ads/import", strings.NewReader(`{"ads":[{"title":"bike","price":1,"photo_url":"https://example.com/a"}]}`))
			r.Header.Set("Authorization", "aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa")
			r.Header.Set("Content-Type", "application/json")
			for _, key := range tc.keys {
				r.Header.Add("Idempotency-Key", key)
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, r)
			require.Equal(t, tc.status, w.Code)
			require.Equal(t, tc.status == 201, called)
		})
	}
}

type responseObserver struct {
	*httptest.ResponseRecorder
	written bool
}

func (w *responseObserver) WriteHeader(code int) {
	w.written = true
	w.ResponseRecorder.WriteHeader(code)
}
func (w *responseObserver) Write(b []byte) (int, error) {
	w.written = true
	return w.ResponseRecorder.Write(b)
}

func TestAdRoutesRequestContext(t *testing.T) {
	for _, tc := range []struct {
		name    string
		failure error
		state   string
		status  int
	}{
		{"wrapped deadline", fmt.Errorf("storage: %w", context.DeadlineExceeded), "", 504},
		{"wrapped cancellation", fmt.Errorf("storage: %w", context.Canceled), "", 0},
		{"driver deadline", errors.New("private driver error"), "deadline", 504},
		{"driver cancellation", errors.New("private driver error"), "cancel", 0},
		{"late success deadline", nil, "deadline", 504},
		{"late success cancellation", nil, "cancel", 0},
		{"ordinary failure", errors.New("private driver error"), "", 500},
	} {
		for _, route := range []struct{ method, path, body string }{
			{"GET", "/ads", ""}, {"GET", "/ads/1", ""}, {"DELETE", "/ads/1", ""},
			{"POST", "/ads", `{"title":"bike","price":1,"photo_url":"https://example.com/a"}`},
			{"POST", "/ads/import", `{"ads":[{"title":"bike","price":1,"photo_url":"https://example.com/a"}]}`},
		} {
			t.Run(tc.name+"/"+route.method+route.path, func(t *testing.T) {
				ctx := context.Background()
				if tc.state == "deadline" {
					var cancel context.CancelFunc
					ctx, cancel = context.WithDeadline(ctx, time.Now().Add(-time.Second))
					defer cancel()
				}
				if tc.state == "cancel" {
					var cancel context.CancelFunc
					ctx, cancel = context.WithCancel(ctx)
					cancel()
				}
				router := NewRouter(Usecases{
					ListAds:  func(context.Context, usecase.ListAdsRequest) ([]domain.Ad, error) { return nil, tc.failure },
					GetAd:    func(context.Context, domain.AdID) (usecase.AdDetails, error) { return usecase.AdDetails{}, tc.failure },
					DeleteAd: func(context.Context, usecase.DeleteAdRequest) error { return tc.failure },
					CreateAd: func(context.Context, usecase.CreateAdRequest) (domain.AdID, error) { return 1, tc.failure },
					ImportAds: func(context.Context, usecase.ImportAdsRequest) ([]domain.AdID, error) {
						return []domain.AdID{1}, tc.failure
					},
				})
				r := httptest.NewRequest(route.method, route.path, strings.NewReader(route.body)).WithContext(ctx)
				r.Header.Set("Authorization", "aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa")
				r.Header.Set("Content-Type", "application/json")
				r.Header.Set("Idempotency-Key", "retry")
				w := &responseObserver{ResponseRecorder: httptest.NewRecorder()}
				router.ServeHTTP(w, r)
				if tc.status == 0 {
					require.False(t, w.written)
					require.Empty(t, w.Body.String())
					return
				}
				require.Equal(t, tc.status, w.Code)
				require.Equal(t, "application/json", w.Header().Get("Content-Type"))
				require.NotContains(t, w.Body.String(), "private")
				if tc.status == 504 {
					require.JSONEq(t, `{"error":"request deadline exceeded"}`, w.Body.String())
				}
			})
		}
	}
}

// Body reads can fail with driver/transport errors after cancellation as well.
type interruptedBody struct{ err error }

func (b interruptedBody) Read([]byte) (int, error) { return 0, b.err }
func (b interruptedBody) Close() error             { return nil }
func TestWriteBodyCancellation(t *testing.T) {
	for _, path := range []string{"/ads", "/ads/import"} {
		r := httptest.NewRequest(http.MethodPost, path, nil)
		ctx, cancel := context.WithCancel(r.Context())
		cancel()
		r = r.WithContext(ctx)
		r.Body = interruptedBody{errors.New("private transport error")}
		r.Header.Set("Authorization", "aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa")
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Idempotency-Key", "retry")
		w := &responseObserver{ResponseRecorder: httptest.NewRecorder()}
		NewRouter(Usecases{}).ServeHTTP(w, r)
		require.False(t, w.written, path)
	}
}
