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
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/nats-io/jwt"
	"github.com/nats-io/nats-account-server/server/conf"
)

const (
	extension = "jwt"
)

// DirJWTStore implements the JWT Store interface, keeping JWTs in an optionally sharded
// directory structure
type DirJWTStore struct {
	sync.Mutex

	directory     string
	readonly      bool
	shard         bool
	changed       JWTChanged
	errorOccurred JWTError
	watcher       *fsnotify.Watcher
	done          chan bool
}

// NewDirJWTStore returns an empty, mutable directory-based JWT store
func NewDirJWTStore(dirPath string, shard bool, create bool, changeNotification JWTChanged, errorNotification JWTError) (JWTStore, error) {
	fullPath, err := conf.ValidateDirPath(dirPath)

	if err != nil {
		if !create {
			return nil, err
		}

		err = os.MkdirAll(dirPath, 0755)

		if err != nil {
			return nil, err
		}

		fullPath, err = conf.ValidateDirPath(dirPath)

		if err != nil {
			return nil, err
		}
	}

	theStore := &DirJWTStore{
		directory:     fullPath,
		readonly:      false,
		shard:         shard,
		changed:       changeNotification,
		errorOccurred: errorNotification,
	}

	if changeNotification != nil && errorNotification != nil {
		err = theStore.startWatching()

		if err != nil {
			theStore.Close()
			return nil, err
		}
	}

	return theStore, err
}

// NewImmutableDirJWTStore returns an immutable store with the provided directory
func NewImmutableDirJWTStore(dirPath string, sharded bool, changeNotification JWTChanged, errorNotification JWTError) (JWTStore, error) {
	dirPath, err := conf.ValidateDirPath(dirPath)

	if err != nil {
		return nil, err
	}

	theStore := &DirJWTStore{
		directory:     dirPath,
		readonly:      true,
		shard:         sharded,
		changed:       changeNotification,
		errorOccurred: errorNotification,
	}

	if changeNotification != nil && errorNotification != nil {
		err = theStore.startWatching()

		if err != nil {
			theStore.Close()
			return nil, err
		}
	}

	return theStore, err
}

