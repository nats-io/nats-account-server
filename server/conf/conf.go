/*
 * Copyright 2019 The NATS Authors
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

package conf

import (
	natsserver "github.com/nats-io/nats-server/v2/server"
)

// LogConfig allows configuration of the logging details
type LogConfig struct {
	Time   bool
	Debug  bool
	Trace  bool
	Colors bool
	PID    bool
	Custom natsserver.Logger
}

// AccountServerConfig is the root structure for an account server configuration file.
type AccountServerConfig struct {
	Logging LogConfig
	NATS    NATSConfig
	HTTP    HTTPConfig
	Store   StoreConfig

	OperatorJWTPath      string
	SystemAccountJWTPath string
	SignRequestSubject   string
	SignRequestTimeout   int //milliseconds

	// Below options are only to copy jwt from an old account server for initialization
	Primary            string
	ReplicationTimeout int //milliseconds
	MaxReplicationPack int // maximum number of JWTS to grab on startup
}

// TLSConf holds the configuration for a TLS connection/server
type TLSConf struct {
	Key  string
	Cert string
	Root string
}

// HTTPConfig is used to specify the host/port/tls for an HTTP server
type HTTPConfig struct {
	Host         string
	Port         int
	TLS          TLSConf
	ReadTimeout  int //milliseconds
	WriteTimeout int //milliseconds
}

// NATSConfig configuration for a NATS connection
type NATSConfig struct {
	Servers []string

	ConnectTimeout int //milliseconds
	ReconnectWait  int //milliseconds
	MaxReconnects  int

	TLS             TLSConf
	UserCredentials string
}

// StoreConfig is a catch-all for the store options, the store created
// depends on the contents of the config:
// if NSC is set the read-only NSC store is used
// if Dir is set a folder store is used, mutability is based on ReadOnly
// otherwise a memory store is used, mutability is based on ReadOnly (which means the r/o store will be stuck empty)
type StoreConfig struct {
	Dir             string // the path to a folder for mutable storage
	Shard           bool   // optional setting to shard the directory store, avoiding too many files in one folder
	CleanupInterval int    // interval at which expiration is checked

	NSC string // removed support for this, keep so that we can warn when used
}

// DefaultServerConfig generates a default configuration with
// logging set to colors, time, debug and trace
func DefaultServerConfig() *AccountServerConfig {
	return &AccountServerConfig{
		Logging: LogConfig{
			Colors: true,
			Time:   true,
			Debug:  false,
			Trace:  false,
		},
		HTTP: HTTPConfig{
			ReadTimeout:  5000,
			WriteTimeout: 5000,
			Host:         "localhost",
			Port:         9090,
		},
		NATS: NATSConfig{
			ConnectTimeout: 5000,
			ReconnectWait:  1000,
			MaxReconnects:  -1,
		},
		Store: StoreConfig{
			Dir:             ".",
			CleanupInterval: 0,
		}, // in memory store
		ReplicationTimeout: 5000,
		MaxReplicationPack: 10000,
		SignRequestTimeout: 1000,
	}
}
