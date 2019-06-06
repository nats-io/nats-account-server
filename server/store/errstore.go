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
)

// ErrJWTStore returns errors when possible
type ErrJWTStore struct {
	Loads  int
	Saves  int
	Closes int
}

// NewErrJWTStore returns an empty, mutable in-memory JWT store
func NewErrJWTStore() JWTStore {
	return &ErrJWTStore{}
}

// Load checks the memory store and returns the matching JWT or an error
func (store *ErrJWTStore) Load(publicKey string) (string, error) {
	store.Loads++
	return "", fmt.Errorf("always error")
}

// Save puts the JWT in a map by public key, no checks are performed
func (store *ErrJWTStore) Save(publicKey string, theJWT string) error {
	store.Saves++
	return fmt.Errorf("always error")
}

// IsReadOnly returns a flag determined at creation time
func (store *ErrJWTStore) IsReadOnly() bool {
	return false
}

// Close is a no-op for a mem store
func (store *ErrJWTStore) Close() {
	store.Closes++
}
