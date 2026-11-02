//go:build integration

package postgres

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	adshttp "github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/http"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/usecase"
)

func TestIntegrationHTTPRejectsAmbiguousInput(t *testing.T) {
	d, _ := integrationDAO(t)
	server := httptest.NewServer(adshttp.NewRouter(adshttp.Usecases{ListAds: usecase.NewListAdsFunc(d), ImportAds: usecase.NewImportAdsFunc(d)}))
	defer server.Close()
	client := server.Client()
	for _, query := range []string{"title=%00", "title=%FF", "min_price=%ZZ"} {
		resp, err := client.Get(server.URL + "/ads?" + query)
		require.NoError(t, err)
		require.Equal(t, 400, resp.StatusCode, query)
		resp.Body.Close()
	}
	for _, keys := range [][]string{{"first", "second"}, {"same", "same"}} {
		r, err := http.NewRequest("POST", server.URL+"/ads/import", strings.NewReader(euroBatch))
		require.NoError(t, err)
		r.Header.Set("Authorization", string(testOwner))
		r.Header.Set("Content-Type", "application/json")
		for _, key := range keys {
			r.Header.Add("Idempotency-Key", key)
		}
		resp, err := client.Do(r)
		require.NoError(t, err)
		require.Equal(t, 400, resp.StatusCode)
		resp.Body.Close()
		checkImportCounts(t, d, 0, 0)
	}
	// Rejected headers have not claimed either key; a corrected request can succeed.
	ids := importResponse(t, client, server.URL, euroBatch, "first", 201)
	require.Len(t, ids, 2)
	checkImportCounts(t, d, 2, 1)
	require.Equal(t, ids, importResponse(t, client, server.URL, euroBatch, "first", 201))
	checkImportCounts(t, d, 2, 1)
}
