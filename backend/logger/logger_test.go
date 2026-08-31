package logger

import (
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestParseLevel(t *testing.T) {
	cases := []struct {
		input    string
		expected logrus.Level
	}{
		{"debug", logrus.DebugLevel},
		{"DEBUG", logrus.DebugLevel},
		{"warn", logrus.WarnLevel},
		{"warning", logrus.WarnLevel},
		{"error", logrus.ErrorLevel},
		{"fatal", logrus.FatalLevel},
		{"info", logrus.InfoLevel},
		{"", logrus.InfoLevel},
		{"unknown", logrus.InfoLevel},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			assert.Equal(t, tc.expected, parseLevel(tc.input))
		})
	}
}

func TestSanitizeBody_RedactsKnownSensitiveKeys(t *testing.T) {
	input := map[string]interface{}{
		"password":     "secret",
		"token":        "tok_abc",
		"access_token": "at_xyz",
		"api_key":      "key_123",
		"username":     "alice",
		"amount":       42.5,
	}

	result := SanitizeBody(input).(map[string]interface{})

	assert.Equal(t, "[REDACTED]", result["password"])
	assert.Equal(t, "[REDACTED]", result["token"])
	assert.Equal(t, "[REDACTED]", result["access_token"])
	assert.Equal(t, "[REDACTED]", result["api_key"])
	assert.Equal(t, "alice", result["username"], "non-sensitive field must be unchanged")
	assert.Equal(t, 42.5, result["amount"])
}

func TestSanitizeBody_HandlesNestedObjects(t *testing.T) {
	input := map[string]interface{}{
		"user": map[string]interface{}{
			"email":    "user@example.com",
			"password": "hunter2",
			"profile": map[string]interface{}{
				"pin": "1234",
				"age": 30,
			},
		},
	}

	result := SanitizeBody(input).(map[string]interface{})
	user := result["user"].(map[string]interface{})
	profile := user["profile"].(map[string]interface{})

	// "email" is now a sensitive key — value must be redacted
	assert.Equal(t, "[REDACTED]", user["email"])
	assert.Equal(t, "[REDACTED]", user["password"])
	assert.Equal(t, "[REDACTED]", profile["pin"])
	assert.Equal(t, 30, profile["age"])
}

func TestSanitizeBody_NonMapPassthrough(t *testing.T) {
	assert.Equal(t, "just a string", SanitizeBody("just a string"))
	assert.Equal(t, 42, SanitizeBody(42))
	assert.Nil(t, SanitizeBody(nil))
}

func TestSanitizeBody_EmptyMap(t *testing.T) {
	input := map[string]interface{}{}
	result := SanitizeBody(input).(map[string]interface{})
	assert.Empty(t, result)
}

func TestSanitizeBody_MasksPIIInStringValues(t *testing.T) {
	input := map[string]interface{}{
		"message": "Contact user@example.com or call 800-555-0100",
		"note":    "SSN: 123-45-6789",
	}
	result := SanitizeBody(input).(map[string]interface{})
	assert.Equal(t, "Contact [EMAIL REDACTED] or call [PHONE REDACTED]", result["message"])
	assert.Equal(t, "SSN: [SSN REDACTED]", result["note"])
}

func TestWithCorrelation_ReturnsEntry(t *testing.T) {
	entry := WithCorrelation("corr-123", "req-456", uint(7))
	assert.NotNil(t, entry)
	assert.Equal(t, "corr-123", entry.Data["correlation_id"])
	assert.Equal(t, "req-456", entry.Data["request_id"])
	assert.Equal(t, uint(7), entry.Data["user_id"])
}

func TestInit_SetsJSONFormatterInProduction(t *testing.T) {
	Init("production")
	_, ok := Log.Formatter.(*logrus.JSONFormatter)
	assert.True(t, ok, "production env must use JSONFormatter")
}

func TestInit_SetsTextFormatterOutsideProduction(t *testing.T) {
	Init("development")
	_, ok := Log.Formatter.(*logrus.TextFormatter)
	assert.True(t, ok, "non-production env must use TextFormatter")
}

// --- MaskPII ---

func TestMaskPII_Email(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"user@example.com", "[EMAIL REDACTED]"},
		{"Send to foo.bar+tag@sub.domain.org please", "Send to [EMAIL REDACTED] please"},
		{"no email here", "no email here"},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.expected, MaskPII(tc.input), tc.input)
	}
}

func TestMaskPII_Phone(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"800-555-0100", "[PHONE REDACTED]"},
		{"(800) 555-0100", "[PHONE REDACTED]"},
		{"+1 800 555 0100", "[PHONE REDACTED]"},
		{"call 8005550100 now", "call [PHONE REDACTED] now"},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.expected, MaskPII(tc.input), tc.input)
	}
}

func TestMaskPII_SSN(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"123-45-6789", "[SSN REDACTED]"},
		{"123 45 6789", "[SSN REDACTED]"},
		{"SSN is 078-05-1120.", "SSN is [SSN REDACTED]."},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.expected, MaskPII(tc.input), tc.input)
	}
}

func TestMaskPII_MultiplePII(t *testing.T) {
	input := "Email: user@example.com, Phone: 800-555-0100, SSN: 123-45-6789"
	expected := "Email: [EMAIL REDACTED], Phone: [PHONE REDACTED], SSN: [SSN REDACTED]"
	assert.Equal(t, expected, MaskPII(input))
}

func TestMaskPII_NoChange(t *testing.T) {
	assert.Equal(t, "nothing sensitive", MaskPII("nothing sensitive"))
	assert.Equal(t, "", MaskPII(""))
}

// --- SanitizeFields ---

func TestSanitizeFields_RedactsSensitiveKeys(t *testing.T) {
	fields := logrus.Fields{
		"email":    "user@example.com",
		"ssn":      "123-45-6789",
		"phone":    "800-555-0100",
		"username": "alice",
	}
	result := SanitizeFields(fields)
	assert.Equal(t, "[REDACTED]", result["email"])
	assert.Equal(t, "[REDACTED]", result["ssn"])
	assert.Equal(t, "[REDACTED]", result["phone"])
	assert.Equal(t, "alice", result["username"])
}

func TestSanitizeFields_MasksPIIInStringValues(t *testing.T) {
	fields := logrus.Fields{
		"error_msg": "failed to reach user@example.com",
	}
	result := SanitizeFields(fields)
	assert.Equal(t, "failed to reach [EMAIL REDACTED]", result["error_msg"])
}

func TestSanitizeFields_NonStringValuesPassthrough(t *testing.T) {
	fields := logrus.Fields{
		"count":  42,
		"active": true,
	}
	result := SanitizeFields(fields)
	assert.Equal(t, 42, result["count"])
	assert.Equal(t, true, result["active"])
}
