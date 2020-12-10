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
	"crypto/sha256"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	gnatsd "github.com/nats-io/nats-server/v2/test"

	natsserver "github.com/nats-io/nats-server/v2/server"

	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nats-account-server/server/conf"
	"github.com/nats-io/nats-account-server/server/store"
	nats "github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	"github.com/stretchr/testify/require"
)

// ErrJWTStore returns errors when possible
type ErrJWTStore struct {
	Loads  int
	Saves  int
	Closes int
}

// NewErrJWTStore returns an empty, mutable in-memory JWT store
func NewErrJWTStore() store.JWTStore {
	return &ErrJWTStore{}
}

// Load checks the memory store and returns the matching JWT or an error
func (store *ErrJWTStore) LoadAcc(publicKey string) (string, error) {
	store.Loads++
	return "", fmt.Errorf("always error")
}

// Save puts the JWT in a map by public key, no checks are performed
func (store *ErrJWTStore) SaveAcc(publicKey string, theJWT string) error {
	store.Saves++
	return fmt.Errorf("always error")
}

// Load checks the memory store and returns the matching JWT or an error
func (store *ErrJWTStore) LoadAct(publicKey string) (string, error) {
	return store.LoadAcc(publicKey)
}

// Save puts the JWT in a map by public key, no checks are performed
func (store *ErrJWTStore) SaveAct(publicKey string, theJWT string) error {
	return store.SaveAcc(publicKey, theJWT)
}

// IsReadOnly returns a flag determined at creation time
func (store *ErrJWTStore) IsReadOnly() bool {
	return false
}

// Close is a no-op for a mem store
func (store *ErrJWTStore) Close() {
	store.Closes++
}

func TestCoverageForPrintOnlyCallbacks(t *testing.T) {
	config := conf.DefaultServerConfig()
	testEnv, err := SetupTestServer(config, false, true)
	defer testEnv.Cleanup()
	require.NoError(t, err)

	server := testEnv.Server

	server.natsError(server.nats, nil, fmt.Errorf("coverage"))
	server.natsReconnected(server.nats)
	server.natsDisconnected(server.nats)
	server.natsDiscoveredServers(server.nats)
}

func TestCantConnectIfNotRunnning(t *testing.T) {
	config := conf.DefaultServerConfig()
	testEnv, err := SetupTestServer(config, false, true)
	require.NoError(t, err)

	server := testEnv.Server
	testEnv.Cleanup()

	require.Nil(t, server.nats)

	err = server.connectToNATS()
	require.NoError(t, err)

	require.Nil(t, server.nats)
}

func TestBadAccountNotification(t *testing.T) {
	config := conf.DefaultServerConfig()
	testEnv, err := SetupTestServer(config, false, true)
	defer testEnv.Cleanup()
	require.NoError(t, err)

	server := testEnv.Server

	server.JWTStore = NewErrJWTStore()
	errStore := server.JWTStore.(*ErrJWTStore)

	server.handleAccountNotification(&nats.Msg{
		Data:    []byte("hello"),
		Subject: "test",
	})
	require.Equal(t, 0, errStore.Loads)
}

func TestErrorCoverageOnAccountNotification(t *testing.T) {
	config := conf.DefaultServerConfig()
	testEnv, err := SetupTestServer(config, false, true)
	defer testEnv.Cleanup()
	require.NoError(t, err)

	server := testEnv.Server

	server.JWTStore = NewErrJWTStore()
	errStore := server.JWTStore.(*ErrJWTStore)

	operatorKey := testEnv.OperatorKey
	accountKey, err := nkeys.CreateAccount()
	require.NoError(t, err)

	pubKey, err := accountKey.PublicKey()
	require.NoError(t, err)

	account := jwt.NewAccountClaims(pubKey)
	acctJWT, err := account.Encode(operatorKey)
	require.NoError(t, err)

	server.handleAccountNotification(&nats.Msg{
		Data:    []byte(acctJWT),
		Subject: "test",
	})
	require.Equal(t, 0, errStore.Loads)
	require.Equal(t, 1, errStore.Saves)
	require.Equal(t, 0, errStore.Closes)
}

func TestAccountNotifyWithoutNatsOK(t *testing.T) {
	config := conf.DefaultServerConfig()
	testEnv, err := SetupTestServer(config, false, false)
	defer testEnv.Cleanup()
	require.NoError(t, err)

	server := testEnv.Server

	operatorKey := testEnv.OperatorKey
	accountKey, err := nkeys.CreateAccount()
	require.NoError(t, err)

	pubKey, err := accountKey.PublicKey()
	require.NoError(t, err)

	account := jwt.NewAccountClaims(pubKey)
	acctJWT, err := account.Encode(operatorKey)
	require.NoError(t, err)

	err = server.sendAccountNotification(account.Subject, []byte(acctJWT))
	require.NoError(t, err)
}

