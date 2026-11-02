package userclient

import (
	"container/list"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/userlookup"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/common/model"
	users "github.mpi-internal.com/leboncoin/backend-hiring-assessment/users/pkg"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/users/pkg/api"
)

func classify(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err == nil {
		return nil
	}
	var known *userlookup.Failure
	if errors.As(err, &known) {
		return err
	}
	if errors.Is(err, context.Canceled) {
		return err
	}
	if errors.Is(err, api.ErrNotFound) {
		return &userlookup.Failure{Reason: "not_found", Cause: err}
	}
	var syntax *json.SyntaxError
	var typ *json.UnmarshalTypeError
	if errors.As(err, &syntax) || errors.As(err, &typ) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return &userlookup.Failure{Reason: "invalid_response", Cause: err}
	}
	var network net.Error
	if errors.As(err, &network) {
		if network.Timeout() {
			return &userlookup.Failure{Reason: "timeout", Cause: err}
		}
	}
	var connection *net.OpError
	if errors.As(err, &connection) {
		return &userlookup.Failure{Reason: "unavailable", Cause: err}
	}
	// HTTP status errors from the supplied client are opaque. Never parse their
	// text or silently assume that an authentication/configuration error is transient.
	return &userlookup.Failure{Reason: "unexpected", Cause: err}
}

type Lookup interface {
	GetUserByID(context.Context, model.UserID) (users.User, error)
}
type Options struct {
	CacheSize        int
	TTL              time.Duration
	MaxConcurrent    int
	FailureThreshold int
	OpenDuration     time.Duration
}

func DefaultOptions() Options { return Options{1000, time.Minute, 20, 5, 5 * time.Second} }

type entry struct {
	id      model.UserID
	user    users.User
	expires time.Time
}
type pending struct {
	done chan struct{}
	user users.User
	err  error
}

// Resilient owns bounded per-process state. Only valid successes are cached.
// LRU limits memory; expired names are never served. Names are not authorization.
type Resilient struct {
	next             Lookup
	opts             Options
	now              func() time.Time
	mu               sync.Mutex
	cache            map[model.UserID]*list.Element
	lru              *list.List
	calls            map[model.UserID]*pending
	active, failures int
	openUntil        time.Time
	probing          bool
	generation       uint64
}

func NewResilient(next Lookup, opts Options) *Resilient {
	if next == nil || opts.CacheSize < 0 || opts.TTL <= 0 || opts.MaxConcurrent < 1 || opts.FailureThreshold < 1 || opts.OpenDuration <= 0 {
		panic("invalid user client options")
	}
	return &Resilient{next: next, opts: opts, now: time.Now, cache: make(map[model.UserID]*list.Element), lru: list.New(), calls: make(map[model.UserID]*pending)}
}
func (c *Resilient) GetUserByID(ctx context.Context, id model.UserID) (users.User, error) {
	if err := ctx.Err(); err != nil {
		return users.User{}, err
	}
	parsed, err := uuid.Parse(string(id))
	if err != nil || parsed == uuid.Nil {
		return users.User{}, &userlookup.Failure{Reason: "invalid_response"}
	}
	id = model.UserID(parsed.String())
	c.mu.Lock()
	if e := c.cache[id]; e != nil {
		v := e.Value.(entry)
		if c.now().Before(v.expires) {
			c.lru.MoveToFront(e)
			c.mu.Unlock()
			return v.user, nil
		}
		c.lru.Remove(e)
		delete(c.cache, id)
	}
	if p := c.calls[id]; p != nil {
		c.mu.Unlock()
		select {
		case <-ctx.Done():
			return users.User{}, ctx.Err()
		case <-p.done:
			if ctx.Err() != nil {
				return users.User{}, ctx.Err()
			}
			// A different caller cancelling the shared leader must not cancel this request.
			if p.err == context.Canceled || p.err == context.DeadlineExceeded {
				return users.User{}, &userlookup.Failure{Reason: "unavailable", Cause: p.err}
			}
			return p.user, p.err
		}
	}
	probe := false
	if !c.openUntil.IsZero() {
		if c.now().Before(c.openUntil) || c.probing {
			c.mu.Unlock()
			return users.User{}, &userlookup.Failure{Reason: "circuit_open"}
		}
		probe = true
	}
	if c.active >= c.opts.MaxConcurrent {
		c.mu.Unlock()
		return users.User{}, &userlookup.Failure{Reason: "capacity"}
	}
	if probe {
		c.probing = true
	}
	c.active++
	generation := c.generation
	p := &pending{done: make(chan struct{})}
	c.calls[id] = p
	c.mu.Unlock()
	user, err := c.next.GetUserByID(ctx, id)
	err = classify(ctx, err)
	if err == nil {
		returned, e := uuid.Parse(string(user.UserID))
		if e != nil || returned != parsed || user.Nickname == "" {
			err = &userlookup.Failure{Reason: "invalid_response"}
		} else {
			user.UserID = id
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.active--
	if generation == c.generation {
		if probe {
			c.probing = false
		}
		reason := userlookup.Reason(err)
		switch {
		case err == nil || reason == "not_found":
			c.failures = 0
			c.openUntil = time.Time{}
			if probe {
				c.generation++
			}
		case reason == "timeout" || reason == "unavailable":
			c.failures++
			if probe || c.failures >= c.opts.FailureThreshold {
				c.openUntil = c.now().Add(c.opts.OpenDuration)
				c.generation++
			}
		case probe:
			c.openUntil = c.now().Add(c.opts.OpenDuration)
			c.generation++
		default:
			c.failures = 0
		}
	}
	if err == nil && c.opts.CacheSize > 0 {
		e := c.lru.PushFront(entry{id, user, c.now().Add(c.opts.TTL)})
		c.cache[id] = e
		if c.lru.Len() > c.opts.CacheSize {
			last := c.lru.Back()
			delete(c.cache, last.Value.(entry).id)
			c.lru.Remove(last)
		}
	}
	if err != nil {
		user = users.User{}
	}
	p.user, p.err = user, err
	delete(c.calls, id)
	close(p.done)
	return user, err
}
