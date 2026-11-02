package http

import (
	"context"
	"net/http"
	"time"

	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/usecase"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/common/middleware"
)

// Usecases groups the business functions the HTTP layer depends on.
type Usecases struct {
	ListAds   usecase.ListAdsFunc
	GetAd     usecase.GetAdByIDFunc
	CreateAd  usecase.CreateAdFunc
	DeleteAd  usecase.DeleteAdFunc
	ImportAds usecase.ImportAdsFunc
}

// NewRouter wires the routes of the API.
func NewRouter(usecases Usecases) http.Handler {
	mux := http.NewServeMux()

	mux.Handle("GET /ads", handlerListAds(usecases.ListAds))
	mux.Handle("GET /ads/{adID}", handlerGetAd(usecases.GetAd))
	mux.Handle("DELETE /ads/{adID}", middleware.RequireAuth(handlerDeleteAd(usecases.DeleteAd)))
	mux.Handle("POST /ads", middleware.RequireAuth(handlerCreateAd(usecases.CreateAd)))
	mux.Handle("POST /ads/import", middleware.RequireAuth(handlerImportAds(usecases.ImportAds)))

	mux.HandleFunc("GET /health", func(rw http.ResponseWriter, _ *http.Request) {
		rw.WriteHeader(http.StatusOK)
	})

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		mux.ServeHTTP(w, r.WithContext(ctx))
	})
}
