//go:build integration

package postgres

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	adshttp "github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/http"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/usecase"
)

const euroBatch = `{"ads":[{"title":"bike","price":20,"photo_url":"https://example.com/b.jpg"},{"title":"chair","price":30,"photo_url":"https://example.com/c.jpg"}]}`
const centBatch = `{"ads":[{"title":"bike","price_cents":2000,"photo_url":"https://example.com/b.jpg"},{"title":"chair","price_cents":3000,"photo_url":"https://example.com/c.jpg"}]}`

func sendImport(t *testing.T, client *http.Client, url, body, key string) (*http.Response, error) {
	t.Helper()
	req, err := http.NewRequest("POST", url+"/ads/import", bytes.NewBufferString(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", string(testOwner))
	req.Header.Set("Idempotency-Key", key)
	return client.Do(req)
}
func importResponse(t *testing.T, client *http.Client, url, body, key string, status int) []int64 {
	t.Helper()
	resp, err := sendImport(t, client, url, body, key)
	require.NoError(t, err)
	defer func() { require.NoError(t, resp.Body.Close()) }()
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, status, resp.StatusCode, string(raw))
	require.Equal(t, "application/json", resp.Header.Get("Content-Type"))
	if status != 201 {
		var failure map[string]string
		require.NoError(t, json.Unmarshal(raw, &failure))
		require.NotEmpty(t, failure["error"])
		return nil
	}
	var result struct {
		IDs []int64 `json:"ids"`
	}
	require.NoError(t, json.Unmarshal(raw, &result))
	return result.IDs
}
func checkImportCounts(t *testing.T, d *DAO, ads, receipts int) {
	t.Helper()
	for table, want := range map[string]int{"ads": ads, "ad_imports": receipts, "ad_publications": 0} {
		var n int
		require.NoError(t, d.db.QueryRow("SELECT count(*) FROM "+table).Scan(&n))
		require.Equal(t, want, n, table)
	}
}
func TestIntegrationHTTPImportLostResponse(t *testing.T) {
	d, _ := integrationDAO(t)
	router := adshttp.NewRouter(adshttp.Usecases{ImportAds: usecase.NewImportAdsFunc(d)})
	var dropped atomic.Bool
	type delivery struct {
		body []byte
		err  error
	}
	committed := make(chan delivery, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Test-only buffer: finish the real handler and commit, then close the
		// connection before delivering any response bytes to the caller.
		buffered := httptest.NewRecorder()
		router.ServeHTTP(buffered, r)
		if buffered.Code == 201 && dropped.CompareAndSwap(false, true) {
			conn, _, err := w.(http.Hijacker).Hijack()
			if err == nil {
				err = conn.Close()
			}
			committed <- delivery{append([]byte(nil), buffered.Body.Bytes()...), err}
			return
		}
		for k, v := range buffered.Header() {
			w.Header()[k] = v
		}
		w.WriteHeader(buffered.Code)
		_, _ = w.Write(buffered.Body.Bytes())
	}))
	t.Cleanup(server.Close)
	// Fresh connections prevent automatic retries on reused connections from
	// concealing the injected disconnect of an idempotency-marked POST.
	transport := &http.Transport{DisableKeepAlives: true}
	t.Cleanup(transport.CloseIdleConnections)
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	invalid := `{"ads":[{"title":"valid","price":20,"photo_url":"https://example.com/b.jpg"},{"title":"invalid","price":1.001,"photo_url":"https://example.com/c.jpg"}]}`
	importResponse(t, client, server.URL, invalid, "batch-1", 400)
	checkImportCounts(t, d, 0, 0)
	resp, err := sendImport(t, client, server.URL, euroBatch, "batch-1")
	if resp != nil {
		require.NoError(t, resp.Body.Close())
	}
	require.Error(t, err, "client must observe a transport failure")
	var original delivery
	select {
	case original = <-committed:
	case <-time.After(5 * time.Second):
		t.Fatal("handler did not complete committed response")
	}
	require.NoError(t, original.err)
	var first struct {
		IDs []int64 `json:"ids"`
	}
	require.NoError(t, json.Unmarshal(original.body, &first))
	require.Len(t, first.IDs, 2)
	checkImportCounts(t, d, 2, 1)
	for _, id := range first.IDs {
		var exists bool
		require.NoError(t, d.db.QueryRow("SELECT EXISTS(SELECT 1 FROM ads WHERE id=$1)", id).Scan(&exists))
		require.True(t, exists)
	}
	// The caller cannot see first.IDs; it only knows its persisted request key.
	replay := importResponse(t, client, server.URL, centBatch, "batch-1", 201)
	require.Equal(t, first.IDs, replay)
	checkImportCounts(t, d, 2, 1)
	changed := `{"ads":[{"title":"changed","price":20,"photo_url":"https://example.com/b.jpg"}]}`
	importResponse(t, client, server.URL, changed, "batch-1", 409)
	checkImportCounts(t, d, 2, 1)
	require.Equal(t, first.IDs, importResponse(t, client, server.URL, euroBatch, "batch-1", 201))
	checkImportCounts(t, d, 2, 1)
}

func TestIntegrationHTTPImportRollbackRetry(t *testing.T) {
	d, _ := integrationDAO(t)
	_, err := d.db.Exec(`CREATE FUNCTION reject_chair() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.title='chair' THEN RAISE EXCEPTION 'injected second-row failure'; END IF; RETURN NEW; END $$;
 CREATE TRIGGER reject_chair BEFORE INSERT ON ads FOR EACH ROW EXECUTE FUNCTION reject_chair()`)
	require.NoError(t, err)
	server := httptest.NewServer(adshttp.NewRouter(adshttp.Usecases{ImportAds: usecase.NewImportAdsFunc(d)}))
	t.Cleanup(server.Close)
	client := server.Client()
	client.Timeout = 5 * time.Second
	importResponse(t, client, server.URL, euroBatch, "retry-batch", 500)
	checkImportCounts(t, d, 0, 0)
	_, err = d.db.Exec("DROP TRIGGER reject_chair ON ads")
	require.NoError(t, err)
	ids := importResponse(t, client, server.URL, euroBatch, "retry-batch", 201)
	require.Len(t, ids, 2)
	checkImportCounts(t, d, 2, 1)
	require.Equal(t, ids, importResponse(t, client, server.URL, euroBatch, "retry-batch", 201))
	checkImportCounts(t, d, 2, 1)
}
