package terms_service_client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestClient(t *testing.T, server *httptest.Server, site string, events []string) *termsServiceImpl {
	t.Helper()
	return &termsServiceImpl{
		client:  *server.Client(),
		baseURL: server.URL,
		site:    site,
		events:  events,
	}
}

func TestIsTermsAcceptanceRequired_True(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Contains(t, r.URL.Path, "/svcrest/terms/presentation/isrequired")
		assert.Equal(t, "testuser", r.URL.Query().Get("login"))
		assert.Equal(t, "lightwell", r.URL.Query().Get("site"))
		assert.Equal(t, []string{"network", "academic"}, r.URL.Query()["event"])
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("True"))
	}))
	defer server.Close()

	client := newTestClient(t, server, "lightwell", []string{"network", "academic"})
	result, err := client.IsTermsAcceptanceRequired(context.Background(), "testuser")
	require.NoError(t, err)
	assert.True(t, result)
}

func TestIsTermsAcceptanceRequired_False(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("False"))
	}))
	defer server.Close()

	client := newTestClient(t, server, "lightwell", []string{"network", "academic"})
	result, err := client.IsTermsAcceptanceRequired(context.Background(), "testuser")
	require.NoError(t, err)
	assert.False(t, result)
}

func TestIsTermsAcceptanceRequired_LowercaseTrue(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("true"))
	}))
	defer server.Close()

	client := newTestClient(t, server, "lightwell", []string{"network", "academic"})
	result, err := client.IsTermsAcceptanceRequired(context.Background(), "testuser")
	require.NoError(t, err)
	assert.True(t, result)
}

func TestIsTermsAcceptanceRequired_Non200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("error"))
	}))
	defer server.Close()

	client := newTestClient(t, server, "lightwell", []string{"network", "academic"})
	_, err := client.IsTermsAcceptanceRequired(context.Background(), "testuser")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestIsTermsAcceptanceRequired_MalformedBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("garbage"))
	}))
	defer server.Close()

	client := newTestClient(t, server, "lightwell", []string{"network", "academic"})
	result, err := client.IsTermsAcceptanceRequired(context.Background(), "testuser")
	require.NoError(t, err)
	assert.False(t, result)
}

// --- GetRequiredEvents tests ---

func TestGetRequiredEvents_BothRequired(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "testuser", r.URL.Query().Get("login"))
		assert.Equal(t, "lightwell", r.URL.Query().Get("site"))
		// Each call should have exactly one event
		events := r.URL.Query()["event"]
		assert.Len(t, events, 1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("True"))
	}))
	defer server.Close()

	client := newTestClient(t, server, "lightwell", []string{"network", "academic"})
	events, err := client.GetRequiredEvents(context.Background(), "testuser")
	require.NoError(t, err)
	assert.Equal(t, []string{"network", "academic"}, events)
}

func TestGetRequiredEvents_OnlyOneRequired(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		event := r.URL.Query().Get("event")
		w.WriteHeader(http.StatusOK)
		if event == "academic" {
			_, _ = w.Write([]byte("True"))
		} else {
			_, _ = w.Write([]byte("False"))
		}
	}))
	defer server.Close()

	client := newTestClient(t, server, "lightwell", []string{"network", "academic"})
	events, err := client.GetRequiredEvents(context.Background(), "testuser")
	require.NoError(t, err)
	assert.Equal(t, []string{"academic"}, events)
}

func TestGetRequiredEvents_NoneRequired(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("False"))
	}))
	defer server.Close()

	client := newTestClient(t, server, "lightwell", []string{"network", "academic"})
	events, err := client.GetRequiredEvents(context.Background(), "testuser")
	require.NoError(t, err)
	assert.Empty(t, events)
}

func TestGetRequiredEvents_ErrorPropagates(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount++
		if callCount == 1 {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("False"))
		} else {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("error"))
		}
	}))
	defer server.Close()

	client := newTestClient(t, server, "lightwell", []string{"network", "academic"})
	events, err := client.GetRequiredEvents(context.Background(), "testuser")
	assert.Error(t, err)
	assert.Nil(t, events)
}
