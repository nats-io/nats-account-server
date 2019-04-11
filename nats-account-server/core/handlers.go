package core

import (
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
	"github.com/nats-io/nkeys"
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

// UpdateAccountJWT is the target of the post request that updates an account JWT
// Sends a nats notificaiton
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

	if claim.Issuer != claim.Subject {
		server.sendErrorResponse(http.StatusBadRequest, "bad JWT Issuer/Subject pair in request", claim.Subject, err, w)
		return
	}

	if !nkeys.IsValidPublicAccountKey(claim.Issuer) {
		server.sendErrorResponse(http.StatusBadRequest, "bad JWT Issuer in request", claim.Issuer, err, w)
		return
	}

	if !nkeys.IsValidPublicAccountKey(claim.Subject) {
		server.sendErrorResponse(http.StatusBadRequest, "bad JWT Subject in request", claim.Subject, err, w)
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
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		w.WriteHeader(http.StatusOK)
		return
	}

	check := strings.ToLower(r.URL.Query().Get("check")) == "true"
	decode := strings.ToLower(r.URL.Query().Get("decode")) == "true"
	text := strings.ToLower(r.URL.Query().Get("text")) == "true"

	theJWT, err := server.jwtStore.Load(pubKey)

	if err != nil {
		server.sendErrorResponse(http.StatusInternalServerError, "error loading JWT", shortCode, err, w)
		return
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
	maxAge := int64(time.Unix(decoded.Expires, 0).Sub(time.Now().UTC()).Seconds())
	staleWhile := 60 * 60 // One hour
	staleError := 60 * 60 // One hour
	cacheControl := fmt.Sprintf("max-age=%d, stale-while-revalidate=%d, stale-if-error=%d", maxAge, staleWhile, staleError)
	w.Header().Set("Etag", e)
	w.Header().Set("Cache-Control", cacheControl)

	if match := r.Header.Get("If-None-Match"); match != "" {
		if strings.Contains(match, e) {
			w.WriteHeader(http.StatusNotModified)
			return
		}
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

func (server *AccountServer) writeDecodedJWT(w http.ResponseWriter, pubKey string, theJWT string) {

	claim, err := jwt.DecodeGeneric(theJWT)
	if err != nil {
		server.sendErrorResponse(http.StatusInternalServerError, "error decoding account claim", pubKey, err, w)
		return
	}

	parts := strings.Split(theJWT, ".")
	head := parts[0]
	sig := parts[2]
	headerString, err := base64.RawURLEncoding.DecodeString(head)
	if err != nil {
		server.sendErrorResponse(http.StatusInternalServerError, "error decoding account claim header", pubKey, err, w)
		return
	}
	header := jwt.Header{}
	if err := json.Unmarshal(headerString, &header); err != nil {
		server.sendErrorResponse(http.StatusInternalServerError, "error unmarshalling account claim header", pubKey, err, w)
		return
	}

	headerJSON, err := json.MarshalIndent(header, "", "    ")
	if err != nil {
		server.sendErrorResponse(http.StatusInternalServerError, "error marshalling account claim header", pubKey, err, w)
		return
	}

	claimJSON, err := json.MarshalIndent(claim, "", "    ")
	if err != nil {
		server.sendErrorResponse(http.StatusInternalServerError, "error marshalling account claim", pubKey, err, w)
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
			token, subErr := json.MarshalIndent(activateToken, "                ", "    ")

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
		server.sendErrorResponse(http.StatusInternalServerError, "error marshalling account tokens", pubKey, subErr, w)
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
		server.sendErrorResponse(http.StatusInternalServerError, "error marshalling account tokens", pubKey, subErr, w)
		return
	}

	newLineBytes := []byte("\r\n")
	jsonBuff := []byte{}
	jsonBuff = append(jsonBuff, headerJSON...)
	jsonBuff = append(jsonBuff, newLineBytes...)
	jsonBuff = append(jsonBuff, claimJSON...)
	jsonBuff = append(jsonBuff, newLineBytes...)
	jsonBuff = append(jsonBuff, []byte(sig)...)

	w.Header().Add(ContentType, TextPlain)
	w.WriteHeader(http.StatusOK)
	_, err = w.Write(jsonBuff)

	if err != nil {
		server.logger.Errorf("error writing decoded JWT as text for %s - %s", ShortKey(pubKey), err.Error())
	} else {
		server.logger.Tracef("returning decoded JWT as text for - %s", ShortKey(pubKey))
	}
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
