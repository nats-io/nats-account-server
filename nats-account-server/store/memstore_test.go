package store

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMemStoreSimpleSetGet(t *testing.T) {
	store := NewMemJWTStore()
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
}

func TestMemStoreReadOnly(t *testing.T) {
	expected := map[string]string{
		"one":   "alpha",
		"two":   "beta",
		"three": "gamma",
		"four":  "delta",
	}
	store := NewImmutableMemJWTStore(expected)

	require.True(t, store.IsReadOnly())

	err := store.Save("five", "omega")
	require.Error(t, err)

	for k, v := range expected {
		got, err := store.Load(k)
		require.NoError(t, err)
		require.Equal(t, v, got)
	}
}
