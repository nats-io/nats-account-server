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
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"

	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nats-account-server/server/conf"
	"github.com/stretchr/testify/require"
)

func TestServerReloadNotification(t *testing.T) {

	// Skip the file notification test on travis
	if os.Getenv("TRAVIS_GO_VERSION") != "" {
		return
	}

	_, _, kp := CreateOperatorKey(t)
	_, apub, _ := CreateAccountKey(t)
	path, store := CreateTestStoreForOperator(t, "x")

	c := jwt.NewAccountClaims(apub)
	c.Name = "foo"
	cd, err := c.Encode(kp)
	require.NoError(t, err)
	store(apub, cd)

	config := conf.DefaultServerConfig()
	config.Store.Dir = path

	testEnv, err := SetupTestServer(config, false, true)
	defer testEnv.Cleanup()
	require.NoError(t, err)

	url := testEnv.URLForPath(fmt.Sprintf("/jwt/v1/accounts/%s", apub))

	resp, err := testEnv.HTTP.Get(url)
	require.NoError(t, err)
	require.True(t, resp.StatusCode == http.StatusOK)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	jwt := string(body)
	require.Equal(t, cd, jwt)

	notificationJWT := ""
	subject := fmt.Sprintf(accountNotificationFormat, apub)
	sub, err := testEnv.NC.SubscribeSync(subject)
	require.NoError(t, err)

	c.Tags.Add("red")
	cd, err = c.Encode(kp)
	require.NoError(t, err)
	store(apub, cd)

	testEnv.Server.JWTStore.(*natsserver.DirJWTStore).Reload()

	resp, err = testEnv.HTTP.Get(url)
	require.NoError(t, err)
	require.True(t, resp.StatusCode == http.StatusOK)
	body, err = io.ReadAll(resp.Body)
	require.NoError(t, err)

	jwt = string(body)
	require.Equal(t, cd, jwt)

	time.Sleep(3 * time.Second)

	testEnv.Server.nats.Flush()
	testEnv.NC.Flush()

	msg, err := sub.NextMsg(time.Second)
	require.NoError(t, err)
	notificationJWT = string(msg.Data)
	require.Equal(t, notificationJWT, jwt)
}
