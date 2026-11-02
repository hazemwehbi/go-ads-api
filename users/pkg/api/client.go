package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/common/model"
)

// ErrNotFound is returned when the requested user does not exist.
var ErrNotFound = errors.New("user not found")

// User represents a user returned by the identity service.
type User struct {
	UserID   model.UserID `json:"user_id"`
	Nickname string       `json:"nickname"`
}

// Client is an HTTP client for the identity service.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient returns a client calling the identity service at baseURL. Requests exceeding timeout are aborted.
func NewClient(baseURL string, timeout time.Duration) *Client {
	return &Client{
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		httpClient: &http.Client{Timeout: timeout},
	}
}

// GetUserByID returns the user with the given ID from the identity service.
func (c *Client) GetUserByID(ctx context.Context, userID string) (User, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/users/"+userID, nil)
	if err != nil {
		return User{}, fmt.Errorf("unable to build get user request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return User{}, fmt.Errorf("unable to call identity service: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return User{}, ErrNotFound
	}

	if resp.StatusCode != http.StatusOK {
		return User{}, fmt.Errorf("identity service answered %d", resp.StatusCode)
	}

	var user User
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return User{}, fmt.Errorf("unable to decode user response: %w", err)
	}

	return user, nil
}

// ListUsers returns all users registered in the identity service.
func (c *Client) ListUsers(ctx context.Context) ([]User, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/users", nil)
	if err != nil {
		return nil, fmt.Errorf("unable to build users request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("unable to call identity service: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("identity service answered %d", resp.StatusCode)
	}

	var users []User
	if err := json.NewDecoder(resp.Body).Decode(&users); err != nil {
		return nil, fmt.Errorf("unable to decode users response: %w", err)
	}

	return users, nil
}