func TestBadActivationNotification(t *testing.T) {
	config := conf.DefaultServerConfig()
	testEnv, err := SetupTestServer(config, false, true)
	defer testEnv.Cleanup()
	require.NoError(t, err)

	server := testEnv.Server

	server.JWTStore = NewErrJWTStore()
	errStore := server.JWTStore.(*ErrJWTStore)

	server.handleActivationNotification(&nats.Msg{
		Data:    []byte("hello"),
		Subject: "test",
	})
	require.Equal(t, 0, errStore.Loads)
	require.Equal(t, 0, errStore.Saves)
	require.Equal(t, 0, errStore.Closes)
}

func TestActivationNotifyWithoutNatsOK(t *testing.T) {
	config := conf.DefaultServerConfig()
	testEnv, err := SetupTestServer(config, false, false)
	defer testEnv.Cleanup()
	require.NoError(t, err)

	server := testEnv.Server

	accountKey, err := nkeys.CreateAccount()
	require.NoError(t, err)
	account2Key, err := nkeys.CreateAccount()
	require.NoError(t, err)

	acctPubKey, err := accountKey.PublicKey()
	require.NoError(t, err)
	acct2PubKey, err := account2Key.PublicKey()
	require.NoError(t, err)

	expireAt := time.Now().Add(24 * time.Hour).Unix()
	act := jwt.NewActivationClaims(acct2PubKey)
	act.ImportType = jwt.Stream
	act.Name = "times"
	act.ImportSubject = "times.*"
	act.Expires = expireAt
	actJWT, err := act.Encode(accountKey)
	require.NoError(t, err)

	act, err = jwt.DecodeActivationClaims(actJWT)
	require.NoError(t, err)

	hash, err := act.HashID()
	require.NoError(t, err)

	err = server.sendActivationNotification(hash, acctPubKey, []byte(actJWT))
	require.NoError(t, err)
}

func TestStoreErrorCoverageOnActivationNotification(t *testing.T) {
	config := conf.DefaultServerConfig()
	testEnv, err := SetupTestServer(config, false, false)
	defer testEnv.Cleanup()
	require.NoError(t, err)

	server := testEnv.Server

	accountKey, err := nkeys.CreateAccount()
	require.NoError(t, err)
	account2Key, err := nkeys.CreateAccount()
	require.NoError(t, err)

	acct2PubKey, err := account2Key.PublicKey()
	require.NoError(t, err)

	expireAt := time.Now().Add(24 * time.Hour).Unix()
	act := jwt.NewActivationClaims(acct2PubKey)
	act.ImportType = jwt.Stream
	act.Name = "times"
	act.ImportSubject = "times.*"
	act.Expires = expireAt
	actJWT, err := act.Encode(accountKey)
	require.NoError(t, err)

	server.JWTStore = NewErrJWTStore()
	errStore := server.JWTStore.(*ErrJWTStore)

	server.handleActivationNotification(&nats.Msg{
		Data:    []byte(actJWT),
		Subject: "test",
	})
	require.Equal(t, 0, errStore.Loads)
	require.Equal(t, 1, errStore.Saves)
	require.Equal(t, 0, errStore.Closes)
}

func TestLookup(t *testing.T) {
	config := conf.DefaultServerConfig()
	testEnv, err := SetupTestServer(config, false, true)
	defer testEnv.Cleanup()
	require.NoError(t, err)

	accountKey, err := nkeys.CreateAccount()
	require.NoError(t, err)
	acctPubKey, err := accountKey.PublicKey()
	require.NoError(t, err)

	act := jwt.NewAccountClaims(acctPubKey)
	jwt, err := act.Encode(testEnv.OperatorKey)
	require.NoError(t, err)

	lock := sync.Mutex{}
	received := false

	url := testEnv.URLForPath(fmt.Sprintf("/jwt/v1/accounts/%s", acctPubKey))

	// test lookup without responder
	resp, err := testEnv.HTTP.Get(url)
	require.NoError(t, err)
	require.True(t, resp.StatusCode == http.StatusNotFound)

	// test lookup when there is a responder
	_, err = testEnv.NC.Subscribe(fmt.Sprintf(accountLookupRequest, "*"), func(m *nats.Msg) {
		lock.Lock()
		received = true
		m.Respond([]byte(jwt))
		lock.Unlock()
	})
	require.NoError(t, err)

	resp, err = testEnv.HTTP.Get(url)
	require.NoError(t, err)
	require.True(t, resp.StatusCode == http.StatusOK)
	lock.Lock()
	require.True(t, received)
	lock.Unlock()

	body, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, jwt, string(body))

	// test lookup when account is stored, received expected to remain false
	lock.Lock()
	received = false
	lock.Unlock()

	err = testEnv.Server.JWTStore.(*natsserver.DirJWTStore).SaveAcc(acctPubKey, jwt)
	require.NoError(t, err)

	resp, err = testEnv.HTTP.Get(url)
	require.NoError(t, err)
	require.True(t, resp.StatusCode == http.StatusOK)
	lock.Lock()
	require.False(t, received)
	lock.Unlock()

	body, err = ioutil.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, jwt, string(body))
}

