package handler

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/seeds"
	"github.com/redhatinsights/platform-go-middlewares/v2/identity"
)

var MockAccountNumber = seeds.RandomAccountId()
var MockOrgId = seeds.RandomOrgId()
var MockIdentity = identity.XRHID{
	Identity: identity.Identity{
		AccountNumber: MockAccountNumber,
		Internal: identity.Internal{
			OrgID: MockOrgId,
		},
		User: &identity.User{Username: "user"},
		Type: "Associate",
	},
}

func EncodedIdentity(t *testing.T) string {
	return EncodedCustomIdentity(t, MockIdentity)
}

func EncodedCustomIdentity(t *testing.T, xrhid identity.XRHID) string {
	jsonIdentity, err := json.Marshal(xrhid)
	if err != nil {
		t.Error("Could not marshal JSON")
	}
	return base64.StdEncoding.EncodeToString(jsonIdentity)
}
