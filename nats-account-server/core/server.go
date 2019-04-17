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

package core

import (
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/nats-io/account-server/nats-account-server/conf"
	"github.com/nats-io/account-server/nats-account-server/logging"
	"github.com/nats-io/account-server/nats-account-server/store"
	nats "github.com/nats-io/go-nats"
	"github.com/nats-io/jwt"
)

var version = "0.0-dev"

// AccountServer is the core structure for the server.
type AccountServer struct {
	sync.Mutex
	running bool

	startTime time.Time

	logger logging.Logger
	config conf.AccountServerConfig

	nats      *nats.Conn
	natsTimer *time.Timer

	listener net.Listener
	http     *http.Server
	protocol string
	port     int
	hostPort string

	jwtStore            store.JWTStore
	trustedKeys         []string
	systemAccountClaims *jwt.AccountClaims
	systemAccountJWT    string
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

func (server *AccountServer) checkRunning() bool {
	server.Lock()
	defer server.Unlock()
	return server.running
}

// InitializeFromFlags is called from main to configure the server, the server
// will decide what needs to happen based on the flags. On reload the same flags are
// passed
func (server *AccountServer) InitializeFromFlags(flags Flags) error {
	server.config = conf.DefaultServerConfig()

	if flags.ConfigFile != "" {
		return server.ApplyConfigFile(flags.ConfigFile)
	}

	if flags.NSCFolder != "" {
		server.config.Store = conf.StoreConfig{
			NSC: flags.NSCFolder,
		}

		operatorName := filepath.Base(flags.NSCFolder)
		operatorPath := filepath.Join(flags.NSCFolder, fmt.Sprintf("%s.jwt", operatorName))

		server.config.OperatorJWTPath = operatorPath
	} else if flags.Directory != "" {
		server.config.Store = conf.StoreConfig{
			Dir:      flags.Directory,
			ReadOnly: false,
		}
	}

	if flags.NATSURL != "" {
		server.config.NATS.Servers = []string{flags.NATSURL}
	}

	if flags.Creds != "" {
		server.config.NATS.UserCredentials = flags.Creds
	}

	if flags.Debug || flags.DebugAndVerbose {
		server.config.Logging.Debug = true
	}

	if flags.Verbose || flags.DebugAndVerbose {
		server.config.Logging.Trace = true
	}

	return nil
}

// ApplyConfigFile applies the config file to the server's config
func (server *AccountServer) ApplyConfigFile(configFile string) error {
	config := server.config

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

	return nil
}

// InitializeFromConfig initialize the server's configuration to an existing config object, useful for tests
// Does not change the config at all, use DefaultServerConfig() to create a default config
func (server *AccountServer) InitializeFromConfig(config conf.AccountServerConfig) error {
	server.config = config
	return nil
}

// Start the server, will lock the server, assumes the config is loaded
func (server *AccountServer) Start() error {
	server.Lock()
	defer server.Unlock()

	if server.logger != nil {
		server.logger.Close()
	}

	server.running = true
	server.startTime = time.Now()
	server.logger = logging.NewNATSLogger(server.config.Logging)

	server.logger.Noticef("starting NATS Account server, version %s", version)
	server.logger.Noticef("server time is %s", server.startTime.Format(time.UnixDate))

	if err := server.initializeTrustedKeys(); err != nil {
		return err
	}

	if err := server.initializeSystemAccount(); err != nil {
		return err
	}

	store, err := server.createStore()

	if err != nil {
		return err
	}

	server.jwtStore = store

	if err := server.connectToNATS(); err != nil {
		return err
	}

	if err := server.startHTTP(); err != nil {
		return err
	}

	return nil
}

func (server *AccountServer) createStore() (store.JWTStore, error) {
	config := server.config.Store

	if config.NSC != "" {
		server.logger.Noticef("creating a read-only store for the NSC folder at %s", config.NSC)
		return store.NewNSCJWTStore(config.NSC)
	}

	if config.Dir != "" {
		if config.ReadOnly {
			server.logger.Noticef("creating a read-only store at %s", config.Dir)
			return store.NewImmutableDirJWTStore(config.Dir)
		}

		server.logger.Noticef("creating a store at %s", config.Dir)
		return store.NewDirJWTStore(config.Dir, true)
	}

	if config.ReadOnly {
		server.logger.Noticef("creating a read-only, empty, in-memory store")
		return store.NewImmutableMemJWTStore(map[string]string{}), nil
	}

	server.logger.Noticef("creating an in-memory store")
	return store.NewMemJWTStore(), nil
}

func (server *AccountServer) initializeTrustedKeys() error {
	opPath := server.config.OperatorJWTPath

	if opPath == "" {
		return nil
	}

	server.logger.Noticef("loading operator from %s", opPath)

	data, err := ioutil.ReadFile(opPath)
	if err != nil {
		return err
	}

	operatorJWT, err := jwt.DecodeOperatorClaims(string(data))
	if err != nil {
		return err
	}

	keys := []string{}

	keys = append(keys, operatorJWT.Subject)
	keys = append(keys, operatorJWT.SigningKeys...)

	server.trustedKeys = keys

	return nil
}

func (server *AccountServer) initializeSystemAccount() error {
	jwtPath := server.config.SystemAccountJWTPath

	if jwtPath == "" {
		return nil
	}

	server.logger.Noticef("loading system account from %s", jwtPath)

	data, err := ioutil.ReadFile(jwtPath)
	if err != nil {
		return err
	}

	systemAccount, err := jwt.DecodeAccountClaims(string(data))
	if err != nil {
		return err
	}

	server.systemAccountClaims = systemAccount
	server.systemAccountJWT = string(data)

	return nil
}

// Stop the account server
func (server *AccountServer) Stop() {
	server.Lock()
	defer server.Unlock()

	if !server.running {
		return // already stopped
	}

	server.logger.Noticef("stopping account server")

	server.running = false

	if server.natsTimer != nil {
		server.natsTimer.Stop()
	}

	if server.nats != nil {
		server.nats.Close()
		server.logger.Noticef("disconnected from NATS")
	}

	server.stopHTTP()
}

// FatalError stops the server, prints the messages and exits
func (server *AccountServer) FatalError(format string, args ...interface{}) {
	server.Stop()
	log.Fatalf(format, args...)
	os.Exit(-1)
}
