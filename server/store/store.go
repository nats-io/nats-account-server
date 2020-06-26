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

// JWTStore is the interface for all store implementations in the account server
// The store provides a handful of methods for setting and getting a JWT.
// The data doesn't really have to be a JWT, no validation is expected at this level
type JWTStore interface {
	LoadAcc(publicKey string) (string, error)
	SaveAcc(publicKey string, theJWT string) error
	IsReadOnly() bool
	Close()
}

// JWTStore extension to also store activations.
type JWTActivationStore interface {
	LoadAct(hash string) (string, error)
	SaveAct(hash string, theJWT string) error
}

// PackableJWTStore is implemented by stores that can pack up their content or
// merge content from another stores pack. The format of a packed store is a
// single string with 1 JWT per line, \n is as the line separator. The line format is:
// pubkey|encodedjwt\n
// Stores with locking may be locked during pack/merge which should be considered
// in very high performance situations.
// No preference is required on the JWTs included if maxJWTS is less than the total, that
// is store dependent. Merge implies writability and does not check the "is readonly" flag
// of a store
type PackableJWTStore interface {
	// Pack the jwts, up to maxJWTs. If maxJWTs is negative, do not limit.
	Pack(maxJWTs int) (string, error)
	Merge(pack string) error
}

// JWTChanged functions are called when the store file watcher notices a JWT changed
type JWTChanged func(publicKey string)

// JWTError functions are called when the store file watcher has an error
type JWTError func(err error)

type StorageError struct {
	msg string
	err error
}

func NewError(msg string, wrappedErr error) error {
	return &StorageError{msg, wrappedErr}
}
func (e *StorageError) Unwrap() error { return e.err }
func (e *StorageError) Error() string { return e.msg }
func (e *StorageError) FullError() string {
	if e.err != nil {
		return fmt.Sprintf("%s %s", e.msg, e.err.Error())
	}
	return e.msg
}
