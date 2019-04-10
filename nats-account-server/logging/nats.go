package logging

import (
	"github.com/nats-io/gnatsd/logger"
)

// NewNATSLogger creates a new logger that uses the gnatsd library
func NewNATSLogger(conf Config) Logger {
	l := logger.NewStdLogger(conf.Time, conf.Debug, conf.Trace, conf.Colors, conf.PID)
	return &NATSLogger{
		logger: l,
	}
}

// NATSLogger - uses the gnatsd logging code
type NATSLogger struct {
	logger *logger.Logger
}

// Close forwards to the nats logger
func (logger *NATSLogger) Close() error {
	return logger.logger.Close()
}

// Debugf forwards to the nats logger
func (logger *NATSLogger) Debugf(format string, v ...interface{}) {
	logger.logger.Debugf(format, v...)
}

// Errorf forwards to the nats logger
func (logger *NATSLogger) Errorf(format string, v ...interface{}) {
	logger.logger.Errorf(format, v...)
}

// Fatalf forwards to the nats logger
func (logger *NATSLogger) Fatalf(format string, v ...interface{}) {
	logger.logger.Fatalf(format, v...)
}

// Noticef  forwards to the nats logger
func (logger *NATSLogger) Noticef(format string, v ...interface{}) {
	logger.logger.Noticef(format, v...)
}

// Tracef forwards to the nats logger
func (logger *NATSLogger) Tracef(format string, v ...interface{}) {
	logger.logger.Tracef(format, v...)
}

// Warnf forwards to the nats logger
func (logger *NATSLogger) Warnf(format string, v ...interface{}) {
	logger.logger.Warnf(format, v...)
}
