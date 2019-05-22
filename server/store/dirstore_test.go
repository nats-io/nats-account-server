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
	"io/ioutil"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestShardedDirStoreWriteAndReadonly(t *testing.T) {
	dir, err := ioutil.TempDir(os.TempDir(), "jwtstore_test")
	require.NoError(t, err)

	store, err := NewDirJWTStore(dir, true, false, func(pubKey string) {}, func(err error) {})
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
	store.Close()

	// re-use the folder for readonly mode
	store, err = NewImmutableDirJWTStore(dir, true, func(pubKey string) {}, func(err error) {})
	require.NoError(t, err)

	require.True(t, store.IsReadOnly())

	err = store.Save("five", "omega")
	require.Error(t, err)

	for k, v := range expected {
		got, err := store.Load(k)
		require.NoError(t, err)
		require.Equal(t, v, got)
	}
	store.Close()
}

func TestUnshardedDirStoreWriteAndReadonly(t *testing.T) {
	dir, err := ioutil.TempDir(os.TempDir(), "jwtstore_test")
	require.NoError(t, err)

	store, err := NewDirJWTStore(dir, false, false, func(pubKey string) {}, func(err error) {})
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
	store.Close()

	// re-use the folder for readonly mode
	store, err = NewImmutableDirJWTStore(dir, false, func(pubKey string) {}, func(err error) {})
	require.NoError(t, err)

	require.True(t, store.IsReadOnly())

	err = store.Save("five", "omega")
	require.Error(t, err)

	for k, v := range expected {
		got, err := store.Load(k)
		require.NoError(t, err)
		require.Equal(t, v, got)
	}
	store.Close()
}

func TestReadonlyRequiresDir(t *testing.T) {
	_, err := NewImmutableDirJWTStore("/a/b/c", true, func(pubKey string) {}, func(err error) {})
	require.Error(t, err)
}

func TestNoCreateRequiresDir(t *testing.T) {
	_, err := NewDirJWTStore("/a/b/c", true, false, func(pubKey string) {}, func(err error) {})
	require.Error(t, err)
}

func TestShardedDirStoreNotifications(t *testing.T) {

	// Skip the file notification test on travis
	if os.Getenv("TRAVIS_GO_VERSION") != "" {
		return
	}

	dir, err := ioutil.TempDir(os.TempDir(), "jwtstore_test")
	require.NoError(t, err)

	notified := make(chan bool)
	jwtChanges := int32(0)
	errors := int32(0)

	store, err := NewDirJWTStore(dir, true, false, func(pubKey string) {
		atomic.AddInt32(&jwtChanges, 1)
		notified <- true
	}, func(err error) {
		atomic.AddInt32(&errors, 1)
		notified <- true
	})
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

	time.Sleep(time.Second)

	for k, v := range expected {
		got, err := store.Load(k)
		require.NoError(t, err)
		require.Equal(t, v, got)
	}

	store.Save("one", "zip")

	select {
	case <-notified:
	case <-time.After(3 * time.Second):
	}
	require.Equal(t, int32(1), atomic.LoadInt32(&jwtChanges))
	require.Equal(t, int32(0), atomic.LoadInt32(&errors))

	// re-use the folder for readonly mode
	roNotified := make(chan bool)
	roJWTChanges := int32(0)
	roErrors := int32(0)

	readOnlyStore, err := NewImmutableDirJWTStore(dir, true, func(pubKey string) {
		atomic.AddInt32(&roJWTChanges, 1)
		roNotified <- true
	}, func(err error) {
		atomic.AddInt32(&roErrors, 1)
		roNotified <- true
	})
	require.NoError(t, err)
	require.True(t, readOnlyStore.IsReadOnly())

	got, err := readOnlyStore.Load("one")
	require.NoError(t, err)
	require.Equal(t, "zip", got)

	store.Save("two", "zap")

	select {
	case <-roNotified:
	case <-time.After(3 * time.Second):
	}
	require.Equal(t, int32(1), atomic.LoadInt32(&roJWTChanges))
	require.Equal(t, int32(0), atomic.LoadInt32(&roErrors))

	select {
	case <-notified:
	case <-time.After(3 * time.Second):
	}
	require.Equal(t, int32(2), atomic.LoadInt32(&jwtChanges)) // still have the changes from before
	require.Equal(t, int32(0), atomic.LoadInt32(&errors))

	store.Close()
	readOnlyStore.Close()
}

func TestUnShardedDirStoreNotifications(t *testing.T) {

	// Skip the file notification test on travis
	if os.Getenv("TRAVIS_GO_VERSION") != "" {
		return
	}

	dir, err := ioutil.TempDir(os.TempDir(), "jwtstore_test")
	require.NoError(t, err)

	notified := make(chan bool)
	jwtChanges := int32(0)
	errors := int32(0)

	store, err := NewDirJWTStore(dir, false, false, func(pubKey string) {
		atomic.AddInt32(&jwtChanges, 1)
		notified <- true
	}, func(err error) {
		atomic.AddInt32(&errors, 1)
		notified <- true
	})
	require.NoError(t, err)
	require.False(t, store.IsReadOnly())

	expected := map[string]string{
		"one":   "alpha",
		"two":   "beta",
		"three": "gamma",
		"four":  "delta",
	}

	for k, v := range expected {
		store.Save(k, v)
	}

	time.Sleep(time.Second)

	for k, v := range expected {
		got, err := store.Load(k)
		require.NoError(t, err)
		require.Equal(t, v, got)
	}

	store.Save("one", "zip")

	select {
	case <-notified:
	case <-time.After(5 * time.Second):
	}
	require.Equal(t, int32(1), atomic.LoadInt32(&jwtChanges))
	require.Equal(t, int32(0), atomic.LoadInt32(&errors))

	// re-use the folder for readonly mode
	roNotified := make(chan bool)
	roJWTChanges := int32(0)
	roErrors := int32(0)

	readOnlyStore, err := NewImmutableDirJWTStore(dir, false, func(pubKey string) {
		atomic.AddInt32(&roJWTChanges, 1)
		roNotified <- true
	}, func(err error) {
		atomic.AddInt32(&roErrors, 1)
		roNotified <- true
	})
	require.NoError(t, err)
	require.True(t, readOnlyStore.IsReadOnly())

	got, err := readOnlyStore.Load("one")
	require.NoError(t, err)
	require.Equal(t, "zip", got)

	store.Save("two", "zap")

	select {
	case <-roNotified:
	case <-time.After(5 * time.Second):
	}
	require.Equal(t, int32(1), atomic.LoadInt32(&roJWTChanges))
	require.Equal(t, int32(0), atomic.LoadInt32(&roErrors))

	select {
	case <-notified:
	case <-time.After(5 * time.Second):
	}
	require.Equal(t, int32(2), atomic.LoadInt32(&jwtChanges))
	require.Equal(t, int32(0), atomic.LoadInt32(&errors))

	store.Close()
	readOnlyStore.Close()
}
