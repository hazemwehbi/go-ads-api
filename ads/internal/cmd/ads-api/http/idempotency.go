package http

import (
	"errors"
	"net/http"

	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/usecase"
)

const idempotencyKeyHeader = "Idempotency-Key"

func requestKey(r *http.Request, required bool) (string, error) {
	values, present := r.Header[http.CanonicalHeaderKey(idempotencyKeyHeader)]
	if !present && !required {
		return "", nil
	}
	if len(values) != 1 {
		return "", errors.New("supply one Idempotency-Key")
	}
	if err := usecase.ValidateRequestKey(values[0]); err != nil {
		return "", err
	}
	return values[0], nil
}
