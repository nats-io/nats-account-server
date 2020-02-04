package jwt

import (
	"encoding/json"
	"fmt"
)

type v1NatsOperator struct {
	Identities          []Identity `json:"identity,omitempty"`
	SigningKeys         StringList `json:"signing_keys,omitempty"`
	AccountServerURL    string     `json:"account_server_url,omitempty"`
	OperatorServiceURLs StringList `json:"operator_service_urls,omitempty"`
}

func loadOperator(data []byte, version int) (*OperatorClaims, error) {
	switch version {
	case 1:
		var v1a v1OperatorClaims
		if err := json.Unmarshal(data, &v1a); err != nil {
			return nil, err
		}
		return v1a.Migrate()
	case 2:
		var v2a OperatorClaims
		if err := json.Unmarshal(data, &v2a); err != nil {
			return nil, err
		}
		return &v2a, nil
	default:
		return nil, fmt.Errorf("library supports version %d or less - received %d", libVersion, version)
	}
}

type v1OperatorClaims struct {
	ClaimsData
	v1ClaimsDataDeletedFields
	v1NatsOperator `json:"nats,omitempty"`
}

func (oa v1OperatorClaims) Migrate() (*OperatorClaims, error) {
	return oa.migrateV1()
}

func (oa v1OperatorClaims) migrateV1() (*OperatorClaims, error) {
	var a OperatorClaims
	// copy the base claim
	a.ClaimsData = oa.ClaimsData
	// move the moved fields
	a.Operator.Type = oa.v1ClaimsDataDeletedFields.Type
	a.Operator.Tags = oa.v1ClaimsDataDeletedFields.Tags
	// copy the account data
	a.Operator.Identities = oa.v1NatsOperator.Identities
	a.Operator.SigningKeys = oa.v1NatsOperator.SigningKeys
	a.Operator.AccountServerURL = oa.v1NatsOperator.AccountServerURL
	a.Operator.OperatorServiceURLs = oa.v1NatsOperator.OperatorServiceURLs
	return &a, nil
}
