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

package conf

import (
	"github.com/nats-io/account-server/nats-account-server/logging"
)

// AccountServerConfig is the root structure for an account server configuration file.
type AccountServerConfig struct {
	Logging logging.Config
	NATS    NATSConfig
	HTTP    HTTPConfig
	Store   StoreConfig

	OperatorJWTPath      string
	SystemAccountJWTPath string
}

// TLSConf holds the configuration for a TLS connection/server
type TLSConf struct {
	Key  string
	Cert string
	Root string
}

// HTTPConfig is used to specify the host/port/tls for an HTTP server
type HTTPConfig struct {
	HTTP         HostPort
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
	NSC      string // an nsc operator folder
	Dir      string // the path to a folder for mutable storage
	ReadOnly bool   // flag to indicate read-only status
}

// DefaultServerConfig generates a default configuration with
// logging set to colors, time, debug and trace
func DefaultServerConfig() AccountServerConfig {
	return AccountServerConfig{
		Logging: logging.Config{
			Colors: true,
			Time:   true,
			Debug:  false,
			Trace:  false,
		},
		HTTP: HTTPConfig{
			ReadTimeout:  5000,
			WriteTimeout: 5000,
		},
		NATS: NATSConfig{
			ConnectTimeout: 5000,
			ReconnectWait:  1000,
			MaxReconnects:  0,
		},
		Store: StoreConfig{}, // in memory store
	}
}
