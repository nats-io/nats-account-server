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
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/jwt"
	"github.com/nats-io/nats-account-server/server/conf"
	"github.com/nats-io/nats-account-server/server/store"
	srvlogger "github.com/nats-io/nats-server/v2/logger"
	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
)

const version = "0.8.6"

// AccountServer is the core structure for the server.
type AccountServer struct {
	sync.Mutex
	running bool

	startTime time.Time

	logger natsserver.Logger
	config *conf.AccountServerConfig

	nats      *nats.Conn
	natsTimer *time.Timer

	listener net.Listener
	http     *http.Server
	protocol string
	port     int
	hostPort string

	jwtStore            store.JWTStore
	trustedKeys         []string
	operatorJWT         string
	systemAccountClaims *jwt.AccountClaims
	systemAccountJWT    string

	// In replica mode the server uses a directory or memory for storage. Requests
	// are checked against the http cache settings and try to update from the primary
	// if necessary. However, if a version of the JWT is available in the persistent store
	// it will be returned if the primary is down, regarldess of the cache situation.
	primary    string
	cacheLock  sync.Mutex
	validUntil map[string]time.Time // map of pubkey to stale time
	httpClient *http.Client
}

// NewAccountServer creates a new account server with a default logger
func NewAccountServer() *AccountServer {
	ac := &AccountServer{
		logger: NewNilLogger(),
	}
	return ac
}

func (server *AccountServer) ConfigureLogger() natsserver.Logger {
	opts := server.config.Logging
	if isWindowsService() {
		srvlogger.SetSyslogName("nats-account-server")
		return srvlogger.NewSysLogger(opts.Debug, opts.Trace)
	} else {
		return srvlogger.NewStdLogger(opts.Time, opts.Debug, opts.Trace, opts.Colors, opts.PID)
	}
}

// Logger hosts a shared logger
func (server *AccountServer) Logger() natsserver.Logger {
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
	server.ConfigureLogger()

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
			ReadOnly: flags.ReadOnly,
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
	server.logger = server.ConfigureLogger()
	server.validUntil = map[string]time.Time{}

	server.logger.Noticef("starting NATS Account server, version %s", version)
	server.logger.Noticef("server time is %s", server.startTime.Format(time.UnixDate))

	server.httpClient = server.createHTTPClient()
	server.primary = server.config.Primary

	if server.primary != "" {
		server.logger.Noticef("starting in replicated mode, with primary at %s", server.primary)

		if len(server.config.NATS.Servers) == 0 {
			server.logger.Noticef("running in replicated mode without NATS notifications can result in delayed updates")
		}
	}

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

	if server.primary != "" {
		err := server.initializeFromPrimary()

		if err != nil {
			return err
		}
	}

	if err := server.startHTTP(); err != nil {
		return err
	}

	server.logger.Noticef("nats-account-server is running")
	server.logger.Noticef("configure the nats-server with:")
	server.logger.Noticef("  resolver: URL(%s://%s/jwt/v1/accounts/)", server.protocol, server.hostPort)

	return nil
}

func (server *AccountServer) jwtChangedCallback(pubKey string) {
	if nkeys.IsValidPublicAccountKey(pubKey) {
		theJWT, err := server.jwtStore.Load(pubKey)
		if err != nil {
			server.logger.Noticef("error trying to send notification from file change for %s, %s", ShortKey(pubKey), err.Error())
			return
		}

		decoded, err := jwt.DecodeAccountClaims(theJWT)
		if err != nil {
			server.logger.Noticef("error trying to send notification from file change for %s, %s", ShortKey(pubKey), err.Error())
			return
		}

		err = server.sendAccountNotification(decoded, []byte(theJWT))
		if err != nil {
			server.logger.Noticef("error trying to send notification from file change for %s, %s", ShortKey(pubKey), err.Error())
			return
		}
	}
}

func (server *AccountServer) storeErrorCallback(err error) {
	server.logger.Errorf("The NSC store encountered an error, shutting down ...")
	server.Stop()
}

func (server *AccountServer) createStore() (store.JWTStore, error) {
	config := server.config.Store

	if server.primary != "" && config.NSC != "" {
		return nil, fmt.Errorf("replicas cannot be run in NSC mode")
	}

	if server.primary != "" && config.ReadOnly {
		return nil, fmt.Errorf("replica mode cannot be used in read-only mode, but will not allow POST operations")
	}

	if config.Dir != "" {
		if config.ReadOnly {
			server.logger.Noticef("creating a read-only store at %s", config.Dir)
			return store.NewImmutableDirJWTStore(config.Dir, config.Shard, server.jwtChangedCallback, server.storeErrorCallback)
		}

		server.logger.Noticef("creating a store at %s", config.Dir)
		return store.NewDirJWTStore(config.Dir, config.Shard, true, nil, nil)
	}

	if config.NSC != "" {
		server.logger.Noticef("creating a read-only store for the NSC folder at %s", config.NSC)
		return store.NewNSCJWTStore(config.NSC, server.jwtChangedCallback, server.storeErrorCallback)
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
	server.operatorJWT = string(data)

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

func (server *AccountServer) createHTTPClient() *http.Client {
	timeout := time.Duration(time.Duration(server.config.ReplicationTimeout) * time.Millisecond)
	tr := &http.Transport{
		MaxIdleConnsPerHost: 1,
	}

	client := http.Client{
		Transport: tr,
		Timeout:   timeout,
	}

	return &client
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
		server.nats = nil
		server.logger.Noticef("disconnected from NATS")
	}

	server.stopHTTP()

	if server.jwtStore != nil {
		server.jwtStore.Close()
		server.jwtStore = nil
		server.logger.Noticef("closed JWT store")
	}
}

func (server *AccountServer) initializeFromPrimary() error {
	packer, ok := server.jwtStore.(store.PackableJWTStore)
	if !ok {
		server.logger.Noticef("skipping initial JWT pack from primary, configured store doesn't support it")
		return nil
	}

	if server.config.MaxReplicationPack == 0 {
		server.logger.Noticef("skipping initial JWT pack from primary, config has MaxReplicationPack of 0")
		return nil
	}

	server.logger.Noticef("grabbing initial JWT pack from primary %s", server.primary)
	primary := server.primary

	if strings.HasSuffix(primary, "/") {
		primary = primary[:len(primary)-1]
	}

	url := fmt.Sprintf("%s/jwt/v1/pack?max=%d", primary, server.config.MaxReplicationPack)

	resp, err := server.httpClient.Get(url)

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

	body, err := ioutil.ReadAll(resp.Body)
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
