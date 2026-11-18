// Some tests require local testdata fixtures (POM, OSV JSONs) under
// testdata/ that are not committed to git. They skip automatically
// when the files are absent.

package jfrog_bridge

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func simulateRequest(t *testing.T, bh *BridgeHandler, payload string) (*httptest.ResponseRecorder, error) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/admin/bridge/test/simulate", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	c := echoContextWithBridge(e.NewContext(req, rec), bh)

	h := &adminHandler{}
	return rec, h.simulate(c)
}

func TestSimulateHandler_BadPayload(t *testing.T) {
	_, err := simulateRequest(t, newTestBridgeHandler(&fakePartner{name: "test"}, nil), `{invalid`)
	require.Error(t, err)
}

func TestSimulate_SuccessPayload(t *testing.T) {
	fp := &fakePartner{name: "test"}
	rec, err := simulateRequest(t, newTestBridgeHandler(fp, nil), springPayload)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "success")
	require.Len(t, fp.calls, 1)
	assert.Equal(t, springRemediation(), fp.calls[0])
}

func TestSimulate_PartnerFailure(t *testing.T) {
	fp := &fakePartner{name: "test", err: errors.New("upload failed")}
	rec, err := simulateRequest(t, newTestBridgeHandler(fp, nil), springPayload)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "failed")
	assert.Contains(t, rec.Body.String(), "upload failed")
}

func TestSimulate_JFrogPipeline(t *testing.T) {
	skipIfFixturesAbsent(t, fixturePOM, fixtureOSVDir)
	pomData := readSpringPOM(t)

	registryServer := httptest.NewServer(pipelineRegistryHandler(pomData, pomData, testSBOMBody, testASCBody))
	defer registryServer.Close()

	osvServer := httptest.NewServer(pipelineOSVHandler())
	defer osvServer.Close()

	jfrogServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			w.WriteHeader(http.StatusCreated)
		case http.MethodGet:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer jfrogServer.Close()

	bh := newTestBridgeHandler(newJFrogPipelineHandler(registryServer, osvServer, jfrogServer), nil)
	rec, err := simulateRequest(t, bh, springPayload)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "success")
}

func TestSimulate_SonatypePipeline(t *testing.T) {
	skipIfFixturesAbsent(t, fixturePOM, fixtureOSVDir)
	pomData := readSpringPOM(t)

	registryServer := httptest.NewServer(pipelineRegistryHandler(pomData, pomData, testSBOMBody, testASCBody))
	defer registryServer.Close()

	osvServer := httptest.NewServer(pipelineOSVHandler())
	defer osvServer.Close()

	s3Server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer s3Server.Close()

	bh := newTestBridgeHandler(newSonatypePipelineHandler(registryServer, osvServer, s3Server), nil)
	rec, err := simulateRequest(t, bh, springPayload)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "success")
}

func TestSimulateHandler_EmbargoRejection(t *testing.T) {
	fp := &fakePartner{name: "test"}
	payload := `{"specversion":"1.0","type":"com.redhat.console.lightwelll.lightwell-advisory-created","eventtype":"java-remediated","data":[{"metadata":{},"payload":{"package_name":"org.test:test","releases":[{"release_names":[{"name":"1.0.rhlw-00001"}],"related_cve":[{"cve":"LTWL-0001","severity":"important"}]}]}}]}`
	rec, err := simulateRequest(t, newTestBridgeHandler(fp, nil), payload)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "rejected")
	assert.Empty(t, fp.calls)
}
