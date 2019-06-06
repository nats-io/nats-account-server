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
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/nats-io/jwt"
	"github.com/nats-io/nats-account-server/server/conf"
	gnatsserver "github.com/nats-io/nats-server/v2/server"
	gnatsd "github.com/nats-io/nats-server/v2/test"
	nats "github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	nsc "github.com/nats-io/nsc/cmd"
)

const (
	certFile = "../../resources/certs/server-cert.pem"
	keyFile  = "../../resources/certs/server-key.pem"
	caFile   = "../../resources/certs/ca.pem"
)

// TestSetup is used to return the useful elements of a test environment from SetupTestServer
type TestSetup struct {
	GNATSD *gnatsserver.Server
	NC     *nats.Conn
	Server *AccountServer

	OperatorKey     nkeys.KeyPair
	OperatorPubKey  string
	OperatorClaims  *jwt.OperatorClaims
	OperatorJWTFile string

	SystemAccount        nkeys.KeyPair
	SystemAccountPubKey  string
	SystemAccountClaims  *jwt.AccountClaims
	SystemAccountJWTFile string

	SystemUser          nkeys.KeyPair
	SystemUserPubKey    string
	SystemUserClaims    *jwt.UserClaims
	SystemUserCredsFile string

	HTTP *http.Client
}

// Cleanup closes down the test http server, gnatsd and nats connection
func (ts *TestSetup) Cleanup() {
	if ts.Server != nil {
		ts.Server.Stop()
	}
	if ts.NC != nil {
		ts.NC.Close()
	}
	if ts.GNATSD != nil {
		ts.GNATSD.Shutdown()
	}

	if ts.SystemUserCredsFile != "" {
		os.Remove(ts.SystemUserCredsFile)
	}

	if ts.SystemAccountJWTFile != "" {
		os.Remove(ts.SystemAccountJWTFile)
	}

	if ts.OperatorJWTFile != "" {
		os.Remove(ts.SystemUserCredsFile)
	}
}

// URLForPath converts a path to a full URL that matches the test server
func (ts *TestSetup) URLForPath(path string) string {
	if !strings.HasPrefix(path, "/") {
		path = fmt.Sprintf("/%s", path)
	}

	hostPort := ts.Server.hostPort

	return fmt.Sprintf("%s://%s%s", ts.Server.protocol, hostPort, path)
}

func (ts *TestSetup) initKeys() error {
	operatorKey, err := nkeys.CreateOperator()
	if err != nil {
		return err
	}

	opk, err := operatorKey.PublicKey()
	if err != nil {
		return err
	}

	ts.OperatorKey = operatorKey
	ts.OperatorPubKey = opk
	ts.OperatorClaims = jwt.NewOperatorClaims(opk)

	file, err := ioutil.TempFile(os.TempDir(), "operator")
	if err != nil {
		return err
	}

	opJWT, err := ts.OperatorClaims.Encode(operatorKey)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(file.Name(), []byte(opJWT), 0644)
	if err != nil {
		return err
	}

	ts.OperatorJWTFile = file.Name()

	accountKey, err := nkeys.CreateAccount()
	if err != nil {
		return err
	}

	apk, err := accountKey.PublicKey()
	if err != nil {
		return err
	}

	ts.SystemAccount = accountKey
	ts.SystemAccountPubKey = apk
	ts.SystemAccountClaims = jwt.NewAccountClaims(apk)

	file, err = ioutil.TempFile(os.TempDir(), "sysacct")
	if err != nil {
		return err
	}

	sysAcctJWT, err := ts.SystemAccountClaims.Encode(operatorKey)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(file.Name(), []byte(sysAcctJWT), 0644)
	if err != nil {
		return err
	}

	ts.SystemAccountJWTFile = file.Name()

	userKey, err := nkeys.CreateUser()
	if err != nil {
		return err
	}

	upk, err := userKey.PublicKey()
	if err != nil {
		return err
	}

	ts.SystemUser = userKey
	ts.SystemUserPubKey = upk
	ts.SystemUserClaims = jwt.NewUserClaims(upk)

	file, err = ioutil.TempFile(os.TempDir(), "sysuser")
	if err != nil {
		return err
	}

	userJWT, err := ts.SystemUserClaims.Encode(accountKey)
	if err != nil {
		return err
	}

	seed, err := userKey.Seed()
	if err != nil {
		return err
	}

	creds := nsc.FormatConfig("User", userJWT, string(seed))

	err = ioutil.WriteFile(file.Name(), creds, 0644)
	if err != nil {
		return err
	}

	ts.SystemUserCredsFile = file.Name()

	return nil
}

