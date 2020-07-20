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
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
	"github.com/stretchr/testify/require"
)

func TestShardedDirStoreWriteAndReadonly(t *testing.T) {
	dir, err := ioutil.TempDir(os.TempDir(), "jwtstore_test")
	require.NoError(t, err)

	store, err := NewDirJWTStore(dir, true, false, nil, nil)
	require.NoError(t, err)

	expected := map[string]string{
		"one":   "alpha",
		"two":   "beta",
		"three": "gamma",
		"four":  "delta",
	}

	require.False(t, store.IsReadOnly())

	for k, v := range expected {
		store.SaveAcc(k, v)
	}

	for k, v := range expected {
		got, err := store.LoadAcc(k)
		require.NoError(t, err)
		require.Equal(t, v, got)
	}

	got, err := store.LoadAcc("random")
	require.Error(t, err)
	require.Equal(t, "", got)

	got, err = store.LoadAcc("")
	require.Error(t, err)
	require.Equal(t, "", got)

	err = store.SaveAcc("", "onetwothree")
	require.Error(t, err)
	store.Close()

	// re-use the folder for readonly mode
	store, err = NewImmutableDirJWTStore(dir, true, func(pubKey string) {}, func(err error) {})
	require.NoError(t, err)

	require.True(t, store.IsReadOnly())

	err = store.SaveAcc("five", "omega")
	require.Error(t, err)

	for k, v := range expected {
		got, err := store.LoadAcc(k)
		require.NoError(t, err)
		require.Equal(t, v, got)
	}
	store.Close()
}

