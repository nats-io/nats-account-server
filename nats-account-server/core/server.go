package core

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/nats-io/account-server/nats-account-server/conf"
	"github.com/nats-io/account-server/nats-account-server/logging"
)

var version = "0.0-dev"

// AccountServer is the core structure for the server.
type AccountServer struct {
	runningLock sync.Mutex
	running     bool

	startTime time.Time

	logger logging.Logger
	config conf.AccountServerConfig
}

// NewAccountServer creates a new account server with a default logger
func NewAccountServer() *AccountServer {
	return &AccountServer{
		logger: logging.NewNATSLogger(logging.Config{
			Colors: true,
			Time:   true,
			Debug:  true,
			Trace:  true,
		}),
	}
}

// Logger hosts a shared logger
func (server *AccountServer) Logger() logging.Logger {
	return server.logger
}

// LoadConfigFile initialize the server's configuration from a file
func (server *AccountServer) LoadConfigFile(configFile string) error {
	config := conf.DefaultServerConfig()

	if configFile == "" {
		configFile = os.Getenv("NATS_ACCOUNT_SERVER_CONFIG")
		if configFile != "" {
			server.logger.Noticef("using config specified in $NATS_ACCOUNT_SERVER_CONFIG %q", configFile)
		}
	} else {
		server.logger.Noticef("loading configuration from %q", configFile)
	}

	if configFile == "" {
		return fmt.Errorf("no config file specified")
	}

	if err := conf.LoadConfigFromFile(configFile, &config, false); err != nil {
		return err
	}

	server.config = config
	return nil
}

// LoadConfig initialize the server's configuration to an existing config object, useful for tests
// Does not initialize the config at all, use DefaultServerConfig() to create a default config
func (server *AccountServer) LoadConfig(config conf.AccountServerConfig) error {
	server.config = config
	return nil
}

// Start the server, will lock the server, assumes the config is loaded
func (server *AccountServer) Start() error {
	server.runningLock.Lock()
	defer server.runningLock.Unlock()

	if server.logger != nil {
		server.logger.Close()
	}

	server.running = true
	server.startTime = time.Now()
	server.logger = logging.NewNATSLogger(server.config.Logging)

	server.logger.Noticef("starting NATS Account server, version %s", version)
	server.logger.Noticef("server time is %s", server.startTime.Format(time.UnixDate))

	/*
		if err := server.connectToNATS(); err != nil {
			return err
		}

		if err := server.connectToSTAN(); err != nil {
			return err
		}

		if err := server.initializeConnectors(); err != nil {
			return err
		}

		if err := server.startConnectors(); err != nil {
			return err
		}

		if err := server.startMonitoring(); err != nil {
			return err
		}*/

	return nil
}

// Stop the account server
func (server *AccountServer) Stop() {
	server.runningLock.Lock()
	defer server.runningLock.Unlock()

	if !server.running {
		return // already stopped
	}

	server.logger.Noticef("stopping account server")

	server.running = false
}
