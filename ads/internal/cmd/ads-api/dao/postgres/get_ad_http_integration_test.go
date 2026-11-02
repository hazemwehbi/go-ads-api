//go:build integration

package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/domain"
	adshttp "github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/http"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/usecase"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/userclient"
)

// Verify selection through HTTP, the usecase and real PostgreSQL, not just 200 OK.
func TestIntegrationHTTPGetAdSelectsRequestedRecord(t *testing.T) {
	d, _ := integrationDAO(t)
	fixtures := []struct {
		request  usecase.CreateAdRequest
		nickname string
		id       domain.AdID
	}{
		{request: usecase.CreateAdRequest{Title: "Bike", PriceCents: 10000, PhotoURL: "https://example.com/bike.jpg", OwnerID: testOwner}, nickname: "alice"},
		{request: usecase.CreateAdRequest{Title: "Chair", PriceCents: 3000, PhotoURL: "https://example.com/chair.jpg", OwnerID: otherOwner}, nickname: "bob"},
	}
	create := usecase.NewCreateAdFunc(d)
	for i := range fixtures {
		id, err := create(context.Background(), fixtures[i].request)
		require.NoError(t, err)
		require.Positive(t, int64(id))
		fixtures[i].id = id
	}
	require.NotEqual(t, fixtures[0].id, fixtures[1].id)
	upstreamMux := http.NewServeMux()
	upstreamMux.HandleFunc("GET /v1/users/{userID}", func(w http.ResponseWriter, r *http.Request) {
		for _, f := range fixtures {
			if r.PathValue("userID") == string(f.request.OwnerID) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]string{"user_id": string(f.request.OwnerID), "nickname": f.nickname, "email": "private@example.test", "phone": "private-phone", "internal_note": "private-note"})
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
	})
	upstream := httptest.NewServer(upstreamMux)
	t.Cleanup(upstream.Close)
	lookup := userclient.NewResilient(userclient.New(upstream.URL, time.Second), userclient.DefaultOptions())
	server := httptest.NewServer(adshttp.NewRouter(adshttp.Usecases{GetAd: usecase.NewGetAdByIDFunc(d, lookup)}))
	t.Cleanup(server.Close)
	client := server.Client()
	client.Timeout = 5 * time.Second
	// Start with the second row to catch accidental first-row selection, then switch
	// IDs and repeat to detect responses or enrichment reused for the wrong ad.
	for _, index := range []int{1, 0, 1} {
		f := fixtures[index]
		t.Run(f.request.Title, func(t *testing.T) {
			resp, err := client.Get(fmt.Sprintf("%s/ads/%d", server.URL, f.id))
			require.NoError(t, err)
			defer func() { require.NoError(t, resp.Body.Close()) }()
			require.Equal(t, http.StatusOK, resp.StatusCode)
			require.Equal(t, "application/json", resp.Header.Get("Content-Type"))
			var body struct {
				ID         int64       `json:"id"`
				Title      string      `json:"title"`
				Price      json.Number `json:"price"`
				PriceCents int64       `json:"price_cents"`
				PhotoURL   string      `json:"photo_url"`
				UserID     string      `json:"user_id"`
				Username   *string     `json:"username"`
			}
			var raw json.RawMessage
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&raw))
			require.NoError(t, json.Unmarshal(raw, &body))
			var fields map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(raw, &fields))
			keys := make([]string, 0, len(fields))
			for key := range fields {
				keys = append(keys, key)
			}
			require.ElementsMatch(t, []string{"id", "title", "price", "price_cents", "photo_url", "user_id", "username", "created_at"}, keys)
			require.NotContains(t, string(raw), "private@example.test")
			require.NotContains(t, string(raw), "private-phone")
			require.NotContains(t, string(raw), "private-note")
			require.Equal(t, int64(f.id), body.ID)
			require.Equal(t, f.request.Title, body.Title)
			require.Equal(t, f.request.PriceCents, body.PriceCents)
			euros, err := body.Price.Float64()
			require.NoError(t, err)
			require.Equal(t, float64(f.request.PriceCents)/100, euros)
			require.Equal(t, f.request.PhotoURL, body.PhotoURL)
			require.Equal(t, string(f.request.OwnerID), body.UserID)
			require.NotNil(t, body.Username)
			require.Equal(t, f.nickname, *body.Username)
		})
	}
}
