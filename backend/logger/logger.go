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
