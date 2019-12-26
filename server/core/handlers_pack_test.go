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
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/nats-io/jwt"
	"github.com/nats-io/nats-account-server/server/conf"
	"github.com/nats-io/nats-account-server/server/store"
	"github.com/nats-io/nkeys"
	"github.com/stretchr/testify/require"
)

func TestPackJWTs(t *testing.T) {
	testEnv, err := SetupTestServer(conf.DefaultServerConfig(), false, false)
	defer testEnv.Cleanup()
	require.NoError(t, err)

	operatorKey := testEnv.OperatorKey

	pubKeys := map[string]string{}

	for i := 0; i < 100; i++ {
		accountKey, err := nkeys.CreateAccount()
		require.NoError(t, err)

		pubKey, err := accountKey.PublicKey()
		require.NoError(t, err)

		account := jwt.NewAccountClaims(pubKey)
		acctJWT, err := account.Encode(operatorKey)
		require.NoError(t, err)

		pubKeys[pubKey] = acctJWT

		path := fmt.Sprintf("/jwt/v1/accounts/%s", pubKey)
		url := testEnv.URLForPath(path)

		resp, err := testEnv.HTTP.Post(url, "application/json", bytes.NewBuffer([]byte(acctJWT)))
		require.NoError(t, err)
		require.True(t, resp.StatusCode == http.StatusOK)
	}

	resp, err := testEnv.HTTP.Get(testEnv.URLForPath("/jwt/v1/pack?max=foo"))
	require.NoError(t, err)
	require.True(t, resp.StatusCode == http.StatusBadRequest)

	resp, err = testEnv.HTTP.Get(testEnv.URLForPath("/jwt/v1/pack"))
	require.NoError(t, err)
	require.True(t, resp.StatusCode == http.StatusOK)
	body, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)

	pack := string(body)
	split := strings.Split(pack, "\n")

	for _, line := range split {
		if line == "" {
			continue
		}

		s := strings.Split(line, "|")
		require.Len(t, s, 2)

		pubKey := s[0]
		jwt := s[1]

		existing, ok := pubKeys[pubKey]
		require.True(t, ok)
		require.NotEmpty(t, existing)
		require.Equal(t, existing, jwt)
	}
}

func TestPackJWTsWithMax(t *testing.T) {

	testEnv, err := SetupTestServer(conf.DefaultServerConfig(), false, false)
	defer testEnv.Cleanup()
	require.NoError(t, err)

	operatorKey := testEnv.OperatorKey

	pubKeys := map[string]string{}

	for i := 0; i < 100; i++ {
		accountKey, err := nkeys.CreateAccount()
		require.NoError(t, err)

		pubKey, err := accountKey.PublicKey()
		require.NoError(t, err)

		account := jwt.NewAccountClaims(pubKey)
		acctJWT, err := account.Encode(operatorKey)
		require.NoError(t, err)

		pubKeys[pubKey] = acctJWT

		path := fmt.Sprintf("/jwt/v1/accounts/%s", pubKey)
		url := testEnv.URLForPath(path)

		resp, err := testEnv.HTTP.Post(url, "application/json", bytes.NewBuffer([]byte(acctJWT)))
		require.NoError(t, err)
		require.True(t, resp.StatusCode == http.StatusOK)
	}

	resp, err := testEnv.HTTP.Get(testEnv.URLForPath("/jwt/v1/pack?max=2"))
	require.NoError(t, err)
	require.True(t, resp.StatusCode == http.StatusOK)
	body, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)

	pack := string(body)
	split := strings.Split(pack, "\n")

	require.Len(t, split, 2)

	for _, line := range split {
		s := strings.Split(line, "|")
		require.Len(t, s, 2)

		pubKey := s[0]
		jwt := s[1]

		existing, ok := pubKeys[pubKey]
		require.True(t, ok)
		require.NotEmpty(t, existing)
		require.Equal(t, existing, jwt)
	}
}

