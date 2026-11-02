package userclient

import (
	"context"
	"time"

	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/common/model"
	usersclient "github.mpi-internal.com/leboncoin/backend-hiring-assessment/users/pkg"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/users/pkg/api"
)

// Client calls the user service over HTTP
type Client struct {
	api *api.Client
}

// New returns a client calling the user service at baseURL
func New(baseURL string, timeout time.Duration) Client {
	return Client{api: api.NewClient(baseURL, timeout)}
}

// GetUserByID returns the user with the given ID
func (c Client) GetUserByID(ctx context.Context, userID model.UserID) (usersclient.User, error) {
	user, err := c.api.GetUserByID(ctx, userID.String())
	if err != nil {
		return usersclient.User{}, classify(ctx, err)
	}

	return usersclient.User(user), nil
}
