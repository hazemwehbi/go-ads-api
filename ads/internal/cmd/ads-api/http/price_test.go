package http

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestMoney(t *testing.T) {
	for raw, want := range map[string]int64{"0": 0, "0.1": 10, "19.99": 1999, "1073741824.09": 107374182409, "1e2": 10000, "10000000000": 1000000000000} {
		got, err := eurosToCents(json.Number(raw))
		require.NoError(t, err, raw)
		require.Equal(t, want, got)
		back, err := eurosToCents(centsToEuros(got))
		require.NoError(t, err)
		require.Equal(t, got, back)
	}
	for _, raw := range []string{"-1", "12.345", "10000000000.01", "1e9999999", "NaN", "null", `"1.2"`, "true", "1/2", "01"} {
		_, err := eurosToCents(json.Number(raw))
		require.Error(t, err, raw)
	}
	require.Equal(t, json.Number("123.45"), centsToEuros(12345))
}
