package httpserver

import (
	"context"
	"github.com/stretchr/testify/require"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestGracefulShutdownDrainsRequest(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		_, _ = w.Write([]byte("finished"))
	})}
	done := make(chan error, 1)
	go func() { done <- serve(ctx, server, listener) }()
	defer func() { _ = server.Close() }()
	shutdown := make(chan struct{})
	server.RegisterOnShutdown(func() { close(shutdown) })
	response := make(chan string, 1)
	go func() {
		client := http.Client{Timeout: 3 * time.Second}
		resp, e := client.Get("http://" + listener.Addr().String())
		if e != nil {
			response <- e.Error()
			return
		}
		defer func() { _ = resp.Body.Close() }()
		b, e := io.ReadAll(resp.Body)
		if e != nil {
			response <- e.Error()
			return
		}
		response <- string(b)
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("request did not start")
	}
	cancel()
	select {
	case <-shutdown:
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown did not start")
	}
	select {
	case err := <-done:
		t.Fatalf("returned before draining: %v", err)
	default:
	}
	release <- struct{}{}
	select {
	case body := <-response:
		require.Equal(t, "finished", body)
	case <-time.After(3 * time.Second):
		t.Fatal("request did not finish")
	}
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown did not finish")
	}
}
func TestRunListenFailure(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = l.Close() }()
	require.Error(t, Run(context.Background(), l.Addr().String(), http.NewServeMux()))
}
