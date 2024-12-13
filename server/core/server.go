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

package core

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/jwt/v2" // only used to decode jwt subjects
	"github.com/nats-io/nats-account-server/server/conf"
	"github.com/nats-io/nats-account-server/server/store"
	srvlogger "github.com/nats-io/nats-server/v2/logger"
	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
)

const version = "1.0.0"

// AccountServer is the core structure for the server.
type AccountServer struct {
	sync.Mutex
	running bool

	startTime time.Time

	logger natsserver.Logger
	config *conf.AccountServerConfig

	respSeqNo    int64
	nats         *nats.Conn
	natsTimer    *time.Timer
	shutdownNats func()

	listener net.Listener
	http     *http.Server
	protocol string
	port     int
	hostPort string

	store.JWTStore
	jwt JwtHandler
	id  string
}

// NewAccountServer creates a new account server with a default logger
func NewAccountServer() *AccountServer {
	kp, _ := nkeys.CreateServer()
	pub, _ := kp.PublicKey()
	ac := &AccountServer{
		logger: NewNilLogger(),
		id:     pub,
	}
	return ac
}

func (server *AccountServer) Config() *conf.AccountServerConfig {
	return server.config
}

// ConfigureLogger configures the logger for this account server
func (server *AccountServer) ConfigureLogger() natsserver.Logger {
	opts := server.config.Logging
	if opts.Custom != nil {
		return opts.Custom
	}
	if isWindowsService() {
		srvlogger.SetSyslogName("NatsAccountServer")
		return srvlogger.NewSysLogger(opts.Debug, opts.Trace)
	}
	return srvlogger.NewStdLogger(opts.Time, opts.Debug, opts.Trace, opts.Colors, opts.PID)
}

