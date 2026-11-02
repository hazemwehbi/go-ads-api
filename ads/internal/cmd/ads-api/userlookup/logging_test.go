package userlookup

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFailureStructuredDiagnostics(t *testing.T) {
	for _, tc := range []struct {
		name  string
		cause error
		want  string
	}{
		{"upstream status", errors.New("identity service answered 503"), "identity service answered 503"},
		{"request URL", &url.Error{Op: "Get", URL: "https://user:password@identity.test/v1/users/1?token=secret", Err: errors.New("connection refused")}, "connection refused"},
		{"wrapped request URL", fmt.Errorf("request: %w", &url.Error{Op: "Get", URL: "https://identity.test/?token=secret", Err: errors.New("connection refused")}), "connection refused"},
		{"no cause", nil, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			failure := &Failure{Reason: "unexpected", Cause: tc.cause}
			var output bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&output, nil))
			logger.Warn("user lookup failed", "error", failure)
			var record struct {
				Error struct{ Reason, Cause, Operation string }
			}
			require.NoError(t, json.Unmarshal(output.Bytes(), &record))
			require.Equal(t, "unexpected", record.Error.Reason)
			require.Equal(t, tc.want, record.Error.Cause)
			for _, secret := range []string{"password", "token=", "secret", "https://"} {
				require.NotContains(t, output.String(), secret)
			}
			require.Equal(t, "user lookup: unexpected", failure.Error())
			if tc.cause != nil {
				require.ErrorIs(t, failure, tc.cause)
			}
		})
	}
}
