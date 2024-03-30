package loggerconfig

import "github.com/sirupsen/logrus"

// NewLogger creates and returns a new logrus Logger with predefined configuration.
func NewLogger() *logrus.Logger {
	logger := logrus.New()
	logger.SetFormatter(&logrus.TextFormatter{FullTimestamp: true})
	logger.SetLevel(logrus.InfoLevel)

	return logger
}
