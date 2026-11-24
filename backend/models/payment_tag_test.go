package models

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeAndValidateTags_ValidAndDeduplicated(t *testing.T) {
	tags, err := NormalizeAndValidateTags([]string{
		"  Family  Support ",
		"family support",
		"RENT-2026",
		"groceries_home",
		"Cafe\u0301", // NFD accented e -> NFC
	})
	require.NoError(t, err)
	assert.Equal(t, []string{
		"family support",
		"rent-2026",
		"groceries_home",
		"caf\u00e9",
	}, tags)
}

func TestNormalizeAndValidateTags_EmptySlice(t *testing.T) {
	tags, err := NormalizeAndValidateTags(nil)
	require.NoError(t, err)
	assert.Equal(t, []string{}, tags)
}

func TestNormalizeAndValidateTags_RejectsEmptyTag(t *testing.T) {
	_, err := NormalizeAndValidateTags([]string{"valid", "   "})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}

func TestNormalizeAndValidateTags_RejectsTooLongTag(t *testing.T) {
	longTag := strings.Repeat("a", MaxTagLength+1)
	_, err := NormalizeAndValidateTags([]string{longTag})
	require.Error(t, err)
	assert.Contains(t, err.Error(), longTag)
	assert.Contains(t, err.Error(), "maximum length")
}

func TestNormalizeAndValidateTags_RejectsTooManyTags(t *testing.T) {
	raw := make([]string, MaxTagsPerPayment+1)
	for i := range raw {
		raw[i] = "tag-" + string(rune('a'+i))
	}
	_, err := NormalizeAndValidateTags(raw)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at most")
}

func TestNormalizeAndValidateTags_RejectsInvalidChars(t *testing.T) {
	for _, bad := range []string{"food!", "tag@home", "a/b", "100%"} {
		_, err := NormalizeAndValidateTags([]string{bad})
		require.Error(t, err, "expected error for %q", bad)
		assert.Contains(t, err.Error(), bad)
	}
}
