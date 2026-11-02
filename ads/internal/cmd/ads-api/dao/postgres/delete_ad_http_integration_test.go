//go:build integration

package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/domain"
	adshttp "github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/http"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/usecase"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/userclient"
)

func TestIntegrationHTTPDeleteOwnershipAndIsolation(t *testing.T) {
	d, _ := integrationDAO(t)
	create := usecase.NewCreateAdFunc(d)
	target, err := create(context.Background(), adRequest(testOwner))
	require.NoError(t, err)
	survivorRequest := adRequest(testOwner)
	survivorRequest.Title = "Chair"
	survivorRequest.PriceCents = 3000
	survivorRequest.PhotoURL = "https://example.com/chair.jpg"
	survivor, err := create(context.Background(), survivorRequest)
	require.NoError(t, err)
	require.NotEqual(t, target, survivor)
	// Both ads have the same owner: an overly broad owner-only DELETE must fail this test.
	var userCalls atomic.Int32
	var unavailable atomic.Bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userCalls.Add(1)
		if unavailable.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		if r.URL.Path != "/v1/users/"+string(testOwner) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"user_id": string(testOwner), "nickname": "alice"})
	}))
	t.Cleanup(upstream.Close)
	lookup := userclient.NewResilient(userclient.New(upstream.URL, time.Second), userclient.DefaultOptions())
	server := httptest.NewServer(adshttp.NewRouter(adshttp.Usecases{
		GetAd: usecase.NewGetAdByIDFunc(d, lookup), DeleteAd: usecase.NewDeleteAdFunc(d),
	}))
	t.Cleanup(server.Close)
	client := server.Client()
	client.Timeout = 5 * time.Second
	request := func(method string, id domain.AdID, owner domain.UserID, want int) []byte {
		t.Helper()
		req, e := http.NewRequest(method, fmt.Sprintf("%s/ads/%d", server.URL, id), nil)
		require.NoError(t, e)
		if owner != "" {
			req.Header.Set("Authorization", string(owner))
		}
		resp, e := client.Do(req)
		require.NoError(t, e)
		defer func() { require.NoError(t, resp.Body.Close()) }()
		body, e := io.ReadAll(resp.Body)
		require.NoError(t, e)
		require.Equal(t, want, resp.StatusCode, string(body))
		if want == http.StatusNoContent {
			require.Empty(t, body)
		} else {
			require.Equal(t, "application/json", resp.Header.Get("Content-Type"))
			if want >= 400 {
				var failure map[string]string
				require.NoError(t, json.Unmarshal(body, &failure))
				require.Len(t, failure, 1)
				require.NotEmpty(t, failure["error"])
			}
		}
		return body
	}
	original := request("GET", target, "", 200)
	otherBefore := request("GET", survivor, "", 200)
	// Warm display cache must never authorize another caller. Deletion must also
	// remain independent of an unavailable profile service.
	unavailable.Store(true)
	beforeDelete := userCalls.Load()
	request("DELETE", target, otherOwner, 404)
	require.Equal(t, beforeDelete, userCalls.Load(), "DELETE must not consult profile service")
	require.JSONEq(t, string(original), string(request("GET", target, "", 200)), "unauthorized delete must not change the target")
	require.JSONEq(t, string(otherBefore), string(request("GET", survivor, "", 200)))
	request("DELETE", target, testOwner, 204)
	require.Equal(t, beforeDelete, userCalls.Load(), "owner delete must not depend on cached username or upstream availability")
	request("GET", target, "", 404)
	request("DELETE", target, testOwner, 404)
	require.JSONEq(t, string(otherBefore), string(request("GET", survivor, "", 200)), "deleting one ad must preserve the other ad")
	var count int
	require.NoError(t, d.db.QueryRow("SELECT count(*) FROM ads").Scan(&count))
	require.Equal(t, 1, count)
}
