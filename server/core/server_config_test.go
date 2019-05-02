/*
 * Copyright 2012-2019 The NATS Authors
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
	"os"
	"path/filepath"
	"testing"

	"github.com/nats-io/jwt"
	"github.com/nats-io/nats-account-server/server/conf"
	"github.com/nats-io/nkeys"
	nsc "github.com/nats-io/nsc/cmd/store"
	"github.com/stretchr/testify/require"
)

func TestStartWithDirFlag(t *testing.T) {
	path, err := ioutil.TempDir(os.TempDir(), "store")
	require.NoError(t, err)

	flags := Flags{
		Debug:     true,
		Verbose:   true,
		Directory: path,
	}

	server := NewAccountServer()
	server.InitializeFromFlags(flags)
	err = server.Start()
	require.NoError(t, err)
	defer server.Stop()

	httpClient, err := testHTTPClient(false)
	require.NoError(t, err)

	resp, err := httpClient.Get(fmt.Sprintf("http://localhost:%d/jwt/v1/help", server.port))
	require.NoError(t, err)
	require.True(t, resp.StatusCode == http.StatusOK)
	body, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)

	help := string(body)
	require.Equal(t, jwtAPIHelp, help)
}

func CreateOperatorKey(t *testing.T) ([]byte, string, nkeys.KeyPair) {
	kp, err := nkeys.CreateOperator()
	require.NoError(t, err)

	seed, err := kp.Seed()
	require.NoError(t, err)

	pub, err := kp.PublicKey()
	require.NoError(t, err)

	return seed, pub, kp
}

func CreateAccountKey(t *testing.T) ([]byte, string, nkeys.KeyPair) {
	kp, err := nkeys.CreateAccount()
	require.NoError(t, err)

	seed, err := kp.Seed()
	require.NoError(t, err)

	pub, err := kp.PublicKey()
	require.NoError(t, err)

	return seed, pub, kp
}

func MakeTempStore(t *testing.T, name string, kp nkeys.KeyPair) (*nsc.Store, string) {
	p, err := ioutil.TempDir("", "store_test")
	require.NoError(t, err)

	var nk *nsc.NamedKey
	if kp != nil {
		nk = &nsc.NamedKey{Name: name, KP: kp}
	}

	s, err := nsc.CreateStore(name, p, nk)
	require.NoError(t, err)
	require.NotNil(t, s)
	return s, p
}

func CreateTestStoreForOperator(t *testing.T, name string, operator nkeys.KeyPair) (*nsc.Store, string) {
	s, p := MakeTempStore(t, name, operator)

	require.NotNil(t, s)
	require.FileExists(t, filepath.Join(s.Dir, ".nsc"))
	require.True(t, s.Has("", ".nsc"))

	if operator != nil {
		tokenName := fmt.Sprintf("%s.jwt", nsc.SafeName(name))
		require.FileExists(t, filepath.Join(s.Dir, tokenName))
		require.True(t, s.Has("", tokenName))
	}

	return s, p
}

func TestStartWithNSCFlag(t *testing.T) {
	_, _, kp := CreateOperatorKey(t)
	_, apub, _ := CreateAccountKey(t)
	s, path := CreateTestStoreForOperator(t, "x", kp)

	c := jwt.NewAccountClaims(apub)
	c.Name = "foo"
	cd, err := c.Encode(kp)
	require.NoError(t, err)
	err = s.StoreClaim([]byte(cd))
	require.NoError(t, err)

	flags := Flags{
		DebugAndVerbose: true,
		NSCFolder:       filepath.Join(path, "x"),
	}

	server := NewAccountServer()
	server.InitializeFromFlags(flags)
	err = server.Start()
	require.NoError(t, err)
	defer server.Stop()

	httpClient, err := testHTTPClient(false)
	require.NoError(t, err)

	resp, err := httpClient.Get(fmt.Sprintf("http://localhost:%d/jwt/v1/accounts/%s", server.port, apub))
	require.NoError(t, err)
	require.True(t, resp.StatusCode == http.StatusOK)
	body, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)

	jwt := string(body)
	require.Equal(t, cd, jwt)
}

func TestStartWithConfigFileFlag(t *testing.T) {
	path, err := ioutil.TempDir(os.TempDir(), "store")
	require.NoError(t, err)

	file, err := ioutil.TempFile(os.TempDir(), "config")
	require.NoError(t, err)

	configString := `
	{
		store: {
			Dir: %s,
		},
		http: {
			ReadTimeout: 2000,
		}
	}
	`
	configString = fmt.Sprintf(configString, path)

	fullPath, err := conf.ValidateFilePath(file.Name())
	require.NoError(t, err)

	err = ioutil.WriteFile(fullPath, []byte(configString), 0644)
	require.NoError(t, err)

	flags := Flags{
		ConfigFile: fullPath,
	}

	server := NewAccountServer()
	server.InitializeFromFlags(flags)
	err = server.Start()
	require.NoError(t, err)
	defer server.Stop()

	require.Equal(t, server.config.Store.Dir, path)
	require.Equal(t, server.config.HTTP.ReadTimeout, 2000)

	httpClient, err := testHTTPClient(false)
	require.NoError(t, err)

	resp, err := httpClient.Get(fmt.Sprintf("http://localhost:%d/jwt/v1/help", server.port))
	require.NoError(t, err)
	require.True(t, resp.StatusCode == http.StatusOK)
	body, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)

	help := string(body)
	require.Equal(t, jwtAPIHelp, help)
}
