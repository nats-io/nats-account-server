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
	"io/ioutil"
	"net/http"
	"strings"
	"testing"

	"github.com/nats-io/nats-account-server/server/conf"
	"github.com/stretchr/testify/require"
)

func TestHealthZ(t *testing.T) {
	testEnv, err := SetupTestServer(conf.DefaultServerConfig(), false, false)
	defer testEnv.Cleanup()
	require.NoError(t, err)

	path := fmt.Sprintf("/healthz")
	url := testEnv.URLForPath(path)

	resp, err := testEnv.HTTP.Get(url)
	require.NoError(t, err)
	require.True(t, resp.StatusCode == http.StatusOK)
}

func TestHealthZTLS(t *testing.T) {
	testEnv, err := SetupTestServer(conf.DefaultServerConfig(), true, false)
	defer testEnv.Cleanup()
	require.NoError(t, err)

	path := fmt.Sprintf("/healthz")
	url := testEnv.URLForPath(path)

	resp, err := testEnv.HTTP.Get(url)
	require.NoError(t, err)
	require.True(t, resp.StatusCode == http.StatusOK)
}

func TestJWTHelp(t *testing.T) {
	testEnv, err := SetupTestServer(conf.DefaultServerConfig(), false, false)
	defer testEnv.Cleanup()
	require.NoError(t, err)

	path := fmt.Sprintf("/jwt/v1/help")
	url := testEnv.URLForPath(path)

	resp, err := testEnv.HTTP.Get(url)
	require.NoError(t, err)
	require.True(t, resp.StatusCode == http.StatusOK)
	body, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)

	help := string(body)
	require.Equal(t, jwtAPIHelp, help)
}

func TestJWTHelpTLS(t *testing.T) {
	testEnv, err := SetupTestServer(conf.DefaultServerConfig(), true, false)
	defer testEnv.Cleanup()
	require.NoError(t, err)

	path := fmt.Sprintf("/jwt/v1/help")
	url := testEnv.URLForPath(path)

	resp, err := testEnv.HTTP.Get(url)
	require.NoError(t, err)
	require.True(t, resp.StatusCode == http.StatusOK)
	body, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)

	help := string(body)
	require.Equal(t, jwtAPIHelp, help)
}

func TestOperatorJWT(t *testing.T) {
	testEnv, err := SetupTestServer(conf.DefaultServerConfig(), false, false)
	defer testEnv.Cleanup()
	require.NoError(t, err)

	path := fmt.Sprintf("/jwt/v1/operator")
	url := testEnv.URLForPath(path)

	resp, err := testEnv.HTTP.Get(url)
	require.NoError(t, err)
	require.True(t, resp.StatusCode == http.StatusOK)
	body, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)

	operator := string(body)
	require.Equal(t, testEnv.Server.operatorJWT, operator)

	path = fmt.Sprintf("/jwt/v1/operator?text=true")
	url = testEnv.URLForPath(path)
	resp, err = testEnv.HTTP.Get(url)
	require.NoError(t, err)
	require.True(t, resp.StatusCode == http.StatusOK)
	body, err = ioutil.ReadAll(resp.Body)
	require.NoError(t, err)
	operator = string(body)
	require.Equal(t, testEnv.Server.operatorJWT, operator)

	path = fmt.Sprintf("/jwt/v1/operator?decode=true")
	url = testEnv.URLForPath(path)
	resp, err = testEnv.HTTP.Get(url)
	require.NoError(t, err)
	require.True(t, resp.StatusCode == http.StatusOK)
	body, err = ioutil.ReadAll(resp.Body)
	require.NoError(t, err)
	operator = string(body)
	require.True(t, strings.Contains(operator, `"alg": "ed25519"`)) // header prefix doesn't change
}

func TestOperatorJWTTLS(t *testing.T) {
	testEnv, err := SetupTestServer(conf.DefaultServerConfig(), true, false)
	defer testEnv.Cleanup()
	require.NoError(t, err)

	path := fmt.Sprintf("/jwt/v1/operator")
	url := testEnv.URLForPath(path)

	resp, err := testEnv.HTTP.Get(url)
	require.NoError(t, err)
	require.True(t, resp.StatusCode == http.StatusOK)
	body, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)

	operator := string(body)
	require.Equal(t, testEnv.Server.operatorJWT, operator)
}
