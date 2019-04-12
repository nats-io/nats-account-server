package core

import (
	"crypto/tls"
	"fmt"
	"github.com/nats-io/nkeys"
	"net/http"
	"strings"
	"time"

	"github.com/nats-io/account-server/nats-account-server/conf"
	gnatsserver "github.com/nats-io/gnatsd/server"
	gnatsd "github.com/nats-io/gnatsd/test"
	nats "github.com/nats-io/go-nats"
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

	OperatorKey    nkeys.KeyPair
	OperatorPubKey string

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
}

// URLForPath converts a path to a full URL that matches the test server
func (ts *TestSetup) URLForPath(path string) string {
	if !strings.HasPrefix(path, "/") {
		path = fmt.Sprintf("/%s", path)
	}

	hostPort := ts.Server.hostPort

	return fmt.Sprintf("%s://%s%s", ts.Server.protocol, hostPort, path)
}

// SetupTestServer creates an operator, gnatsd, context, config and test http server with a router
// It also sets some required env vars to run tests.
func SetupTestServer(config conf.AccountServerConfig, useTLS bool) (*TestSetup, error) {
	var err error

	testSetup := &TestSetup{}

	opts := gnatsd.DefaultTestOptions
	opts.Port = -1

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

	natsURL := fmt.Sprintf("nats://localhost:%d", opts.Port)

	if useTLS {
		natsURL = fmt.Sprintf("tls://localhost:%d", opts.Port)
	}

	var nc *nats.Conn

	if useTLS {
		nc, err = nats.Connect(natsURL, nats.RootCAs(caFile))
	} else {
		nc, err = nats.Connect(natsURL)
	}

	if err != nil {
		return testSetup, err
	}
	testSetup.NC = nc

	operatorKey, err := nkeys.CreateOperator()
	if err != nil {
		return testSetup, err
	}

	opk, err := operatorKey.PublicKey()
	if err != nil {
		return testSetup, err
	}

	testSetup.OperatorKey = operatorKey
	testSetup.OperatorPubKey = opk

	config.Operator.TrustedKeys = []string{opk}
	config.NATS.Servers = []string{natsURL}

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
	server.LoadConfig(config)
	err = server.Start()

	if err != nil {
		return testSetup, err
	}

	testSetup.Server = server

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
