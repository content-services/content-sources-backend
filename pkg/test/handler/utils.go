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

func EncodedIdentity(t *testing.T) string {
	mockIdentity := identity.XRHID{
		Identity: identity.Identity{
			AccountNumber: MockAccountNumber,
			Internal: identity.Internal{
				OrgID: MockOrgId,
			},
			Type: "Associate",
		},
	}
	jsonIdentity, err := json.Marshal(mockIdentity)
	if err != nil {
		t.Error("Could not marshal JSON")
	}
	return base64.StdEncoding.EncodeToString(jsonIdentity)
}
