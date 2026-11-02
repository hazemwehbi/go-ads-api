package httpserver

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os/signal"
	"syscall"
	"time"
)

// Run serves handler on addr until the process receives SIGINT or SIGTERM, then gives in-flight requests a few seconds to complete.
func Run(ctx context.Context, addr string, handler http.Handler) error {
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	server := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("unable to listen: %w", err)
	}
	return serve(ctx, server, listener)
}

// serve accepts a listener so shutdown can be tested without reserving a fixed port.
func serve(ctx context.Context, server *http.Server, listener net.Listener) error {
	defer listener.Close()
	serverErrors := make(chan error, 1)
	go func() {
		slog.Info("listening", "address", listener.Addr().String())
		serverErrors <- server.Serve(listener)
	}()

	select {
	case err := <-serverErrors:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("unable to serve: %w", err)
		}

		return nil
	case <-ctx.Done():
		slog.Info("shutting down")
	}

	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		_ = server.Close()
		return fmt.Errorf("unable to shut down gracefully: %w", err)
	}

	return nil
}
