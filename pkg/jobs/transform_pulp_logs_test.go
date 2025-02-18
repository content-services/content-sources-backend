package jobs

import (
	"compress/gzip"
	"io"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/content-services/content-sources-backend/pkg/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type TransformPulpLogsSuite struct {
	suite.Suite
}

func TestTransformPulpLogsSuite(t *testing.T) {
	r := TransformPulpLogsSuite{}
	suite.Run(t, &r)
}

var PULP_LOG_1 = `10.128.35.104 [27/Jan/2025:20:44:09 +0000] "GET /api/pulp-content/mydomain/gaudi-rhel-9.4/repodata/-primary.xml.gz HTTP/1.0" 302 791 "-" "libdnf (Red Hat Enterprise Linux 9.4; generic; Linux.x86_64)" "MISS" "21547" "789"`
var FULL_MESSAGE = `{
    "@timestamp": "2025-01-27T20:44:09.468383983Z",
    "hostname": "ip-10-110-168-88.ec2.internal",
    "kubernetes": {
        "annotations": {
            "configHash": "94aa153d7615e87cf1c6f0c0b42de188656cd1c287018760d9333f6640302f13",
            "k8s.ovn.org/pod-networks": "{\"default\":{\"ip_addresses\":[\"10.130.22.232/23\"],\"mac_address\":\"0a:58:0a:82:16:e8\",\"gateway_ips\":[\"10.130.22.1\"],\"routes\":[{\"dest\":\"10.128.0.0/14\",\"nextHop\":\"10.130.22.1\"},{\"dest\":\"172.30.0.0/16\",\"nextHop\":\"10.130.22.1\"},{\"dest\":\"169.254.169.5/32\",\"nextHop\":\"10.130.22.1\"},{\"dest\":\"100.64.0.0/16\",\"nextHop\":\"10.130.22.1\"}],\"ip_address\":\"10.130.22.232/23\",\"gateway_ip\":\"10.130.22.1\"}}",
            "k8s.v1.cni.cncf.io/network-status": "[{\n    \"name\": \"ovn-kubernetes\",\n    \"interface\": \"eth0\",\n    \"ips\": [\n        \"10.130.22.232\"\n    ],\n    \"mac\": \"0a:58:0a:82:16:e8\",\n    \"default\": true,\n    \"dns\": {}\n}]",
            "openshift.io/scc": "restricted-v2",
            "seccomp.security.alpha.kubernetes.io/pod": "runtime/default"
        },
        "container_id": "cri-o://08546922adc81f7baa8b3c230d4e14de03ed34e1c35edf71008e4801efcb93cb",
        "container_image": "quay.io/cloudservices/pulp-ubi:05fd4bd",
        "container_image_id": "quay.io/cloudservices/pulp-ubi@sha256:1af7e77010b6f8f3331aea3753605fac199e2a53284e895161278a5323378ff1",
        "container_name": "pulp-content",
        "labels": {
            "app": "pulp",
            "pod": "pulp-content",
            "pod-template-hash": "797d78c958"
        },
        "namespace_id": "660113ad-c221-4f49-878b-ab2e0a7fca25",
        "namespace_labels": {
            "kubernetes_io_metadata_name": "pulp-prod",
            "name": "pulp-prod",
            "openshift_io_workload-monitoring": "true",
            "pod-security_kubernetes_io_audit": "baseline",
            "pod-security_kubernetes_io_audit-version": "v1.24",
            "pod-security_kubernetes_io_warn": "baseline",
            "pod-security_kubernetes_io_warn-version": "v1.24",
            "service": "pulp"
        },
        "namespace_name": "pulp-prod",
        "pod_id": "0561cd60-8c50-470f-b2e6-a8c032ac2da6",
        "pod_ip": "10.130.22.232",
        "pod_name": "pulp-content-797d78c958-ngj5w",
        "pod_owner": "ReplicaSet/pulp-content-797d78c958"
    },
    "level": "default",
    "log_source": "container",
    "log_type": "application",
    "message": "10.128.35.104 [27/Jan/2025:20:44:09 +0000] \"GET /api/pulp-content/mydomain/gaudi-rhel-9.4/repodata/ff044ba6207abde56d0539134f6c371f49f74ad78d22331303d244fa72171da9-primary.xml.gz HTTP/1.0\" 302 791 \"-\" \"libdnf (Red Hat Enterprise Linux 9.4; generic; Linux.x86_64)\" \"MISS\" \"21547\" \"9999\"",
    "openshift": {
        "cluster_id": "33f28efd-3f10-41df-ae1f-d48036f42349",
        "sequence": 1738010651787697179
    },
    "stream_name": "pulp-prod_pulp-content-797d78c958-ngj5w_pulp-content"
}
`

func (s *TransformPulpLogsSuite) TestTransformLogEvents() {
	t := TransformPulpLogsJob{
		domainMap: map[string]string{"mydomain": "1234"},
	}
	events := t.transformLogs([]types.FilteredLogEvent{{
		Message:   &FULL_MESSAGE,
		Timestamp: utils.Ptr(int64(123456)),
	}})

	assert.Len(s.T(), events, 1)
	event := events[0]
	assert.Equal(s.T(), "1234", event.OrgId)
	assert.Equal(s.T(), "/api/pulp-content/mydomain/gaudi-rhel-9.4/repodata/ff044ba6207abde56d0539134f6c371f49f74ad78d22331303d244fa72171da9-primary.xml.gz", event.Path)
	assert.Equal(s.T(), "mydomain", event.DomainName)
	assert.Equal(s.T(), "9999", event.RequestOrgId)
}

func (s *TransformPulpLogsSuite) TestParsePulpLogMessage() {
	t := TransformPulpLogsJob{
		domainMap: map[string]string{"mydomain": "1234"},
	}

	event := t.parsePulpLogMessage(PULP_LOG_1)
	assert.NotNil(s.T(), event)
	assert.NotEmpty(s.T(), event.Timestamp)
	assert.Equal(s.T(), "1234", event.OrgId)
	assert.Equal(s.T(), "789", event.RequestOrgId)
	assert.Equal(s.T(), "/api/pulp-content/mydomain/gaudi-rhel-9.4/repodata/-primary.xml.gz", event.Path)
	assert.Equal(s.T(), "mydomain", event.DomainName)
}

func (s *TransformPulpLogsSuite) TestExtractDomainName() {
	path := "/api/pulp-content/mydomain/gaudi-rhel-9.4/repodata/ff044ba6207abde56d0539134f6c371f49f74ad78d22331303d244fa72171da9-primary.xml.gz"
	domain := domainNameFromPath(path)
	require.NotNil(s.T(), domain)
	assert.Equal(s.T(), "mydomain", *domain)
}

func (s *TransformPulpLogsSuite) TestConvertToCsv() {
	event := PulpLogEvent{
		Timestamp:    0,
		Path:         "/foo",
		FileSize:     "99999",
		OrgId:        "123",
		UserAgent:    "telegraph",
		DomainName:   ".com",
		RequestOrgId: "456",
	}

	csv, err := convertToCsv([]PulpLogEvent{event})
	assert.NoError(s.T(), err)

	g, err := gzip.NewReader(csv)
	assert.NoError(s.T(), err)

	// _, err = g.Read(read)
	data, err := io.ReadAll(g)
	assert.NoError(s.T(), err)

	assert.Equal(s.T(), "0,456,123,.com,/foo,telegraph,99999\n", string(data))
}