func TestReplicatedInit(t *testing.T) {
	testEnv, err := SetupTestServer(conf.DefaultServerConfig(), false, true)
	defer testEnv.Cleanup()
	require.NoError(t, err)

	operatorKey := testEnv.OperatorKey

	pubKeys := map[string]string{}

	for i := 0; i < 100; i++ {
		accountKey, err := nkeys.CreateAccount()
		require.NoError(t, err)

		pubKey, err := accountKey.PublicKey()
		require.NoError(t, err)

		account := jwt.NewAccountClaims(pubKey)
		acctJWT, err := account.Encode(operatorKey)
		require.NoError(t, err)

		pubKeys[pubKey] = acctJWT

		path := fmt.Sprintf("/jwt/v1/accounts/%s", pubKey)
		url := testEnv.URLForPath(path)

		resp, err := testEnv.HTTP.Post(url, "application/json", bytes.NewBuffer([]byte(acctJWT)))
		require.NoError(t, err)
		require.True(t, resp.StatusCode == http.StatusOK)
	}

	// Now start up the replica
	tempDir, err := ioutil.TempDir(os.TempDir(), "prefix")
	require.NoError(t, err)

	replica, err := testEnv.CreateReplica(tempDir)
	require.NoError(t, err)
	defer replica.Stop()

	// Turn off the main server, so we only get local content from the replica
	testEnv.Server.Stop()

	// Replica should have initialized
	for pubkey, jwt := range pubKeys {
		path := fmt.Sprintf("/jwt/v1/accounts/%s", pubkey)
		url := fmt.Sprintf("%s://%s%s", replica.protocol, replica.hostPort, path)
		resp, err := testEnv.HTTP.Get(url)
		require.NoError(t, err)
		require.True(t, resp.StatusCode == http.StatusOK)
		body, err := ioutil.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Equal(t, jwt, string(body))
	}
}

func TestReplicatedInitWithMax(t *testing.T) {
	testEnv, err := SetupTestServer(conf.DefaultServerConfig(), false, true)
	defer testEnv.Cleanup()
	require.NoError(t, err)

	operatorKey := testEnv.OperatorKey

	pubKeys := map[string]string{}

	for i := 0; i < 100; i++ {
		accountKey, err := nkeys.CreateAccount()
		require.NoError(t, err)

		pubKey, err := accountKey.PublicKey()
		require.NoError(t, err)

		account := jwt.NewAccountClaims(pubKey)
		acctJWT, err := account.Encode(operatorKey)
		require.NoError(t, err)

		pubKeys[pubKey] = acctJWT

		path := fmt.Sprintf("/jwt/v1/accounts/%s", pubKey)
		url := testEnv.URLForPath(path)

		resp, err := testEnv.HTTP.Post(url, "application/json", bytes.NewBuffer([]byte(acctJWT)))
		require.NoError(t, err)
		require.True(t, resp.StatusCode == http.StatusOK)
	}

	// Now start up the replica
	tempDir, err := ioutil.TempDir(os.TempDir(), "prefix")
	require.NoError(t, err)

	replica, err := testEnv.CreateReplicaWithMaxPack(tempDir, 10)
	require.NoError(t, err)
	defer replica.Stop()

	// Turn off the main server, so we only get local content from the replica
	testEnv.Server.Stop()

	count := 0

	// Replica should have initialized
	for pubkey, jwt := range pubKeys {
		path := fmt.Sprintf("/jwt/v1/accounts/%s", pubkey)
		url := fmt.Sprintf("%s://%s%s", replica.protocol, replica.hostPort, path)
		resp, err := testEnv.HTTP.Get(url)

		// only count the ones that we have
		if err == nil && resp.StatusCode == http.StatusOK {
			body, err := ioutil.ReadAll(resp.Body)
			require.NoError(t, err)
			require.Equal(t, jwt, string(body))
			count++
		}
	}

	require.Equal(t, 10, count)
}

