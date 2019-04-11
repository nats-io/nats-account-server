package core

import (
	"fmt"
	"github.com/nats-io/nkeys"
	"strings"

	"github.com/nats-io/account-server/nats-account-server/conf"
	gnatsserver "github.com/nats-io/gnatsd/server"
	gnatsd "github.com/nats-io/gnatsd/test"
	nats "github.com/nats-io/go-nats"
)

// TestSetup is used to return the useful elements of a test environment from SetupTestServer
type TestSetup struct {
	GNATSD *gnatsserver.Server
	NC     *nats.Conn
	Server *AccountServer

	OperatorKey    nkeys.KeyPair
	OperatorPubKey string
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
	if hostPort == "" {
		hostPort = fmt.Sprintf("localhost:%d", ts.Server.port)
	}

	return fmt.Sprintf("%s://%s%s", ts.Server.protocol, hostPort, path)
}

// SetupTestServer creates an operator, gnatsd, context, config and test http server with a router
// It also sets some required env vars to run tests.
func SetupTestServer(config conf.AccountServerConfig) (*TestSetup, error) {
	var err error

	testSetup := &TestSetup{}

	opts := gnatsd.DefaultTestOptions
	opts.Port = -1
	testSetup.GNATSD = gnatsd.RunServer(&opts)
	natsURL := fmt.Sprintf("nats://localhost:%d", opts.Port)
	nc, err := nats.Connect(natsURL)
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

	server := NewAccountServer()
	server.LoadConfig(config)
	err = server.Start()

	if err != nil {
		return testSetup, err
	}

	testSetup.Server = server

	return testSetup, nil
}
