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
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/nats-io/jwt"
	"github.com/nats-io/nats-account-server/server/conf"
	"github.com/nats-io/nkeys"
	"github.com/stretchr/testify/require"
)

func TestGetReplicatedAccountJWT(t *testing.T) {
	testEnv, err := SetupTestServer(conf.DefaultServerConfig(), false, true)
	defer testEnv.Cleanup()
	require.NoError(t, err)

	// Put an account on the main server
	accountKey, err := nkeys.CreateAccount()
	require.NoError(t, err)

	pubKey, err := accountKey.PublicKey()
	require.NoError(t, err)

	account := jwt.NewAccountClaims(pubKey)
	account.Expires = time.Now().Add(24 * time.Hour).Unix()
	acctJWT, err := account.Encode(testEnv.OperatorKey)
	require.NoError(t, err)

	path := fmt.Sprintf("/jwt/v1/accounts/%s", pubKey)
	url := testEnv.URLForPath(path)

	resp, err := testEnv.HTTP.Post(url, "application/json", bytes.NewBuffer([]byte(acctJWT)))
	require.NoError(t, err)
	require.True(t, resp.StatusCode == http.StatusOK)

	// Get the URL from the main server
	resp, err = testEnv.HTTP.Get(url)
	require.NoError(t, err)
	require.True(t, resp.StatusCode == http.StatusOK)
	body, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)

	savedJWT := string(body)

	// Now start up the replica
	replica, err := testEnv.CreateReplica("")
	require.NoError(t, err)
	defer replica.Stop()

	// Try to get the account from the replica

	url = fmt.Sprintf("%s://%s%s", replica.protocol, replica.hostPort, path)
	resp, err = testEnv.HTTP.Get(url)
	require.NoError(t, err)
	require.True(t, resp.StatusCode == http.StatusOK)
	body, err = ioutil.ReadAll(resp.Body)
	require.NoError(t, err)

	replicatedJWT := string(body)

	require.Equal(t, savedJWT, replicatedJWT)

	// Update the account
	account.Tags = append(account.Tags, "alpha")
	acctJWT, err = account.Encode(testEnv.OperatorKey)
	require.NoError(t, err)

	url = testEnv.URLForPath(path)
	resp, err = testEnv.HTTP.Post(url, "application/json", bytes.NewBuffer([]byte(acctJWT)))
	require.NoError(t, err)
	require.True(t, resp.StatusCode == http.StatusOK)

	// Let the nats notification propagate
	time.Sleep(3 * time.Second)

	// Check that they match
	resp, err = testEnv.HTTP.Get(url)
	require.NoError(t, err)
	require.True(t, resp.StatusCode == http.StatusOK)
	body, err = ioutil.ReadAll(resp.Body)
	require.NoError(t, err)

	savedJWT = string(body)

	url = fmt.Sprintf("%s://%s%s", replica.protocol, replica.hostPort, path)
	resp, err = testEnv.HTTP.Get(url)
	require.NoError(t, err)
	require.True(t, resp.StatusCode == http.StatusOK)
	body, err = ioutil.ReadAll(resp.Body)
	require.NoError(t, err)

	replicatedJWT = string(body)

	require.Equal(t, savedJWT, replicatedJWT)

	// set the pub key to stale
	replica.validUntil[pubKey] = time.Now().Add(-time.Hour)

	resp, err = testEnv.HTTP.Get(url)
	require.NoError(t, err)
	require.True(t, resp.StatusCode == http.StatusOK)
	body, err = ioutil.ReadAll(resp.Body)
	require.NoError(t, err)

	forcedReload := string(body)

	require.Equal(t, savedJWT, forcedReload)
}

