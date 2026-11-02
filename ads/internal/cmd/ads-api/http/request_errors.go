package http

import (
	"context"
	"errors"
	"net/http"

	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/common/httpresponse"
)

// handleRequestContext stops response handling after cancellation or a deadline.
// Drivers may return their own error when the request context has expired.
func handleRequestContext(w http.ResponseWriter, r *http.Request, err error) bool {
	if requestErr := r.Context().Err(); requestErr != nil {
		err = requestErr
	}
	if errors.Is(err, context.Canceled) {
		return true
	}
	if errors.Is(err, context.DeadlineExceeded) {
		httpresponse.WriteError(w, http.StatusGatewayTimeout, "request deadline exceeded")
		return true
	}
	return false
}
