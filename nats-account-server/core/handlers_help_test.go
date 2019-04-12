package core

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/nats-io/account-server/nats-account-server/conf"
	"github.com/stretchr/testify/require"
)

func TestJWTHelp(t *testing.T) {
	testEnv, err := SetupTestServer(conf.DefaultServerConfig(), false)
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
