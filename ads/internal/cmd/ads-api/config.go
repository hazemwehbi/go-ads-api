package main

import (
	"os"
	"strconv"
	"time"

	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/userclient"
)

// Config holds everything the API needs to start.
type Config struct {
	IdentityResilience userclient.Options
	// HTTPAddr is the address the API listens on, for example ":8080".
	HTTPAddr string
	// DatabaseURL is the PostgreSQL connection string.
	DatabaseURL string
	// IdentityURL is the base URL of the identity service.
	IdentityURL string
	// IdentityTimeout bounds every call to the identity service.
	IdentityTimeout time.Duration
}

// Load reads the configuration from the environment.
func Load() Config {
	return Config{
		IdentityResilience: userclient.Options{CacheSize: integer("IDENTITY_CACHE_SIZE", 1000, 0), TTL: duration("IDENTITY_CACHE_TTL", time.Minute), MaxConcurrent: integer("IDENTITY_MAX_CONCURRENT", 20, 1), FailureThreshold: integer("IDENTITY_FAILURE_THRESHOLD", 5, 1), OpenDuration: duration("IDENTITY_OPEN_DURATION", 5*time.Second)},
		HTTPAddr:           text("HTTP_ADDR", ":8080"),
		DatabaseURL:        text("DATABASE_URL", "postgres://ads:ads@localhost:5432/ads?sslmode=disable"),
		IdentityURL:        text("IDENTITY_URL", "http://localhost:8081"),
		IdentityTimeout:    duration("IDENTITY_TIMEOUT", 250*time.Millisecond),
	}
}

func text(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}

	return fallback
}

func duration(name string, fallback time.Duration) time.Duration {
	value, err := time.ParseDuration(os.Getenv(name))
	if err != nil || value <= 0 {
		return fallback
	}

	return value
}

func integer(name string, fallback, minimum int) int {
	v, err := strconv.Atoi(os.Getenv(name))
	if err != nil || v < minimum {
		return fallback
	}
	return v
}
