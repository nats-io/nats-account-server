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
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/julienschmidt/httprouter"
	"github.com/nats-io/jwt/v2" // only used to decode and validate, not for storage
	"github.com/nats-io/nkeys"
)

func (h *JwtHandler) loadAccountJWT(publicKey string) (bool, *jwt.AccountClaims) {
	theJwt, err := h.jwtStore.LoadAcc(publicKey)

	if err != nil {
		return false, nil
	}

	ac, err := jwt.DecodeAccountClaims(theJwt)
	if err != nil {
		return false, nil
	}

	return true, ac
}

// UpdateAccountJWT is the target of the post request that updates an account JWT
// Sends a nats notification
func (h *JwtHandler) UpdateAccountJWT(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	h.logger.Tracef("%s: %s", r.RemoteAddr, r.URL.String())
	theJWT, err := io.ReadAll(r.Body)
	defer r.Body.Close()
	if err != nil {
		h.sendErrorResponse(http.StatusBadRequest, "bad JWT in request", "", err, w)
		return
	}

	claim, err := jwt.DecodeAccountClaims(string(theJWT))
	if err != nil || claim == nil {
		h.sendErrorResponse(http.StatusBadRequest, "bad JWT in request", "", err, w)
		return
	}
	shortCode := ShortKey(claim.Subject)

	if paramPubKey := params.ByName("pubkey"); paramPubKey != "" && claim.Subject != paramPubKey {
		h.sendErrorResponse(http.StatusBadRequest, "pub keys don't match", shortCode, err, w)
		return
	}

	if !nkeys.IsValidPublicAccountKey(claim.Issuer) && !nkeys.IsValidPublicOperatorKey(claim.Issuer) {
		h.sendErrorResponse(http.StatusBadRequest, "bad JWT Issuer in request", shortCode, err, w)
		return
	}

	if !nkeys.IsValidPublicAccountKey(claim.Subject) {
		h.sendErrorResponse(http.StatusBadRequest, "bad JWT Subject in request", shortCode, err, w)
		return
	}

	msg := ""
	// First check that operator didn't sign the claims
	// if operator signed, we don't have to check the account signer
	_, didSign := h.trustedKeys[claim.Issuer]
	if h.sign != nil && !didSign {
		found, existingClaim := h.loadAccountJWT(claim.Subject)
		if !found && claim.Issuer != claim.Subject {
			h.sendErrorResponse(http.StatusBadRequest, "bad JWT Issuer/Subject pair in request", shortCode, err, w)
			return
		}

		// an issuer must be in the known jwt and on the new one
		if found && (!existingClaim.DidSign(claim) || !claim.DidSign(claim)) {
			h.sendErrorResponse(http.StatusBadRequest, "bad JWT issuer is not trusted", shortCode, err, w)
			return
		}

		// sign self signed account jwt
		if theJWT, msg, err = h.sign(claim.Subject, theJWT); err != nil {
			if msg != "" {
				h.logger.Errorf("%s - %s - %s", shortCode, "error when signing account", err.Error())
				http.Error(w, msg, http.StatusInternalServerError)
			} else {
				h.sendErrorResponse(http.StatusInternalServerError, msg, shortCode, err, w)
			}
			return
		}

		if theJWT == nil {
			h.logger.Noticef("%s Initiated JWT signing process for %s", shortCode, claim.ID)
			w.WriteHeader(http.StatusAccepted)
			if msg != "" {
				w.Header().Set(ContentType, TextPlain)
				w.Write([]byte(msg))
			}
			return
		}
		if claim, err = jwt.DecodeAccountClaims(string(theJWT)); err != nil || claim == nil {
			h.sendErrorResponse(http.StatusBadRequest, "bad JWT returned when signing account jwt", shortCode, err, w)
			return
		}
		shortCode = ShortKey(claim.Subject)
	}

	if !nkeys.IsValidPublicOperatorKey(claim.Issuer) {
		if claim.Issuer == claim.Subject {
			h.sendErrorResponse(http.StatusBadRequest, "Signing service not enabled", claim.Issuer, err, w)
		} else {
			h.sendErrorResponse(http.StatusBadRequest, "Bad JWT Issuer in request", claim.Issuer, err, w)
		}
		return
	}

	if _, didSign := h.trustedKeys[claim.Issuer]; !didSign {
		h.sendErrorResponse(http.StatusBadRequest, "untrusted issuer in request", claim.Issuer, err, w)
		return
	}

	vr := &jwt.ValidationResults{}

	claim.Validate(vr)

	if vr.IsBlocking(true) {
		var lines []string
		lines = append(lines, "The server was unable to update your account JWT. One more more validation issues occurred.")
		for _, vi := range vr.Issues {
			lines = append(lines, fmt.Sprintf("\t - %s\n", vi.Description))
		}
		msg := strings.Join(lines, "\n")
		h.logger.Errorf("attempt to update JWT %s with blocking validation errors", shortCode)
		http.Error(w, msg, http.StatusBadRequest)
		return
	}

	if err := h.jwtStore.SaveAcc(claim.Subject, string(theJWT)); err != nil {
		h.sendErrorResponse(http.StatusInternalServerError, "error saving JWT", shortCode, err, w)
		return
	}

	if h.sendAccountNotification != nil {
		if err := h.sendAccountNotification(claim.Subject, theJWT); err != nil {
			h.sendErrorResponse(http.StatusInternalServerError, "error sending notification of change", shortCode, err, w)
			return
		}
	}

	h.logger.Noticef("updated JWT for account - %s - %s", shortCode, claim.ID)
	w.WriteHeader(http.StatusOK)
	if msg != "" {
		w.Header().Set(ContentType, TextPlain)
		w.Write([]byte(msg))
	}
}

