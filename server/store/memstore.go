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
	"strings"
)

// MemJWTStore implements the JWT Store interface, keeping all data in memory
// The store is not thread safe
type MemJWTStore struct {
	jwts     map[string]string
	readonly bool
}

// NewMemJWTStore returns an empty, mutable in-memory JWT store
func NewMemJWTStore() JWTStore {
	return &MemJWTStore{
		jwts:     map[string]string{},
		readonly: false,
	}
}

// NewImmutableMemJWTStore returns an immutable store with the provided map
func NewImmutableMemJWTStore(theJWTs map[string]string) JWTStore {
	return &MemJWTStore{
		jwts:     theJWTs,
		readonly: true,
	}
}

// Load checks the memory store and returns the matching JWT or an error
func (store *MemJWTStore) Load(publicKey string) (string, error) {
	theJWT, ok := store.jwts[publicKey]

	if ok {
		return theJWT, nil
	}

	return "", fmt.Errorf("no matching JWT found")
}

// Save puts the JWT in a map by public key, no checks are performed
func (store *MemJWTStore) Save(publicKey string, theJWT string) error {
	if store.readonly {
		return fmt.Errorf("store is read-only")
	}
	store.jwts[publicKey] = theJWT
	return nil
}

// IsReadOnly returns a flag determined at creation time
func (store *MemJWTStore) IsReadOnly() bool {
	return store.readonly
}

// Close is a no-op for a mem store
func (store *MemJWTStore) Close() {
}

// Pack up to maxJWTs into a package
func (store *MemJWTStore) Pack(maxJWTs int) (string, error) {
	count := 0
	var pack []string

	if maxJWTs > 0 {
		pack = make([]string, 0, maxJWTs)
	} else {
		pack = []string{}
	}

	for pubkey, jwt := range store.jwts {
		pack = append(pack, fmt.Sprintf("%s|%s", pubkey, jwt))
		count++

		if count == maxJWTs { // won't match negative
			break
		}
	}

	return strings.Join(pack, "\n"), nil
}

// Merge takes the JWTs from package and adds them to the store
// Merge is destructive in the sense that it doesn't check if the JWT
// is newer or anything like that.
func (store *MemJWTStore) Merge(pack string) error {
	newJWTs := strings.Split(pack, "\n")

	for _, line := range newJWTs {
		if line == "" { // ignore blank lines
			continue
		}

		split := strings.Split(line, "|")
		if len(split) != 2 {
			return fmt.Errorf("line in package didn't contain 2 entries: %q", line)
		}

		store.Save(split[0], split[1])
	}

	return nil
}
