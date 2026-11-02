package http

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/domain"
)

func FuzzPriceRoundTrip(f *testing.F) {
	for _, seed := range []string{"0", "0.29", "120.50", "1e2", "1.001", "1e9999999", "null"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		cents, err := eurosToCents(json.Number(raw))
		if err != nil {
			return
		}
		if cents < 0 || cents > domain.MaxPriceCents {
			t.Fatalf("accepted out-of-range price: %q", raw)
		}
		back, err := eurosToCents(centsToEuros(cents))
		if err != nil || back != cents {
			t.Fatalf("round trip failed: %q", raw)
		}
	})
}
func FuzzDecodeBody(f *testing.F) {
	for _, seed := range []string{`{"title":"bike","price":1,"photo_url":"https://example.com/a"}`, `{"price":1,"PRICE":2}`, `{}`, `null`, `{"title":"\u0000"}`, `{"price":1}{}`} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		if len(raw) > 1<<20 {
			t.Skip()
		}
		req := httptest.NewRequest("POST", "/ads", strings.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		var body createAdBody
		err := decodeBody(req, &body)
		if err == nil && (!json.Valid([]byte(raw)) || !utf8.ValidString(raw)) {
			t.Fatal("accepted invalid JSON or UTF-8")
		}
	})
}
