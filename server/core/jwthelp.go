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

package core

const jwtAPIHelp = `
# NATS Account Server JWT API HELP

This document describes the various URL paths that encompass the HTTP API for working 
with JWTs on the NATS Account Server

## GET /jwt/v1/help

Returns this page.

## GET /jwt/v1/accounts/<pubkey>

Retieve an account JWT by the public key. The result is either an error
or the encoded JWT.

The response contains cache control headers, and uses the JTI as the ETag.

The response has content type application/jwt and may cause a download in a browser.

The JWT is not validated for expiration or revocation. [see check below]

A 304 is returned if the request contains the appropriate If-None-Match header.

A status 404 is returned if the JWT is not found.

Three optional query parameters are supported:

  * check - can be set to "true" which will tell the server to return 404 if the JWT is expired
  * text - can be set to "true" to change the content type to text/plain
  * decode - can be set to "true" to display the JSON for the JWT header and body

## POST /jwt/v1/accounts/<pubkey> (optional)

Update, or store, an account JWT. The JWT Subject should match the pubkey.

The JWT must be signed by the operator specified in the server's configuration.

A status 400 is returned if there is a problem with the JWT or the server is in read-only mode. In rare
cases a status 500 may be returned if there was an issue saving the JWT.
`
