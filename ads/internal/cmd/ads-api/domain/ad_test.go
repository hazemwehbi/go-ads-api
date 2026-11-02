package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_AdID_String(t *testing.T) {
	assert.Equal(t, "42", AdID(42).String())
}
