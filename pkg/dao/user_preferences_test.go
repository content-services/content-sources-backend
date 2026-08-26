package dao

import (
	"context"
	"testing"

	ce "github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type UserPreferenceSuite struct {
	*DaoSuite
}

func TestUserPreferenceSuite(t *testing.T) {
	m := DaoSuite{}
	r := UserPreferenceSuite{DaoSuite: &m}
	suite.Run(t, &r)
}

func (s *UserPreferenceSuite) TestListEmpty() {
	dao := userPreferenceDaoImpl{db: s.tx}
	prefs, err := dao.List(context.Background(), orgIDTest, "user-1")
	assert.NoError(s.T(), err)
	assert.Empty(s.T(), prefs)
}

func (s *UserPreferenceSuite) TestSetAndList() {
	dao := userPreferenceDaoImpl{db: s.tx}
	orgID := orgIDTest
	userID := "user-1"

	pref, err := dao.Set(context.Background(), orgID, userID, models.UserPreferenceLightwellNotificationEnabled, "true")
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), models.UserPreferenceLightwellNotificationEnabled, pref.Label)
	assert.Equal(s.T(), "true", pref.Value)

	pref, err = dao.Set(context.Background(), orgID, userID, models.UserPreferenceLightwellNotificationMinimum, "critical")
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), models.UserPreferenceLightwellNotificationMinimum, pref.Label)
	assert.Equal(s.T(), "critical", pref.Value)

	prefs, err := dao.List(context.Background(), orgID, userID)
	assert.NoError(s.T(), err)
	assert.Len(s.T(), prefs, 2)
	assert.Equal(s.T(), models.UserPreferenceLightwellNotificationEnabled, prefs[0].Label)
	assert.Equal(s.T(), "true", prefs[0].Value)
	assert.Equal(s.T(), models.UserPreferenceLightwellNotificationMinimum, prefs[1].Label)
	assert.Equal(s.T(), "critical", prefs[1].Value)
}

func (s *UserPreferenceSuite) TestSetUpsert() {
	dao := userPreferenceDaoImpl{db: s.tx}
	orgID := orgIDTest
	userID := "user-2"

	_, err := dao.Set(context.Background(), orgID, userID, models.UserPreferenceLightwellNotificationEnabled, "true")
	assert.NoError(s.T(), err)

	pref, err := dao.Set(context.Background(), orgID, userID, models.UserPreferenceLightwellNotificationEnabled, "false")
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), "false", pref.Value)

	prefs, err := dao.List(context.Background(), orgID, userID)
	assert.NoError(s.T(), err)
	assert.Len(s.T(), prefs, 1)
	assert.Equal(s.T(), "false", prefs[0].Value)
}

func (s *UserPreferenceSuite) TestSetInvalidLabel() {
	dao := userPreferenceDaoImpl{db: s.tx}
	userID := "user-3"

	_, err := dao.Set(context.Background(), orgIDTest, userID, "not-a-valid-label", "true")
	require.Error(s.T(), err)
	daoErr, ok := err.(*ce.DaoError)
	require.True(s.T(), ok)
	assert.True(s.T(), daoErr.BadValidation)

	prefs, err := dao.List(context.Background(), orgIDTest, userID)
	assert.NoError(s.T(), err)
	assert.Empty(s.T(), prefs)
}

func (s *UserPreferenceSuite) TestSetInvalidValue() {
	dao := userPreferenceDaoImpl{db: s.tx}
	userID := "user-4"

	_, err := dao.Set(context.Background(), orgIDTest, userID, models.UserPreferenceLightwellNotificationEnabled, "yes")
	require.Error(s.T(), err)
	daoErr, ok := err.(*ce.DaoError)
	require.True(s.T(), ok)
	assert.True(s.T(), daoErr.BadValidation)

	_, err = dao.Set(context.Background(), orgIDTest, userID, models.UserPreferenceLightwellNotificationMinimum, "urgent")
	require.Error(s.T(), err)
	daoErr, ok = err.(*ce.DaoError)
	require.True(s.T(), ok)
	assert.True(s.T(), daoErr.BadValidation)

	prefs, err := dao.List(context.Background(), orgIDTest, userID)
	assert.NoError(s.T(), err)
	assert.Empty(s.T(), prefs)
}

func (s *UserPreferenceSuite) TestListScopedToUser() {
	dao := userPreferenceDaoImpl{db: s.tx}
	orgID := orgIDTest

	_, err := dao.Set(context.Background(), orgID, "user-a", models.UserPreferenceLightwellNotificationEnabled, "true")
	assert.NoError(s.T(), err)
	_, err = dao.Set(context.Background(), orgID, "user-b", models.UserPreferenceLightwellNotificationEnabled, "false")
	assert.NoError(s.T(), err)

	prefs, err := dao.List(context.Background(), orgID, "user-a")
	assert.NoError(s.T(), err)
	assert.Len(s.T(), prefs, 1)
	assert.Equal(s.T(), "true", prefs[0].Value)
}

func (s *UserPreferenceSuite) TestListDistinctOrgsByPreference() {
	dao := userPreferenceDaoImpl{db: s.tx}

	// Count existing orgs with this preference before seeding
	initialOrgs, err := dao.ListDistinctOrgsByPreference(context.Background(), models.UserPreferenceLightwellNotificationEnabled, "true")
	assert.NoError(s.T(), err)

	_, err = dao.Set(context.Background(), "org-1", "user-a", models.UserPreferenceLightwellNotificationEnabled, "true")
	assert.NoError(s.T(), err)
	_, err = dao.Set(context.Background(), "org-1", "user-b", models.UserPreferenceLightwellNotificationEnabled, "true")
	assert.NoError(s.T(), err)
	_, err = dao.Set(context.Background(), "org-2", "user-c", models.UserPreferenceLightwellNotificationEnabled, "true")
	assert.NoError(s.T(), err)
	_, err = dao.Set(context.Background(), "org-3", "user-d", models.UserPreferenceLightwellNotificationEnabled, "false")
	assert.NoError(s.T(), err)

	orgs, err := dao.ListDistinctOrgsByPreference(context.Background(), models.UserPreferenceLightwellNotificationEnabled, "true")
	assert.NoError(s.T(), err)
	assert.Len(s.T(), orgs, len(initialOrgs)+2)
	assert.Contains(s.T(), orgs, "org-1")
	assert.Contains(s.T(), orgs, "org-2")
}

func (s *UserPreferenceSuite) TestListDistinctOrgsByPreferenceEmpty() {
	dao := userPreferenceDaoImpl{db: s.tx}

	// Query for a value that no one has set to verify we get an empty result
	// (regardless of what other preferences may exist in the database)
	orgs, err := dao.ListDistinctOrgsByPreference(context.Background(), models.UserPreferenceLightwellNotificationEnabled, "invalid-value-not-in-use")
	assert.NoError(s.T(), err)
	assert.Empty(s.T(), orgs)
}