func TestUnshardedDirStoreWriteAndReadonly(t *testing.T) {
	dir, err := ioutil.TempDir(os.TempDir(), "jwtstore_test")
	require.NoError(t, err)

	store, err := NewDirJWTStore(dir, false, false, nil, nil)
	require.NoError(t, err)

	expected := map[string]string{
		"one":   "alpha",
		"two":   "beta",
		"three": "gamma",
		"four":  "delta",
	}

	require.False(t, store.IsReadOnly())

	for k, v := range expected {
		store.SaveAcc(k, v)
	}

	for k, v := range expected {
		got, err := store.LoadAcc(k)
		require.NoError(t, err)
		require.Equal(t, v, got)
	}

	got, err := store.LoadAcc("random")
	require.Error(t, err)
	require.Equal(t, "", got)

	got, err = store.LoadAcc("")
	require.Error(t, err)
	require.Equal(t, "", got)

	err = store.SaveAcc("", "onetwothree")
	require.Error(t, err)
	store.Close()

	// re-use the folder for readonly mode
	store, err = NewImmutableDirJWTStore(dir, false, func(pubKey string) {}, func(err error) {})
	require.NoError(t, err)

	require.True(t, store.IsReadOnly())

	err = store.SaveAcc("five", "omega")
	require.Error(t, err)

	for k, v := range expected {
		got, err := store.LoadAcc(k)
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

func TestCreateMakesDir(t *testing.T) {
	dir, err := ioutil.TempDir(os.TempDir(), "jwtstore_test")
	require.NoError(t, err)

	fullPath := filepath.Join(dir, "a/b")

	_, err = os.Stat(fullPath)
	require.Error(t, err)
	require.True(t, os.IsNotExist(err))

	s, err := NewDirJWTStore(fullPath, false, true, func(pubKey string) {}, func(err error) {})
	require.NoError(t, err)
	s.Close()

	_, err = os.Stat(fullPath)
	require.NoError(t, err)
}

func TestDirStoreNotifications(t *testing.T) {

	// Skip the file notification test on travis
	if os.Getenv("TRAVIS_GO_VERSION") != "" {
		return
	}

	for _, test := range []struct {
		name    string
		sharded bool
	}{
		{"sharded", true},
		{"unsharded", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir, err := ioutil.TempDir(os.TempDir(), "jwtstore_test")
			require.NoError(t, err)
			defer os.RemoveAll(dir)

			notified := make(chan bool, 1)
			errors := make(chan error, 10)
			wStoreState := int32(0)

			store, err := NewDirJWTStore(dir, test.sharded, false, func(pubKey string) {
				n := atomic.LoadInt32(&wStoreState)
				switch n {
				case 0:
					return
				case 1:
					if pubKey == "one" {
						notified <- true
						atomic.StoreInt32(&wStoreState, 0)
					}
				case 2:
					if pubKey == "two" {
						notified <- true
						atomic.StoreInt32(&wStoreState, 0)
					}
				}
			}, func(err error) {
				errors <- err
			})
			require.NoError(t, err)
			defer store.Close()
			require.True(t, store.IsReadOnly())

			expected := map[string]string{
				"one":   "alpha",
				"two":   "beta",
				"three": "gamma",
				"four":  "delta",
			}

			for k, v := range expected {
				require.Error(t, store.SaveAcc(k, v))
				if test.sharded {
					require.NoError(t, os.MkdirAll(fmt.Sprintf("%s/%s/", dir, k[len(k)-2:]), 0755))
					require.NoError(t, ioutil.WriteFile(fmt.Sprintf("%s/%s/%s.jwt", dir, k[len(k)-2:], k), []byte(v), 0644))
				} else {
					require.NoError(t, ioutil.WriteFile(fmt.Sprintf("%s/%s.jwt", dir, k), []byte(v), 0644))
				}
			}

			time.Sleep(time.Second)

			for k, v := range expected {
				got, err := store.LoadAcc(k)
				require.NoError(t, err)
				require.Equal(t, v, got)
			}

			atomic.StoreInt32(&wStoreState, 1)
			require.Error(t, store.SaveAcc("one", "zip"))
			if test.sharded {
				require.NoError(t, ioutil.WriteFile(fmt.Sprintf("%s/ne/%s.jwt", dir, "one"), []byte("zip"), 0644))
			} else {
				require.NoError(t, ioutil.WriteFile(fmt.Sprintf("%s/%s.jwt", dir, "one"), []byte("zip"), 0644))
			}

			check := func() {
				t.Helper()
				select {
				case <-notified:
				case e := <-errors:
					t.Fatal(e.Error())
				case <-time.After(5 * time.Second):
					t.Fatalf("Did not get notified")
				}
			}
			check()

			// re-use the folder for readonly mode
			roStoreState := int32(0)
			readOnlyStore, err := NewImmutableDirJWTStore(dir, test.sharded, func(pubKey string) {
				n := atomic.LoadInt32(&roStoreState)
				switch n {
				case 0:
					return
				case 1:
					if pubKey == "two" {
						notified <- true
						atomic.StoreInt32(&roStoreState, 0)
					}
				}
			}, func(err error) {
				errors <- err
			})
			require.NoError(t, err)
			defer readOnlyStore.Close()
			require.True(t, readOnlyStore.IsReadOnly())

			got, err := readOnlyStore.LoadAcc("one")
			require.NoError(t, err)
			require.Equal(t, "zip", got)

			atomic.StoreInt32(&roStoreState, 1)
			atomic.StoreInt32(&wStoreState, 2)

			if test.sharded {
				require.NoError(t, ioutil.WriteFile(fmt.Sprintf("%s/wo/%s.jwt", dir, "two"), []byte("zap"), 0644))
			} else {
				require.NoError(t, ioutil.WriteFile(fmt.Sprintf("%s/%s.jwt", dir, "two"), []byte("zap"), 0644))
			}

			for i := 0; i < 2; i++ {
				check()
			}
		})
	}
}

func TestShardedDirStorePackMerge(t *testing.T) {
	dir, err := ioutil.TempDir(os.TempDir(), "jwtstore_test")
	require.NoError(t, err)
	dir2, err := ioutil.TempDir(os.TempDir(), "jwtstore_test")
	require.NoError(t, err)
	dir3, err := ioutil.TempDir(os.TempDir(), "jwtstore_test")
	require.NoError(t, err)

	store, err := NewDirJWTStore(dir, true, false, nil, nil)
	require.NoError(t, err)

	expected := map[string]string{
		"one":   "alpha",
		"two":   "beta",
		"three": "gamma",
		"four":  "delta",
	}

	require.False(t, store.IsReadOnly())

	for k, v := range expected {
		store.SaveAcc(k, v)
	}

	for k, v := range expected {
		got, err := store.LoadAcc(k)
		require.NoError(t, err)
		require.Equal(t, v, got)
	}

	got, err := store.LoadAcc("random")
	require.Error(t, err)
	require.Equal(t, "", got)

	packable, ok := store.(PackableJWTStore)
	require.True(t, ok)

	pack, err := packable.Pack(-1)
	require.NoError(t, err)

	inc, err := NewDirJWTStore(dir2, true, false, nil, nil)
	require.NoError(t, err)

	incP, ok := inc.(PackableJWTStore)
	require.True(t, ok)

	incP.Merge(pack)

	for k, v := range expected {
		got, err := inc.LoadAcc(k)
		require.NoError(t, err)
		require.Equal(t, v, got)
	}

	got, err = inc.LoadAcc("random")
	require.Error(t, err)
	require.Equal(t, "", got)

	limitedPack, err := packable.Pack(1)
	require.NoError(t, err)

	limited, err := NewDirJWTStore(dir3, true, false, nil, nil)

	require.NoError(t, err)
	limitedP, ok := limited.(PackableJWTStore)
	require.True(t, ok)

	limitedP.Merge(limitedPack)

	count := 0
	for k, v := range expected {
		got, err := limited.LoadAcc(k)
		if err == nil {
			count++
			require.Equal(t, v, got)
		}
	}

	require.Equal(t, 1, count)

	got, err = inc.LoadAcc("random")
	require.Error(t, err)
	require.Equal(t, "", got)
}

func TestShardedToUnsharedDirStorePackMerge(t *testing.T) {
	dir, err := ioutil.TempDir(os.TempDir(), "jwtstore_test")
	require.NoError(t, err)
	dir2, err := ioutil.TempDir(os.TempDir(), "jwtstore_test")
	require.NoError(t, err)

	store, err := NewDirJWTStore(dir, true, false, nil, nil)
	require.NoError(t, err)

	expected := map[string]string{
		"one":   "alpha",
		"two":   "beta",
		"three": "gamma",
		"four":  "delta",
	}

	require.False(t, store.IsReadOnly())

	for k, v := range expected {
		store.SaveAcc(k, v)
	}

	for k, v := range expected {
		got, err := store.LoadAcc(k)
		require.NoError(t, err)
		require.Equal(t, v, got)
	}

	got, err := store.LoadAcc("random")
	require.Error(t, err)
	require.Equal(t, "", got)

	packable, ok := store.(PackableJWTStore)
	require.True(t, ok)

	pack, err := packable.Pack(-1)
	require.NoError(t, err)

	inc, err := NewDirJWTStore(dir2, false, false, nil, nil)
	require.NoError(t, err)

	incP, ok := inc.(PackableJWTStore)
	require.True(t, ok)

	incP.Merge(pack)

	for k, v := range expected {
		got, err := inc.LoadAcc(k)
		require.NoError(t, err)
		require.Equal(t, v, got)
	}

	got, err = inc.LoadAcc("random")
	require.Error(t, err)
	require.Equal(t, "", got)

	err = packable.Merge("foo")
	require.Error(t, err)

	err = packable.Merge("") // will skip it
	require.NoError(t, err)

	err = packable.Merge("a|something") // should fail on a for sharding
	require.Error(t, err)
}

func TestMergeOnlyOnNewer(t *testing.T) {
	dir, err := ioutil.TempDir(os.TempDir(), "jwtstore_test")
	require.NoError(t, err)

	store, err := NewDirJWTStore(dir, true, false, nil, nil)
	require.NoError(t, err)

	dirStore := store.(*DirJWTStore)

	accountKey, err := nkeys.CreateAccount()
	require.NoError(t, err)

	pubKey, err := accountKey.PublicKey()
	require.NoError(t, err)

	account := jwt.NewAccountClaims(pubKey)
	account.Name = "old"
	olderJWT, err := account.Encode(accountKey)
	require.NoError(t, err)

	time.Sleep(2 * time.Second)

	account.Name = "new"
	newerJWT, err := account.Encode(accountKey)
	require.NoError(t, err)

	// Should work
	err = dirStore.SaveAcc(pubKey, olderJWT)
	require.NoError(t, err)
	fromStore, err := dirStore.LoadAcc(pubKey)
	require.NoError(t, err)
	require.Equal(t, olderJWT, fromStore)

	// should replace
	err = dirStore.saveIfNewer(pubKey, newerJWT)
	require.NoError(t, err)
	fromStore, err = dirStore.LoadAcc(pubKey)
	require.NoError(t, err)
	require.Equal(t, newerJWT, fromStore)

	// should fail
	err = dirStore.saveIfNewer(pubKey, olderJWT)
	require.NoError(t, err)
	fromStore, err = dirStore.LoadAcc(pubKey)
	require.NoError(t, err)
	require.Equal(t, newerJWT, fromStore)
}

func createAccount(t *testing.T, dirStore *DirJWTStore, expSec int, accKey nkeys.KeyPair) {
	pubKey, err := accKey.PublicKey()
	require.NoError(t, err)

	account := jwt.NewAccountClaims(pubKey)
	if expSec > 0 {
		account.Expires = time.Now().Add(time.Second * time.Duration(expSec)).Unix()
	}
	olderJWT, err := account.Encode(accKey)
	require.NoError(t, err)
	dirStore.SaveAcc(pubKey, olderJWT)

}

func assertStoreSize(t *testing.T, dirStore *DirJWTStore, length int) {
	f, err := ioutil.ReadDir(dirStore.directory)
	require.NoError(t, err)
	require.Len(t, f, length)
	dirStore.Lock()
	require.Len(t, dirStore.expiration.idx, length)
	require.Equal(t, dirStore.expiration.lru.Len(), length)
	require.Len(t, dirStore.expiration.heap, length)
	dirStore.Unlock()
}

func TestExpiration(t *testing.T) {
	dir, err := ioutil.TempDir(os.TempDir(), "jwtstore_test")
	require.NoError(t, err)

	store, err := NewExpiringDirJWTStore(dir, false, false, time.Millisecond*100, 10)
	require.NoError(t, err)
	defer store.Close()

	dirStore := store.(*DirJWTStore)

	account := func(expSec int) {
		accountKey, err := nkeys.CreateAccount()
		require.NoError(t, err)
		createAccount(t, dirStore, expSec, accountKey)
	}

	h := dirStore.Hash()

	for i := 1; i <= 5; i++ {
		account(i * 2)
		nh := dirStore.Hash()
		require.NotEqual(t, h, nh)
		h = nh
	}
	time.Sleep(1 * time.Second)
	for i := 5; i > 0; i-- {
		f, err := ioutil.ReadDir(dir)
		require.NoError(t, err)
		require.Len(t, f, i)
		assertStoreSize(t, dirStore, i)

		time.Sleep(2 * time.Second)

		nh := dirStore.Hash()
		require.NotEqual(t, h, nh)
		h = nh
	}
	assertStoreSize(t, dirStore, 0)
}

func TestLimit(t *testing.T) {
	dir, err := ioutil.TempDir(os.TempDir(), "jwtstore_test")
	require.NoError(t, err)

	store, err := NewExpiringDirJWTStore(dir, false, false, time.Millisecond*100, 5)
	require.NoError(t, err)
	defer store.Close()

	dirStore := store.(*DirJWTStore)

	account := func(expSec int) {
		accountKey, err := nkeys.CreateAccount()
		require.NoError(t, err)
		createAccount(t, dirStore, expSec, accountKey)
	}

	h := dirStore.Hash()

	accountKey, err := nkeys.CreateAccount()
	require.NoError(t, err)
	// update first account
	for i := 0; i < 10; i++ {
		createAccount(t, dirStore, 50, accountKey)
		assertStoreSize(t, dirStore, 1)
	}
	// new accounts
	for i := 0; i < 10; i++ {
		account(i)
		nh := dirStore.Hash()
		require.NotEqual(t, h, nh)
		h = nh
	}
	// first account should be gone now accountKey.PublicKey()
	key, _ := accountKey.PublicKey()
	_, err = os.Stat(fmt.Sprintf("%s/%s.jwt", dir, key))
	require.True(t, os.IsNotExist(err))

	// update first account
	for i := 0; i < 10; i++ {
		createAccount(t, dirStore, 50, accountKey)
		assertStoreSize(t, dirStore, 5)
	}
}

func TestLru(t *testing.T) {
	dir, err := ioutil.TempDir(os.TempDir(), "jwtstore_test")
	require.NoError(t, err)

	store, err := NewExpiringDirJWTStore(dir, false, false, time.Millisecond*100, 2)
	require.NoError(t, err)
	defer store.Close()

	dirStore := store.(*DirJWTStore)

	accountKey1, err := nkeys.CreateAccount()
	require.NoError(t, err)
	pKey1, err := accountKey1.PublicKey()
	require.NoError(t, err)
	accountKey2, err := nkeys.CreateAccount()
	require.NoError(t, err)
	accountKey3, err := nkeys.CreateAccount()
	require.NoError(t, err)
	pKey3, err := accountKey3.PublicKey()
	require.NoError(t, err)

	createAccount(t, dirStore, 10, accountKey1)
	assertStoreSize(t, dirStore, 1)
	createAccount(t, dirStore, 10, accountKey2)
	assertStoreSize(t, dirStore, 2)
	createAccount(t, dirStore, 10, accountKey3)
	assertStoreSize(t, dirStore, 2)
	_, err = os.Stat(fmt.Sprintf("%s/%s.jwt", dir, pKey1))
	require.True(t, os.IsNotExist(err))

	// update
	createAccount(t, dirStore, 10, accountKey2)
	assertStoreSize(t, dirStore, 2)
	// recreate
	createAccount(t, dirStore, 1, accountKey1)
	assertStoreSize(t, dirStore, 2)
	_, err = os.Stat(fmt.Sprintf("%s/%s.jwt", dir, pKey3))
	require.True(t, os.IsNotExist(err))
	// let key1 expire
	time.Sleep(2 * time.Second)
	assertStoreSize(t, dirStore, 1)
	_, err = os.Stat(fmt.Sprintf("%s/%s.jwt", dir, pKey1))
	require.True(t, os.IsNotExist(err))
	// recreate key3 - no eviction
	createAccount(t, dirStore, 10, accountKey3)
	assertStoreSize(t, dirStore, 2)
}

func TestExpirationUpdate(t *testing.T) {
	dir, err := ioutil.TempDir(os.TempDir(), "jwtstore_test")
	require.NoError(t, err)

	store, err := NewExpiringDirJWTStore(dir, false, false, time.Millisecond*100, 10)
	require.NoError(t, err)
	defer store.Close()

	dirStore := store.(*DirJWTStore)

	accountKey, err := nkeys.CreateAccount()
	require.NoError(t, err)

	h := dirStore.Hash()

	createAccount(t, dirStore, 0, accountKey)
	nh := dirStore.Hash()
	require.NotEqual(t, h, nh)
	h = nh

	time.Sleep(2 * time.Second)
	f, err := ioutil.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, f, 1)

	createAccount(t, dirStore, 5, accountKey)
	nh = dirStore.Hash()
	require.NotEqual(t, h, nh)
	h = nh

	time.Sleep(2 * time.Second)
	f, err = ioutil.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, f, 1)

	createAccount(t, dirStore, 0, accountKey)
	nh = dirStore.Hash()
	require.NotEqual(t, h, nh)
	h = nh

	time.Sleep(2 * time.Second)
	f, err = ioutil.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, f, 1)

	createAccount(t, dirStore, 1, accountKey)
	nh = dirStore.Hash()
	require.NotEqual(t, h, nh)
	h = nh

	time.Sleep(2 * time.Second)
	f, err = ioutil.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, f, 0)

	empty := [32]byte{}
	h = dirStore.Hash()
	require.Equal(t, h, empty)
}
