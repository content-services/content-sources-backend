package handler

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/seeds"
	"github.com/redhatinsights/platform-go-middlewares/identity"
)

var MockAccountNumber = seeds.RandomAccountId()
var MockOrgId = seeds.RandomOrgId()
var MockIdentity = identity.XRHID{
	Identity: identity.Identity{
		AccountNumber: MockAccountNumber,
		Internal: identity.Internal{
			OrgID: MockOrgId,
		},
		Type: "Associate",
	},
}

func EncodedIdentity(t *testing.T) string {
	jsonIdentity, err := json.Marshal(MockIdentity)
	if err != nil {
		t.Error("Could not marshal JSON")
	}
	return base64.StdEncoding.EncodeToString(jsonIdentity)
}
