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

	"github.com/nats-io/nats-account-server/server/conf"
	"github.com/stretchr/testify/require"
)

func TestListenWithoutHost(t *testing.T) {
	config := conf.DefaultServerConfig()
	config.HTTP.Host = ""
	testEnv, err := SetupTestServer(config, false, false)
	defer testEnv.Cleanup()
	require.NoError(t, err)
	require.Equal(t, fmt.Sprintf("127.0.0.1:%d", testEnv.Server.port), testEnv.Server.hostPort)
}

func TestListenBadHost(t *testing.T) {
	config := conf.DefaultServerConfig()
	config.HTTP.Host = "abc"
	testEnv, err := SetupTestServer(config, false, false)
	defer testEnv.Cleanup()
	require.Error(t, err)
}

func TestListenWithoutHostTLS(t *testing.T) {
	config := conf.DefaultServerConfig()
	config.HTTP.Host = ""
	testEnv, err := SetupTestServer(config, true, false)
	defer testEnv.Cleanup()
	require.NoError(t, err)
	require.Equal(t, fmt.Sprintf("127.0.0.1:%d", testEnv.Server.port), testEnv.Server.hostPort)
}

func TestListenBadHostTLS(t *testing.T) {
	config := conf.DefaultServerConfig()
	config.HTTP.Host = "abc"
	testEnv, err := SetupTestServer(config, true, false)
	defer testEnv.Cleanup()
	require.Error(t, err)
}

func TestBadTLSConfigInMake(t *testing.T) {
	config := conf.DefaultServerConfig()
	testEnv, err := SetupTestServer(config, true, false)
	defer testEnv.Cleanup()
	require.NoError(t, err)

	tlsConfig, err := testEnv.Server.makeTLSConfig(conf.TLSConf{})
	require.NoError(t, err)
	require.Nil(t, tlsConfig)

	_, err = testEnv.Server.makeTLSConfig(conf.TLSConf{
		Cert: "/a/b/c",
		Key:  keyFile,
	})
	require.Error(t, err)
}

func TestBadTLSConfigOnStart(t *testing.T) {
	config := conf.DefaultServerConfig()
	config.HTTP.TLS.Cert = "abc"
	testEnv, err := SetupTestServer(config, false, false)
	defer testEnv.Cleanup()
	require.Error(t, err)
}
