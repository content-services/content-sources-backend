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

type LightwellPackagesSuite struct {
	LightwellSuite
}

func TestLightwellPackagesSuite(t *testing.T) {
	suite.Run(t, new(LightwellPackagesSuite))
}

func (s *LightwellPackagesSuite) TestPackagesRoute() {
	t := s.T()

	path := fmt.Sprintf("%s/packages", LightwellAPIPath)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := s.serveRouter(req)
	assert.Nil(t, err)
	// Route should be registered - we don't care about the exact response code
	assert.NotEqual(t, http.StatusNotFound, code, "Route should be registered")
}

func (s *LightwellPackagesSuite) TestPackageVersionsRoute() {
	t := s.T()

	path := fmt.Sprintf("%s/package_versions", LightwellAPIPath)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set(api.IdentityHeader, test_handler.EncodedIdentity(t))

	code, _, err := s.serveRouter(req)
	assert.Nil(t, err)
	assert.NotEqual(t, http.StatusNotFound, code, "Route should be registered")
}
