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
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/julienschmidt/httprouter"
	"github.com/nats-io/jwt"
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
func (server *AccountServer) JWTHelp(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	server.logger.Tracef("%s: %s", r.RemoteAddr, r.URL.String())
	w.Header().Add(ContentType, TextPlain)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(jwtAPIHelp))
}

// HealthZ returns a status OK
func (server *AccountServer) HealthZ(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	server.logger.Tracef("%s: %s", r.RemoteAddr, r.URL.String())
	w.WriteHeader(http.StatusOK)
}

// GetOperatorJWT returns the known operator JWT
func (server *AccountServer) GetOperatorJWT(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	server.logger.Tracef("%s: %s", r.RemoteAddr, r.URL.String())

	if server.operatorJWT == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	decode := strings.ToLower(r.URL.Query().Get("decode")) == "true"
	text := strings.ToLower(r.URL.Query().Get("text")) == "true"

	if text {
		server.writeJWTAsText(w, "", server.operatorJWT)
		return
	}

	if decode {
		server.writeDecodedJWT(w, "", server.operatorJWT)
		return
	}

	w.Header().Add(ContentType, ApplicationJWT)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(server.operatorJWT))
}

func (server *AccountServer) sendErrorResponse(httpStatus int, msg string, account string, err error, w http.ResponseWriter) error {
	account = ShortKey(account)
	if err != nil {
		if account != "" {
			server.logger.Errorf("%s - %s - %s", account, msg, err.Error())
		} else {
			server.logger.Errorf("%s - %s", msg, err.Error())
		}
	} else {
		if account != "" {
			server.logger.Errorf("%s - %s", account, msg)
		} else {
			server.logger.Errorf("%s", msg)
		}
	}

	w.Header().Set(ContentType, TextPlain)
	w.WriteHeader(httpStatus)
	fmt.Fprintln(w, msg)
	return err
}

func (server *AccountServer) writeJWTAsText(w http.ResponseWriter, pubKey string, theJWT string) {
	w.Header().Add(ContentType, TextPlain)
	w.WriteHeader(http.StatusOK)
	_, err := w.Write([]byte(theJWT))

	if err != nil {
		server.logger.Errorf("error writing JWT as text for %s - %s", ShortKey(pubKey), err.Error())
	} else {
		server.logger.Tracef("returning JWT as text for - %s", ShortKey(pubKey))
	}
}