func (ts *TestSetup) CreateReplicaConfig(dir string) conf.AccountServerConfig {
	config := conf.DefaultServerConfig()
	config.Primary = ts.URLForPath("/")
	config.NATS = ts.Server.config.NATS
	config.HTTP.Port = int(atomic.AddUint64(&port, 1))
	config.OperatorJWTPath = ts.OperatorJWTFile
	config.SystemAccountJWTPath = ts.SystemAccountJWTFile
	config.Logging.Trace = true
	config.Logging.Debug = true
	config.Store.Dir = dir
	config.HTTP.TLS = ts.Server.config.HTTP.TLS
	return config
}

func (ts *TestSetup) CreateReplica(dir string) (*AccountServer, error) {
	config := ts.CreateReplicaConfig(dir)
	replica := NewAccountServer()
	replica.InitializeFromConfig(config)
	return replica, replica.Start()
}

var port = uint64(14222)

// SetupTestServer creates an operator, gnatsd, context, config and test http server with a router
// It also sets some required env vars to run tests.
func SetupTestServer(config conf.AccountServerConfig, useTLS bool, enableNats bool) (*TestSetup, error) {
	var err error

	testSetup := &TestSetup{}
	testSetup.initKeys()

	natsPort := atomic.AddUint64(&port, 1)
	natsURL := fmt.Sprintf("nats://localhost:%d", natsPort)

	config.HTTP.Port = int(atomic.AddUint64(&port, 1))

	config.OperatorJWTPath = testSetup.OperatorJWTFile
	config.SystemAccountJWTPath = testSetup.SystemAccountJWTFile

	config.Logging.Trace = true
	config.Logging.Debug = true

	config.NATS = conf.NATSConfig{
		Servers:         []string{natsURL},
		MaxReconnects:   -1, // keep trying, since we start account server before the gnatsd
		ReconnectWait:   100,
		UserCredentials: testSetup.SystemUserCredsFile,
	}

	if useTLS {
		config.HTTP.TLS = conf.TLSConf{
			Key:  keyFile,
			Cert: certFile,
		}
		config.NATS.TLS = conf.TLSConf{
			Root: caFile,
		}
	}

	server := NewAccountServer()
	server.InitializeFromConfig(config)
	err = server.Start()

	if err != nil {
		return testSetup, err
	}

	testSetup.Server = server

	if enableNats {
		opts := gnatsd.DefaultTestOptions
		opts.Port = int(natsPort)
		opts.TrustedKeys = append(opts.TrustedKeys, testSetup.OperatorPubKey)
		opts.SystemAccount = testSetup.SystemAccountPubKey

		urlResolver, err := gnatsserver.NewURLAccResolver(testSetup.URLForPath("/jwt/v1/accounts/"))
		if err != nil {
			return testSetup, err
		}
		opts.AccountResolver = urlResolver

		if useTLS {
			opts.TLSCert = certFile
			opts.TLSKey = keyFile
			opts.TLSTimeout = 5

			tc := gnatsserver.TLSConfigOpts{}
			tc.CertFile = opts.TLSCert
			tc.KeyFile = opts.TLSKey

			opts.TLSConfig, err = gnatsserver.GenTLSConfig(&tc)

			if err != nil {
				return testSetup, err
			}
		}

		testSetup.GNATSD = gnatsd.RunServer(&opts)

		if useTLS {
			natsURL = fmt.Sprintf("tls://localhost:%d", opts.Port)
		}

		var nc *nats.Conn

		if useTLS {
			nc, err = nats.Connect(natsURL, nats.RootCAs(caFile), nats.UserCredentials(testSetup.SystemUserCredsFile))
		} else {
			nc, err = nats.Connect(natsURL, nats.UserCredentials(testSetup.SystemUserCredsFile))
		}

		if err != nil {
			return testSetup, err
		}
		testSetup.NC = nc

		// wait for the server to get connected
		for i := 0; i < 5; i++ {
			if testSetup.Server.getNatsConnection() != nil {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}

		if testSetup.Server.nats == nil {
			return testSetup, fmt.Errorf("server didn't connect to nats")
		}
	}

	httpClient, err := testHTTPClient(useTLS)
	if err != nil {
		return testSetup, err
	}

	testSetup.HTTP = httpClient

	return testSetup, nil
}

func testHTTPClient(useTLS bool) (*http.Client, error) {
	timeout := time.Duration(5 * time.Second)
	tr := &http.Transport{
		MaxIdleConnsPerHost: 1,
	}

	if useTLS {
		tr.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: true,
		}
	}

	client := http.Client{
		Transport: tr,
		Timeout:   timeout,
	}

	return &client, nil
}
