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
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nats-account-server/server/conf"
	nats "github.com/nats-io/nats.go"
	"github.com/nats-io/nkeys"
	"github.com/stretchr/testify/require"
)

func TestUploadGetActivationJWT(t *testing.T) {
	lock := sync.Mutex{}

	testEnv, err := SetupTestServer(conf.DefaultServerConfig(), false, true)
	defer testEnv.Cleanup()
	require.NoError(t, err)

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

	notificationJWT := ""
	subject := fmt.Sprintf(activationNotificationFormat, acctPubKey, hash)
	_, err = testEnv.NC.Subscribe(subject, func(m *nats.Msg) {
		lock.Lock()
		notificationJWT = string(m.Data)
		lock.Unlock()
	})
	require.NoError(t, err)

	// try to get the remote copy - should fail
	path := fmt.Sprintf("/jwt/v1/activations/%s", hash)
	getURL := testEnv.URLForPath(path)
	resp, err := testEnv.HTTP.Get(getURL)
	require.NoError(t, err)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)

	// Execute the command
	postURL := testEnv.URLForPath("/jwt/v1/activations")
	_, err = testEnv.HTTP.Post(postURL, "application/json", bytes.NewBuffer([]byte(actJWT)))
	require.NoError(t, err)

	// get the remote copy
	resp, err = testEnv.HTTP.Get(getURL)
	require.NoError(t, err)
	require.True(t, resp.StatusCode == http.StatusOK)
	body, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)

	savedJWT := string(body)

	testEnv.Server.nats.Flush()
	testEnv.NC.Flush()

	lock.Lock()
	require.Equal(t, notificationJWT, string(savedJWT))
	lock.Unlock()

	savedClaims, err := jwt.DecodeActivationClaims(string(savedJWT))
	require.NoError(t, err)
	hash2, err := act.HashID()
	require.NoError(t, err)

	require.Equal(t, act.Subject, savedClaims.Subject)
	require.Equal(t, act.Issuer, savedClaims.Issuer)
	require.Equal(t, hash, hash2)

	path = fmt.Sprintf("/jwt/v1/activations/%s?check=true", hash)
	url := testEnv.URLForPath(path)
	resp, err = testEnv.HTTP.Get(url)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	path = fmt.Sprintf("/jwt/v1/activations/%s?text=true", hash)
	url = testEnv.URLForPath(path)
	resp, err = testEnv.HTTP.Get(url)
	require.NoError(t, err)
	require.True(t, resp.StatusCode == http.StatusOK)
	body, err = ioutil.ReadAll(resp.Body)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(string(body), "eyJ0eXAiOiJqd3QiLCJhbGciOiJlZDI1NTE5In0")) // header prefix doesn't change

	path = fmt.Sprintf("/jwt/v1/activations/%s?decode=true", hash)
	url = testEnv.URLForPath(path)
	resp, err = testEnv.HTTP.Get(url)
	require.NoError(t, err)
	require.True(t, resp.StatusCode == http.StatusOK)
	body, err = ioutil.ReadAll(resp.Body)
	require.NoError(t, err)

	decoded := string(body)
	require.True(t, strings.Contains(decoded, `"alg": "ed25519"`))          // header prefix doesn't change
	require.True(t, strings.Contains(decoded, UnixToDate(int64(expireAt)))) // expires are resolved to readable form

	path = fmt.Sprintf("/jwt/v1/activations/%s?notify=true", hash)
	url = testEnv.URLForPath(path)
	resp, err = testEnv.HTTP.Get(url)
	require.NoError(t, err)
	require.True(t, resp.StatusCode == http.StatusOK)
	testEnv.Server.nats.Flush()
	testEnv.NC.Flush()

	lock.Lock()
	require.Equal(t, notificationJWT, string(savedJWT))
	lock.Unlock()
}

func TestCacheControlActivationJWT(t *testing.T) {
	testEnv, err := SetupTestServer(conf.DefaultServerConfig(), false, true)
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
	jti := act.ID

	hash, err := act.HashID()
	require.NoError(t, err)

	// Post the activation
	postURL := testEnv.URLForPath("/jwt/v1/activations")
	_, err = testEnv.HTTP.Post(postURL, "application/json", bytes.NewBuffer([]byte(actJWT)))
	require.NoError(t, err)

	path := fmt.Sprintf("/jwt/v1/activations/%s", hash)
	getURL := testEnv.URLForPath(path)
	resp, err := testEnv.HTTP.Get(getURL)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	etag := resp.Header.Get("ETag")
	etag = strings.Trim(etag, "\"")
	require.Equal(t, etag, jti)
	cacheHeader := resp.Header.Get("Cache-Control")
	require.True(t, strings.Contains(cacheHeader, "stale-while-revalidate=3600, stale-if-error=3600"))

	request, err := http.NewRequest("GET", getURL, nil)
	require.NoError(t, err)
	etag = `"` + etag + `"`
	request.Header.Set("If-None-Match", etag)

	resp, err = testEnv.HTTP.Do(request)
	require.NoError(t, err)
	require.Equal(t, http.StatusNotModified, resp.StatusCode)
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