func (store *DirJWTStore) startWatching() error {
	store.Lock()
	defer store.Unlock()

	watcher, err := fsnotify.NewWatcher()
	done := make(chan bool, 1)

	if err != nil {
		return err
	}

	store.watcher = watcher

	// Watch the top level dir (could be sharded)
	dirPath := store.directory
	watcher.Add(dirPath)

	var files []string
	err = filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if info.IsDir() && store.shard && filepath.Dir(path) == store.directory {
			files = append(files, path)
		}

		if !info.IsDir() && strings.HasSuffix(path, extension) {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return err
	}

	for _, file := range files {
		watcher.Add(file)
	}

	store.done = done

	go func() {
		running := true
		store.Lock()
		watcher := store.watcher
		store.Unlock()

		for running && watcher != nil {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}

				if event.Op&fsnotify.Write == fsnotify.Write {
					// Check for jwt change, ignore others
					if strings.HasSuffix(event.Name, extension) {
						fileName := filepath.Base(event.Name)
						pubKey := strings.Replace(fileName, ".jwt", "", -1)
						store.changed(pubKey)
					}
				} else if event.Op&fsnotify.Create == fsnotify.Create {
					if strings.HasSuffix(event.Name, extension) {
						err := watcher.Add(event.Name)
						if err != nil {
							store.errorOccurred(err)
						}
					} else if filepath.Dir(event.Name) == store.directory && store.shard { // Only go 1 level down
						err := watcher.Add(event.Name)
						if err != nil {
							store.errorOccurred(err)
						}
						var files []string
						err = filepath.Walk(event.Name, func(path string, info os.FileInfo, err error) error {
							if !info.IsDir() && strings.HasSuffix(path, extension) {
								files = append(files, path)
							}
							return nil
						})

						if err != nil {
							store.errorOccurred(err)
							break
						}

						for _, file := range files {
							watcher.Add(file)
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

// Load checks the memory store and returns the matching JWT or an error
func (store *DirJWTStore) Load(publicKey string) (string, error) {
	store.Lock()
	defer store.Unlock()

	path := store.pathForKey(publicKey)

	if path == "" {
		return "", fmt.Errorf("invalid public key")
	}

	data, err := ioutil.ReadFile(path)

	if err != nil {
		return "", err
	}

	return string(data), nil
}

// Save puts the JWT in a map by public key, no checks are performed
func (store *DirJWTStore) Save(publicKey string, theJWT string) error {
	store.Lock()
	defer store.Unlock()

	if store.readonly {
		return fmt.Errorf("store is read-only")
	}

	path := store.pathForKey(publicKey)

	if path == "" {
		return fmt.Errorf("invalid public key")
	}

	dirPath := filepath.Dir(path)
	_, err := conf.ValidateDirPath(dirPath)
	if err != nil {
		err := os.MkdirAll(dirPath, 0755)
		if err != nil {
			return err
		}
	}

	return ioutil.WriteFile(path, []byte(theJWT), 0644)
}

// IsReadOnly returns a flag determined at creation time
func (store *DirJWTStore) IsReadOnly() bool {
	return store.readonly
}

func (store *DirJWTStore) pathForKey(publicKey string) string {
	if len(publicKey) < 2 {
		return ""
	}

	var dirPath string

	if store.shard {
		last := publicKey[len(publicKey)-2:]
		fileName := fmt.Sprintf("%s.%s", publicKey, extension)
		dirPath = filepath.Join(store.directory, last, fileName)
	} else {
		fileName := fmt.Sprintf("%s.%s", publicKey, extension)
		dirPath = filepath.Join(store.directory, fileName)
	}

	return dirPath
}

// Close is a no-op for a directory store
func (store *DirJWTStore) Close() {
	store.Lock()
	defer store.Unlock()

	if store.done != nil {
		store.done <- true
	}
	if store.watcher != nil {
		store.watcher.Close()
	}
	store.watcher = nil
	store.done = nil
}

// Pack up to maxJWTs into a package
func (store *DirJWTStore) Pack(maxJWTs int) (string, error) {
	count := 0
	var pack []string

	if maxJWTs > 0 {
		pack = make([]string, 0, maxJWTs)
	} else {
		pack = []string{}
	}

	store.Lock()

	dirPath := store.directory

	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if !info.IsDir() && strings.HasSuffix(path, extension) { // this is a JWT
			if count == maxJWTs { // won't match negative
				return nil
			}

			pubKey := filepath.Base(path)
			pubKey = pubKey[0:strings.Index(pubKey, ".")]

			jwtBytes, err := ioutil.ReadFile(path)
			if err != nil {
				return err
			}

			pack = append(pack, fmt.Sprintf("%s|%s", pubKey, string(jwtBytes)))
			count++
		}
		return nil
	})

	store.Unlock()

	if err != nil {
		return "", err
	}

	return strings.Join(pack, "\n"), nil
}

// Merge takes the JWTs from package and adds them to the store
// Merge is destructive in the sense that it doesn't check if the JWT
// is newer or anything like that.
func (store *DirJWTStore) Merge(pack string) error {
	newJWTs := strings.Split(pack, "\n")

	store.Lock()
	defer store.Unlock()

	for _, line := range newJWTs {
		if line == "" { // ignore blank lines
			continue
		}

		split := strings.Split(line, "|")
		if len(split) != 2 {
			return fmt.Errorf("line in package didn't contain 2 entries: %q", line)
		}

		if err := store.saveIfNewer(split[0], split[1]); err != nil {
			return err
		}
	}

	return nil
}

// Assumes the lock is held, and only updates if the jwt is new, or the one on disk is older
func (store *DirJWTStore) saveIfNewer(publicKey string, theJWT string) error {
	path := store.pathForKey(publicKey)

	if path == "" {
		return fmt.Errorf("invalid public key")
	}

	dirPath := filepath.Dir(path)
	_, err := conf.ValidateDirPath(dirPath)
	if err != nil {
		err := os.MkdirAll(dirPath, 0755)
		if err != nil {
			return err
		}
	}

	if _, err := os.Stat(path); err == nil {
		newJWT, err := jwt.DecodeGeneric(theJWT)
		if err != nil {
			return err
		}

		existing, err := ioutil.ReadFile(path)
		if err != nil {
			return err
		}

		existingJWT, err := jwt.DecodeGeneric(string(existing))
		if err != nil {
			return err
		}

		if existingJWT.IssuedAt > newJWT.IssuedAt {
			return nil
		}
	}

	return ioutil.WriteFile(path, []byte(theJWT), 0644)
}
