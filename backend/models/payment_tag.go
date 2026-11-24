package models

import (
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Gpay-Remit/gpay-remit/errors"
	"golang.org/x/text/unicode/norm"
)

const (
	MaxTagLength      = 32
	MaxTagsPerPayment = 10
)

// PaymentTag stores a normalized user-scoped tag attached to a payment.
type PaymentTag struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	PaymentID uint      `gorm:"not null;index:idx_payment_tags_payment_id;uniqueIndex:uq_payment_tag" json:"payment_id"`
	UserID    uint      `gorm:"not null;index:idx_payment_tags_user_tag" json:"user_id"`
	Tag       string    `gorm:"size:32;not null;uniqueIndex:uq_payment_tag;index:idx_payment_tags_user_tag" json:"tag"`
}

// TableName overrides the table name.
func (PaymentTag) TableName() string {
	return "payment_tags"
}

// NormalizeTag applies Unicode NFC normalization, trims surrounding whitespace,
// collapses internal whitespace to single spaces, and lowercases the tag.
func NormalizeTag(raw string) string {
	normalized := norm.NFC.String(strings.TrimSpace(raw))
	fields := strings.Fields(normalized)
	return strings.ToLower(strings.Join(fields, " "))
}

// IsValidTagChar reports whether r is permitted in a payment tag:
// Unicode letters, digits, spaces, hyphen (-), or underscore (_).
func IsValidTagChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ' || r == '-' || r == '_'
}

// NormalizeAndValidateTags normalizes, validates, and deduplicates a slice of tags.
// Invalid tags return a ValidationError naming the tag and the violated rule.
func NormalizeAndValidateTags(raw []string) ([]string, error) {
	if len(raw) == 0 {
		return []string{}, nil
	}

	seen := make(map[string]struct{}, len(raw))
	out := make([]string, 0, len(raw))

	for _, item := range raw {
		tag := NormalizeTag(item)
		if tag == "" {
			return nil, errors.NewValidationError(
				fmt.Sprintf("Invalid tag %q: tag cannot be empty", item),
				map[string]string{"tag": item, "rule": "non_empty"},
			)
		}
		if utf8.RuneCountInString(tag) > MaxTagLength {
			return nil, errors.NewValidationError(
				fmt.Sprintf("Invalid tag %q: exceeds maximum length of %d characters", tag, MaxTagLength),
				map[string]string{"tag": tag, "rule": "max_length"},
			)
		}
		for _, r := range tag {
			if !IsValidTagChar(r) {
				return nil, errors.NewValidationError(
					fmt.Sprintf("Invalid tag %q: only letters, digits, spaces, hyphens, and underscores are allowed", tag),
					map[string]string{"tag": tag, "rule": "allowed_characters"},
				)
			}
		}
		if _, exists := seen[tag]; exists {
			continue
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
		if len(out) > MaxTagsPerPayment {
			return nil, errors.NewValidationError(
				fmt.Sprintf("Invalid tag %q: a payment can have at most %d tags", tag, MaxTagsPerPayment),
				map[string]string{"tag": tag, "rule": "max_tags_per_payment"},
			)
		}
	}

	return out, nil
}
