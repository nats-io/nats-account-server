package jwt

import (
	"encoding/json"
	"fmt"
)

// Migration adds GenericFields
type v1NatsActivation struct {
	ImportSubject Subject    `json:"subject,omitempty"`
	ImportType    ExportType `json:"type,omitempty"`
	Limits
}

type v1ActivationClaims struct {
	ClaimsData
	v1ClaimsDataDeletedFields
	v1NatsActivation `json:"nats,omitempty"`
}

func loadActivation(data []byte, version int) (*ActivationClaims, error) {
	switch version {
	case 1:
		var v1a v1ActivationClaims
		if err := json.Unmarshal(data, &v1a); err != nil {
			return nil, err
		}
		return v1a.Migrate()
	case 2:
		var v2a ActivationClaims
		if err := json.Unmarshal(data, &v2a); err != nil {
			return nil, err
		}
		return &v2a, nil
	default:
		return nil, fmt.Errorf("library supports version %d or less - received %d", libVersion, version)
	}
}

func (oa v1ActivationClaims) Migrate() (*ActivationClaims, error) {
	return oa.migrateV1()
}

func (oa v1ActivationClaims) migrateV1() (*ActivationClaims, error) {
	var a ActivationClaims
	// copy the base claim
	a.ClaimsData = oa.ClaimsData
	// move the moved fields
	a.Activation.Type = oa.v1ClaimsDataDeletedFields.Type
	a.Activation.Tags = oa.v1ClaimsDataDeletedFields.Tags
	a.Activation.IssuerAccount = oa.v1ClaimsDataDeletedFields.IssuerAccount
	// copy the activation data
	a.ImportSubject = oa.ImportSubject
	a.ImportType = oa.ImportType
	a.Limits = oa.Limits
	return &a, nil
}
