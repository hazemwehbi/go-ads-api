package http

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/stretchr/testify/require"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/dao"
	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/usecase"
	"go.yaml.in/yaml/v3"
	"net/http/httptest"
	"os"
	"testing"
)

func TestApplicationErrorBodies(t *testing.T) {
	for _, tc := range []struct {
		name, path, auth string
		failure          error
		status           int
	}{
		{"missing identity", "/ads/1", "", nil, 401}, {"bad id", "/ads/0", "00000000-0000-0000-0000-000000000001", nil, 400},
		{"missing ad", "/ads/1", "00000000-0000-0000-0000-000000000001", dao.ErrNotFound, 404},
		{"storage failure", "/ads/1", "00000000-0000-0000-0000-000000000001", errors.New("secret database address"), 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := NewRouter(Usecases{DeleteAd: func(context.Context, usecase.DeleteAdRequest) error { return tc.failure }})
			req := httptest.NewRequest("DELETE", tc.path, nil)
			req.Header.Set("Authorization", tc.auth)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			require.Equal(t, tc.status, w.Code)
			require.Equal(t, "application/json", w.Header().Get("Content-Type"))
			var body map[string]string
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			require.Len(t, body, 1)
			require.NotEmpty(t, body["error"])
			require.NotContains(t, w.Body.String(), "secret database address")
		})
	}
}
func TestOpenAPIApplicationErrorsAndLimits(t *testing.T) {
	raw, err := os.ReadFile("../specs/openapi.yaml")
	require.NoError(t, err)
	var spec struct {
		Paths map[string]map[string]struct {
			Responses map[string]struct {
				Content map[string]struct {
					Schema struct {
						Ref string `yaml:"$ref"`
					}
				}
			}
		}
		Components struct {
			Schemas map[string]struct {
				Properties map[string]struct {
					MinItems int `yaml:"minItems"`
					MaxItems int `yaml:"maxItems"`
				}
			}
		}
	}
	require.NoError(t, yaml.Unmarshal(raw, &spec))
	for _, tc := range []struct {
		path, method string
		codes        []string
	}{
		{"/ads", "get", []string{"400", "500", "504"}}, {"/ads", "post", []string{"400", "401", "409", "413", "429", "500", "504"}},
		{"/ads/import", "post", []string{"400", "401", "409", "413", "500", "504"}},
		{"/ads/{adID}", "get", []string{"400", "404", "500", "502", "504"}}, {"/ads/{adID}", "delete", []string{"400", "401", "404", "500", "504"}},
	} {
		for _, code := range tc.codes {
			response, ok := spec.Paths[tc.path][tc.method].Responses[code]
			require.True(t, ok, tc.path+code)
			require.Equal(t, "#/components/schemas/Error", response.Content["application/json"].Schema.Ref)
		}
	}
	batch := spec.Components.Schemas["ImportAdsBody"].Properties["ads"]
	require.Equal(t, 1, batch.MinItems)
	require.Equal(t, 100, batch.MaxItems)
}

func TestOpenAPINullableUsername(t *testing.T) {
	raw, err := os.ReadFile("../specs/openapi.yaml")
	require.NoError(t, err)
	var spec struct {
		Components struct {
			Schemas map[string]struct {
				AllOf []struct {
					Required   []string
					Properties map[string]struct {
						Type     string
						Nullable bool
					}
				} `yaml:"allOf"`
			}
		}
	}
	require.NoError(t, yaml.Unmarshal(raw, &spec))
	detail := spec.Components.Schemas["AdDetails"].AllOf[1]
	require.Contains(t, detail.Required, "username")
	require.True(t, detail.Properties["username"].Nullable)
	require.Equal(t, "string", detail.Properties["username"].Type)
}
