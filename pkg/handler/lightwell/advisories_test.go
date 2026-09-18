package lightwell

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/api"
	test_handler "github.com/content-services/content-sources-backend/pkg/test/handler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type LightwellAdvisorySuite struct {
	LightwellSuite
}

func TestLightwellAdvisorySuite(t *testing.T) {
	suite.Run(t, new(LightwellAdvisorySuite))
}

func (s *LightwellAdvisorySuite) TestAdvisoriesRoute() {
	t := s.T()

	path := fmt.Sprintf("%s/advisories", LightwellAPIPath)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := s.serveRouter(req)
	assert.Nil(t, err)
	// Route should be registered - we don't care about the exact response code
	assert.NotEqual(t, http.StatusNotFound, code, "Route should be registered")
}
