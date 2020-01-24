/*
 * Copyright 2020 The NATS Authors
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
 */

package core

import (
	natsserver "github.com/nats-io/nats-server/v2/server"
)

type NilLogger struct {
}

func NewNilLogger() natsserver.Logger {
	return &NilLogger{}
}

func (l *NilLogger) Noticef(format string, v ...interface{}) {}
func (l *NilLogger) Warnf(format string, v ...interface{})   {}
func (l *NilLogger) Errorf(format string, v ...interface{})  {}
func (l *NilLogger) Fatalf(format string, v ...interface{})  {}
func (l *NilLogger) Debugf(format string, v ...interface{})  {}
func (l *NilLogger) Tracef(format string, v ...interface{})  {}
