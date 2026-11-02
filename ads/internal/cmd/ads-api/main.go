package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/dao/postgres"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/http"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/usecase"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/userclient"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/common/httpserver"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	if err := run(); err != nil {
		slog.Error("api stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx := context.Background()
	cfg := Load()

	startupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	adDAO, err := postgres.New(startupCtx, cfg.DatabaseURL)
	cancel()
	if err != nil {
		return err
	}
	defer func() {
		if err := adDAO.Close(); err != nil {
			slog.Error("unable to close the database", "error", err)
		}
	}()

	userClient := userclient.NewResilient(userclient.New(cfg.IdentityURL, cfg.IdentityTimeout), cfg.IdentityResilience)

	router := http.NewRouter(http.Usecases{
		CreateAd:  usecase.NewCreateAdFunc(adDAO),
		DeleteAd:  usecase.NewDeleteAdFunc(adDAO),
		GetAd:     usecase.NewGetAdByIDFunc(adDAO, userClient),
		ListAds:   usecase.NewListAdsFunc(adDAO),
		ImportAds: usecase.NewImportAdsFunc(adDAO),
	})

	return httpserver.Run(ctx, cfg.HTTPAddr, router)
}
