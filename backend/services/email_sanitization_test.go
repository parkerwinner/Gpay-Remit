package services

import (
	"bytes"
	"html/template"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSanitizeEmailField_StripsHTMLTags is the regression test for #194:
// user-controlled fields (name, recipient account, reason) must not carry
// markup into the email body data.
func TestSanitizeEmailField_StripsHTMLTags(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "script tag is removed",
			input:    "<script></script>John",
			expected: "John",
		},
		{
			name:     "img onerror tag is removed",
			input:    `<img src=x onerror="alert(1)">Jane`,
			expected: "Jane",
		},
		{
			name:     "plain text is unaffected",
			input:    "John Doe",
			expected: "John Doe",
		},
		{
			name:     "nested/broken tags are stripped",
			input:    "<b><i>Bold</i></b>",
			expected: "Bold",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, sanitizeEmailField(tt.input))
		})
	}
}

// TestSanitizeEmailField_StripsControlCharacters guards against
// CR/LF-style control characters riding along in a user-controlled field.
func TestSanitizeEmailField_StripsControlCharacters(t *testing.T) {
	input := "John\r\nBcc: attacker@evil.com\x00Doe"
	got := sanitizeEmailField(input)

	assert.NotContains(t, got, "\r")
	assert.NotContains(t, got, "\n")
	assert.NotContains(t, got, "\x00")
	assert.Equal(t, "JohnBcc: attacker@evil.comDoe", got)
}

// TestSanitizeEmailField_TrimsWhitespace confirms leading/trailing
// whitespace left behind by tag stripping is trimmed.
func TestSanitizeEmailField_TrimsWhitespace(t *testing.T) {
	assert.Equal(t, "John", sanitizeEmailField("  <b>John</b>  "))
}

// TestHTMLTemplate_AutoEscapesUserInput pins the underlying property that
// makes #194 defense-in-depth rather than the only line of defense:
// html/template contextually escapes every {{.Field}} substitution, so even
// unsanitized malicious input can't produce executable markup in the
// rendered body.
func TestHTMLTemplate_AutoEscapesUserInput(t *testing.T) {
	tmpl, err := template.New("test").Parse(`<span>{{.UserName}}</span>`)
	assert.NoError(t, err)

	var buf bytes.Buffer
	err = tmpl.Execute(&buf, map[string]interface{}{
		"UserName": "<script>alert('xss')</script>",
	})
	assert.NoError(t, err)

	rendered := buf.String()
	assert.NotContains(t, rendered, "<script>")
	assert.Contains(t, rendered, "&lt;script&gt;")
}