// UnescapedIndentedMarshal handle indention for decoded JWTs
func UnescapedIndentedMarshal(v interface{}, prefix, indent string) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent(prefix, indent)

	err := enc.Encode(v)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (server *AccountServer) writeDecodedJWT(w http.ResponseWriter, pubKey string, theJWT string) {

	claim, err := jwt.DecodeGeneric(theJWT)
	if err != nil {
		server.sendErrorResponse(http.StatusInternalServerError, "error decoding claim", pubKey, err, w)
		return
	}

	parts := strings.Split(theJWT, ".")
	head := parts[0]
	sig := parts[2]
	headerString, err := base64.RawURLEncoding.DecodeString(head)
	if err != nil {
		server.sendErrorResponse(http.StatusInternalServerError, "error decoding claim header", pubKey, err, w)
		return
	}
	header := jwt.Header{}
	if err := json.Unmarshal(headerString, &header); err != nil {
		server.sendErrorResponse(http.StatusInternalServerError, "error unmarshaling claim header", pubKey, err, w)
		return
	}

	headerJSON, err := UnescapedIndentedMarshal(header, "", "    ")
	if err != nil {
		server.sendErrorResponse(http.StatusInternalServerError, "error marshaling claim header", pubKey, err, w)
		return
	}

	claimJSON, err := UnescapedIndentedMarshal(claim, "", "    ")
	if err != nil {
		server.sendErrorResponse(http.StatusInternalServerError, "error marshaling claim", pubKey, err, w)
		return
	}

	var subErr error

	r := regexp.MustCompile(`"token":.*?"(.*?)",`)
	claimJSON = r.ReplaceAllFunc(claimJSON, func(m []byte) []byte {
		if subErr != nil {
			return []byte(fmt.Sprintf(`"token": <bad token - %s>,`, subErr.Error()))
		}

		tokenStr := string(m)

		tokenStr = tokenStr[0 : len(tokenStr)-2] // strip the ",
		index := strings.LastIndex(tokenStr, "\"")
		tokenStr = tokenStr[index+1:]

		activateToken, subErr := jwt.DecodeActivationClaims(tokenStr)

		if subErr == nil {
			token, subErr := UnescapedIndentedMarshal(activateToken, "                ", "    ")

			tokenStr = string(token)
			tokenStr = strings.TrimSpace(tokenStr) // get rid of leading whitespace

			if subErr == nil {
				decoded := fmt.Sprintf(`"token": %s,`, tokenStr)
				return []byte(decoded)
			}
		}

		return []byte(fmt.Sprintf(`"token": <bad token - %s>,`, subErr.Error()))
	})

	if subErr != nil {
		server.sendErrorResponse(http.StatusInternalServerError, "error marshaling tokens", pubKey, subErr, w)
		return
	}

	r = regexp.MustCompile(`"iat":.*?(\d?),`)
	claimJSON = r.ReplaceAllFunc(claimJSON, func(m []byte) []byte {
		if subErr != nil {
			return []byte(fmt.Sprintf(`"iat": <parse error - %s>,`, subErr.Error()))
		}

		var iat int
		iatStr := string(m)
		iatStr = iatStr[0 : len(iatStr)-1] // strip the ,
		index := strings.LastIndex(iatStr, " ")
		iatStr = iatStr[index+1:]
		iat, subErr = strconv.Atoi(iatStr)

		if subErr != nil {
			return []byte(fmt.Sprintf(`"iat": <parse error - %s>,`, subErr.Error()))
		}

		formatted := UnixToDate(int64(iat))
		decoded := fmt.Sprintf(`"iat": %s (%s),`, iatStr, formatted)

		return []byte(decoded)
	})

	r = regexp.MustCompile(`"exp":.*?(\d?),`)
	claimJSON = r.ReplaceAllFunc(claimJSON, func(m []byte) []byte {
		if subErr != nil {
			return []byte(fmt.Sprintf(`"exp": <parse error - %s>,`, subErr.Error()))
		}

		var iat int
		iatStr := string(m)
		iatStr = iatStr[0 : len(iatStr)-1] // strip the ,
		index := strings.LastIndex(iatStr, " ")
		iatStr = iatStr[index+1:]
		iat, subErr = strconv.Atoi(iatStr)

		if subErr != nil {
			return []byte(fmt.Sprintf(`"exp": <parse error - %s>,`, subErr.Error()))
		}

		formatted := UnixToDate(int64(iat))
		decoded := fmt.Sprintf(`"exp": %s (%s),`, iatStr, formatted)

		return []byte(decoded)
	})

	if subErr != nil {
		server.sendErrorResponse(http.StatusInternalServerError, "error marshaling tokens", pubKey, subErr, w)
		return
	}

	newLineBytes := []byte("\r\n")
	jsonBuff := []byte{}
	jsonBuff = append(jsonBuff, headerJSON...)
	jsonBuff = append(jsonBuff, newLineBytes...)
	jsonBuff = append(jsonBuff, claimJSON...)
	jsonBuff = append(jsonBuff, newLineBytes...)
	jsonBuff = append(jsonBuff, []byte(sig)...)
	// if this last new line is not set curls will show a '%' in the output.
	jsonBuff = append(jsonBuff, '\n')

	w.Header().Add(ContentType, TextPlain)
	w.WriteHeader(http.StatusOK)
	_, err = w.Write(jsonBuff)

	if err != nil {
		server.logger.Errorf("error writing decoded JWT as text for %s - %s", ShortKey(pubKey), err.Error())
	} else {
		server.logger.Tracef("returning decoded JWT as text for - %s", ShortKey(pubKey))
	}
}

func (server *AccountServer) cacheControlForExpiration(pubKey string, expires int64) string {
	now := time.Now().UTC()
	maxAge := int64(time.Unix(expires, 0).Sub(now).Seconds())
	stale := int64(60 * 60) // One hour

	if server.primary != "" && maxAge > 0 {
		staleAt, ok := server.validUntil[pubKey]

		if ok {
			stale = int64(staleAt.Sub(now).Seconds())
		} else {
			return ""
		}
	}
	return fmt.Sprintf("max-age=%d, stale-while-revalidate=%d, stale-if-error=%d", maxAge, stale, stale)
}

func (server *AccountServer) loadReplicatedJWT(pubKey string, path string) (string, error) {
	now := time.Now().UTC()
	server.cacheLock.Lock()
	staleAt, ok := server.validUntil[pubKey]
	server.cacheLock.Unlock()
	stale := true // no valid until -> stale

	if ok {
		stale = int64(staleAt.Sub(now).Seconds()) < 0
	}

	// if we aren't stale and we have the jwt, return it
	if !stale {
		theJWT, err := server.jwtStore.Load(pubKey)

		if err == nil && theJWT != "" {
			return theJWT, nil
		}
	}

	primary := server.primary

	if strings.HasSuffix(primary, "/") {
		primary = primary[:len(primary)-1]
	}

	url := fmt.Sprintf("%s/%s/%s", primary, path, pubKey)

	resp, err := server.httpClient.Get(url)

	// if we can't contact the primary, fallback to what we have on disk
	if err != nil {
		theJWT, err := server.jwtStore.Load(pubKey)
		return theJWT, err
	}

	// but if the primary wasn't happy with the request, return an error
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("primary did not return with status OK")
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	theJWT := string(body)

	err = server.jwtStore.Save(pubKey, theJWT)
	if err != nil {
		return "", err
	}

	// Default cache time is 1 hour (see cacheControl)
	server.cacheLock.Lock()
	server.validUntil[pubKey] = time.Now().Add(time.Hour)
	server.cacheLock.Unlock()

	return theJWT, nil
}

func (server *AccountServer) loadJWT(pubKey string, path string) (string, error) {
	if server.primary != "" {
		return server.loadReplicatedJWT(pubKey, path)
	}

	return server.jwtStore.Load(pubKey)
}
