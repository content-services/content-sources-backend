package jfrog_bridge

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateNotificationCVEs_AllValid(t *testing.T) {
	rem := Remediation{
		GroupID:    "org.glassfish.jaxb",
		ArtifactID: "jaxb-core",
		Version:    "4.0.4.rhlw003",
		CVEsFixed: []string{
			"CVE-2026-1111",
			"CVE-2026-2222",
			"CVE-2026-9999",
		},
	}
	assert.NoError(t, ValidateNotificationCVEs(rem))
}

func TestValidateNotificationCVEs_Empty(t *testing.T) {
	rem := Remediation{
		GroupID:    "org.test",
		ArtifactID: "test",
		Version:    "1.0.rhlw-00001",
		CVEsFixed:  []string{},
	}
	err := ValidateNotificationCVEs(rem)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no CVEs")
}

func TestValidateNotificationCVEs_Nil(t *testing.T) {
	rem := Remediation{
		GroupID:    "org.test",
		ArtifactID: "test",
		Version:    "1.0.rhlw-00001",
		CVEsFixed:  nil,
	}
	err := ValidateNotificationCVEs(rem)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no CVEs")
}

func TestValidateNotificationCVEs_MixedList(t *testing.T) {
	rem := Remediation{
		GroupID:    "org.test",
		ArtifactID: "test",
		Version:    "1.0.rhlw-00001",
		CVEsFixed:  []string{"CVE-2026-1111", "LTWL-0001"},
	}
	err := ValidateNotificationCVEs(rem)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "LTWL-0001")
	assert.Contains(t, err.Error(), "non-public")
}

func TestValidateNotificationCVEs_InternalOnly(t *testing.T) {
	rem := Remediation{
		GroupID:    "org.test",
		ArtifactID: "test",
		Version:    "1.0.rhlw-00001",
		CVEsFixed:  []string{"LTWL-0001", "RHSA-2023:1234"},
	}
	err := ValidateNotificationCVEs(rem)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "LTWL-0001")
	assert.Contains(t, err.Error(), "RHSA-2023:1234")
}

func TestValidateNotificationCVEs_RejectsInvalidFormats(t *testing.T) {
	tests := []struct {
		name string
		id   string
	}{
		{"RHSA advisory", "RHSA-2023:1234"},
		{"Bugzilla", "BZ-12345"},
		{"GHSA", "GHSA-abcd-efgh-ijkl"},
		{"lowercase cve", "cve-2023-20860"},
		{"underscore separator", "CVE_2023_20860"},
		{"too-short suffix", "CVE-2023-123"},
		{"internal tracker", "LTWL-0001"},
		{"empty string", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rem := Remediation{
				GroupID:    "org.test",
				ArtifactID: "test",
				Version:    "1.0.rhlw-00001",
				CVEsFixed:  []string{tt.id},
			}
			err := ValidateNotificationCVEs(rem)
			assert.Error(t, err)
		})
	}
}

func TestValidateNotificationCVEs_AcceptsValidFormats(t *testing.T) {
	tests := []struct {
		name string
		id   string
	}{
		{"standard", "CVE-2023-20860"},
		{"long suffix", "CVE-2026-123456"},
		{"old cve", "CVE-1999-0001"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rem := Remediation{
				GroupID:    "org.test",
				ArtifactID: "test",
				Version:    "1.0.rhlw-00001",
				CVEsFixed:  []string{tt.id},
			}
			assert.NoError(t, ValidateNotificationCVEs(rem))
		})
	}
}

func TestValidateOSVRecords_AllValid(t *testing.T) {
	records := []OSVRecord{
		{CVEID: "CVE-2023-20860"},
		{CVEID: "CVE-2024-38808"},
	}
	assert.NoError(t, ValidateOSVRecords(records))
}

func TestValidateOSVRecords_Empty(t *testing.T) {
	err := ValidateOSVRecords(nil)
	require.Error(t, err)
	var embargoErr *EmbargoError
	assert.True(t, errors.As(err, &embargoErr))

	err = ValidateOSVRecords([]OSVRecord{})
	require.Error(t, err)
	assert.True(t, errors.As(err, &embargoErr))
}

func TestValidateOSVRecords_NonCVE(t *testing.T) {
	records := []OSVRecord{
		{CVEID: "CVE-2023-20860"},
		{CVEID: "LTWL-0042"},
	}
	err := ValidateOSVRecords(records)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "LTWL-0042")
	assert.Contains(t, err.Error(), "non-public")
}

func TestValidateOSVRecords_InternalID(t *testing.T) {
	records := []OSVRecord{
		{CVEID: "x_RHLW-CVE-2023-20860-5.3.18"},
	}
	err := ValidateOSVRecords(records)
	require.Error(t, err)
}