func TestPack(t *testing.T) {
	config := conf.DefaultServerConfig()
	testEnv, err := SetupTestServer(config, false, true)
	defer testEnv.Cleanup()
	require.NoError(t, err)

	accountKey, err := nkeys.CreateAccount()
	require.NoError(t, err)
	acctPubKey, err := accountKey.PublicKey()
	require.NoError(t, err)

	act := jwt.NewAccountClaims(acctPubKey)
	jwt, err := act.Encode(testEnv.OperatorKey)
	require.NoError(t, err)
	jwtHash := sha256.Sum256([]byte(jwt))

	err = testEnv.Server.JWTStore.(*natsserver.DirJWTStore).SaveAcc(acctPubKey, jwt)
	require.NoError(t, err)

	respChan := make(chan *nats.Msg, 10)

	// test pack
	ib := testEnv.NC.NewRespInbox()
	sub, err := testEnv.NC.ChanSubscribe(ib, respChan)
	require.NoError(t, err)
	testEnv.NC.PublishRequest(accountPackRequest, ib, nil)
	m := <-respChan
	require.True(t, strings.HasPrefix(string(m.Data), acctPubKey+"|"))
	require.True(t, strings.HasSuffix(string(m.Data), jwt))
	m = <-respChan
	require.Equal(t, m.Data, []byte{})
	sub.Unsubscribe()

	// test pack while in sync
	ib = testEnv.NC.NewRespInbox()
	sub, err = testEnv.NC.ChanSubscribe(ib, respChan)
	require.NoError(t, err)
	testEnv.NC.PublishRequest(accountPackRequest, ib, jwtHash[:])
	m = <-respChan
	require.Equal(t, m.Data, []byte{})
	sub.Unsubscribe()

	close(respChan)
}

func createConfFile(t *testing.T, content []byte) string {
	t.Helper()
	conf, err := ioutil.TempFile("", "")
	if err != nil {
		t.Fatalf("Error creating conf file: %v", err)
	}
	fName := conf.Name()
	conf.Close()
	if err := ioutil.WriteFile(fName, content, 0666); err != nil {
		os.Remove(fName)
		t.Fatalf("Error writing conf file: %v", err)
	}
	return fName
}

