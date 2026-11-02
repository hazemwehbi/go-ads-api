//go:build integration

package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	adshttp "github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/http"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/usecase"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/userclient"
)

func TestIntegrationHTTPUserEnrichment(t *testing.T) {
	d, _ := integrationDAO(t)
	id, err := usecase.NewCreateAdFunc(d)(context.Background(), adRequest(testOwner))
	require.NoError(t, err)
	var mode atomic.Int32
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		switch mode.Load() {
		case 1:
			w.WriteHeader(404)
		case 2:
			<-r.Context().Done()
		case 3:
			_, _ = w.Write([]byte("{"))
		case 4:
			w.WriteHeader(503)
		default:
			_, _ = fmt.Fprintf(w, `{"user_id":%q,"nickname":"alice"}`, testOwner)
		}
	}))
	defer upstream.Close()
	opts := userclient.DefaultOptions()
	opts.CacheSize = 0
	lookup := userclient.NewResilient(userclient.New(upstream.URL, 30*time.Millisecond), opts)
	server := httptest.NewServer(adshttp.NewRouter(adshttp.Usecases{GetAd: usecase.NewGetAdByIDFunc(d, lookup)}))
	defer server.Close()
	for _, tc := range []struct {
		mode   int32
		status int
		name   string
	}{
		{0, 200, "success"}, {1, 200, "not found"}, {2, 200, "timeout"}, {3, 502, "invalid JSON"}, {4, 502, "opaque HTTP failure"}, {0, 200, "recovery"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mode.Store(tc.mode)
			resp, e := server.Client().Get(fmt.Sprintf("%s/ads/%d", server.URL, id))
			require.NoError(t, e)
			defer resp.Body.Close()
			require.Equal(t, tc.status, resp.StatusCode)
			var body map[string]json.RawMessage
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
			if tc.status == 200 {
				v, ok := body["username"]
				require.True(t, ok)
				if tc.mode == 0 {
					require.JSONEq(t, `"alice"`, string(v))
				} else {
					require.Equal(t, "null", string(v))
				}
			} else {
				require.NotEmpty(t, body["error"])
			}
		})
	}
	before := calls.Load()
	resp, err := server.Client().Get(server.URL + "/ads/9223372036854775807")
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, 404, resp.StatusCode)
	require.Equal(t, before, calls.Load())
}
