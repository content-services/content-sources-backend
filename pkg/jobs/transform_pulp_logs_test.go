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

var PULP_LOG_1 = `10.128.35.104 [27/Jan/2025:20:44:09 +0000] "GET /api/pulp-content/mydomain/gaudi-rhel-9.4/repodata/-primary.xml.gz HTTP/1.0" 302 791 "-" "libdnf (Red Hat Enterprise Linux 9.4; generic; Linux.x86_64)" cache:"MISS" artifact_size:"21547" rh_org_id:"789"`
var FULL_MESSAGE = `{
    "@timestamp": "2025-05-21T19:23:44.403067792Z",
    "hostname": "ip-10-110-154-130.ec2.internal",
    "kubernetes": {
        "annotations": {
            "configHash": "89916124c57361a137853509cb30ff83affba06bf5a6c9ed2187c69f8bd363a9",
            "openshift.io/scc": "restricted-v2",
            "seccomp.security.alpha.kubernetes.io/pod": "runtime/default"
        }
    },
    "level": "default",
    "log_source": "container",
    "log_type": "application",
    "message": "192.168.1.1 [21/May/2025:19:23:44 +0000] \"GET /api/pulp-content/mydomain/templates/8c18e4a4-077f-468b-ba26-73a507e86f6e/content/dist/rhel9/9/x86_64/appstream/os/repodata/repomd.xml HTTP/1.1\" 302 728 \"-\" \"libdnf (Red Hat Enterprise Linux 9.5; generic; Linux.x86_64)\" cache:\"HIT\" artifact_size:\"3141\" rh_org_id:\"5483888\"",
    "openshift": {
        "cluster_id": "33f28efd-3f10-41df-ae1f-d48036f42349",
        "sequence": 1747855430177207299
    },
    "stream_name": "pulp-prod_pulp-content-857ffc5b9f-grljs_pulp-content"
}
`

func (s *TransformPulpLogsSuite) TestTransformLogEvents() {
	t := TransformPulpLogsJob{
		DomainMap: map[string]string{"mydomain": "1234"},
	}
	events := t.transformLogs([]types.FilteredLogEvent{{
		Message:   &FULL_MESSAGE,
		Timestamp: utils.Ptr(int64(123456)),
	}})

	assert.Len(s.T(), events, 1)
	event := events[0]
	assert.Equal(s.T(), "1234", event.OrgId)
	assert.Equal(s.T(), "/api/pulp-content/mydomain/templates/8c18e4a4-077f-468b-ba26-73a507e86f6e/content/dist/rhel9/9/x86_64/appstream/os/repodata/repomd.xml", event.Path)
	assert.Equal(s.T(), "mydomain", event.DomainName)
	assert.Equal(s.T(), "5483888", event.RequestOrgId)
}

func (s *TransformPulpLogsSuite) TestParsePulpLogMessage() {
	t := TransformPulpLogsJob{
		DomainMap: map[string]string{"mydomain": "1234"},
	}

	event := t.ParsePulpLogMessage(PULP_LOG_1)
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
