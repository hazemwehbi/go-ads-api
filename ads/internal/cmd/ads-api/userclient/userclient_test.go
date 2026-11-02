package userclient_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/userclient"
	usersclient "github.mpi-internal.com/leboncoin/backend-hiring-assessment/users/pkg"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/users/pkg/api"
)

func newUserServer(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/users/{userID}", func(rw http.ResponseWriter, r *http.Request) {
		if r.PathValue("userID") != "00000000-0000-0000-0000-000000000001" {
			rw.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = rw.Write([]byte(`{"user_id":"00000000-0000-0000-0000-000000000001","nickname":"alice"}`))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return srv
}

func TestGetUserByID(t *testing.T) {
	ctx := context.Background()
	client := userclient.New(newUserServer(t).URL, time.Second)

	t.Run("nominal", func(t *testing.T) {
		got, err := client.GetUserByID(ctx, "00000000-0000-0000-0000-000000000001")

		require.NoError(t, err)
		assert.Equal(t, usersclient.User{UserID: "00000000-0000-0000-0000-000000000001", Nickname: "alice"}, got)
	})

	t.Run("error user not found", func(t *testing.T) {
		_, err := client.GetUserByID(ctx, "unknown")

		require.Error(t, err)
		assert.ErrorIs(t, err, api.ErrNotFound)
	})
}

func TestClientTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(time.Second):
		}
	}))
	defer srv.Close()
	client := userclient.New(srv.URL, 20*time.Millisecond)
	_, err := client.GetUserByID(context.Background(), "00000000-0000-0000-0000-000000000001")
	require.Error(t, err)
	var timeout interface{ Timeout() bool }
	require.ErrorAs(t, err, &timeout)
	require.True(t, timeout.Timeout())
}