// Logger hosts a shared logger
func (server *AccountServer) Logger() natsserver.Logger {
	server.Lock()
	defer server.Unlock()
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
		if err := server.ApplyConfigFile(flags.ConfigFile); err != nil {
			return err
		}
	}
	server.logger = server.ConfigureLogger()

	if flags.Directory != "" {
		server.config.Store = conf.StoreConfig{
			Dir: flags.Directory,
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

	if flags.OperatorJWTPath != "" {
		server.config.OperatorJWTPath = flags.OperatorJWTPath
	}

	if flags.HostPort != "" {
		h, p, err := net.SplitHostPort(flags.HostPort)
		if err != nil {
			return fmt.Errorf("error parsing hostport: %v", err)
		}
		server.config.HTTP.Host = h
		server.config.HTTP.Port, err = strconv.Atoi(p)
		if err != nil {
			return fmt.Errorf("error parsing hostport: %v", err)

		}
	}

	if flags.Primary != "" {
		server.config.Primary = flags.Primary
	}

	return nil
}

// ApplyConfigFile applies the config file to the server's config
func (server *AccountServer) ApplyConfigFile(configFile string) error {
	if configFile == "" {
		return fmt.Errorf("no config file specified")
	}
	server.logger.Noticef("loading configuration from %q", configFile)

	if err := conf.LoadConfigFromFile(configFile, server.config, false); err != nil {
		return err
	}

	return nil
}

// InitializeFromConfig initialize the server's configuration to an existing config object, useful for tests
// Does not change the config at all, use DefaultServerConfig() to create a default config
func (server *AccountServer) InitializeFromConfig(config *conf.AccountServerConfig) error {
	server.config = config
	return nil
}

// Start the server, will lock the server, assumes the config is loaded
func (server *AccountServer) Start() error {
	server.Lock()
	defer server.Unlock()

	if server.logger != nil {
		if l, ok := server.logger.(io.Closer); ok {
			if err := l.Close(); err != nil {
				server.logger.Errorf("Error closing logger: %v", err)
			}
		}
	}

	server.running = true
	server.startTime = time.Now()

	server.logger.Noticef("starting NATS Account server, version %s", version)
	server.logger.Noticef("server time is %s", server.startTime.Format(time.UnixDate))

	server.jwt = NewJwtHandler(server.logger)

	store, err := server.createStore()
	if err != nil {
		return err
	} else {
		server.JWTStore = store
	}
	if len(server.config.NATS.Servers) > 0 {
		store = server
	}
	server.Unlock()
	err = server.initializeFromPrimary()
	server.Lock()
	if err != nil {
		return err
	}

	if err := server.connectToNATS(); err != nil {
		return err
	}

	var sign accountSignup
	if server.config.SignRequestSubject != "" {
		sign = server.accountSignatureRequest
	}
	if opJWT, err := server.readJWT(server.config.OperatorJWTPath, "operator"); err != nil {
		return err
	} else if sysJWT, err := server.readJWT(server.config.SystemAccountJWTPath, "system account"); err != nil {
		return err
	} else if err := server.jwt.Initialize(opJWT, sysJWT, store, server.config.MaxReplicationPack, server.sendAccountNotification, server.sendActivationNotification, sign); err != nil {
		return err
	}

	if err := server.startHTTP(); err != nil {
		return err
	}

	server.logger.Noticef("nats-account-server is running")
	server.logger.Noticef("configure the nats-server with:")

	// provide a more accurate resolver URL in the case where the port is self-assigned
	port := server.listener.Addr().(*net.TCPAddr).Port
	h, _, _ := net.SplitHostPort(server.hostPort)
	hp := fmt.Sprintf("%s:%d", h, port)
	server.logger.Noticef("  resolver: URL(%s://%s/jwt/v1/accounts/)", server.protocol, hp)

	return nil
}

func (server *AccountServer) jwtChangedCallback(pubKey string) {
	if nkeys.IsValidPublicAccountKey(pubKey) {
		server.Lock()
		jwtStore := server.JWTStore
		nc := server.nats
		server.Unlock()
		if nc == nil {
			return
		}
		theJWT, err := jwtStore.LoadAcc(pubKey)
		if err != nil {
			server.logger.Noticef("error trying to send notification from file change for %s, %s", ShortKey(pubKey), err.Error())
			return
		}

		decoded, err := jwt.DecodeAccountClaims(theJWT)
		if err != nil {
			server.logger.Noticef("error trying to send notification from file change for %s, %s", ShortKey(pubKey), err.Error())
			return
		}

		if err = server.sendAccountNotification(decoded.Subject, []byte(theJWT)); err != nil {
			server.logger.Noticef("error trying to send notification from file change for %s, %s", ShortKey(pubKey), err.Error())
			return
		}
	}
}

const commonErr = `
use a dedicated store directory and specify the operator jwt path instead
synchronize using: nsc push --all --account-jwt-server-url <account-server-host-port>/jwt/v1`

const RoError = `support for read only directory access with file system updates has been removed` + commonErr

const NscError = `support for direct access of the nsc folder has been removed` + commonErr

func (server *AccountServer) createStore() (store.JWTStore, error) {
	config := server.config.Store
	if config.NSC != "" {
		return nil, errors.New(NscError)
	}
	if config.ReadOnly {
		return nil, errors.New(RoError)
	}
	if config.Dir == "" {
		return nil, errors.New("store directory is required")
	}
	server.logger.Noticef("creating a store with cleanup functions at %s", config.Dir)
	return natsserver.NewExpiringDirJWTStore(config.Dir, config.Shard, true, natsserver.NoDelete,
		time.Duration(config.CleanupInterval)*time.Millisecond, 0, false, 0, server.jwtChangedCallback)
}

func (server *AccountServer) readJWT(opPath string, jwtType string) ([]byte, error) {
	if opPath == "" {
		return nil, nil
	}

	server.logger.Noticef("loading %s from %s", jwtType, opPath)

	if data, err := os.ReadFile(opPath); err != nil {
		return nil, err
	} else {
		return data, nil
	}
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

	shutdown := server.shutdownNats
	if shutdown != nil {
		server.Unlock()
		shutdown()
		server.Lock()
		server.nats = nil
	}

	server.stopHTTP()

	if server.JWTStore != nil {
		server.Close()
		server.logger.Noticef("closed JWT store")
	}
	server.jwt = NewJwtHandler(server.logger)
}

// this functionality is only used to initialize the server from an old server
func (server *AccountServer) initializeFromPrimary() error {
	primary := server.config.Primary
	if primary == "" {
		return nil
	}
	packer, ok := server.JWTStore.(store.PackableJWTStore)
	if !ok {
		server.logger.Noticef("skipping initial JWT pack from primary, configured store doesn't support it")
		return nil
	}

	if server.config.MaxReplicationPack == 0 {
		server.logger.Noticef("skipping initial JWT pack from primary, config has MaxReplicationPack of 0")
		return nil
	}

	server.logger.Noticef("grabbing initial JWT pack from primary %s", primary)

	primary = strings.TrimSuffix(primary, "/")

	url := fmt.Sprintf("%s/jwt/v1/pack?max=%d", primary, server.config.MaxReplicationPack)

	httpClient := &http.Client{
		Transport: &http.Transport{
			MaxIdleConnsPerHost: 1,
		},
		Timeout: time.Duration(server.config.ReplicationTimeout) * time.Millisecond,
	}

	resp, err := httpClient.Get(url)

	// if we can't contact the primary, fallback to what we have on disk
	if err != nil {
		server.logger.Noticef("unable to initialize from primary, %s, will use what is on disk", err.Error())
		return nil
	} else if resp != nil && resp.StatusCode != http.StatusOK {
		server.logger.Noticef("unable to initialize from primary, server returned status %q, will use what is on disk", resp.Status)
		return nil
	} else if resp == nil {
		server.logger.Noticef("unable to initialize from primary, http call didn't return a response, will use what is on disk")
		return nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if err := packer.Merge(string(body)); err != nil {
		return err
	}

	return nil
}

func (server *AccountServer) ReadyForConnections(dur time.Duration) bool {
	end := time.Now().Add(dur)
	for time.Now().Before(end) {
		server.Lock()
		ok := server.listener != nil
		server.Unlock()
		if ok {
			return true
		}
		time.Sleep(25 * time.Millisecond)
	}
	return false
}
