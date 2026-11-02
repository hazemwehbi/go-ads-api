package userclient

import (
	"context"
	"errors"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/userlookup"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/common/model"
	users "github.mpi-internal.com/leboncoin/backend-hiring-assessment/users/pkg"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/users/pkg/api"
)

const owner model.UserID = "aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa"
const other model.UserID = "bbbbbbbb-bbbb-4bbb-bbbb-bbbbbbbbbbbb"

type lookupFunc func(context.Context, model.UserID) (users.User, error)

func (f lookupFunc) GetUserByID(c context.Context, id model.UserID) (users.User, error) {
	return f(c, id)
}
func good(id model.UserID) users.User { return users.User{UserID: id, Nickname: "alice"} }
func TestCacheExpiryEvictionAndDisabled(t *testing.T) {
	for _, size := range []int{0, 1} {
		t.Run(string(rune('0'+size)), func(t *testing.T) {
			calls := 0
			opts := DefaultOptions()
			opts.CacheSize = size
			c := NewResilient(lookupFunc(func(_ context.Context, id model.UserID) (users.User, error) { calls++; return good(id), nil }), opts)
			now := time.Now()
			c.now = func() time.Time { return now }
			ctx := context.Background()
			_, err := c.GetUserByID(ctx, owner)
			require.NoError(t, err)
			_, err = c.GetUserByID(ctx, owner)
			require.NoError(t, err)
			if size == 0 {
				require.Equal(t, 2, calls)
			} else {
				require.Equal(t, 1, calls)
			}
			now = now.Add(opts.TTL)
			_, err = c.GetUserByID(ctx, owner)
			require.NoError(t, err)
			before := calls
			_, err = c.GetUserByID(ctx, other)
			require.NoError(t, err)
			_, err = c.GetUserByID(ctx, owner)
			require.NoError(t, err)
			require.Equal(t, before+2, calls)
			require.LessOrEqual(t, len(c.cache), size)
		})
	}
}
func TestBreakerRecoveryAndCategories(t *testing.T) {
	opts := DefaultOptions()
	opts.CacheSize = 0
	opts.FailureThreshold = 2
	failure := error(&userlookup.Failure{Reason: "unavailable"})
	calls := 0
	c := NewResilient(lookupFunc(func(_ context.Context, id model.UserID) (users.User, error) { calls++; return good(id), failure }), opts)
	now := time.Now()
	c.now = func() time.Time { return now }
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		_, err := c.GetUserByID(ctx, owner)
		require.Equal(t, "unavailable", userlookup.Reason(err))
	}
	_, err := c.GetUserByID(ctx, owner)
	require.Equal(t, "circuit_open", userlookup.Reason(err))
	require.Equal(t, 2, calls)
	now = now.Add(opts.OpenDuration)
	_, err = c.GetUserByID(ctx, owner)
	require.Equal(t, "unavailable", userlookup.Reason(err))
	_, err = c.GetUserByID(ctx, owner)
	require.Equal(t, "circuit_open", userlookup.Reason(err))
	now = now.Add(opts.OpenDuration)
	failure = nil
	u, err := c.GetUserByID(ctx, owner)
	require.NoError(t, err)
	require.Equal(t, "alice", u.Nickname)
	for _, e := range []error{api.ErrNotFound, errors.New("opaque status"), &userlookup.Failure{Reason: "invalid_response"}} {
		failure = e
		for i := 0; i < 4; i++ {
			_, err = c.GetUserByID(ctx, owner)
			require.NotEqual(t, "circuit_open", userlookup.Reason(err))
		}
	}
}
func TestCapacityCoalescingAndCancellation(t *testing.T) {
	opts := DefaultOptions()
	opts.MaxConcurrent = 1
	entered := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	c := NewResilient(lookupFunc(func(ctx context.Context, id model.UserID) (users.User, error) {
		calls.Add(1)
		close(entered)
		select {
		case <-release:
			return good(id), nil
		case <-ctx.Done():
			return users.User{}, ctx.Err()
		}
	}), opts)
	done := make(chan error, 1)
	go func() { _, e := c.GetUserByID(context.Background(), owner); done <- e }()
	<-entered
	_, err := c.GetUserByID(context.Background(), other)
	require.Equal(t, "capacity", userlookup.Reason(err))
	require.Zero(t, c.failures)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = c.GetUserByID(ctx, owner)
	require.ErrorIs(t, err, context.Canceled)
	waitctx, stop := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer stop()
	_, err = c.GetUserByID(waitctx, owner)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	var wg sync.WaitGroup
	results := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := c.GetUserByID(context.Background(), owner); results <- e }()
	}
	close(release)
	require.NoError(t, <-done)
	wg.Wait()
	close(results)
	for e := range results {
		require.NoError(t, e)
	}
	require.Equal(t, int32(1), calls.Load())
}
func TestHalfOpenAllowsOnlyOneProbe(t *testing.T) {
	opts := DefaultOptions()
	opts.CacheSize = 0
	opts.FailureThreshold = 1
	entered := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	c := NewResilient(lookupFunc(func(ctx context.Context, id model.UserID) (users.User, error) {
		if calls.Add(1) == 1 {
			return users.User{}, &userlookup.Failure{Reason: "unavailable"}
		}
		close(entered)
		<-release
		return good(id), nil
	}), opts)
	now := time.Now()
	c.now = func() time.Time { return now }
	_, _ = c.GetUserByID(context.Background(), owner)
	now = now.Add(opts.OpenDuration)
	done := make(chan error, 1)
	go func() { _, e := c.GetUserByID(context.Background(), owner); done <- e }()
	<-entered
	_, err := c.GetUserByID(context.Background(), other)
	require.Equal(t, "circuit_open", userlookup.Reason(err))
	close(release)
	require.NoError(t, <-done)
	require.True(t, c.openUntil.IsZero())
	require.False(t, c.probing)
}
func TestLeaderCancellationReleasesState(t *testing.T) {
	opts := DefaultOptions()
	entered := make(chan struct{})
	c := NewResilient(lookupFunc(func(ctx context.Context, id model.UserID) (users.User, error) {
		close(entered)
		<-ctx.Done()
		return users.User{}, ctx.Err()
	}), opts)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, e := c.GetUserByID(ctx, owner); done <- e }()
	<-entered
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
	require.Zero(t, c.active)
	require.Empty(t, c.calls)
	require.Empty(t, c.cache)
	require.Zero(t, c.failures)
	c.next = lookupFunc(func(_ context.Context, id model.UserID) (users.User, error) { return good(id), nil })
	_, err := c.GetUserByID(context.Background(), owner)
	require.NoError(t, err)
}
func TestResponseClassification(t *testing.T) {
	for _, tc := range []struct {
		name, body, reason string
		status             int
	}{
		{"success", `{"user_id":"aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa","nickname":"alice"}`, "unclassified", 200},
		{"missing", "", "not_found", 404}, {"opaque 503", "", "unexpected", 503}, {"opaque 401", "", "unexpected", 401},
		{"bad JSON", "{", "invalid_response", 200}, {"wrong type", `{"nickname":123}`, "invalid_response", 200},
		{"wrong owner", `{"user_id":"bbbbbbbb-bbbb-4bbb-bbbb-bbbbbbbbbbbb","nickname":"alice"}`, "invalid_response", 200},
		{"empty name", `{"user_id":"aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa","nickname":""}`, "invalid_response", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			c := NewResilient(New(srv.URL, time.Second), DefaultOptions())
			_, err := c.GetUserByID(context.Background(), owner)
			require.Equal(t, tc.reason, userlookup.Reason(err))
			if tc.status == 200 && tc.name == "success" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				require.Empty(t, c.cache)
			}
		})
	}
}
func TestTimeoutVersusParentDeadline(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer srv.Close()
	c := NewResilient(New(srv.URL, 20*time.Millisecond), DefaultOptions())
	_, err := c.GetUserByID(context.Background(), owner)
	require.Equal(t, "timeout", userlookup.Reason(err))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = c.GetUserByID(ctx, owner)
	require.ErrorIs(t, err, context.Canceled)
	ctx, cancel = context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	_, err = c.GetUserByID(ctx, owner)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.NotEqual(t, "timeout", userlookup.Reason(err))
}

