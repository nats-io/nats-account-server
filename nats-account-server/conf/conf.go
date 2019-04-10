package conf

import (
	"github.com/nats-io/account-server/nats-account-server/logging"
)

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
	ReadTimeout  int //Seconds
	WriteTimeout int //Seconds
}

// NATSConfig configuration for a NATS connection
type NATSConfig struct {
	Servers []string

	ConnectTimeout int //milliseconds
	ReconnectWait  int //milliseconds
	MaxReconnects  int

	TLS      TLSConf
	Username string
	Password string
}

// AccountServerConfig is the root structure for an account server configuration file.
type AccountServerConfig struct {
	Logging logging.Config
	NATS    NATSConfig
	HTTP    HTTPConfig
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
	}
}
