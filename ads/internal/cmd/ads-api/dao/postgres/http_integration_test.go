//go:build integration

package postgres

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	adshttp "github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/http"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/usecase"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// Uses the real router, middleware, usecases and SQL in an isolated test schema.
func TestIntegrationHTTPCreateRetryAndQuota(t *testing.T) {
	d, _ := integrationDAO(t)
	server := httptest.NewServer(adshttp.NewRouter(adshttp.Usecases{CreateAd: usecase.NewCreateAdFunc(d), ImportAds: usecase.NewImportAdsFunc(d), DeleteAd: usecase.NewDeleteAdFunc(d)}))
	t.Cleanup(server.Close)
	client := server.Client()
	client.Timeout = 10 * time.Second
	call := func(body, key, path string) (int, map[string]json.RawMessage) {
		t.Helper()
		req, err := http.NewRequest("POST", server.URL+path, bytes.NewBufferString(body))
		require.NoError(t, err)
		req.Header.Set("Authorization", string(testOwner))
		req.Header.Set("Content-Type", "application/json")
		if key != "" {
			req.Header.Set("Idempotency-Key", key)
		}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer func() { require.NoError(t, resp.Body.Close()) }()
		require.Equal(t, "application/json", resp.Header.Get("Content-Type"))
		var result map[string]json.RawMessage
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
		return resp.StatusCode, result
	}
	body := `{"title":"bike","price":19.99,"photo_url":"https://example.com/bike.jpg"}`
	status, first := call(body, "publication-1", "/ads")
	require.Equal(t, 201, status)
	var id int64
	require.NoError(t, json.Unmarshal(first["id"], &id))
	require.Positive(t, id)
	status, replay := call(`{"title":"  bike  ","price_cents":1999,"photo_url":"https://example.com/bike.jpg"}`, "publication-1", "/ads")
	require.Equal(t, 201, status)
	require.Equal(t, first, replay)
	status, conflict := call(`{"title":"changed","price":19.99,"photo_url":"https://example.com/bike.jpg"}`, "publication-1", "/ads")
	require.Equal(t, 409, status)
	require.NotEmpty(t, conflict["error"])
	for i := 0; i < 4; i++ {
		status, _ = call(body, "", "/ads")
		require.Equal(t, 201, status)
	}
	status, _ = call(body, "", "/ads")
	require.Equal(t, 429, status)
	status, replay = call(body, "publication-1", "/ads")
	require.Equal(t, 201, status)
	require.Equal(t, first, replay)
	var publications, ads int
	require.NoError(t, d.db.QueryRow("SELECT count(*) FROM ad_publications").Scan(&publications))
	require.Equal(t, 5, publications)
	require.NoError(t, d.db.QueryRow("SELECT count(*) FROM ads").Scan(&ads))
	require.Equal(t, 5, ads)
	status, imported := call(`{"ads":[`+body+`]}`, "publication-1", "/ads/import")
	require.Equal(t, 201, status)
	status, importReplay := call(`{"ads":[`+body+`]}`, "publication-1", "/ads/import")
	require.Equal(t, 201, status)
	require.Equal(t, imported, importReplay)
	require.NoError(t, d.db.QueryRow("SELECT count(*) FROM ad_publications").Scan(&publications))
	require.Equal(t, 5, publications)
	require.NoError(t, d.db.QueryRow("SELECT count(*) FROM ads").Scan(&ads))
	require.Equal(t, 6, ads)

	// Deleting an ad must not refund its publication slot.
	deleteRequest, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/ads/%d", server.URL, id), nil)
	require.NoError(t, err)
	deleteRequest.Header.Set("Authorization", string(testOwner))
	deleted, err := client.Do(deleteRequest)
	require.NoError(t, err)
	deletedBody, err := io.ReadAll(deleted.Body)
	require.NoError(t, deleted.Body.Close())
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, deleted.StatusCode)
	require.Empty(t, deletedBody)
	status, denied := call(body, "", "/ads")
	require.Equal(t, http.StatusTooManyRequests, status)
	require.NotEmpty(t, denied["error"])
	require.NoError(t, d.db.QueryRow("SELECT count(*) FROM ad_publications").Scan(&publications))
	require.Equal(t, 5, publications)
	require.NoError(t, d.db.QueryRow("SELECT count(*) FROM ads").Scan(&ads))
	require.Equal(t, 5, ads)
	// Import again after deletion, while the individual publication quota is full.
	status, afterDeleteImport := call(`{"ads":[`+body+`]}`, "after-delete-import", "/ads/import")
	require.Equal(t, http.StatusCreated, status)
	require.NotEmpty(t, afterDeleteImport["ids"])
	require.NoError(t, d.db.QueryRow("SELECT count(*) FROM ad_publications").Scan(&publications))
	require.Equal(t, 5, publications)
	status, denied = call(body, "", "/ads")
	require.Equal(t, http.StatusTooManyRequests, status)
	require.NotEmpty(t, denied["error"])
	// Move only the deleted ad's retained ledger entry outside the rolling window
	// in this isolated test schema. No clock changes, sleeps or live data changes.
	aged, err := d.db.Exec("UPDATE ad_publications SET published_at=clock_timestamp()-interval '24 hours 1 second' WHERE ad_id=$1 AND owner_id=$2::uuid", id, string(testOwner))
	require.NoError(t, err)
	affected, err := aged.RowsAffected()
	require.NoError(t, err)
	require.Equal(t, int64(1), affected)
	var active int
	require.NoError(t, d.db.QueryRow("SELECT count(*) FROM ad_publications WHERE published_at > statement_timestamp()-interval '24 hours'").Scan(&active))
	require.Equal(t, 4, active)
	status, newPublication := call(body, "", "/ads")
	require.Equal(t, http.StatusCreated, status)
	var newID int64
	require.NoError(t, json.Unmarshal(newPublication["id"], &newID))
	require.Positive(t, newID)
	require.NotEqual(t, id, newID)
	status, denied = call(body, "", "/ads")
	require.Equal(t, http.StatusTooManyRequests, status)
	require.NotEmpty(t, denied["error"])
	require.NoError(t, d.db.QueryRow("SELECT count(*) FROM ad_publications WHERE published_at > statement_timestamp()-interval '24 hours'").Scan(&active))
	require.Equal(t, 5, active)
	require.NoError(t, d.db.QueryRow("SELECT count(*) FROM ad_publications").Scan(&publications))
	require.Equal(t, 6, publications)
	require.NoError(t, d.db.QueryRow("SELECT count(*) FROM ads").Scan(&ads))
	require.Equal(t, 7, ads)

}