// GetAccountJWT looks up an account JWT by public key and returns it
// Supports cache control
func (h *JwtHandler) GetAccountJWT(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	h.logger.Tracef("%s: %s", r.RemoteAddr, r.URL.String())
	pubKey := params.ByName("pubkey")
	shortCode := ShortKey(pubKey)

	if pubKey == "" {
		h.logger.Tracef("server sent resolver check")
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		w.WriteHeader(http.StatusOK)
		return
	}

	h.logger.Tracef("request for JWT for - %s", ShortKey(pubKey))

	if jti := r.URL.Query().Get("jti"); jti != "" {
		h.sendErrorResponse(http.StatusNotImplemented, "lookup by jti is no longer supported", shortCode, nil, w)
		return
	}

	check := strings.ToLower(r.URL.Query().Get("check")) == "true"
	notify := strings.ToLower(r.URL.Query().Get("notify")) == "true" //TODO not done in ngs
	decode := strings.ToLower(r.URL.Query().Get("decode")) == "true"
	text := strings.ToLower(r.URL.Query().Get("text")) == "true"

	theJWT, err := h.jwtStore.LoadAcc(pubKey)

	if err != nil {
		if pubKey == h.sysAccSubject && h.sysAccJWT != "" {
			theJWT = h.sysAccJWT
			h.logger.Tracef("returning system JWT from configuration")
		} else {
			h.sendErrorResponse(http.StatusNotFound, "no matching account JWT", shortCode, err, w)
			return
		}
	}

	if text {
		h.writeJWTAsText(w, pubKey, theJWT)
		return
	}

	if decode {
		h.writeDecodedJWT(w, pubKey, theJWT)
		return
	}

	decoded, err := jwt.DecodeAccountClaims(theJWT)

	if err != nil {
		h.sendErrorResponse(http.StatusInternalServerError, "error loading JWT", shortCode, err, w)
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
		h.logger.Tracef("trying to send notification for - %s", shortCode)
		if err := h.sendAccountNotification(decoded.Subject, []byte(theJWT)); err != nil {
			h.sendErrorResponse(http.StatusInternalServerError, "error sending notification of change", shortCode, err, w)
			return
		}
	}

	w.Header().Set("Etag", e)

	cacheControl := cacheControlForExpiration(pubKey, decoded.Expires)

	if cacheControl != "" {
		w.Header().Set("Cache-Control", cacheControl)
	}

	w.Header().Add(ContentType, ApplicationJWT)
	w.WriteHeader(http.StatusOK)
	_, err = w.Write([]byte(theJWT))

	if err != nil {
		h.logger.Errorf("error writing JWT for %s - %s", shortCode, err.Error())
	} else {
		h.logger.Tracef("returning JWT for - %s", shortCode)
	}
}
