package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/users/pkg/api"
)

func newUsersServer(mux *http.ServeMux) (*httptest.Server, *api.Client) {
	srv := httptest.NewServer(mux)
	client := api.NewClient(srv.URL, 0)
	return srv, client
}

func TestClientListUsers(t *testing.T) {
	ctx := context.Background()

	t.Run("returns all users on success", func(t *testing.T) {
		want := []api.User{
			{UserID: "00000000-0000-0000-0000-000000000001", Nickname: "alice"},
			{UserID: "00000000-0000-0000-0000-000000000002", Nickname: "bob"},
		}
		mux := http.NewServeMux()
		mux.HandleFunc("GET /v1/users", func(rw http.ResponseWriter, _ *http.Request) {
			rw.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(rw).Encode(want)
		})
		srv, client := newUsersServer(mux)
		defer srv.Close()

		got, err := client.ListUsers(ctx)

		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("returns empty slice when server returns empty array", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("GET /v1/users", func(rw http.ResponseWriter, _ *http.Request) {
			rw.Header().Set("Content-Type", "application/json")
			_, _ = rw.Write([]byte("[]"))
		})
		srv, client := newUsersServer(mux)
		defer srv.Close()

		got, err := client.ListUsers(ctx)

		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("returns error when server responds with non-200", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("GET /v1/users", func(rw http.ResponseWriter, _ *http.Request) {
			rw.WriteHeader(http.StatusInternalServerError)
		})
		srv, client := newUsersServer(mux)
		defer srv.Close()

		_, err := client.ListUsers(ctx)

		require.Error(t, err)
	})
}

func TestClientGetUserByID(t *testing.T) {
	ctx := context.Background()

	t.Run("returns the user on success", func(t *testing.T) {
		want := api.User{UserID: "00000000-0000-0000-0000-000000000001", Nickname: "alice"}
		mux := http.NewServeMux()
		mux.HandleFunc("GET /v1/users/{userID}", func(rw http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "00000000-0000-0000-0000-000000000001", r.PathValue("userID"))
			rw.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(rw).Encode(want)
		})
		srv, client := newUsersServer(mux)
		defer srv.Close()

		got, err := client.GetUserByID(ctx, "00000000-0000-0000-0000-000000000001")

		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("returns ErrNotFound when server responds with 404", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("GET /v1/users/{userID}", func(rw http.ResponseWriter, _ *http.Request) {
			rw.WriteHeader(http.StatusNotFound)
		})
		srv, client := newUsersServer(mux)
		defer srv.Close()

		_, err := client.GetUserByID(ctx, "unknown-id")

		require.Error(t, err)
		assert.ErrorIs(t, err, api.ErrNotFound)
	})

	t.Run("returns error when server responds with non-200", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("GET /v1/users/{userID}", func(rw http.ResponseWriter, _ *http.Request) {
			rw.WriteHeader(http.StatusInternalServerError)
		})
		srv, client := newUsersServer(mux)
		defer srv.Close()

		_, err := client.GetUserByID(ctx, "user-1")

		require.Error(t, err)
		assert.NotErrorIs(t, err, api.ErrNotFound)
	})
}
