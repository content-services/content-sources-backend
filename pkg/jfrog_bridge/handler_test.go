package jfrog_bridge

import (
	"context"
	"errors"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSBOMSerialNumber(t *testing.T) {
	got, err := sbomSerialNumber(testSBOMBody)
	require.NoError(t, err)
	assert.Equal(t, "urn:uuid:"+testSBOMUUID, got)

	_, err = sbomSerialNumber([]byte(`{"bomFormat":"CycloneDX"}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty")

	_, err = sbomSerialNumber([]byte(`{`))
	require.Error(t, err)
}

func TestVerifyJARChecksum_Match(t *testing.T) {
	checksums := map[string]string{
		"spring-core-5.3.18.rhlw-00003.jar": "abcdef1234567890",
	}
	err := verifyJARChecksum("spring-core-5.3.18.rhlw-00003.jar", "abcdef1234567890", checksums)
	assert.NoError(t, err)
}

func TestVerifyJARChecksum_Mismatch(t *testing.T) {
	checksums := map[string]string{
		"spring-core-5.3.18.rhlw-00003.jar": "expected_hash",
	}
	err := verifyJARChecksum("spring-core-5.3.18.rhlw-00003.jar", "actual_hash", checksums)
	require.Error(t, err)
	var checksumErr *ChecksumMismatchError
	assert.True(t, errors.As(err, &checksumErr))
	assert.Contains(t, err.Error(), "expected_hash")
	assert.Contains(t, err.Error(), "actual_hash")
}

func TestVerifyJARChecksum_NoChecksums(t *testing.T) {
	assert.NoError(t, verifyJARChecksum("test.jar", "abc123", nil),
		"nil checksums should skip verification")
	assert.NoError(t, verifyJARChecksum("test.jar", "abc123", map[string]string{}),
		"empty checksums should skip verification")
}

func TestVerifyJARChecksum_MissingKey(t *testing.T) {
	checksums := map[string]string{
		"other-artifact-1.0.jar": "somehash",
	}
	err := verifyJARChecksum("spring-core-5.3.18.rhlw-00003.jar", "abc123", checksums)
	require.Error(t, err)
	var checksumErr *ChecksumMismatchError
	assert.True(t, errors.As(err, &checksumErr))
	assert.Contains(t, err.Error(), "absent from checksums map")
	assert.Contains(t, err.Error(), "other-artifact-1.0.jar")
}

func TestGAVDedup(t *testing.T) {
	handler := newTestBridgeHandler(&fakePartner{name: "test"}, nil)

	rem := Remediation{
		GroupID:    "org.test",
		ArtifactID: "test",
		Version:    "1.0.rhlw-00001",
	}

	gav := gavKey(rem)

	assert.False(t, handler.processed.Contains(gav))

	handler.processed.Add(gav)

	assert.True(t, handler.processed.Contains(gav))
}

func TestFilterAndParse_CloudEventEventTypeAccepted(t *testing.T) {
	// The production bridge topic carries the ecosystem gate as the
	// CloudEvents "eventtype" extension. java-remediated must be accepted.
	handler := newTestBridgeHandler(&fakePartner{name: "test"}, nil)

	payload := []byte(`{"specversion":"1.0","type":"com.redhat.console.lightwelll.lightwell-advisory-created","eventtype":"java-remediated","data":[{"metadata":{},"payload":{"package_name":"org.test:test","releases":[{"release_names":[{"name":"1.0.rhlw-00001"}],"related_cve":[{"cve":"CVE-2024-00001"}]}]}}]}`)

	remediations, shouldCommit, err := handler.filterAndParse(payload)
	require.NoError(t, err)
	assert.False(t, shouldCommit)
	require.Len(t, remediations, 1)
	assert.Equal(t, "org.test", remediations[0].GroupID)
	assert.Equal(t, "test", remediations[0].ArtifactID)
	assert.Equal(t, "1.0.rhlw-00001", remediations[0].Version)
	assert.Equal(t, "1.0", remediations[0].BaseVersion)
	assert.Contains(t, remediations[0].CVEsFixed, "CVE-2024-00001")
}

func TestFilterAndParse_CloudEventEventTypeRejected(t *testing.T) {
	// A CloudEvents message for a non-remediated ecosystem must be rejected
	// (and committed so the consumer moves on).
	handler := newTestBridgeHandler(&fakePartner{name: "test"}, nil)

	payload := []byte(`{"specversion":"1.0","type":"com.redhat.console.lightwelll.lightwell-advisory-created","eventtype":"java-predisclosure","data":[{"metadata":{},"payload":{"package_name":"org.test:test","releases":[{"release_names":[{"name":"1.0.rhlw-00001"}],"related_cve":[{"cve":"CVE-2024-00001"}]}]}}]}`)

	_, shouldCommit, err := handler.filterAndParse(payload)
	assert.Error(t, err)
	assert.True(t, shouldCommit)
	assert.Contains(t, err.Error(), "java-remediated")
}

func TestConsumeClaim_SuccessCommitsAndSkipsProcessedGAV(t *testing.T) {
	reg := prometheus.NewRegistry()
	fp := &fakePartner{name: "test"}
	handler := newTestBridgeHandler(fp, reg)

	session := &mockConsumerGroupSession{ctx: context.Background()}
	require.NoError(t, handler.ConsumeClaim(session, claimWith(springPayload, springPayload)))

	assert.Len(t, session.marked, 2)
	assert.Len(t, fp.calls, 1, "an already-processed GAV must not be delivered again")
	assert.Equal(t, float64(2), counterValue(t, reg, "content_sources_bridge_messages_received_total", "test"))
	assert.Equal(t, float64(1), counterValue(t, reg, "content_sources_bridge_messages_processed_total", "test"))
}

func TestConsumeClaim_RetryableFailureDoesNotCommit(t *testing.T) {
	reg := prometheus.NewRegistry()
	handler := newTestBridgeHandler(&fakePartner{name: "test", err: errors.New("upload failed")}, reg)

	session := &mockConsumerGroupSession{ctx: context.Background()}
	require.NoError(t, handler.ConsumeClaim(session, claimWith(springPayload)))

	assert.Empty(t, session.marked, "offset must not be committed for a retryable failure")
	assert.Equal(t, float64(1), counterValue(t, reg, "content_sources_bridge_messages_failed_total", "test"))
}

func TestConsumeClaim_NotificationEmbargoCommitsWithoutDelivery(t *testing.T) {
	reg := prometheus.NewRegistry()
	fp := &fakePartner{name: "test"}
	handler := newTestBridgeHandler(fp, reg)

	payload := `{"specversion":"1.0","type":"com.redhat.console.lightwelll.lightwell-advisory-created","eventtype":"java-remediated","data":[{"metadata":{},"payload":{"package_name":"org.test:test","releases":[{"release_names":[{"name":"1.0.rhlw-00001"}],"related_cve":[{"cve":"LTWL-0001","severity":"important"}]}]}}]}`
	session := &mockConsumerGroupSession{ctx: context.Background()}
	require.NoError(t, handler.ConsumeClaim(session, claimWith(payload)))

	assert.Len(t, session.marked, 1)
	assert.Empty(t, fp.calls, "embargoed notifications must not reach the partner")
	assert.Equal(t, float64(1), counterValue(t, reg, "content_sources_bridge_embargo_rejections_total", "test"))
}

func TestConsumeClaim_NonRetryableErrorsCommit(t *testing.T) {
	tests := map[string]struct {
		err    error
		metric string
	}{
		"content embargo":   {&EmbargoError{msg: "embargoed"}, "content_sources_bridge_embargo_rejections_total"},
		"checksum mismatch": {&ChecksumMismatchError{msg: "mismatch"}, "content_sources_bridge_checksum_rejections_total"},
		"object conflict":   {&ObjectConflictError{Key: "vex/x.bom.json"}, "content_sources_bridge_conflict_rejections_total"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			reg := prometheus.NewRegistry()
			handler := newTestBridgeHandler(&fakePartner{name: "test", err: tc.err}, reg)

			session := &mockConsumerGroupSession{ctx: context.Background()}
			require.NoError(t, handler.ConsumeClaim(session, claimWith(springPayload)))

			assert.Len(t, session.marked, 1, "offset should be committed for a non-retryable error")
			assert.Equal(t, float64(1), counterValue(t, reg, tc.metric, "test"))
			assert.Equal(t, float64(1), counterValue(t, reg, "content_sources_bridge_messages_failed_total", "test"))
		})
	}
}

func TestPartnerFailure_DoesNotCommitOtherGroup(t *testing.T) {
	// Each partner consumes in its own consumer group (its own session). A
	// Sonatype failure must leave the Sonatype offset uncommitted without
	// affecting the JFrog offset, and vice versa.
	reg := prometheus.NewRegistry()
	metrics := newBridgeMetrics(reg)
	jfrog := NewBridgeHandler(&fakePartner{name: jfrogPartnerName}, metrics)
	sonatype := NewBridgeHandler(&fakePartner{name: sonatypePartnerName, err: errors.New("S3 unavailable")}, metrics)

	jfrogSession := &mockConsumerGroupSession{ctx: context.Background()}
	sonatypeSession := &mockConsumerGroupSession{ctx: context.Background()}
	require.NoError(t, jfrog.ConsumeClaim(jfrogSession, claimWith(springPayload)))
	require.NoError(t, sonatype.ConsumeClaim(sonatypeSession, claimWith(springPayload)))

	assert.Len(t, jfrogSession.marked, 1, "JFrog offset is committed")
	assert.Empty(t, sonatypeSession.marked, "Sonatype offset is not committed")
	assert.Equal(t, float64(1), counterValue(t, reg, "content_sources_bridge_messages_processed_total", jfrogPartnerName))
	assert.Equal(t, float64(0), counterValue(t, reg, "content_sources_bridge_messages_failed_total", jfrogPartnerName))
	assert.Equal(t, float64(1), counterValue(t, reg, "content_sources_bridge_messages_failed_total", sonatypePartnerName))
}

func TestMetrics_PartnerLabel(t *testing.T) {
	reg := prometheus.NewRegistry()
	metrics := newBridgeMetrics(reg)
	metrics.forPartner(jfrogPartnerName)
	metrics.forPartner(sonatypePartnerName).conflictRejections.Inc()

	gathered, err := reg.Gather()
	require.NoError(t, err)
	names := map[string]int{}
	for _, mf := range gathered {
		names[mf.GetName()] = len(mf.GetMetric())
	}
	for _, name := range []string{
		"content_sources_bridge_messages_received_total",
		"content_sources_bridge_messages_processed_total",
		"content_sources_bridge_messages_failed_total",
		"content_sources_bridge_embargo_rejections_total",
		"content_sources_bridge_checksum_rejections_total",
		"content_sources_bridge_conflict_rejections_total",
		"content_sources_bridge_pipeline_duration_seconds",
	} {
		assert.Equal(t, 2, names[name], "%s should have one series per partner", name)
	}
	assert.Equal(t, float64(1), counterValue(t, reg, "content_sources_bridge_conflict_rejections_total", sonatypePartnerName))
	assert.Equal(t, float64(0), counterValue(t, reg, "content_sources_bridge_conflict_rejections_total", jfrogPartnerName))
}
