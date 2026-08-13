package dao

import (
	"context"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/config"
	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type LightwellTokenSuite struct {
	*DaoSuite
}

func TestLightwellTokenSuite(t *testing.T) {
	m := DaoSuite{}
	r := LightwellTokenSuite{DaoSuite: &m}
	suite.Run(t, &r)
}

func (s *LightwellTokenSuite) SetupTest() {
	s.DaoSuite.SetupTest()
	config.Get().Options.LightwellTokenPepper = "test-pepper"
}

func (s *LightwellTokenSuite) TestCreateListValidateRevoke() {
	dao := lightwellTokenDaoImpl{db: s.tx}
	orgID := orgIDTest
	userID := "user-lw-1"

	created, err := dao.Create(context.Background(), orgID, userID, "ci-token", config.LightwellAccessValidated, nil)
	require.NoError(s.T(), err)
	assert.NotEmpty(s.T(), created.UUID)
	assert.Equal(s.T(), config.LightwellAccessValidated, created.AccessLevel)
	assert.NotEmpty(s.T(), created.Token)
	assert.True(s.T(), len(created.Token) > len(created.TokenPrefix))
	assert.Equal(s.T(), created.Token[:len(created.TokenPrefix)], created.TokenPrefix)
	plaintext := created.Token

	listed, err := dao.ListByOrg(context.Background(), orgID)
	require.NoError(s.T(), err)
	require.Len(s.T(), listed, 1)
	assert.Empty(s.T(), listed[0].Token)
	assert.Equal(s.T(), created.UUID, listed[0].UUID)
	assert.Equal(s.T(), config.LightwellAccessValidated, listed[0].AccessLevel)

	validated, err := dao.Validate(context.Background(), plaintext)
	require.NoError(s.T(), err)
	assert.Equal(s.T(), orgID, validated.OrgID)
	assert.Equal(s.T(), userID, validated.UserID)
	assert.Equal(s.T(), created.UUID, validated.TokenUUID)
	assert.Equal(s.T(), config.LightwellAccessValidated, validated.AccessLevel)

	err = dao.Revoke(context.Background(), orgID, created.UUID)
	require.NoError(s.T(), err)

	_, err = dao.Validate(context.Background(), plaintext)
	require.Error(s.T(), err)
	daoErr, ok := err.(*ce.DaoError)
	require.True(s.T(), ok)
	assert.True(s.T(), daoErr.NotFound)
}

func (s *LightwellTokenSuite) TestValidateUnknownToken() {
	dao := lightwellTokenDaoImpl{db: s.tx}
	_, err := dao.Validate(context.Background(), "lw_not-a-real-token")
	require.Error(s.T(), err)
}

func (s *LightwellTokenSuite) TestExpiredToken() {
	dao := lightwellTokenDaoImpl{db: s.tx}
	expired := time.Now().UTC().Add(-time.Hour)
	_, err := dao.Create(context.Background(), orgIDTest, "user-exp", "expired", config.LightwellAccessValidated, &expired)
	require.Error(s.T(), err)
	daoErr, ok := err.(*ce.DaoError)
	require.True(s.T(), ok)
	assert.True(s.T(), daoErr.BadValidation)
}

func (s *LightwellTokenSuite) TestCreateRequiresName() {
	dao := lightwellTokenDaoImpl{db: s.tx}
	_, err := dao.Create(context.Background(), orgIDTest, "user", "  ", config.LightwellAccessValidated, nil)
	require.Error(s.T(), err)
}

func (s *LightwellTokenSuite) TestCreateRequiresAccessLevel() {
	dao := lightwellTokenDaoImpl{db: s.tx}
	_, err := dao.Create(context.Background(), orgIDTest, "user", "tok", "novel", nil)
	require.Error(s.T(), err)
	daoErr, ok := err.(*ce.DaoError)
	require.True(s.T(), ok)
	assert.True(s.T(), daoErr.BadValidation)
}

func (s *LightwellTokenSuite) TestCreateRemediatedAccessLevel() {
	dao := lightwellTokenDaoImpl{db: s.tx}
	created, err := dao.Create(context.Background(), orgIDTest, "user", "rem-tok", config.LightwellAccessRemediated, nil)
	require.NoError(s.T(), err)
	assert.Equal(s.T(), config.LightwellAccessRemediated, created.AccessLevel)

	validated, err := dao.Validate(context.Background(), created.Token)
	require.NoError(s.T(), err)
	assert.Equal(s.T(), config.LightwellAccessRemediated, validated.AccessLevel)
}
