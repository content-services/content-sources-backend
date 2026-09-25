package pulp_client

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetLatestVersionHrefMockContract(t *testing.T) {
	m := NewMockPulpClient(t)
	href := "/pulp/api/v3/repositories/rpm/rpm/abc/versions/3/"
	m.On("GetLatestVersionHref", context.Background(), "/pulp/api/v3/repositories/rpm/rpm/abc/").
		Return(&href, nil)
	got, err := m.GetLatestVersionHref(context.Background(), "/pulp/api/v3/repositories/rpm/rpm/abc/")
	assert.NoError(t, err)
	assert.Equal(t, href, *got)
}
