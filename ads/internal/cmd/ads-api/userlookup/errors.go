// Package userlookup defines errors shared by user lookup implementations and
// application usecases. It does not decide whether an ad can omit its username.
package userlookup

import (
	"errors"
	"log/slog"
	"net/url"
)

// Failure preserves the cause and provides a stable diagnostic category.
type Failure struct {
	Reason string
	Cause  error
}

func (e *Failure) Error() string { return "user lookup: " + e.Reason }
func (e *Failure) Unwrap() error { return e.Cause }

// LogValue keeps diagnostics in logs without exposing the request URL, which
// can contain credentials or query tokens. Error() stays safe for callers.
func (e *Failure) LogValue() slog.Value {
	attrs := []slog.Attr{slog.String("reason", e.Reason)}
	cause := e.Cause
	var requestErr *url.Error
	if errors.As(cause, &requestErr) {
		attrs = append(attrs, slog.String("operation", requestErr.Op))
		cause = requestErr.Err
	}
	if cause != nil {
		attrs = append(attrs, slog.String("cause", cause.Error()))
	}
	return slog.GroupValue(attrs...)
}
func Reason(err error) string {
	var f *Failure
	if errors.As(err, &f) {
		return f.Reason
	}
	return "unclassified"
}