func TestOldCompletionCannotCloseNewCircuit(t *testing.T) {
	opts := DefaultOptions()
	opts.CacheSize = 0
	opts.FailureThreshold = 1
	entered := make(chan struct{})
	release := make(chan struct{})
	c := NewResilient(lookupFunc(func(_ context.Context, id model.UserID) (users.User, error) {
		if id == owner {
			close(entered)
			<-release
			return good(id), nil
		}
		return users.User{}, &userlookup.Failure{Reason: "unavailable"}
	}), opts)
	done := make(chan error, 1)
	go func() { _, e := c.GetUserByID(context.Background(), owner); done <- e }()
	<-entered
	_, err := c.GetUserByID(context.Background(), other)
	require.Equal(t, "unavailable", userlookup.Reason(err))
	close(release)
	require.NoError(t, <-done)
	_, err = c.GetUserByID(context.Background(), other)
	require.Equal(t, "circuit_open", userlookup.Reason(err))
}
func TestExpiredCacheDoesNotHideDeletionOrFailure(t *testing.T) {
	opts := DefaultOptions()
	failure := error(nil)
	c := NewResilient(lookupFunc(func(_ context.Context, id model.UserID) (users.User, error) { return good(id), failure }), opts)
	now := time.Now()
	c.now = func() time.Time { return now }
	_, err := c.GetUserByID(context.Background(), owner)
	require.NoError(t, err)
	now = now.Add(opts.TTL)
	failure = api.ErrNotFound
	user, err := c.GetUserByID(context.Background(), owner)
	require.Equal(t, "not_found", userlookup.Reason(err))
	require.Empty(t, user.Nickname)
	require.Empty(t, c.cache)
	failure = nil
	_, err = c.GetUserByID(context.Background(), owner)
	require.NoError(t, err)
	now = now.Add(opts.TTL)
	failure = &userlookup.Failure{Reason: "unavailable"}
	user, err = c.GetUserByID(context.Background(), owner)
	require.Equal(t, "unavailable", userlookup.Reason(err))
	require.Empty(t, user.Nickname)
	require.Empty(t, c.cache)
}
func TestInvalidBaseURLIsNotTransient(t *testing.T) {
	_, err := New("invalid://host", time.Second).GetUserByID(context.Background(), owner)
	require.Equal(t, "unexpected", userlookup.Reason(err))
	require.NotEqual(t, "timeout", userlookup.Reason(err))
}
func TestProbeCancellationDoesNotWedgeCircuit(t *testing.T) {
	opts := DefaultOptions()
	opts.FailureThreshold = 1
	opts.CacheSize = 0
	entered := make(chan struct{})
	var count int
	c := NewResilient(lookupFunc(func(ctx context.Context, id model.UserID) (users.User, error) {
		count++
		if count == 1 {
			return users.User{}, &userlookup.Failure{Reason: "unavailable"}
		}
		if count == 2 {
			close(entered)
			<-ctx.Done()
			return users.User{}, ctx.Err()
		}
		return good(id), nil
	}), opts)
	now := time.Now()
	c.now = func() time.Time { return now }
	_, _ = c.GetUserByID(context.Background(), owner)
	now = now.Add(opts.OpenDuration)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, e := c.GetUserByID(ctx, owner); done <- e }()
	<-entered
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
	require.False(t, c.probing)
	now = now.Add(opts.OpenDuration)
	_, err := c.GetUserByID(context.Background(), owner)
	require.NoError(t, err)
}

// A hot cache entry must not extend its absolute expiry on every read.
func TestCachedUsernameRefreshesAfterRename(t *testing.T) {
	var calls atomic.Int32
	var name atomic.Value
	name.Store("alice")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"user_id":"aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa","nickname":"` + name.Load().(string) + `"}`))
	}))
	defer upstream.Close()
	opts := DefaultOptions()
	c := NewResilient(New(upstream.URL, time.Second), opts)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c.now = func() time.Time { return now }
	readName := func(want string, wantCalls int32) {
		t.Helper()
		user, err := c.GetUserByID(context.Background(), owner)
		require.NoError(t, err)
		require.Equal(t, want, user.Nickname)
		require.Equal(t, wantCalls, calls.Load())
	}
	readName("alice", 1)
	name.Store("alice-renamed")
	now = now.Add(opts.TTL - time.Nanosecond)
	readName("alice", 1)
	now = now.Add(time.Nanosecond)
	readName("alice-renamed", 2)
	readName("alice-renamed", 2)
}