func TestFullDirNatsResolver(t *testing.T) {
	config := conf.DefaultServerConfig()
	testEnv, err := SetupTestServer(config, false, false)
	defer testEnv.Cleanup()
	require.NoError(t, err)

	accKey1, err := nkeys.CreateAccount()
	require.NoError(t, err)
	acctPubKey1, err := accKey1.PublicKey()
	require.NoError(t, err)
	accC1 := jwt.NewAccountClaims(acctPubKey1)
	accJwt1, err := accC1.Encode(testEnv.OperatorKey)
	require.NoError(t, err)

	accKey2, err := nkeys.CreateAccount()
	require.NoError(t, err)
	acctPubKey2, err := accKey2.PublicKey()
	require.NoError(t, err)
	accC2 := jwt.NewAccountClaims(acctPubKey2)
	accJwt2, err := accC2.Encode(testEnv.OperatorKey)
	require.NoError(t, err)

	// store jwt in account server
	err = testEnv.Server.JWTStore.(*natsserver.DirJWTStore).SaveAcc(acctPubKey1, accJwt1)
	require.NoError(t, err)
	sysAccJwt, err := ioutil.ReadFile(testEnv.SystemAccountJWTFile)
	require.NoError(t, err)
	err = testEnv.Server.JWTStore.(*natsserver.DirJWTStore).SaveAcc(testEnv.SystemAccountPubKey, string(sysAccJwt))
	require.NoError(t, err)

	dirA, err := ioutil.TempDir("", "srv-a")
	defer os.RemoveAll(dirA)
	require.NoError(t, err)
	err = ioutil.WriteFile(fmt.Sprintf("%s%c%s.jwt", dirA, os.PathSeparator, acctPubKey2), []byte(accJwt2), 0666)
	require.NoError(t, err)
	confA := createConfFile(t, []byte(fmt.Sprintf(`
		listen: %d
		server_name: srv-A
		operator: %s
		system_account: %s
		resolver: {
			type: full
			dir: %s
			interval: "1s"
		}
    `, atomic.LoadUint64(&port)-1, testEnv.OperatorJWTFile, testEnv.SystemAccountPubKey, dirA)))
	defer os.Remove(confA)
	srv, _ := gnatsd.RunServerWithConfig(confA)
	defer srv.Shutdown()
	time.Sleep(3 * time.Second) // wait for account server and nats server to connect and converge
	// check if the nats server contains the files stored in the account server
	require.FileExists(t, fmt.Sprintf("%s%c%s.jwt", dirA, os.PathSeparator, testEnv.SystemAccountPubKey))
	require.FileExists(t, fmt.Sprintf("%s%c%s.jwt", dirA, os.PathSeparator, acctPubKey1))
	// check if the account server contains the files stored in the account server
	j, err := testEnv.Server.JWTStore.(*natsserver.DirJWTStore).LoadAcc(acctPubKey2)
	require.NoError(t, err)
	require.Equal(t, j, accJwt2)
}

func TestCacheDirNatsResolver(t *testing.T) {
	config := conf.DefaultServerConfig()
	testEnv, err := SetupTestServer(config, false, false)
	defer testEnv.Cleanup()
	require.NoError(t, err)

	accKey1, err := nkeys.CreateAccount()
	require.NoError(t, err)
	acctPubKey1, err := accKey1.PublicKey()
	require.NoError(t, err)
	accC1 := jwt.NewAccountClaims(acctPubKey1)
	accJwt1, err := accC1.Encode(testEnv.OperatorKey)
	require.NoError(t, err)
	usrKey, err := nkeys.CreateUser()
	require.NoError(t, err)
	userPub, err := usrKey.PublicKey()
	require.NoError(t, err)
	userClaim := jwt.NewUserClaims(userPub)
	userJwt, err := userClaim.Encode(accKey1)
	require.NoError(t, err)

	// store jwt in account server
	err = testEnv.Server.JWTStore.(*natsserver.DirJWTStore).SaveAcc(acctPubKey1, accJwt1)
	require.NoError(t, err)
	sysAccJwt, err := ioutil.ReadFile(testEnv.SystemAccountJWTFile)
	require.NoError(t, err)
	err = testEnv.Server.JWTStore.(*natsserver.DirJWTStore).SaveAcc(testEnv.SystemAccountPubKey, string(sysAccJwt))
	require.NoError(t, err)

	port := atomic.LoadUint64(&port) - 1
	dirA, err := ioutil.TempDir("", "srv-a")
	defer os.RemoveAll(dirA)
	require.NoError(t, err)
	confA := createConfFile(t, []byte(fmt.Sprintf(`
		listen: %d
		server_name: srv-A
		operator: %s
		system_account: %s
		resolver: {
			type: cache
			dir: %s
		}
    `, port, testEnv.OperatorJWTFile, testEnv.SystemAccountPubKey, dirA)))
	defer os.Remove(confA)
	srv, _ := gnatsd.RunServerWithConfig(confA)
	defer srv.Shutdown()
	time.Sleep(4 * time.Second) // wait for account server and nats server to connect
	// check if the nats server contains the files stored in the account server
	// the system account lookup is initiated automatically
	require.FileExists(t, fmt.Sprintf("%s%c%s.jwt", dirA, os.PathSeparator, testEnv.SystemAccountPubKey))
	// will exist, after connect
	require.NoFileExists(t, fmt.Sprintf("%s%c%s.jwt", dirA, os.PathSeparator, acctPubKey1))
	nc, err := nats.Connect(fmt.Sprintf("localhost:%d", port), nats.UserJWT(
		func() (string, error) {
			return userJwt, nil
		}, func(nonce []byte) ([]byte, error) {
			sig, _ := usrKey.Sign(nonce)
			return sig, nil
		}))
	require.NoError(t, err)
	nc.Close()
	require.FileExists(t, fmt.Sprintf("%s%c%s.jwt", dirA, os.PathSeparator, acctPubKey1))
}
