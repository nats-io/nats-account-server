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
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/julienschmidt/httprouter"
	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nkeys"
)

// UpdateActivationJWT is the handler for POST requests that update an activation JWT
func (server *AccountServer) UpdateActivationJWT(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	theJWT, err := ioutil.ReadAll(r.Body)
	defer r.Body.Close()
	if err != nil {
		server.sendErrorResponse(http.StatusBadRequest, "bad activation JWT in request", "", err, w)
		return
	}

	claim, err := jwt.DecodeActivationClaims(string(theJWT))

	if err != nil || claim == nil {
		server.sendErrorResponse(http.StatusBadRequest, "bad activation JWT in request", "", err, w)
		return
	}

	if !nkeys.IsValidPublicOperatorKey(claim.Issuer) && !nkeys.IsValidPublicAccountKey(claim.Issuer) {
		server.sendErrorResponse(http.StatusBadRequest, "bad activation JWT Issuer in request", claim.Issuer, err, w)
		return
	}

	if !nkeys.IsValidPublicAccountKey(claim.Subject) {
		server.sendErrorResponse(http.StatusBadRequest, "bad activation JWT Subject in request", claim.Subject, err, w)
		return
	}

	hash, err := claim.HashID()

	if err != nil {
		server.sendErrorResponse(http.StatusBadRequest, "bad activation hash in request", claim.Issuer, err, w)
		return
	}

	if err := server.jwtStore.Save(hash, string(theJWT)); err != nil {
		server.sendErrorResponse(http.StatusInternalServerError, "error saving activation JWT", claim.Issuer, err, w)
		return
	}

	if err := server.sendActivationNotification(hash, claim.Issuer, theJWT); err != nil {
		server.sendErrorResponse(http.StatusInternalServerError, "error saving activation JWT", claim.Issuer, err, w)
		return
	}

	// hash insures that exports has len > 0
	server.logger.Noticef("updated activation JWT - %s-%s - %q", ShortKey(claim.Issuer), ShortKey(claim.Subject), claim.ImportSubject)
	w.WriteHeader(http.StatusOK)
}

// GetActivationJWT looks for an activation token by hash
func (server *AccountServer) GetActivationJWT(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	hash := string(params.ByName("hash"))
	shortCode := ShortKey(hash)

	decode := strings.ToLower(r.URL.Query().Get("decode")) == "true"
	text := strings.ToLower(r.URL.Query().Get("text")) == "true"
	notify := strings.ToLower(r.URL.Query().Get("notify")) == "true"

	theJWT, err := server.loadJWT(hash, "jwt/v1/activations")

	if err != nil {
		server.logger.Errorf("unable to find requested activation JWT for %s - %s", hash, err.Error())
		http.Error(w, "No Matching JWT", http.StatusNotFound)
		return
	}

	if text {
		server.writeJWTAsText(w, hash, theJWT)
		return
	}

	if decode {
		server.writeDecodedJWT(w, hash, theJWT)
		return
	}

	decoded, err := jwt.DecodeActivationClaims(theJWT)

	if err != nil {
		server.sendErrorResponse(http.StatusInternalServerError, "error loading JWT", shortCode, err, w)
		return
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
		if err := server.sendActivationNotification(hash, decoded.Issuer, []byte(theJWT)); err != nil {
			server.sendErrorResponse(http.StatusInternalServerError, "error sending notification of change", shortCode, err, w)
			return
		}
	}

	w.Header().Set("Etag", e)

	cacheControl := server.cacheControlForExpiration(hash, decoded.Expires)

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
