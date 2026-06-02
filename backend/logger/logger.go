package logger

import (
	"os"
	"strings"

	"github.com/sirupsen/logrus"
)

// Log is the application-wide structured logger.
// Initialized with safe defaults so callers never receive a nil logger.
var Log *logrus.Logger

func init() {
	Log = logrus.New()
	Log.SetOutput(os.Stdout)
	Log.SetFormatter(&logrus.TextFormatter{FullTimestamp: true})
	Log.SetLevel(logrus.InfoLevel)
}

// Init configures the global logger for the given environment.
// JSON format is used in production; human-readable text is used elsewhere.
// The log level is controlled by the LOG_LEVEL environment variable (default: info).
func Init(env string) {
	Log = logrus.New()
	Log.SetOutput(os.Stdout)

	if strings.ToLower(env) == "production" {
		Log.SetFormatter(&logrus.JSONFormatter{})
	} else {
		Log.SetFormatter(&logrus.TextFormatter{
			FullTimestamp: true,
		})
	}

	level := parseLevel(os.Getenv("LOG_LEVEL"))
	Log.SetLevel(level)
}

func parseLevel(s string) logrus.Level {
	switch strings.ToLower(s) {
	case "debug":
		return logrus.DebugLevel
	case "warn", "warning":
		return logrus.WarnLevel
	case "error":
		return logrus.ErrorLevel
	case "fatal":
		return logrus.FatalLevel
	default:
		return logrus.InfoLevel
	}
}

// WithFields returns an entry with the given structured fields attached.
func WithFields(fields logrus.Fields) *logrus.Entry {
	return Log.WithFields(fields)
}

// WithCorrelation returns a log entry pre-populated with correlation identifiers.
// Use this whenever you want to tie a log line to a specific request or user.
func WithCorrelation(correlationID, requestID string, userID interface{}) *logrus.Entry {
	return Log.WithFields(logrus.Fields{
		"correlation_id": correlationID,
		"request_id":     requestID,
		"user_id":        userID,
	})
}

// sensitiveKeys is the set of JSON body keys whose values are replaced with
// "[REDACTED]" before the body is written to the log.
var sensitiveKeys = map[string]struct{}{
	"password":              {},
	"password_confirmation": {},
	"current_password":      {},
	"new_password":          {},
	"token":                 {},
	"access_token":          {},
	"refresh_token":         {},
	"secret":                {},
	"api_key":               {},
	"private_key":           {},
	"card_number":           {},
	"cvv":                   {},
	"pin":                   {},
}

// SanitizeBody walks a decoded JSON body (map[string]interface{}) and replaces
// the values of any sensitive keys with the string "[REDACTED]".
// Non-map values are returned unchanged.
func SanitizeBody(body interface{}) interface{} {
	m, ok := body.(map[string]interface{})
	if !ok {
		return body
	}
	out := make(map[string]interface{}, len(m))
	for k, v := range m {
		if _, sensitive := sensitiveKeys[strings.ToLower(k)]; sensitive {
			out[k] = "[REDACTED]"
		} else {
			// Recurse into nested objects.
			out[k] = SanitizeBody(v)
		}
	}
	return out
}
