package main

import (
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestConfig(t *testing.T) {
	for _, key := range []string{"HTTP_ADDR", "DATABASE_URL", "IDENTITY_URL", "IDENTITY_TIMEOUT"} {
		t.Setenv(key, "")
	}
	c := Load()
	require.Equal(t, ":8080", c.HTTPAddr)
	require.Equal(t, "http://localhost:8081", c.IdentityURL)
	require.Equal(t, 250*time.Millisecond, c.IdentityTimeout)
	t.Setenv("HTTP_ADDR", ":9000")
	t.Setenv("DATABASE_URL", "postgres://test")
	t.Setenv("IDENTITY_URL", "http://users:8081")
	t.Setenv("IDENTITY_TIMEOUT", "50ms")
	c = Load()
	require.Equal(t, ":9000", c.HTTPAddr)
	require.Equal(t, "postgres://test", c.DatabaseURL)
	require.Equal(t, "http://users:8081", c.IdentityURL)
	require.Equal(t, 50*time.Millisecond, c.IdentityTimeout)
	for _, value := range []string{"invalid", "0s", "-1s"} {
		t.Setenv("IDENTITY_TIMEOUT", value)
		require.Equal(t, 250*time.Millisecond, Load().IdentityTimeout)
	}
}

func TestIdentityResilienceConfig(t *testing.T) {
	for _, key := range []string{"IDENTITY_CACHE_SIZE", "IDENTITY_CACHE_TTL", "IDENTITY_MAX_CONCURRENT", "IDENTITY_FAILURE_THRESHOLD", "IDENTITY_OPEN_DURATION"} {
		t.Setenv(key, "")
	}
	c := Load().IdentityResilience
	require.Equal(t, 1000, c.CacheSize)
	require.Equal(t, 20, c.MaxConcurrent)
	require.Equal(t, 5, c.FailureThreshold)
	require.Equal(t, time.Minute, c.TTL)
	require.Equal(t, 5*time.Second, c.OpenDuration)
	t.Setenv("IDENTITY_CACHE_SIZE", "0")
	t.Setenv("IDENTITY_CACHE_TTL", "2s")
	t.Setenv("IDENTITY_MAX_CONCURRENT", "3")
	t.Setenv("IDENTITY_FAILURE_THRESHOLD", "2")
	t.Setenv("IDENTITY_OPEN_DURATION", "1s")
	c = Load().IdentityResilience
	require.Zero(t, c.CacheSize)
	require.Equal(t, 3, c.MaxConcurrent)
	require.Equal(t, 2, c.FailureThreshold)
	require.Equal(t, 2*time.Second, c.TTL)
	require.Equal(t, time.Second, c.OpenDuration)
	for _, bad := range []string{"-1", "bad", "999999999999999999999999"} {
		t.Setenv("IDENTITY_CACHE_SIZE", bad)
		require.Equal(t, 1000, Load().IdentityResilience.CacheSize)
	}
	t.Setenv("IDENTITY_MAX_CONCURRENT", "0")
	require.Equal(t, 20, Load().IdentityResilience.MaxConcurrent)
}
