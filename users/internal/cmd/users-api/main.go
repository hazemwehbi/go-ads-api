package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"

	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/common/httpserver"
)

type user struct {
	UserID   string `json:"user_id"`
	Nickname string `json:"nickname"`
}

var users = []user{
	{UserID: "00000000-0000-0000-0000-000000000001", Nickname: "alice"},
	{UserID: "00000000-0000-0000-0000-000000000002", Nickname: "bob"},
	{UserID: "00000000-0000-0000-0000-000000000003", Nickname: "carol"},
	{UserID: "00000000-0000-0000-0000-000000000004", Nickname: "dave"},
	{UserID: "00000000-0000-0000-0000-000000000005", Nickname: "erin"},
	{UserID: "00000000-0000-0000-0000-000000000006", Nickname: "frank"},
	{UserID: "00000000-0000-0000-0000-000000000007", Nickname: "grace"},
	{UserID: "00000000-0000-0000-0000-000000000008", Nickname: "heidi"},
	{UserID: "00000000-0000-0000-0000-000000000009", Nickname: "ivan"},
	{UserID: "00000000-0000-0000-0000-000000000010", Nickname: "judy"},
	{UserID: "00000000-0000-0000-0000-000000000099", Nickname: "ghost"},
}

func main() {
	address := os.Getenv("HTTP_ADDR")
	if address == "" {
		address = ":8081"
	}

	if err := httpserver.Run(context.Background(), address, newRouter()); err != nil {
		slog.Error("fake user service stopped", "error", err)
		os.Exit(1)
	}
}

func newRouter() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(rw http.ResponseWriter, _ *http.Request) {
		rw.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("GET /v1/users", func(rw http.ResponseWriter, _ *http.Request) {
		sendJSON(rw, http.StatusOK, users)
	})

	mux.HandleFunc("GET /v1/users/{userID}", func(rw http.ResponseWriter, r *http.Request) {
		id := r.PathValue("userID")
		for _, u := range users {
			if u.UserID == id {
				sendJSON(rw, http.StatusOK, u)
				return
			}
		}
		rw.WriteHeader(http.StatusNotFound)
	})

	return mux
}

func sendJSON(rw http.ResponseWriter, status int, body any) {
	rw.Header().Set("Content-Type", "application/json; charset=utf-8")
	rw.WriteHeader(status)

	if err := json.NewEncoder(rw).Encode(body); err != nil {
		slog.Error("unable to write response", "error", err)
	}
}
