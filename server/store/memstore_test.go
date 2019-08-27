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

func TestMemStorePackMerge(t *testing.T) {
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

	packable, ok := store.(PackableJWTStore)
	require.True(t, ok)

	pack, err := packable.Pack(-1)
	require.NoError(t, err)

	inc := NewMemJWTStore()
	incP, ok := inc.(PackableJWTStore)
	require.True(t, ok)

	incP.Merge(pack)

	for k, v := range expected {
		got, err := inc.Load(k)
		require.NoError(t, err)
		require.Equal(t, v, got)
	}

	got, err = inc.Load("random")
	require.Error(t, err)
	require.Equal(t, "", got)

	limitedPack, err := packable.Pack(1)
	require.NoError(t, err)

	limited := NewMemJWTStore()
	limitedP, ok := limited.(PackableJWTStore)
	require.True(t, ok)

	limitedP.Merge(limitedPack)

	count := 0
	for k, v := range expected {
		got, err := limited.Load(k)
		if err == nil {
			count++
			require.Equal(t, v, got)
		}
	}

	require.Equal(t, 1, count)

	got, err = inc.Load("random")
	require.Error(t, err)
	require.Equal(t, "", got)

	err = limitedP.Merge("foo")
	require.Error(t, err)

	err = limitedP.Merge("") // will skip it
	require.NoError(t, err)
}