func TestFullCloudEventMessage_EmbargoCheck(t *testing.T) {
	msg := []byte(`{"specversion":"1.0","id":"82cb1ea3-f4b2-41e5-80e6-b5154971d060","source":"urn:redhat:source:console:app:lightwell","type":"com.redhat.console.lightwelll.lightwell-advisory-created","datacontenttype":"application/json","time":"2026-08-26T18:47:26.61073395Z","data":[{"metadata":{},"payload":{"package_link":"https://console.redhat.com/lightwell/packages/org.glassfish.jaxb:jaxb-core","package_name":"org.glassfish.jaxb:jaxb-core","releases":[{"related_cve":[{"cve":"CVE-2026-1111","severity":"important","url":"https://console.redhat.com/api/lightwell/cves/CVE-2026-1111.json"},{"cve":"CVE-2026-2222","severity":"important","url":"https://console.redhat.com/api/lightwell/cves/CVE-2026-2222.json"},{"cve":"CVE-2026-9999","severity":"critical","url":"https://console.redhat.com/api/lightwell/cves/CVE-2026-9999.json"}],"release_names":[{"name":"4.0.4.rhlw003"},{"name":"4.0.4.rhlw004"}]}]}},{"metadata":{},"payload":{"package_link":"https://console.redhat.com/lightwell/packages/org.glassfish.jaxb:jaxb-jxc","package_name":"org.glassfish.jaxb:jaxb-jxc","releases":[{"related_cve":[{"cve":"CVE-2026-2222","severity":"important","url":"https://console.redhat.com/api/lightwell/cves/CVE-2026-2222.json"}],"release_names":[{"name":"4.0.4.rhlw003"}]}]}}]}`)

	rems, err := ParseRemediations(msg)
	require.NoError(t, err)
	require.Len(t, rems, 3)

	for _, rem := range rems {
		assert.NoError(t, ValidateNotificationCVEs(rem),
			"all CVEs in the example message should pass")
	}
}

func TestFullCloudEventMessage_EmbargoCheckWithEmbargo(t *testing.T) {
	msg := []byte(`{"specversion":"1.0","id":"test-embargo","source":"urn:redhat:source:console:app:lightwell","type":"com.redhat.console.lightwelll.lightwell-advisory-created","datacontenttype":"application/json","time":"2026-08-26T18:47:26Z","data":[{"metadata":{},"payload":{"package_name":"org.glassfish.jaxb:jaxb-core","releases":[{"related_cve":[{"cve":"CVE-2026-1111","severity":"important"},{"cve":"LTWL-0001","severity":"critical"}],"release_names":[{"name":"4.0.4.rhlw005"}]}]}}]}`)

	rems, err := ParseRemediations(msg)
	require.NoError(t, err)
	require.Len(t, rems, 1)

	err = ValidateNotificationCVEs(rems[0])
	require.Error(t, err)
	assert.Contains(t, err.Error(), "LTWL-0001")
}

func TestNotificationCVEs_ReturnsEmbargoError(t *testing.T) {
	rem := Remediation{
		GroupID:    "org.test",
		ArtifactID: "test",
		Version:    "1.0.rhlw-00001",
		CVEsFixed:  []string{"LTWL-0001"},
	}
	err := ValidateNotificationCVEs(rem)
	require.Error(t, err)

	var embargoErr *EmbargoError
	assert.True(t, errors.As(err, &embargoErr), "should be *EmbargoError")
}

func TestNotificationCVEs_NoCVEs_ReturnsEmbargoError(t *testing.T) {
	rem := Remediation{
		GroupID:    "org.test",
		ArtifactID: "test",
		Version:    "1.0.rhlw-00001",
		CVEsFixed:  nil,
	}
	err := ValidateNotificationCVEs(rem)
	require.Error(t, err)

	var embargoErr *EmbargoError
	assert.True(t, errors.As(err, &embargoErr), "empty CVEs should be *EmbargoError")
}

func TestOSVRecords_ReturnsEmbargoError(t *testing.T) {
	records := []OSVRecord{{CVEID: "LTWL-0042"}}
	err := ValidateOSVRecords(records)
	require.Error(t, err)

	var embargoErr *EmbargoError
	assert.True(t, errors.As(err, &embargoErr), "should be *EmbargoError")
}

func TestEmbargoError_UnwrapsThroughFmtErrorf(t *testing.T) {
	records := []OSVRecord{{CVEID: "LTWL-0042"}}
	inner := ValidateOSVRecords(records)
	require.Error(t, inner)

	wrapped := fmt.Errorf("content embargo check: %w", inner)

	var embargoErr *EmbargoError
	assert.True(t, errors.As(wrapped, &embargoErr),
		"errors.As should unwrap through fmt.Errorf %%w")
}
