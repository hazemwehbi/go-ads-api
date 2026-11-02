package domain

import (
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestValidateAd(t *testing.T) {
	require.NoError(t, ValidateAd("vélo", 0, "https://example.com/p.jpg"))
	for _, tc := range []struct {
		title string
		price int64
		url   string
	}{
		{" ", 0, "https://example.com"}, {strings.Repeat("界", 201), 0, "https://example.com"}, {"bad\x00title", 0, "https://example.com"},
		{"bike", -1, "https://example.com"}, {"bike", MaxPriceCents + 1, "https://example.com"},
		{"bike", 1, "javascript:alert(1)"}, {"bike", 1, "/relative.jpg"}, {"bike", 1, "https://user:pass@example.com"},
	} {
		require.ErrorIs(t, ValidateAd(tc.title, tc.price, tc.url), ErrInvalid)
	}
}
func TestCanonicalOwner(t *testing.T) {
	got, err := CanonicalOwner("AAAAAAAA-AAAA-4AAA-AAAA-AAAAAAAAAAAA")
	require.NoError(t, err)
	require.Equal(t, UserID("aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa"), got)
	for _, raw := range []UserID{"", "alice", "00000000-0000-0000-0000-000000000000"} {
		_, err = CanonicalOwner(raw)
		require.ErrorIs(t, err, ErrInvalid)
	}
}
