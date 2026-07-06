package dao

import (
	"context"
	"testing"

	"github.com/content-services/content-sources-backend/pkg/models"
	"github.com/content-services/content-sources-backend/pkg/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type MavenPackagesSuite struct {
	*DaoSuite
}

func TestMavenPackagesSuite(t *testing.T) {
	m := DaoSuite{}
	suite.Run(t, &MavenPackagesSuite{DaoSuite: &m})
}

func (s *MavenPackagesSuite) TestCreate() {
	dao := GetMavenPackagesDao(s.tx)

	mavenPackage := &models.MavenPackage{
		GroupID: "org.apache.avalon.framework",
		Name:    "avalon-util-exception",
		Summary: utils.Ptr("Utility classes for Avalon."),
		License: utils.Ptr("Apache-2.0"),
	}

	err := dao.Create(context.Background(), mavenPackage)
	require.NoError(s.T(), err)
	assert.NotEmpty(s.T(), mavenPackage.UUID)
}

func (s *MavenPackagesSuite) TestFetch() {
	dao := GetMavenPackagesDao(s.tx)

	mavenPackage := &models.MavenPackage{
		GroupID: "org.apache.avalon.framework",
		Name:    "avalon-util-exception",
		Summary: utils.Ptr("Utility classes for Avalon."),
		License: utils.Ptr("Apache-2.0"),
	}
	require.NoError(s.T(), dao.Create(context.Background(), mavenPackage))

	fetched, err := dao.Fetch(context.Background(), "org.apache.avalon.framework", "avalon-util-exception")
	require.NoError(s.T(), err)
	require.NotNil(s.T(), fetched)
	assert.Equal(s.T(), mavenPackage.UUID, fetched.UUID)
	assert.Equal(s.T(), "Utility classes for Avalon.", *fetched.Summary)
	assert.Equal(s.T(), "Apache-2.0", *fetched.License)
}

func (s *MavenPackagesSuite) TestCreateSkipsExisting() {
	dao := GetMavenPackagesDao(s.tx)

	require.NoError(s.T(), dao.Create(context.Background(), &models.MavenPackage{
		GroupID: "org.apache.avalon.framework",
		Name:    "avalon-util-exception",
		Summary: utils.Ptr("Original summary."),
		License: utils.Ptr("Apache-2.0"),
	}))

	err := dao.Create(context.Background(), &models.MavenPackage{
		GroupID: "org.apache.avalon.framework",
		Name:    "avalon-util-exception",
		Summary: utils.Ptr("Should not replace original."),
		License: utils.Ptr("MIT"),
	})
	require.NoError(s.T(), err)

	fetched, err := dao.Fetch(context.Background(), "org.apache.avalon.framework", "avalon-util-exception")
	require.NoError(s.T(), err)
	require.NotNil(s.T(), fetched)
	assert.Equal(s.T(), "Original summary.", *fetched.Summary)
	assert.Equal(s.T(), "Apache-2.0", *fetched.License)
}

func (s *MavenPackagesSuite) TestSameNameDifferentGroups() {
	dao := GetMavenPackagesDao(s.tx)

	require.NoError(s.T(), dao.Create(context.Background(), &models.MavenPackage{
		GroupID: "com.example",
		Name:    "utils",
		Summary: utils.Ptr("Example utils."),
	}))
	require.NoError(s.T(), dao.Create(context.Background(), &models.MavenPackage{
		GroupID: "org.other",
		Name:    "utils",
		Summary: utils.Ptr("Other utils."),
	}))

	first, err := dao.Fetch(context.Background(), "com.example", "utils")
	require.NoError(s.T(), err)
	require.NotNil(s.T(), first)
	assert.Equal(s.T(), "Example utils.", *first.Summary)

	second, err := dao.Fetch(context.Background(), "org.other", "utils")
	require.NoError(s.T(), err)
	require.NotNil(s.T(), second)
	assert.Equal(s.T(), "Other utils.", *second.Summary)
}

func (s *MavenPackagesSuite) TestFetchNotFound() {
	dao := GetMavenPackagesDao(s.tx)

	fetched, err := dao.Fetch(context.Background(), "com.example", "missing-package")
	require.NoError(s.T(), err)
	assert.Nil(s.T(), fetched)
}
