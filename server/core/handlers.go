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
	"net/http"
	"strings"

	"github.com/julienschmidt/httprouter"
)

// http headers
const (
	ContentType     = "Content-Type"
	ApplicationJSON = "application/json"
	TextHTML        = "text/html"
	TextPlain       = "text/plain"
	ApplicationJWT  = "application/jwt"
)

// JWTHelp handles get requests for JWT help
func (h *JwtHandler) JWTHelp(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	h.logger.Tracef("%s: %s", r.RemoteAddr, r.URL.String())
	w.Header().Add(ContentType, TextPlain)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(jwtAPIHelp))
}

// GetOperatorJWT returns the known operator JWT
func (h *JwtHandler) GetOperatorJWT(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	h.logger.Tracef("%s: %s", r.RemoteAddr, r.URL.String())

	if h.operatorJWT == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	decode := strings.ToLower(r.URL.Query().Get("decode")) == "true"
	text := strings.ToLower(r.URL.Query().Get("text")) == "true"

	if text {
		h.writeJWTAsText(w, h.operatorSubject, h.operatorJWT)
		return
	}

	if decode {
		h.writeDecodedJWT(w, h.operatorSubject, h.operatorJWT)
		return
	}

	w.Header().Add(ContentType, ApplicationJWT)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(h.operatorJWT))
}

func (h *JwtHandler) writeJWTAsText(w http.ResponseWriter, pubKey string, theJWT string) {
	w.Header().Add(ContentType, TextPlain)
	w.WriteHeader(http.StatusOK)
	_, err := w.Write([]byte(theJWT))

	if err != nil {
		h.logger.Errorf("error writing JWT as text for %s - %s", ShortKey(pubKey), err.Error())
	} else {
		h.logger.Tracef("returning JWT as text for - %s", ShortKey(pubKey))
	}
}
