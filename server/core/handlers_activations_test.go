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
	"net/http"
	"testing"

	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nats-account-server/server/conf"
	"github.com/nats-io/nkeys"
	"github.com/stretchr/testify/require"
)

func TestUploadActivationsAreNotSupported(t *testing.T) {
	testEnv, err := SetupTestServer(conf.DefaultServerConfig(), false, true)
	require.NoError(t, err)
	defer testEnv.Cleanup()

	getURL := testEnv.URLForPath("/jwt/v1/activations/foo")
	resp, err := testEnv.HTTP.Get(getURL)
	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)

	postURL := testEnv.URLForPath("/jwt/v1/activations")
	_, err = testEnv.HTTP.Post(postURL, "application/json", bytes.NewBuffer([]byte{}))
	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestInvalidActivationPost(t *testing.T) {
	testEnv, err := SetupTestServer(conf.DefaultServerConfig(), false, false)
	defer testEnv.Cleanup()
	require.NoError(t, err)

	accountKey, err := nkeys.CreateAccount()
	require.NoError(t, err)
	account2Key, err := nkeys.CreateAccount()
	require.NoError(t, err)

	acct2PubKey, err := account2Key.PublicKey()
	require.NoError(t, err)

	act := jwt.NewActivationClaims(acct2PubKey)
	act.ImportType = jwt.Stream
	act.Name = "times"
	act.ImportSubject = "times.*"
	actJWT, err := act.Encode(accountKey)
	require.NoError(t, err)

	act, err = jwt.DecodeActivationClaims(actJWT)
	require.NoError(t, err)

	hash, err := act.HashID()
	require.NoError(t, err)

	path := "/jwt/v1/activations"
	url := testEnv.URLForPath(path)

	resp, err := testEnv.HTTP.Post(url, "application/json", bytes.NewBuffer([]byte("hello world")))
	require.NoError(t, err)
	require.False(t, resp.StatusCode == http.StatusOK)

	path = fmt.Sprintf("/jwt/v1/activations/%s", hash)
	url = testEnv.URLForPath(path)
	resp, err = testEnv.HTTP.Get(url)
	require.NoError(t, err)
	require.False(t, resp.StatusCode == http.StatusOK)
}

func TestInvalidJWTType(t *testing.T) {
	testEnv, err := SetupTestServer(conf.DefaultServerConfig(), false, false)
	defer testEnv.Cleanup()
	require.NoError(t, err)

	accountKey, err := nkeys.CreateAccount()
	require.NoError(t, err)

	pubKey, err := accountKey.PublicKey()
	require.NoError(t, err)

	account := jwt.NewAccountClaims(pubKey)
	acctJWT, err := account.Encode(accountKey)
	require.NoError(t, err)

	url := testEnv.URLForPath("/jwt/v1/activations")

	resp, err := testEnv.HTTP.Post(url, "application/json", bytes.NewBuffer([]byte(acctJWT)))
	require.NoError(t, err)
	require.False(t, resp.StatusCode == http.StatusOK)
}
