package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	adsdao "github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/dao"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/domain"
)

// ErrIdempotencyKeyReused indicates a request key reused with different content.
var ErrIdempotencyKeyReused = errors.New("idempotency key already used with different content")

// replayRequest returns the ads created by the first request made with the key, as long as it carried the same ads.
func replayRequest(ctx context.Context, tx adsdao.Tx, key adsdao.RequestKey, hash string) ([]domain.AdID, error) {
	receipt, err := tx.GetRequestReceipt(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("unable to replay request: %w", err)
	}

	if receipt.RequestHash != hash {
		return nil, fmt.Errorf("unable to replay request: %w", ErrIdempotencyKeyReused)
	}

	return receipt.AdIDs, nil
}

// hashAdPayload identifies normalized ad content without changing existing receipt fingerprints. Prices are already in cents.
func hashAdPayload(ads []ImportedAd) (string, error) {
	payload, err := json.Marshal(ads)
	if err != nil {
		return "", fmt.Errorf("unable to hash ad payload: %w", err)
	}

	sum := sha256.Sum256(payload)

	return hex.EncodeToString(sum[:]), nil
}

// ValidateRequestKey checks the client key shared by create and import requests.
func ValidateRequestKey(key string) error {
	if len(key) < 1 || len(key) > 128 {
		return fmt.Errorf("%w: Idempotency-Key must have 1..128 visible ASCII characters", domain.ErrInvalid)
	}
	for _, r := range key {
		if r < 33 || r > 126 {
			return fmt.Errorf("%w: invalid Idempotency-Key", domain.ErrInvalid)
		}
	}
	return nil
}
