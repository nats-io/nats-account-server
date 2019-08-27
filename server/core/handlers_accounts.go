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

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/julienschmidt/httprouter"
	"github.com/nats-io/jwt"
	"github.com/nats-io/nkeys"
)

// UpdateAccountJWT is the target of the post request that updates an account JWT
// Sends a nats notification
func (server *AccountServer) UpdateAccountJWT(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	server.logger.Tracef("%s: %s", r.RemoteAddr, r.URL.String())
	theJWT, err := ioutil.ReadAll(r.Body)
	defer r.Body.Close()
	if err != nil {
		server.sendErrorResponse(http.StatusBadRequest, "bad JWT in request", "", err, w)
		return
	}

	claim, err := jwt.DecodeAccountClaims(string(theJWT))

	if err != nil || claim == nil {
		server.sendErrorResponse(http.StatusBadRequest, "bad JWT in request", "", err, w)
		return
	}

	issuer := claim.Issuer

	if !nkeys.IsValidPublicOperatorKey(claim.Issuer) {
		server.sendErrorResponse(http.StatusBadRequest, "bad JWT Issuer in request", claim.Issuer, err, w)
		return
	}

	if !nkeys.IsValidPublicAccountKey(claim.Subject) {
		server.sendErrorResponse(http.StatusBadRequest, "bad JWT Subject in request", claim.Subject, err, w)
		return
	}

	ok := false

	for _, k := range server.trustedKeys {
		if k == issuer {
			ok = true
			break
		}
	}

	if !ok {
		server.sendErrorResponse(http.StatusBadRequest, "untrusted issuer in request", claim.Subject, err, w)
		return
	}

	pubKey := claim.Subject
	shortCode := ShortKey(pubKey)

	vr := &jwt.ValidationResults{}

	claim.Validate(vr)

	if vr.IsBlocking(true) {
		validationResults, err := json.Marshal(vr)

		if err != nil {
			server.sendErrorResponse(http.StatusInternalServerError, "unable to marshal JWT validation", shortCode, err, w)
			return
		}

		server.logger.Errorf("attempt to update JWT %s with blocking validation errors", shortCode)
		http.Error(w, string(validationResults), http.StatusBadRequest)
		return
	}

	if err := server.jwtStore.Save(pubKey, string(theJWT)); err != nil {
		server.sendErrorResponse(http.StatusInternalServerError, "error saving JWT", shortCode, err, w)
		return
	}

	if err := server.sendAccountNotification(claim, theJWT); err != nil {
		server.sendErrorResponse(http.StatusInternalServerError, "error sending notification of change", shortCode, err, w)
		return
	}

	server.logger.Noticef("updated JWT for account - %s - %s", shortCode, claim.ID)
	w.WriteHeader(http.StatusOK)
}

// GetAccountJWT looks up an account JWT by public key and returns it
// Supports cache control
func (server *AccountServer) GetAccountJWT(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	server.logger.Tracef("%s: %s", r.RemoteAddr, r.URL.String())
	pubKey := string(params.ByName("pubkey"))
	shortCode := ShortKey(pubKey)

	if pubKey == "" {
		server.logger.Tracef("server sent resolver check")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		w.WriteHeader(http.StatusOK)
		return
	}

	server.logger.Tracef("request for JWT for - %s", ShortKey(pubKey))

	check := strings.ToLower(r.URL.Query().Get("check")) == "true"
	notify := strings.ToLower(r.URL.Query().Get("notify")) == "true"
	decode := strings.ToLower(r.URL.Query().Get("decode")) == "true"
	text := strings.ToLower(r.URL.Query().Get("text")) == "true"

	theJWT, err := server.loadJWT(pubKey, "jwt/v1/accounts")

	if err != nil {
		if server.systemAccountClaims != nil && pubKey == server.systemAccountClaims.Subject && server.systemAccountJWT != "" {
			theJWT = server.systemAccountJWT
			server.logger.Tracef("returning system JWT from configuration")
		} else {
			server.sendErrorResponse(http.StatusInternalServerError, "error loading JWT", shortCode, err, w)
			return
		}
	}

	if text {
		server.writeJWTAsText(w, pubKey, theJWT)
		return
	}

	if decode {
		server.writeDecodedJWT(w, pubKey, theJWT)
		return
	}

	decoded, err := jwt.DecodeAccountClaims(theJWT)

	if err != nil {
		server.sendErrorResponse(http.StatusInternalServerError, "error loading JWT", shortCode, err, w)
		return
	}

	if check {
		now := time.Now().UTC().Unix()
		if decoded.Expires < now && decoded.Expires > 0 {
			w.WriteHeader(http.StatusNotFound)
			return
		}
	}

	// Check for if not modified, and also set etag and cache control
	e := `"` + decoded.ID + `"`

	if match := r.Header.Get("If-None-Match"); match != "" {
		if strings.Contains(match, e) {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}

	// send notification if requested, even though this is a GET request
	if notify {
		server.logger.Tracef("trying to send notification for - %s", shortCode)
		if err := server.sendAccountNotification(decoded, []byte(theJWT)); err != nil {
			server.sendErrorResponse(http.StatusInternalServerError, "error sending notification of change", shortCode, err, w)
			return
		}
	}

	w.Header().Set("Etag", e)

	cacheControl := server.cacheControlForExpiration(pubKey, decoded.Expires)

	if cacheControl != "" {
		w.Header().Set("Cache-Control", cacheControl)
	}

	w.Header().Add(ContentType, ApplicationJWT)
	w.WriteHeader(http.StatusOK)
	_, err = w.Write([]byte(theJWT))

	if err != nil {
		server.logger.Errorf("error writing JWT for %s - %s", shortCode, err.Error())
	} else {
		server.logger.Tracef("returning JWT for - %s", shortCode)
	}
}
