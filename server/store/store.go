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

// JWTStore is the interface for all store implementations in the account server
// The store provides a handful of methods for setting and getting a JWT.
// The data doesn't really have to be a JWT, no validation is expected at this level
type JWTStore interface {
	Load(publicKey string) (string, error)
	Save(publicKey string, theJWT string) error
	IsReadOnly() bool
}
