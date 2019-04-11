package store

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDirStoreWriteAndReadonly(t *testing.T) {
	dir, err := ioutil.TempDir(os.TempDir(), "jwtstore_test")
	require.NoError(t, err)

	store, err := NewDirJWTStore(dir, false)
	require.NoError(t, err)

	expected := map[string]string{
		"one":   "alpha",
		"two":   "beta",
		"three": "gamma",
		"four":  "delta",
	}

	require.False(t, store.IsReadOnly())

	for k, v := range expected {
		store.Save(k, v)
	}

	for k, v := range expected {
		got, err := store.Load(k)
		require.NoError(t, err)
		require.Equal(t, v, got)
	}

	got, err := store.Load("random")
	require.Error(t, err)
	require.Equal(t, "", got)

	got, err = store.Load("")
	require.Error(t, err)
	require.Equal(t, "", got)

	err = store.Save("", "onetwothree")
	require.Error(t, err)

	// re-use the folder for readonly mode
	store, err = NewImmutableDirJWTStore(dir)
	require.NoError(t, err)

	require.True(t, store.IsReadOnly())

	err = store.Save("five", "omega")
	require.Error(t, err)

	for k, v := range expected {
		got, err := store.Load(k)
		require.NoError(t, err)
		require.Equal(t, v, got)
	}
}

func TestReadonlyRequiresDir(t *testing.T) {
	_, err := NewImmutableDirJWTStore("/a/b/c")
	require.Error(t, err)
}

func TestNoCreateRequiresDir(t *testing.T) {
	_, err := NewDirJWTStore("/a/b/c", false)
	require.Error(t, err)
}