func TestReplicationDefaultsToOnDisk(t *testing.T) {
	testEnv, err := SetupTestServer(conf.DefaultServerConfig(), false, true)
	defer testEnv.Cleanup()
	require.NoError(t, err)

	// Put an account on the main server
	accountKey, err := nkeys.CreateAccount()
	require.NoError(t, err)

	pubKey, err := accountKey.PublicKey()
	require.NoError(t, err)

	account := jwt.NewAccountClaims(pubKey)
	acctJWT, err := account.Encode(testEnv.OperatorKey)
	require.NoError(t, err)

	path := fmt.Sprintf("/jwt/v1/accounts/%s", pubKey)
	url := testEnv.URLForPath(path)

	resp, err := testEnv.HTTP.Post(url, "application/json", bytes.NewBuffer([]byte(acctJWT)))
	require.NoError(t, err)
	require.True(t, resp.StatusCode == http.StatusOK)

	// Now start up the replica
	replica, err := testEnv.CreateReplica("")
	require.NoError(t, err)
	defer replica.Stop()

	// Fill the cache
	url = fmt.Sprintf("%s://%s%s", replica.protocol, replica.hostPort, path)
	resp, err = testEnv.HTTP.Get(url)
	require.NoError(t, err)
	require.True(t, resp.StatusCode == http.StatusOK)
	body, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)

	replicatedJWT := string(body)

	// Turn off the main server
	testEnv.Server.Stop()

	// Get the JWT again
	resp, err = testEnv.HTTP.Get(url)
	require.NoError(t, err)
	require.True(t, resp.StatusCode == http.StatusOK)
	body, err = ioutil.ReadAll(resp.Body)
	require.NoError(t, err)

	primaryDown := string(body)

	require.Equal(t, replicatedJWT, primaryDown)

	// remove the pub key from the cache tracker
	delete(replica.validUntil, pubKey)

	resp, err = testEnv.HTTP.Get(url)
	require.NoError(t, err)
	require.True(t, resp.StatusCode == http.StatusOK)
	body, err = ioutil.ReadAll(resp.Body)
	require.NoError(t, err)

	forcedToUseDisk := string(body)

	require.Equal(t, replicatedJWT, forcedToUseDisk)
}

func TestReplicatedFromDir(t *testing.T) {
	testEnv, err := SetupTestServer(conf.DefaultServerConfig(), false, true)
	defer testEnv.Cleanup()
	require.NoError(t, err)

	// Put an account on the main server
	accountKey, err := nkeys.CreateAccount()
	require.NoError(t, err)

	pubKey, err := accountKey.PublicKey()
	require.NoError(t, err)

	account := jwt.NewAccountClaims(pubKey)
	acctJWT, err := account.Encode(testEnv.OperatorKey)
	require.NoError(t, err)

	path := fmt.Sprintf("/jwt/v1/accounts/%s", pubKey)
	url := testEnv.URLForPath(path)

	resp, err := testEnv.HTTP.Post(url, "application/json", bytes.NewBuffer([]byte(acctJWT)))
	require.NoError(t, err)
	require.True(t, resp.StatusCode == http.StatusOK)

	// Now start up the replica
	tempDir, err := ioutil.TempDir(os.TempDir(), "prefix")
	require.NoError(t, err)

	replica, err := testEnv.CreateReplica(tempDir)
	require.NoError(t, err)
	defer replica.Stop()

	// Fill the cache
	url = fmt.Sprintf("%s://%s%s", replica.protocol, replica.hostPort, path)
	resp, err = testEnv.HTTP.Get(url)
	require.NoError(t, err)
	require.True(t, resp.StatusCode == http.StatusOK)
	body, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)

	replicatedJWT := string(body)

	// Turn off the main server
	testEnv.Server.Stop()

	// Get the JWT again
	resp, err = testEnv.HTTP.Get(url)
	require.NoError(t, err)
	require.True(t, resp.StatusCode == http.StatusOK)
	body, err = ioutil.ReadAll(resp.Body)
	require.NoError(t, err)

	primaryDown := string(body)

	require.Equal(t, replicatedJWT, primaryDown)

	// remove the pub key from the cache tracker
	delete(replica.validUntil, pubKey)

	resp, err = testEnv.HTTP.Get(url)
	require.NoError(t, err)
	require.True(t, resp.StatusCode == http.StatusOK)
	body, err = ioutil.ReadAll(resp.Body)
	require.NoError(t, err)

	forcedToUseDisk := string(body)

	require.Equal(t, replicatedJWT, forcedToUseDisk)
}

