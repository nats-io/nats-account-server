package jwt

import (
	"encoding/json"
	"fmt"
)

type v1NatsAccount struct {
	Imports     Imports        `json:"imports,omitempty"`
	Exports     Exports        `json:"exports,omitempty"`
	Identities  []Identity     `json:"identity,omitempty"`
	Limits      OperatorLimits `json:"limits,omitempty"`
	SigningKeys StringList     `json:"signing_keys,omitempty"`
	Revocations RevocationList `json:"revocations,omitempty"`
}

func loadAccount(data []byte, version int) (*AccountClaims, error) {
	switch version {
	case 1:
		var v1a v1AccountClaims
		if err := json.Unmarshal(data, &v1a); err != nil {
			return nil, err
		}
		return v1a.Migrate()
	case 2:
		var v2a AccountClaims
		if err := json.Unmarshal(data, &v2a); err != nil {
			return nil, err
		}
		return &v2a, nil
	default:
		return nil, fmt.Errorf("library supports version %d or less - received %d", libVersion, version)
	}
}

type v1AccountClaims struct {
	ClaimsData
	v1ClaimsDataDeletedFields
	v1NatsAccount `json:"nats,omitempty"`
}

func (oa v1AccountClaims) Migrate() (*AccountClaims, error) {
	return oa.migrateV1()
}

func (oa v1AccountClaims) migrateV1() (*AccountClaims, error) {
	var a AccountClaims
	// copy the base claim
	a.ClaimsData = oa.ClaimsData
	// move the moved fields
	a.Account.Type = oa.v1ClaimsDataDeletedFields.Type
	a.Account.Tags = oa.v1ClaimsDataDeletedFields.Tags
	// copy the account data
	a.Account.Imports = oa.v1NatsAccount.Imports
	a.Account.Exports = oa.v1NatsAccount.Exports
	a.Account.Identities = oa.v1NatsAccount.Identities
	a.Account.Limits = oa.v1NatsAccount.Limits
	a.Account.SigningKeys = oa.v1NatsAccount.SigningKeys
	a.Account.Revocations = oa.v1NatsAccount.Revocations
	return &a, nil
}
