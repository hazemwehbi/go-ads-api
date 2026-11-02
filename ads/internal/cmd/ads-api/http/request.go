package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.mpi-internal.com/leboncoin/backend-hiring-assessment/ads/internal/cmd/ads-api/domain"
)

var errBodyTooLarge = errors.New("request body exceeds 1 MiB")

func bodyErrorStatus(err error) int {
	if errors.Is(err, errBodyTooLarge) {
		return http.StatusRequestEntityTooLarge
	}
	return http.StatusBadRequest
}
func adIDFromURL(r *http.Request) (domain.AdID, error) {
	n, err := strconv.ParseInt(r.PathValue("adID"), 10, 64)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("positive ad ID required")
	}
	return domain.AdID(n), nil
}
func decodeBody(r *http.Request, target any) error {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		return errors.New("Content-Type must be application/json")
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, (1<<20)+1))
	if err != nil {
		return err
	}
	if len(body) > 1<<20 {
		return errBodyTooLarge
	}
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || trimmed[0] != '{' || !utf8.Valid(body) {
		return errors.New("expected a UTF-8 JSON object")
	}
	d := json.NewDecoder(bytes.NewReader(body))
	d.UseNumber()
	if err = walkJSON(d, 0); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	if _, err = d.Token(); err != io.EOF {
		return errors.New("trailing JSON content")
	}
	d = json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	if err = d.Decode(target); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	return nil
}
func walkJSON(d *json.Decoder, depth int) error {
	if depth > 16 {
		return errors.New("JSON nesting exceeds 16 levels")
	}
	token, err := d.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return err
			}
			name := strings.ToLower(key.(string))
			if seen[name] {
				return errors.New("duplicate JSON key")
			}
			seen[name] = true
			if err = walkJSON(d, depth+1); err != nil {
				return err
			}
		}
	case '[':
		for d.More() {
			if err = walkJSON(d, depth+1); err != nil {
				return err
			}
		}
	default:
		return errors.New("unexpected delimiter")
	}
	_, err = d.Token()
	return err
}
