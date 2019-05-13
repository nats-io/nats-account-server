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

	"github.com/nats-io/nats-account-server/server/conf"
)

const (
	extension = "jwt"
)

// DirJWTStore implements the JWT Store interface, keeping JWTs in an optionally sharded
// directory structure
type DirJWTStore struct {
	directory string
	readonly  bool
	shard     bool
}

// NewDirJWTStore returns an empty, mutable directory-based JWT store
func NewDirJWTStore(dirPath string, shard bool, create bool) (JWTStore, error) {
	dirPath, err := conf.ValidateDirPath(dirPath)

	if err != nil {
		if !create {
			return nil, err
		}

		err := os.MkdirAll(dirPath, 0755)

		if err != nil {
			return nil, err
		}
	}

	return &DirJWTStore{
		directory: dirPath,
		readonly:  false,
		shard:     shard,
	}, nil
}

// NewImmutableDirJWTStore returns an immutable store with the provided directory
func NewImmutableDirJWTStore(dirPath string, sharded bool) (JWTStore, error) {
	dirPath, err := conf.ValidateDirPath(dirPath)

	if err != nil {
		return nil, err
	}

	return &DirJWTStore{
		directory: dirPath,
		readonly:  true,
		shard:     sharded,
	}, nil
}

// Load checks the memory store and returns the matching JWT or an error
func (store *DirJWTStore) Load(publicKey string) (string, error) {
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
