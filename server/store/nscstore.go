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
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
	nsc "github.com/nats-io/nsc/cmd/store"
)

// JWTChanged functions are called when the NSC store notices a JWT changed
type JWTChanged func(publicKey string)

// JWTError functions are called when the NSC store file watcher has an error
type JWTError func(err error)

// NSCJWTStore implements the JWT Store interface, keeping all data in NSC
type NSCJWTStore struct {
	nsc           *nsc.Store
	changed       JWTChanged
	errorOccurred JWTError
	watcher       *fsnotify.Watcher
	done          chan bool
}

// NewNSCJWTStore returns an empty, immutable NSC folder-based JWT store
func NewNSCJWTStore(dirPath string, changeNotification JWTChanged, errorNotification JWTError) (JWTStore, error) {
	nscStore, err := nsc.LoadStore(dirPath)

	if err != nil {
		return nil, err
	}

	theStore := &NSCJWTStore{
		nsc:           nscStore,
		changed:       changeNotification,
		errorOccurred: errorNotification,
	}

	err = theStore.startWatching()

	if err != nil {
		theStore.Close()
		return nil, err
	}

	return theStore, nil
}

func (store *NSCJWTStore) startWatching() error {

	watcher, err := fsnotify.NewWatcher()
	done := make(chan bool, 1)

	if err != nil {
		return err
	}

	store.watcher = watcher

	dirPath := store.nsc.Dir
	accountsPath := filepath.Join(dirPath, nsc.Accounts)

	watcher.Add(accountsPath)

	infos, err := store.nsc.List(nsc.Accounts)
	if err != nil {
		return err
	}

	for _, i := range infos {
		if i.IsDir() {
			accountJWTPath := filepath.Join(dirPath, nsc.Accounts, i.Name(), nsc.JwtName(i.Name()))
			watcher.Add(accountJWTPath)
		}
	}

	store.done = done
	go func() {
		running := true
		for running {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}

				if event.Op&fsnotify.Write == fsnotify.Write {
					// Check for jwt change, ignore others
					if strings.HasSuffix(event.Name, ".jwt") {
						fileName := filepath.Base(event.Name)
						accountName := strings.Replace(fileName, ".jwt", "", -1)
						c, err := store.nsc.LoadClaim(nsc.Accounts, accountName, nsc.JwtName(accountName))
						if err != nil {
							store.errorOccurred(err)
						}
						store.changed(c.Subject)
					}
				}

				if event.Op&fsnotify.Create == fsnotify.Create {
					if filepath.Dir(event.Name) == filepath.Join(store.nsc.Dir, nsc.Accounts) {
						acctName := filepath.Base(event.Name)
						accountJWTPath := filepath.Join(event.Name, nsc.JwtName(acctName))
						err := store.watcher.Add(accountJWTPath)
						if err != nil {
							store.errorOccurred(err)
						}
					}
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				store.errorOccurred(err)
			case <-done:
				running = false
			}
		}
	}()

	return nil
}

// Load checks the NSCory store and returns the matching JWT or an error
func (store *NSCJWTStore) Load(publicKey string) (string, error) {

	infos, err := store.nsc.List(nsc.Accounts)
	if err != nil {
		return "", err
	}

	for _, i := range infos {
		if i.IsDir() {
			c, err := store.nsc.LoadClaim(nsc.Accounts, i.Name(), nsc.JwtName(i.Name()))
			if err != nil {
				return "", err
			}

			if c != nil {
				if c.Subject == publicKey {

					data, err := store.nsc.Read(nsc.Accounts, i.Name(), nsc.JwtName(i.Name()))
					if err != nil {
						return "", err
					}

					return string(data), nil
				}
			}
		}
	}

	return "", fmt.Errorf("no matching JWT found")
}

// Save puts the JWT in a map by public key, no checks are performed
func (store *NSCJWTStore) Save(publicKey string, theJWT string) error {
	return fmt.Errorf("store is read-only")
}

// IsReadOnly returns a flag determined at creation time
func (store *NSCJWTStore) IsReadOnly() bool {
	return true
}

// Close cleans up the dir watchers
func (store *NSCJWTStore) Close() {
	if store.done != nil {
		store.done <- true
	}
	if store.watcher != nil {
		store.watcher.Close()
	}
}
