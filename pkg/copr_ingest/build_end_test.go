package copr_ingest

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/config"
	"github.com/content-services/content-sources-backend/pkg/tasks/client"
	"github.com/content-services/content-sources-backend/pkg/tasks/payloads"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testChroot  = "rhel-9-x86_64"
	testEventID = "test-event-001"
)

func validBuildEndData() buildEndData {
	return buildEndData{
		PublicationHref: "/api/pulp/copr/api/v3/publications/rpm/rpm/0002/",
		Chroot:          testChroot,
		Build:           1001,
		Owner:           "@nvidia",
		Copr:            "cuda",
		Status:          new(1),
	}
}

func expectedValidBuildEnd() validBuildEnd {
	return validBuildEnd{
		PublicationHref: "/api/pulp/copr/api/v3/publications/rpm/rpm/0002/",
		Build:           1001,
		Owner:           "@nvidia",
		Copr:            "cuda",
		Distro:          "rhel",
		Version:         "9",
		Arch:            "x86_64",
	}
}

func expectedPayload() payloads.CreatePartnerSnapshotPayload {
	return payloads.CreatePartnerSnapshotPayload{
		PublicationHref:   "/api/pulp/copr/api/v3/publications/rpm/rpm/0002/",
		RepositoryName:    "@nvidia/cuda for RHEL 9 x86_64",
		RepositoryVersion: "9",
		RepositoryArch:    "x86_64",
		CoprProjectID:     "@nvidia/cuda",
		CoprBuildID:       "1001",
	}
}

func newTestHandler(t *testing.T) (*BuildEndHandler, *client.MockTaskClient) {
	taskClient := client.NewMockTaskClient(t)
	h := NewBuildEndHandler(taskClient, nil)
	h.enqueueRetryBackoff = time.Millisecond
	return h, taskClient
}

func TestParseChroot_Simple(t *testing.T) {
	distro, version, arch, err := parseChroot(testChroot)
	require.NoError(t, err)
	assert.Equal(t, "rhel", distro)
	assert.Equal(t, "9", version)
	assert.Equal(t, "x86_64", arch)
}

func TestParseChroot_TooFewParts(t *testing.T) {
	_, _, _, err := parseChroot("rhel-9")
	assert.ErrorIs(t, err, errInvalidMessage)
	assert.ErrorContains(t, err, `invalid chroot "rhel-9"`)
}

func TestParseChroot_EmptyValue(t *testing.T) {
	_, _, _, err := parseChroot("")
	assert.ErrorIs(t, err, errInvalidMessage)
	assert.ErrorContains(t, err, `invalid chroot ""`)
}

func TestSupportedChroot_RHEL10Aarch64(t *testing.T) {
	assert.True(t, supportedChroot("rhel", config.El10, config.AARCH64))
}

func TestSupportedChroot_UnknownDistro(t *testing.T) {
	assert.False(t, supportedChroot("centos-stream", config.El9, config.X8664))
}

func TestSupportedChroot_WildcardVersion(t *testing.T) {
	assert.False(t, supportedChroot("rhel", config.ANY_VERSION, config.X8664))
}

func TestSupportedChroot_InvalidVersion(t *testing.T) {
	assert.False(t, supportedChroot("rhel", "11", config.X8664))
}

func TestSupportedChroot_WildcardArch(t *testing.T) {
	assert.False(t, supportedChroot("rhel", config.El9, config.ANY_ARCH))
}

func TestSupportedChroot_InvalidArch(t *testing.T) {
	assert.False(t, supportedChroot("rhel", config.El9, "i386"))
}

func TestParseValidBuildEnd_Complete(t *testing.T) {
	build, err := parseValidBuildEnd(validBuildEndData())
	require.NoError(t, err)
	assert.Equal(t, expectedValidBuildEnd(), build)
}

func TestParseValidBuildEnd_MultipleMissingFields(t *testing.T) {
	data := validBuildEndData()
	data.PublicationHref = ""
	data.Build = 0
	data.Copr = ""
	_, err := parseValidBuildEnd(data)
	assert.ErrorIs(t, err, errInvalidMessage)
	assert.ErrorContains(t, err, "invalid build end data: missing publication_href, build, copr")
}

func TestBuildEndHandler_HandleBuildEnd_MissingStatus(t *testing.T) {
	h, _ := newTestHandler(t)
	data := validBuildEndData()
	data.Status = nil
	err := h.handleBuildEnd(context.Background(), testEventID, data)
	assert.ErrorIs(t, err, errInvalidMessage)
	assert.ErrorContains(t, err, "invalid build end data: missing status")
}

func TestToSnapshotTaskPayload_MapsAllFields(t *testing.T) {
	payload, err := toSnapshotTaskPayload(expectedValidBuildEnd())
	require.NoError(t, err)
	assert.Equal(t, expectedPayload(), payload)
}

func TestToSnapshotTaskPayload_RepositoryNameOneOverLimit(t *testing.T) {
	build := expectedValidBuildEnd()
	suffix := " for RHEL 9 x86_64"
	build.Copr = strings.Repeat("a", maxRepositoryNameLength-len(build.Owner+"/")-len(suffix)+1)
	_, err := toSnapshotTaskPayload(build)
	assert.ErrorIs(t, err, errInvalidMessage)
	assert.ErrorContains(t, err, "longer than 255 characters")
}
