package core

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"

	"github.com/nats-io/account-server/nats-account-server/conf"
	nats "github.com/nats-io/go-nats"
	"github.com/nats-io/jwt"
	"github.com/nats-io/nkeys"
	"github.com/stretchr/testify/require"
)

func TestUploadGetAccountJWT(t *testing.T) {
	testEnv, err := SetupTestServer(conf.DefaultServerConfig())
	defer testEnv.Cleanup()
	require.NoError(t, err)

	operatorKey := testEnv.OperatorKey
	opk := testEnv.OperatorPubKey

	accountKey, err := nkeys.CreateAccount()
	require.NoError(t, err)

	pubKey, err := accountKey.PublicKey()
	require.NoError(t, err)

	account := jwt.NewAccountClaims(pubKey)
	acctJWT, err := account.Encode(operatorKey)
	require.NoError(t, err)
	notificationJWT := ""
	subject := fmt.Sprintf(accountNotificationFormat, pubKey)
	_, err = testEnv.NC.Subscribe(subject, func(m *nats.Msg) {
		notificationJWT = string(m.Data)
	})
	require.NoError(t, err)

	path := fmt.Sprintf("/jwt/v1/accounts/%s", pubKey)
	url := testEnv.URLForPath(path)

	resp, err := http.Get(url)
	require.NoError(t, err)
	require.False(t, resp.StatusCode == http.StatusOK)

	resp, err = http.Post(url, "application/json", bytes.NewBuffer([]byte(acctJWT)))
	require.NoError(t, err)
	require.True(t, resp.StatusCode == http.StatusOK)

	resp, err = http.Get(url)
	require.NoError(t, err)
	require.True(t, resp.StatusCode == http.StatusOK)
	body, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)

	savedJWT := string(body)

	testEnv.Server.nats.Flush()
	testEnv.NC.Flush()
	require.Equal(t, notificationJWT, string(savedJWT))

	savedClaims, err := jwt.DecodeAccountClaims(string(savedJWT))
	require.NoError(t, err)

	require.Equal(t, account.Subject, savedClaims.Subject)
	require.Equal(t, opk, savedClaims.Issuer)

	path = fmt.Sprintf("/jwt/v1/accounts/%s?text=true", pubKey)
	url = testEnv.URLForPath(path)
	resp, err = http.Get(url)
	require.NoError(t, err)
	require.True(t, resp.StatusCode == http.StatusOK)
	body, err = ioutil.ReadAll(resp.Body)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(string(body), "eyJ0eXAiOiJqd3QiLCJhbGciOiJlZDI1NTE5In0")) // header prefix doesn't change

	path = fmt.Sprintf("/jwt/v1/accounts/%s?decode=true", pubKey)
	url = testEnv.URLForPath(path)
	resp, err = http.Get(url)
	require.NoError(t, err)
	require.True(t, resp.StatusCode == http.StatusOK)
	body, err = ioutil.ReadAll(resp.Body)
	require.NoError(t, err)
	require.True(t, strings.Contains(string(body), `"alg": "ed25519"`)) // header prefix doesn't change
}
