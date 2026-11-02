package http

import (
	"errors"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/common/httpresponse"
	"log/slog"
	"net/http"

	adsdao "github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/dao"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/domain"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/usecase"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/common/middleware"
)

// handlerDeleteAd serves DELETE /ads/{adID}. It answers 404 both when the ad does not exist and when it is owned by someone else, so the existence of other users' ads is not disclosed.
func handlerDeleteAd(deleteAd usecase.DeleteAdFunc) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		adID, err := adIDFromURL(r)
		if err != nil {
			httpresponse.WriteError(rw, http.StatusBadRequest, err.Error())

			return
		}

		err = deleteAd(r.Context(), usecase.DeleteAdRequest{
			ID:      adID,
			OwnerID: domain.UserID(middleware.UserIDFromRequest(r)),
		})
		if handleRequestContext(rw, r, err) {
			return
		}
		if err != nil {
			if errors.Is(err, adsdao.ErrNotFound) {
				httpresponse.WriteError(rw, http.StatusNotFound, "ad not found")
			} else {
				slog.Error("unable to delete ad", "ad_id", adID, "error", err)
				httpresponse.WriteError(rw, http.StatusInternalServerError, "internal error")
			}

			return
		}

		rw.WriteHeader(http.StatusNoContent)
	}
}
