//go:build integration

package postgres

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	adshttp "github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/http"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/usecase"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/userclient"
)

func TestIntegrationHTTPPriceCompatibility(t *testing.T) {
	d, _ := integrationDAO(t)
	upstream := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(upstream.Close)
	lookup := userclient.NewResilient(userclient.New(upstream.URL, time.Second), userclient.DefaultOptions())
	server := httptest.NewServer(adshttp.NewRouter(adshttp.Usecases{
		CreateAd: usecase.NewCreateAdFunc(d), ListAds: usecase.NewListAdsFunc(d), GetAd: usecase.NewGetAdByIDFunc(d, lookup),
	}))
	t.Cleanup(server.Close)
	client := server.Client()
	client.Timeout = 5 * time.Second
	call := func(method, path, body string, status int) json.RawMessage {
		t.Helper()
		req, err := http.NewRequest(method, server.URL+path, bytes.NewBufferString(body))
		require.NoError(t, err)
		req.Header.Set("Authorization", string(testOwner))
		req.Header.Set("Content-Type", "application/json")
		if status == 400 && method == "POST" {
			req.Header.Set("Idempotency-Key", "invalid-price")
		}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		var raw json.RawMessage
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&raw))
		require.Equal(t, status, resp.StatusCode, string(raw))
		return raw
	}
	type priceItem struct {
		ID    int64       `json:"id"`
		Price json.Number `json:"price"`
		Cents int64       `json:"price_cents"`
	}
	checkPrice := func(item priceItem, cents int64) {
		t.Helper()
		require.Equal(t, cents, item.Cents)
		actual, ok := new(big.Rat).SetString(item.Price.String())
		require.True(t, ok)
		require.Zero(t, actual.Cmp(big.NewRat(cents, 100)), "API euros must match exact stored cents")
	}
	ids := []int64{}
	for _, tc := range []struct {
		field string
		cents int64
	}{
		{`"price":120.50`, 12050}, {`"price_cents":12050`, 12050}, {`"price":0.29`, 29},
	} {
		payload := `{"title":"price fixture","photo_url":"https://example.com/p.jpg",` + tc.field + `}`
		var created struct {
			ID int64 `json:"id"`
		}
		require.NoError(t, json.Unmarshal(call("POST", "/ads", payload, 201), &created))
		require.Positive(t, created.ID)
		ids = append(ids, created.ID)
		var stored int64
		require.NoError(t, d.db.QueryRow("SELECT price_cents FROM ads WHERE id=$1", created.ID).Scan(&stored))
		require.Equal(t, tc.cents, stored)
		var details priceItem
		require.NoError(t, json.Unmarshal(call("GET", fmt.Sprintf("/ads/%d", created.ID), "", 200), &details))
		require.Equal(t, created.ID, details.ID)
		checkPrice(details, tc.cents)
	}
	for _, fields := range []string{`"price":120.50,"price_cents":12050`, `"price":1.001`} {
		raw := call("POST", "/ads", `{"title":"invalid","photo_url":"https://example.com/p.jpg",`+fields+`}`, 400)
		var failure map[string]string
		require.NoError(t, json.Unmarshal(raw, &failure))
		require.NotEmpty(t, failure["error"])
	}
	for _, table := range []string{"ads", "ad_publications"} {
		var n int
		require.NoError(t, d.db.QueryRow("SELECT count(*) FROM "+table).Scan(&n))
		require.Equal(t, 3, n, "invalid prices must not mutate "+table)
	}
	var receipts int
	require.NoError(t, d.db.QueryRow("SELECT count(*) FROM ad_imports").Scan(&receipts))
	require.Zero(t, receipts)
	euro := call("GET", "/ads?min_price=120.50&max_price=120.50", "", 200)
	legacy := call("GET", "/ads?min_price_cents=12050&max_price_cents=12050", "", 200)
	require.JSONEq(t, string(euro), string(legacy))
	var listed struct {
		Ads []priceItem `json:"ads"`
	}
	require.NoError(t, json.Unmarshal(euro, &listed))
	require.Len(t, listed.Ads, 2)
	actualIDs := []int64{}
	for _, item := range listed.Ads {
		actualIDs = append(actualIDs, item.ID)
		checkPrice(item, 12050)
	}
	require.ElementsMatch(t, ids[:2], actualIDs)
	require.NoError(t, json.Unmarshal(call("GET", "/ads?min_price=0.29&max_price=0.29", "", 200), &listed))
	require.Len(t, listed.Ads, 1)
	require.Equal(t, ids[2], listed.Ads[0].ID)
	checkPrice(listed.Ads[0], 29)
	call("GET", "/ads?min_price=1&max_price_cents=12050", "", 400)
}
