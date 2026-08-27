package jfrog_bridge

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func generateTestECDSAKey(t *testing.T) (*ecdsa.PrivateKey, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	der, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)

	pemBlock := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der})
	return key, string(pemBlock)
}

func TestEvidenceCreator_CreateAndDeploy(t *testing.T) {
	testKey, _ := generateTestECDSAKey(t)

	var capturedEnvelope dsseEnvelope
	paeContent := "test-pae-content"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/evidence/api/v1/evidence/prepare":
			resp := prepareResponse{
				DSSEPayload: base64.StdEncoding.EncodeToString([]byte("test-payload")),
				PAE:         paeContent,
				PostURL:     "/evidence/api/v1/evidence/deploy",
			}
			w.Header().Set("Content-Type", "application/json")
			require.NoError(t, json.NewEncoder(w).Encode(resp))

		case "/evidence/api/v1/evidence/deploy":
			require.NoError(t, json.NewDecoder(r.Body).Decode(&capturedEnvelope))
			resp := struct {
				Verified bool   `json:"verified"`
				ID       string `json:"id"`
			}{Verified: true, ID: "evd-123"}
			w.Header().Set("Content-Type", "application/json")
			require.NoError(t, json.NewEncoder(w).Encode(resp))

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	ec := &evidenceCreator{
		httpClient:  server.Client(),
		catalogURL:  server.URL,
		catalogRepo: "test-repo",
		token:       "test-token",
		privateKey:  testKey,
		keyAlias:    "redhat-partner-lightwell",
		maxRetries:  0,
	}

	predicate := []byte(`{"test": "predicate"}`)
	err := ec.CreateAndDeploy(context.Background(), predicate,
		"org/springframework/spring-core/5.3.18.rhlw-00003/spring-core-5.3.18.rhlw-00003.jar",
		"abc123sha256")
	require.NoError(t, err)

	// Verify envelope structure
	assert.Equal(t, "redhat-partner-lightwell", capturedEnvelope.Signatures[0].KeyID)
	assert.Equal(t, "application/vnd.in-toto+json", capturedEnvelope.PayloadType)

	// Verify signature is valid
	sigBytes, err := base64.StdEncoding.DecodeString(capturedEnvelope.Signatures[0].Sig)
	require.NoError(t, err)

	paeHash := sha256.Sum256([]byte(paeContent))
	valid := ecdsa.VerifyASN1(&testKey.PublicKey, paeHash[:], sigBytes)
	assert.True(t, valid, "ECDSA signature should verify against test key")
}

func TestEvidenceCreator_DeployNotVerified(t *testing.T) {
	testKey, _ := generateTestECDSAKey(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/evidence/api/v1/evidence/prepare":
			resp := prepareResponse{
				DSSEPayload: base64.StdEncoding.EncodeToString([]byte("test-payload")),
				PAE:         "test-pae-content",
				PostURL:     "/evidence/api/v1/evidence/deploy",
			}
			w.Header().Set("Content-Type", "application/json")
			require.NoError(t, json.NewEncoder(w).Encode(resp))

		case "/evidence/api/v1/evidence/deploy":
			resp := struct {
				Verified bool   `json:"verified"`
				ID       string `json:"id"`
			}{Verified: false, ID: "evd-fail"}
			w.Header().Set("Content-Type", "application/json")
			require.NoError(t, json.NewEncoder(w).Encode(resp))

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	ec := &evidenceCreator{
		httpClient:  server.Client(),
		catalogURL:  server.URL,
		catalogRepo: "test-repo",
		token:       "test-token",
		privateKey:  testKey,
		keyAlias:    "redhat-partner-lightwell",
		maxRetries:  0,
	}

	predicate := []byte(`{"test": "predicate"}`)
	err := ec.CreateAndDeploy(context.Background(), predicate,
		"org/springframework/spring-core/5.3.18.rhlw-00003/spring-core-5.3.18.rhlw-00003.jar",
		"abc123sha256")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not verified")
}

func TestEvidenceCreator_DeployHTTPError(t *testing.T) {
	testKey, _ := generateTestECDSAKey(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/evidence/api/v1/evidence/prepare":
			resp := prepareResponse{
				DSSEPayload: base64.StdEncoding.EncodeToString([]byte("test-payload")),
				PAE:         "test-pae-content",
				PostURL:     "/evidence/api/v1/evidence/deploy",
			}
			w.Header().Set("Content-Type", "application/json")
			require.NoError(t, json.NewEncoder(w).Encode(resp))

		case "/evidence/api/v1/evidence/deploy":
			w.WriteHeader(http.StatusInternalServerError)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	ec := &evidenceCreator{
		httpClient:  server.Client(),
		catalogURL:  server.URL,
		catalogRepo: "test-repo",
		token:       "test-token",
		privateKey:  testKey,
		keyAlias:    "redhat-partner-lightwell",
		maxRetries:  0,
	}

	predicate := []byte(`{"test": "predicate"}`)
	err := ec.CreateAndDeploy(context.Background(), predicate,
		"org/springframework/spring-core/5.3.18.rhlw-00003/spring-core-5.3.18.rhlw-00003.jar",
		"abc123sha256")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "deploy evidence")
}

func TestParseECDSAPrivateKey_P256Required(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	require.NoError(t, err)

	der, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)

	pemBlock := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der})
	_, err = parseECDSAPrivateKey(string(pemBlock))
	require.Error(t, err)
	errMsg := err.Error()
	assert.True(t, strings.Contains(errMsg, "P-256") || strings.Contains(errMsg, "curve"),
		"error should mention P-256 or curve, got: %s", errMsg)
}
