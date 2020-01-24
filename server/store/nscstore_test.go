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
	"os"
	"testing"
	"time"

	"github.com/nats-io/jwt"
	"github.com/stretchr/testify/require"
)

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

	store, err := NewNSCJWTStore(s.Dir, func(pubKey string) {}, func(err error) {})
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

	store.Close()
}

func TestBadFolderNSCStore(t *testing.T) {
	store, err := NewNSCJWTStore("/a/b/c", func(pubKey string) {}, func(err error) {})
	require.Error(t, err)
	require.Nil(t, store)
}

func TestNSCFileNotifications(t *testing.T) {

	// Skip the file notification test on travis
	if os.Getenv("TRAVIS_GO_VERSION") != "" {
		return
	}

	_, _, kp := CreateOperatorKey(t)
	_, apub, _ := CreateAccountKey(t)
	s := CreateTestStoreForOperator(t, "x", kp)

	notified := make(chan bool, 1)
	errors := make(chan error, 10)

	store, err := NewNSCJWTStore(s.Dir, func(pubKey string) {
		notified <- true
	}, func(err error) {
		errors <- err
	})
	require.NoError(t, err)
	defer store.Close()

	c := jwt.NewAccountClaims(apub)
	c.Name = "foo"
	cd, err := c.Encode(kp)
	require.NoError(t, err)
	err = s.StoreClaim([]byte(cd))
	require.NoError(t, err)

	c.Tags.Add("red")
	cd, err = c.Encode(kp)
	require.NoError(t, err)
	err = s.StoreClaim([]byte(cd))
	require.NoError(t, err)

	// On Windows, we get notifications due to create and update,
	// but sometimes more than once. So the test will be really
	// checking at we get at least 1 notification when we modify.
	check := func() {
		t.Helper()
		var ok bool
		var done bool
		for !done {
			select {
			case <-notified:
				ok = true
			case e := <-errors:
				t.Fatal(e.Error())
			case <-time.After(500 * time.Millisecond):
				done = true
			}
		}
		if !ok {
			t.Fatalf("Did not get any notification")
		}
	}
	check()

	c.Tags.Add("blue")
	cd, err = c.Encode(kp)
	require.NoError(t, err)
	err = s.StoreClaim([]byte(cd))
	require.NoError(t, err)

	check()

	theJWT, err := store.Load(c.Subject)
	require.NoError(t, err)
	require.Equal(t, cd, theJWT)

	select {
	case <-notified:
		t.Fatal("Should not have been notified")
	default:
	}
}
