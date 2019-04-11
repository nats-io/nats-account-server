package store

import ()

// JWTStore is the interface for all store implementations in the account server
// The store provides a handful of methods for setting and getting a JWT.
// The data doesn't really have to be a JWT, no validation is expected at this level
type JWTStore interface {
	Load(publicKey string) (string, error)
	Save(publicKey string, theJWT string) error
	IsReadOnly() bool
}
