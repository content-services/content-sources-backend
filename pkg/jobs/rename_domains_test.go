package jobs

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChangeHrefDomain(t *testing.T) {
	href := "/api/pulp/a54eb76e61/api/v3/repositories/rpm/rpm/0193275f-b292-7cab-a819-d71cf99f9b65/versions/1/"
	expected := "/api/pulp/cs-FOOZ/api/v3/repositories/rpm/rpm/0193275f-b292-7cab-a819-d71cf99f9b65/versions/1/"
	changed, err := ChangeHrefDomain(href, "cs-FOOZ")
	assert.NoError(t, err)
	assert.Equal(t, expected, changed)

	href = "/api/pulp/cs-a54eb76e61/api/v3/publications/rpm/rpm/01932761-11c3-78e4-879f-2234ca294001/"
	expected = "/api/pulp/cs-FOOZ/api/v3/publications/rpm/rpm/01932761-11c3-78e4-879f-2234ca294001/"
	changed, err = ChangeHrefDomain(href, "cs-FOOZ")
	assert.NoError(t, err)
	assert.Equal(t, expected, changed)

	href = "/api/pulp/cs-a54eb76e61/api/v3/distributions/rpm/rpm/01932763-0275-7e1f-84a8-af7ca56ae91c/"
	expected = "/api/pulp/cs-BAR/api/v3/distributions/rpm/rpm/01932763-0275-7e1f-84a8-af7ca56ae91c/"
	changed, err = ChangeHrefDomain(href, "cs-BAR")
	assert.NoError(t, err)
	assert.Equal(t, expected, changed)
}
