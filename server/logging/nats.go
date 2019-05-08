/*
 * Copyright 2012-2019 The NATS Authors
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

package logging

import (
	"github.com/nats-io/nats-server/logger"
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