func TestReplicatedInitWithMaxZero(t *testing.T) {
	testEnv, err := SetupTestServer(conf.DefaultServerConfig(), false, true)
	defer testEnv.Cleanup()
	require.NoError(t, err)

	operatorKey := testEnv.OperatorKey

	pubKeys := map[string]string{}

	for i := 0; i < 100; i++ {
		accountKey, err := nkeys.CreateAccount()
		require.NoError(t, err)

		pubKey, err := accountKey.PublicKey()
		require.NoError(t, err)

		account := jwt.NewAccountClaims(pubKey)
		acctJWT, err := account.Encode(operatorKey)
		require.NoError(t, err)

		pubKeys[pubKey] = acctJWT

		path := fmt.Sprintf("/jwt/v1/accounts/%s", pubKey)
		url := testEnv.URLForPath(path)

		resp, err := testEnv.HTTP.Post(url, "application/json", bytes.NewBuffer([]byte(acctJWT)))
		require.NoError(t, err)
		require.True(t, resp.StatusCode == http.StatusOK)
	}

	// Now start up the replica
	tempDir, err := ioutil.TempDir(os.TempDir(), "prefix")
	require.NoError(t, err)

	replica, err := testEnv.CreateReplicaWithMaxPack(tempDir, 0)
	require.NoError(t, err)
	defer replica.Stop()

	// Turn off the main server, so we only get local content from the replica
	testEnv.Server.Stop()

	count := 0

	// Replica should have initialized
	for pubkey, jwt := range pubKeys {
		path := fmt.Sprintf("/jwt/v1/accounts/%s", pubkey)
		url := fmt.Sprintf("%s://%s%s", replica.protocol, replica.hostPort, path)
		resp, err := testEnv.HTTP.Get(url)

		// only count the ones that we have
		if err == nil && resp.StatusCode == http.StatusOK {
			body, err := ioutil.ReadAll(resp.Body)
			require.NoError(t, err)
			require.Equal(t, jwt, string(body))
			count++
		}
	}

	require.Equal(t, 0, count)
}

func TestPackFailsWithWrongStore(t *testing.T) {
	_, _, kp := store.CreateOperatorKey(t)
	_, apub, _ := store.CreateAccountKey(t)
	s := store.CreateTestStoreForOperator(t, "x", kp)

	c := jwt.NewAccountClaims(apub)
	c.Name = "foo"
	cd, err := c.Encode(kp)
	require.NoError(t, err)
	err = s.StoreClaim([]byte(cd))
	require.NoError(t, err)

	config := conf.DefaultServerConfig()
	config.Store.Dir = ""
	config.Store.NSC = s.Dir
	testEnv, err := SetupTestServer(config, false, false)
	defer testEnv.Cleanup()
	require.NoError(t, err)

	resp, err := testEnv.HTTP.Get(testEnv.URLForPath("/jwt/v1/pack"))
	require.NoError(t, err)
	require.True(t, resp.StatusCode == http.StatusNotFound)
}

func TestReplicatedInitPrimaryDown(t *testing.T) {
	testEnv, err := SetupTestServer(conf.DefaultServerConfig(), false, true)
	defer testEnv.Cleanup()
	require.NoError(t, err)

	operatorKey := testEnv.OperatorKey

	pubKeys := map[string]string{}

	for i := 0; i < 100; i++ {
		accountKey, err := nkeys.CreateAccount()
		require.NoError(t, err)

		pubKey, err := accountKey.PublicKey()
		require.NoError(t, err)

		account := jwt.NewAccountClaims(pubKey)
		acctJWT, err := account.Encode(operatorKey)
		require.NoError(t, err)

		pubKeys[pubKey] = acctJWT

		path := fmt.Sprintf("/jwt/v1/accounts/%s", pubKey)
		url := testEnv.URLForPath(path)

		resp, err := testEnv.HTTP.Post(url, "application/json", bytes.NewBuffer([]byte(acctJWT)))
		require.NoError(t, err)
		require.True(t, resp.StatusCode == http.StatusOK)
	}

	// Now start up the replica
	tempDir, err := ioutil.TempDir(os.TempDir(), "prefix")
	require.NoError(t, err)

	// Turn off the main server, so we only get local content from the replica
	testEnv.Server.Stop()

	replica, err := testEnv.CreateReplica(tempDir)
	require.NoError(t, err)
	defer replica.Stop()

	count := 0

	// Replica should have initialized
	for pubkey, jwt := range pubKeys {
		path := fmt.Sprintf("/jwt/v1/accounts/%s", pubkey)
		url := fmt.Sprintf("%s://%s%s", replica.protocol, replica.hostPort, path)
		resp, err := testEnv.HTTP.Get(url)

		// only count the ones that we have
		if err == nil && resp.StatusCode == http.StatusOK {
			body, err := ioutil.ReadAll(resp.Body)
			require.NoError(t, err)
			require.Equal(t, jwt, string(body))
			count++
		}
	}

	require.Equal(t, 0, count)
}
