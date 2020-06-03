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
	"testing"
	"time"

	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nats-account-server/server/conf"
	"github.com/nats-io/nats-account-server/server/store"
	nats "github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	"github.com/stretchr/testify/require"
)

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

	server.jwtStore = store.NewErrJWTStore()
	errStore := server.jwtStore.(*store.ErrJWTStore)

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

	server.jwtStore = store.NewErrJWTStore()
	errStore := server.jwtStore.(*store.ErrJWTStore)

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

	err = server.sendAccountNotification(account, []byte(acctJWT))
	require.NoError(t, err)
}

func TestBadActivationNotification(t *testing.T) {
	config := conf.DefaultServerConfig()
	testEnv, err := SetupTestServer(config, false, true)
	defer testEnv.Cleanup()
	require.NoError(t, err)

	server := testEnv.Server

	server.jwtStore = store.NewErrJWTStore()
	errStore := server.jwtStore.(*store.ErrJWTStore)

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

	server.jwtStore = store.NewErrJWTStore()
	errStore := server.jwtStore.(*store.ErrJWTStore)

	server.handleActivationNotification(&nats.Msg{
		Data:    []byte(actJWT),
		Subject: "test",
	})
	require.Equal(t, 0, errStore.Loads)
	require.Equal(t, 1, errStore.Saves)
	require.Equal(t, 0, errStore.Closes)
}
