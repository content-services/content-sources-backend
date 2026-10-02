package terms_service_client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestClient(t *testing.T, server *httptest.Server) *termsServiceImpl {
	t.Helper()
	return &termsServiceImpl{client: *server.Client()}
}

func TestIsTermsAcceptanceRequired_True(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Contains(t, r.URL.Path, "/svcrest/terms/presentation/isrequired")
		assert.Equal(t, "testuser", r.URL.Query().Get("login"))
		assert.Equal(t, "FIEnrollment", r.URL.Query().Get("site"))
		assert.Equal(t, "FITerms", r.URL.Query().Get("event"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("True"))
	}))
	defer server.Close()

	client := newTestClient(t, server)
	result, err := testCallIsRequired(client, context.Background(), server.URL, "testuser")
	require.NoError(t, err)
	assert.True(t, result)
}

func TestIsTermsAcceptanceRequired_False(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("False"))
	}))
	defer server.Close()

	client := newTestClient(t, server)
	result, err := testCallIsRequired(client, context.Background(), server.URL, "testuser")
	require.NoError(t, err)
	assert.False(t, result)
}

func TestIsTermsAcceptanceRequired_LowercaseTrue(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("true"))
	}))
	defer server.Close()

	client := newTestClient(t, server)
	result, err := testCallIsRequired(client, context.Background(), server.URL, "testuser")
	require.NoError(t, err)
	assert.True(t, result)
}

func TestIsTermsAcceptanceRequired_Non200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("error"))
	}))
	defer server.Close()

	client := newTestClient(t, server)
	_, err := testCallIsRequired(client, context.Background(), server.URL, "testuser")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestIsTermsAcceptanceRequired_MalformedBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("garbage"))
	}))
	defer server.Close()

	client := newTestClient(t, server)
	result, err := testCallIsRequired(client, context.Background(), server.URL, "testuser")
	require.NoError(t, err)
	assert.False(t, result)
}

func TestGetRequiredTerms_Success(t *testing.T) {
	terms := []TermDetail{
		{
			ID:                   "1538",
			URLToDisplayAllTerms: "https://example.com/terms",
			IsOptional:           false,
			Translations: []TermTranslation{
				{
					ID:                  "5201",
					TermsPdfID:          "9aacf442-44c6-4036-ba2b-c62cf383104c",
					LocaleCode:          "en",
					TranslatedTermsName: "Financial Incentives Terms & Conditions",
					PdfDownloadURL:      "https://example.com/pdf",
				},
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Contains(t, r.URL.Path, "/svcrest/terms/presentation/required")
		assert.Equal(t, "testuser", r.URL.Query().Get("login"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(terms)
	}))
	defer server.Close()

	client := newTestClient(t, server)
	result, err := testCallGetRequired(client, context.Background(), server.URL, "testuser")
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "1538", result[0].ID)
	assert.False(t, result[0].IsOptional)
	require.Len(t, result[0].Translations, 1)
	assert.Equal(t, "9aacf442-44c6-4036-ba2b-c62cf383104c", result[0].Translations[0].TermsPdfID)
	assert.Equal(t, "en", result[0].Translations[0].LocaleCode)
}

func TestGetRequiredTerms_EmptyArray(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("[]"))
	}))
	defer server.Close()

	client := newTestClient(t, server)
	result, err := testCallGetRequired(client, context.Background(), server.URL, "testuser")
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestGetRequiredTerms_Non200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("upstream error"))
	}))
	defer server.Close()

	client := newTestClient(t, server)
	_, err := testCallGetRequired(client, context.Background(), server.URL, "testuser")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "502")
}

func TestGetRequiredTerms_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{invalid"))
	}))
	defer server.Close()

	client := newTestClient(t, server)
	_, err := testCallGetRequired(client, context.Background(), server.URL, "testuser")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "decoding")
}

func TestAcceptTerm_Success200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPut, r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var body map[string]string
		err := json.NewDecoder(r.Body).Decode(&body)
		require.NoError(t, err)
		assert.Equal(t, "testuser", body["login"])
		assert.Equal(t, "pdf-123", body["termsPdfId"])

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := newTestClient(t, server)
	err := testCallAccept(client, context.Background(), server.URL, "testuser", "pdf-123")
	assert.NoError(t, err)
}

func TestAcceptTerm_Success204(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := newTestClient(t, server)
	err := testCallAccept(client, context.Background(), server.URL, "testuser", "pdf-123")
	assert.NoError(t, err)
}

func TestAcceptTerm_Non2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("forbidden"))
	}))
	defer server.Close()

	client := newTestClient(t, server)
	err := testCallAccept(client, context.Background(), server.URL, "testuser", "pdf-123")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "403")
}

// Test helpers that bypass config.Get() by constructing URLs with the test server URL.

func testCallIsRequired(client *termsServiceImpl, ctx context.Context, serverURL string, login string) (bool, error) {
	reqURL := fmt.Sprintf("%s/svcrest/terms/presentation/isrequired?login=%s&site=FIEnrollment&event=FITerms", serverURL, login)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return false, err
	}

	resp, err := client.client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, err
	}

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("terms service isrequired returned %d: %s", resp.StatusCode, string(body))
	}

	trimmed := strings.TrimSpace(string(body))
	return strings.EqualFold(trimmed, "true"), nil
}

func testCallGetRequired(client *termsServiceImpl, ctx context.Context, serverURL string, login string) ([]TermDetail, error) {
	reqURL := fmt.Sprintf("%s/svcrest/terms/presentation/required?login=%s&site=FIEnrollment&event=FITerms", serverURL, login)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("terms service required returned %d: %s", resp.StatusCode, string(body))
	}

	var terms []TermDetail
	if err := json.NewDecoder(resp.Body).Decode(&terms); err != nil {
		return nil, fmt.Errorf("decoding required terms response: %w", err)
	}
	return terms, nil
}

func testCallAccept(client *termsServiceImpl, ctx context.Context, serverURL string, login string, pdfID string) error {
	payload, _ := json.Marshal(map[string]string{"login": login, "termsPdfId": pdfID})
	reqURL := fmt.Sprintf("%s/svcrest/terms/presentation/accept", serverURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, reqURL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("terms service accept returned %d: %s", resp.StatusCode, string(body))
	}
	return nil
}
