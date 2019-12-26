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
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/julienschmidt/httprouter"
	"github.com/nats-io/nats-account-server/server/store"
)

// PackJWTs the JWTS and return
// takes a parameter for max
func (server *AccountServer) PackJWTs(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	server.logger.Tracef("%s: %s", r.RemoteAddr, r.URL.String())

	maxStr := strings.ToLower(r.URL.Query().Get("max"))

	if maxStr == "" {
		maxStr = "-1"
	}

	max, err := strconv.Atoi(maxStr)

	if err != nil {
		server.sendErrorResponse(http.StatusBadRequest, fmt.Sprintf("bad max parameter %q", maxStr), "", err, w)
		return
	}

	server.logger.Tracef("request for JWT Pack - max=%d", max)

	packer, ok := server.jwtStore.(store.PackableJWTStore)
	if !ok {
		server.sendErrorResponse(http.StatusBadRequest, "pack isn't supported", "", nil, w)
		return
	}

	pack, err := packer.Pack(max)
	if err != nil {
		server.sendErrorResponse(http.StatusInternalServerError, "error packing JWTs", "", err, w)
		return
	}

	w.Header().Add(ContentType, TextPlain)
	w.WriteHeader(http.StatusOK)
	_, err = w.Write([]byte(pack))

	if err != nil {
		server.logger.Errorf("error writing JWT Pack - %s", err.Error())
	} else {
		server.logger.Tracef("returning JWT Pack")
	}
}
