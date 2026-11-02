package userlookup

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWrappedFailure(t *testing.T) {
	cause := errors.New("upstream failure")
	failure := &Failure{Reason: "invalid_response", Cause: cause}
	wrapped := fmt.Errorf("lookup: %w", failure)
	require.Equal(t, "invalid_response", Reason(wrapped))
	require.ErrorIs(t, wrapped, cause)
	var got *Failure
	require.ErrorAs(t, wrapped, &got)
	require.Same(t, failure, got)
	require.Equal(t, "unclassified", Reason(nil))
	require.Equal(t, "unclassified", Reason(errors.New("other error")))
}
