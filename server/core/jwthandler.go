package core

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/julienschmidt/httprouter"
	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nats-account-server/server/store"
	natsserver "github.com/nats-io/nats-server/v2/server"
)

// callback to sign a self signed jwt
// It is ok to return msg together with an error
type accountSignup func(pubKey string, theJWT []byte) (jwt []byte, msg string, err error)

// callback to notify if an account has changed
type accountNotification func(pubKey string, theJWT []byte) error

// callback to notify if an activation has changed
type activationNotification func(hash string, account string, theJWT []byte) error

type JwtHandler struct {
	logger natsserver.Logger

	packLimit int
	jwtStore  store.JWTStore

	operatorSubject string
	operatorJWT     string
	trustedKeys     map[string]struct{} // operator subject and signing keys
	sysAccSubject   string
	sysAccJWT       string

	sign                       accountSignup
	sendAccountNotification    accountNotification
	sendActivationNotification activationNotification
}

func NewJwtHandler(logger natsserver.Logger) JwtHandler {
	if logger == nil {
		logger = &NilLogger{}
	}
	return JwtHandler{logger: logger}
}

// Initialize JwtHandler which exposes http handler on top of a jwtStore
// To Close, stop using the jwthandler and close the passed in store.
func (h *JwtHandler) Initialize(opJWT []byte, sysAccJWT []byte, jwtStore store.JWTStore, packLimit int, accNotification accountNotification, actNotification activationNotification, sign accountSignup) error {

	if h == nil {
		return fmt.Errorf("JwtHandler is nil")
	}
	if jwtStore == nil {
		return fmt.Errorf("JwtHandler requires store")
	}
	if _, ok := jwtStore.(store.PackableJWTStore); !ok && packLimit > 0 {
		return fmt.Errorf("JwtStore does not implement PackableJWTStore but packLimit is specified")
	}
	h.jwtStore = jwtStore
	h.sendAccountNotification = accNotification
	h.sendActivationNotification = actNotification
	h.sign = sign
	h.packLimit = packLimit

	if len(sysAccJWT) > 0 {
		accClaim, err := jwt.DecodeAccountClaims(string(sysAccJWT))
		if err != nil {
			return err
		}
		h.sysAccJWT = string(sysAccJWT)
		h.sysAccSubject = accClaim.Subject
	}

	if len(opJWT) > 0 {
		operatorJWT, err := jwt.DecodeOperatorClaims(string(opJWT))
		if err != nil {
			return err
		}

		keys := make(map[string]struct{})
		keys[operatorJWT.Subject] = struct{}{}
		for _, k := range operatorJWT.SigningKeys {
			keys[k] = struct{}{}
		}

		h.operatorSubject = operatorJWT.Subject
		h.trustedKeys = keys
		h.operatorJWT = string(opJWT)
	}
	return nil
}

// BuildRouter initializes the http.Router with default router setup
func (h *JwtHandler) InitRouter(r *httprouter.Router) {
	if r == nil {
		return
	}
	r.GET("/jwt/v1/help", h.JWTHelp)

	if h.operatorJWT != "" {
		r.GET("/jwt/v1/operator", h.GetOperatorJWT)
	}

	// replicas and readonly stores cannot accept post requests
	// replicas use a writable store, thus the extra check
	if !h.jwtStore.IsReadOnly() {
		r.POST("/jwt/v1/accounts/:pubkey", h.UpdateAccountJWT)
		r.POST("/jwt/v1/activations", h.UpdateActivationJWT)
	}

	if _, ok := h.jwtStore.(store.PackableJWTStore); ok {
		r.GET("/jwt/v1/pack", h.PackJWTs)
	}

	r.GET("/jwt/v1/accounts/:pubkey", h.GetAccountJWT)
	r.GET("/jwt/v1/accounts/", h.GetAccountJWT) // Server test point
	r.GET("/jwt/v1/accounts", h.GetAccountJWT)  // Server test point

	r.GET("/jwt/v1/activations/:hash", h.GetActivationJWT)
}

// trace and respond with message
func (h *JwtHandler) sendErrorResponse(httpStatus int, msg string, account string, err error, w http.ResponseWriter) error {
	account = ShortKey(account)
	if err != nil {
		if account != "" {
			h.logger.Errorf("%s - %s - %s", account, msg, err.Error())
		} else {
			h.logger.Errorf("%s - %s", msg, err.Error())
		}
	} else {
		if account != "" {
			h.logger.Errorf("%s - %s", account, msg)
		} else {
			h.logger.Errorf("%s", msg)
		}
	}

	w.Header().Set(ContentType, TextPlain)
	w.WriteHeader(httpStatus)
	fmt.Fprintln(w, msg)
	return err
}

// unescapedIndentedMarshal handle indention for decoded JWTs
func unescapedIndentedMarshal(v interface{}, prefix, indent string) ([]byte, error) {
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

func (h *JwtHandler) writeDecodedJWT(w http.ResponseWriter, pubKey string, theJWT string) {
	parts := strings.Split(theJWT, ".")
	head := parts[0]
	claimSection := parts[1]
	sig := parts[2]
	headerString, err := base64.RawURLEncoding.DecodeString(head)
	if err != nil {
		h.sendErrorResponse(http.StatusInternalServerError, "error decoding account claim header", pubKey, err, w)
		return
	}
	header := jwt.Header{}
	if err := json.Unmarshal(headerString, &header); err != nil {
		h.sendErrorResponse(http.StatusInternalServerError, "error unmarshalling account claim header", pubKey, err, w)
		return
	}

	headerJSON, err := unescapedIndentedMarshal(header, "", "    ")
	if err != nil {
		h.sendErrorResponse(http.StatusInternalServerError, "error marshalling account claim header", pubKey, err, w)
		return
	}

	claimString, err := base64.RawURLEncoding.DecodeString(claimSection)
	if err != nil {
		h.sendErrorResponse(http.StatusInternalServerError, "error decoding account claim", pubKey, err, w)
		return
	}
	claim := map[string]interface{}{}
	err = json.Unmarshal(claimString, &claim)
	if err != nil {
		h.sendErrorResponse(http.StatusInternalServerError, "error decoding account claim", pubKey, err, w)
		return
	}

	claimJSON, err := unescapedIndentedMarshal(claim, "", "    ")
	if err != nil {
		h.sendErrorResponse(http.StatusInternalServerError, "error marshalling account claim", pubKey, err, w)
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
			token, subErr := unescapedIndentedMarshal(activateToken, "                ", "    ")

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
		h.sendErrorResponse(http.StatusInternalServerError, "error marshaling tokens", pubKey, subErr, w)
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
		h.sendErrorResponse(http.StatusInternalServerError, "error marshalling account tokens", pubKey, subErr, w)
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
		h.logger.Errorf("error writing decoded JWT as text for %s - %s", ShortKey(pubKey), err.Error())
	} else {
		h.logger.Tracef("returning decoded JWT as text for - %s", ShortKey(pubKey))
	}
}

func cacheControlForExpiration(pubKey string, expires int64) string {
	now := time.Now().UTC()
	maxAge := int64(time.Unix(expires, 0).Sub(now).Seconds())
	stale := int64(60 * 60) // One hour
	return fmt.Sprintf("max-age=%d, stale-while-revalidate=%d, stale-if-error=%d", maxAge, stale, stale)
}
