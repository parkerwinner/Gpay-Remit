package logger

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestForRequestTagsEntryWithRequestID(t *testing.T) {
	entry := ForRequest("req-xyz")

	assert.Equal(t, "req-xyz", entry.Data["request_id"])
}
