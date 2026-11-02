package domain

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
)

const MaxPriceCents int64 = 1_000_000_000_000

var ErrInvalid = errors.New("invalid input")

func CanonicalOwner(raw UserID) (UserID, error) {
	id, err := uuid.Parse(strings.TrimSpace(string(raw)))
	if err != nil || id == uuid.Nil {
		return "", fmt.Errorf("%w: valid user UUID required", ErrInvalid)
	}
	return UserID(id.String()), nil
}
func ValidateAd(title string, price int64, photo string) error {
	if strings.TrimSpace(title) == "" || strings.ContainsRune(title, 0) || !utf8.ValidString(title) || utf8.RuneCountInString(strings.TrimSpace(title)) > 200 {
		return fmt.Errorf("%w: title must have 1..200 characters", ErrInvalid)
	}
	if price < 0 || price > MaxPriceCents {
		return fmt.Errorf("%w: price out of range", ErrInvalid)
	}
	u, err := url.Parse(photo)
	if err != nil || len(photo) > 2048 || u.Hostname() == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("%w: valid HTTP(S) photo URL required", ErrInvalid)
	}
	return nil
}