func TestGetReplicatedActivationJWT(t *testing.T) {
	testEnv, err := SetupTestServer(conf.DefaultServerConfig(), false, true)
	defer testEnv.Cleanup()
	require.NoError(t, err)

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

	act, err = jwt.DecodeActivationClaims(actJWT)
	require.NoError(t, err)

	hash, err := act.HashID()
	require.NoError(t, err)

	url := testEnv.URLForPath("/jwt/v1/activations")
	resp, err := testEnv.HTTP.Post(url, "application/json", bytes.NewBuffer([]byte(actJWT)))
	require.NoError(t, err)
	require.True(t, resp.StatusCode == http.StatusOK)

	// Get the URL from the main server
	path := fmt.Sprintf("/jwt/v1/activations/%s", hash)
	url = testEnv.URLForPath(path)
	resp, err = testEnv.HTTP.Get(url)
	require.NoError(t, err)
	require.True(t, resp.StatusCode == http.StatusOK)
	body, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)

	savedJWT := string(body)

	// Now start up the replica
	replica, err := testEnv.CreateReplica("")
	require.NoError(t, err)
	defer replica.Stop()

	// Try to get the account from the replica

	url = fmt.Sprintf("%s://%s%s", replica.protocol, replica.hostPort, path)
	resp, err = testEnv.HTTP.Get(url)
	require.NoError(t, err)
	require.True(t, resp.StatusCode == http.StatusOK)
	body, err = ioutil.ReadAll(resp.Body)
	require.NoError(t, err)

	replicatedJWT := string(body)

	require.Equal(t, savedJWT, replicatedJWT)

	// Update the jwt
	expireAt = time.Now().Add(48 * time.Hour).Unix()
	act.Expires = expireAt
	actJWT, err = act.Encode(accountKey)
	require.NoError(t, err)

	url = testEnv.URLForPath("/jwt/v1/activations")
	resp, err = testEnv.HTTP.Post(url, "application/json", bytes.NewBuffer([]byte(actJWT)))
	require.NoError(t, err)
	require.True(t, resp.StatusCode == http.StatusOK)

	// Let the nats notification propagate
	time.Sleep(3 * time.Second)

	// Check that they match
	path = fmt.Sprintf("/jwt/v1/activations/%s", hash)
	url = testEnv.URLForPath(path)

	resp, err = testEnv.HTTP.Get(url)
	require.NoError(t, err)
	require.True(t, resp.StatusCode == http.StatusOK)
	body, err = ioutil.ReadAll(resp.Body)
	require.NoError(t, err)

	savedJWT = string(body)

	url = fmt.Sprintf("%s://%s%s", replica.protocol, replica.hostPort, path)
	resp, err = testEnv.HTTP.Get(url)
	require.NoError(t, err)
	require.True(t, resp.StatusCode == http.StatusOK)
	body, err = ioutil.ReadAll(resp.Body)
	require.NoError(t, err)

	replicatedJWT = string(body)

	require.Equal(t, savedJWT, replicatedJWT)

	// set the hash to stale
	replica.validUntil[hash] = time.Now().Add(-time.Hour)

	resp, err = testEnv.HTTP.Get(url)
	require.NoError(t, err)
	require.True(t, resp.StatusCode == http.StatusOK)
	body, err = ioutil.ReadAll(resp.Body)
	require.NoError(t, err)

	forcedReload := string(body)

	require.Equal(t, savedJWT, forcedReload)
}

// Test that we don't panic if the primary is down or bad
func TestReplicatedStartup(t *testing.T) {
	testEnv, err := SetupTestServer(conf.DefaultServerConfig(), false, true)
	defer testEnv.Cleanup()
	require.NoError(t, err)

	testEnv.Server.Stop()

	// Start up the replica with no server
	replica, err := testEnv.CreateReplica("")
	require.NoError(t, err)
	replica.Stop()

	server := http.Server{
		Addr: testEnv.Server.hostPort,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
		}),
	}

	go server.ListenAndServe()
	defer server.Shutdown(context.Background())

	// Start up the replica with a bad server
	replica, err = testEnv.CreateReplica("")
	require.NoError(t, err)
	replica.Stop()
}
