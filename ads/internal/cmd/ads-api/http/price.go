package http

import (
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/domain"
)

func centsToEuros(cents int64) json.Number {
	return json.Number(fmt.Sprintf("%d.%02d", cents/100, cents%100))
}

// Parse the decimal token exactly; floats are confined to the external API contract.
func eurosToCents(value json.Number) (int64, error) {
	raw := string(value)
	invalid := fmt.Errorf("%w: price must be a nonnegative decimal with at most two fractional digits", domain.ErrInvalid)
	if len(raw) < 1 || len(raw) > 40 || !json.Valid([]byte(raw)) {
		return 0, invalid
	}
	if i := strings.IndexAny(raw, "eE"); i >= 0 {
		e, err := strconv.Atoi(raw[i+1:])
		if err != nil || e < -18 || e > 18 {
			return 0, invalid
		}
	}
	rat, ok := new(big.Rat).SetString(raw)
	if !ok || rat.Sign() < 0 {
		return 0, invalid
	}
	rat.Mul(rat, big.NewRat(100, 1))
	if !rat.IsInt() || !rat.Num().IsInt64() || rat.Num().Int64() > domain.MaxPriceCents {
		return 0, invalid
	}
	return rat.Num().Int64(), nil
}
