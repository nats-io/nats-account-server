package store

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/nats-io/jwt"
	"github.com/nats-io/nkeys"
	nsc "github.com/nats-io/nsc/cmd/store"
	"github.com/stretchr/testify/require"
)

type NKeyFactory func() (nkeys.KeyPair, error)

func CreateAccountKey(t *testing.T) (seed []byte, pub string, kp nkeys.KeyPair) {
	return CreateTestNKey(t, nkeys.CreateAccount)
}

func CreateOperatorKey(t *testing.T) (seed []byte, pub string, kp nkeys.KeyPair) {
	return CreateTestNKey(t, nkeys.CreateOperator)
}

func CreateTestNKey(t *testing.T, f NKeyFactory) ([]byte, string, nkeys.KeyPair) {
	kp, err := f()
	require.NoError(t, err)

	seed, err := kp.Seed()
	require.NoError(t, err)

	pub, err := kp.PublicKey()
	require.NoError(t, err)

	return seed, pub, kp
}

func MakeTempStore(t *testing.T, name string, kp nkeys.KeyPair) *nsc.Store {
	p, err := ioutil.TempDir("", "store_test")
	require.NoError(t, err)

	var nk *nsc.NamedKey
	if kp != nil {
		nk = &nsc.NamedKey{Name: name, KP: kp}
	}

	s, err := nsc.CreateStore(name, p, nk)
	require.NoError(t, err)
	require.NotNil(t, s)
	return s
}

func CreateTestStoreForOperator(t *testing.T, name string, operator nkeys.KeyPair) *nsc.Store {
	s := MakeTempStore(t, name, operator)

	require.NotNil(t, s)
	require.FileExists(t, filepath.Join(s.Dir, ".nsc"))
	require.True(t, s.Has("", ".nsc"))

	if operator != nil {
		tokenName := fmt.Sprintf("%s.jwt", nsc.SafeName(name))
		require.FileExists(t, filepath.Join(s.Dir, tokenName))
		require.True(t, s.Has("", tokenName))
	}

	return s
}

func TestValidNSCStore(t *testing.T) {
	_, _, kp := CreateOperatorKey(t)
	_, apub, _ := CreateAccountKey(t)
	s := CreateTestStoreForOperator(t, "x", kp)

	c := jwt.NewAccountClaims(apub)
	c.Name = "foo"
	cd, err := c.Encode(kp)
	require.NoError(t, err)
	err = s.StoreClaim([]byte(cd))
	require.NoError(t, err)

	store, err := NewNSCJWTStore(s.Dir)
	require.NoError(t, err)

	require.True(t, store.IsReadOnly())

	theJWT, err := store.Load(c.Subject)
	require.NoError(t, err)
	require.Equal(t, cd, theJWT)

	got, err := store.Load("random")
	require.Error(t, err)
	require.Equal(t, "", got)

	got, err = store.Load("")
	require.Error(t, err)
	require.Equal(t, "", got)

	err = store.Save("five", "onetwothree")
	require.Error(t, err)
}

func TestBadFolderNSCStore(t *testing.T) {
	store, err := NewNSCJWTStore("/a/b/c")
	require.Error(t, err)
	require.Nil(t, store)
}
