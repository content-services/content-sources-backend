package config

import (
	_ "embed"
	"testing"
	"time"

	"github.com/content-services/content-sources-backend/pkg/errors"
	"github.com/stretchr/testify/assert"
)

var (
	//go:embed test_files/mock_client.crt
	mockCertPEM string
	//go:embed test_files/mock_client.key
	mockKeyPEM string
	//go:embed test_files/mock_ca.crt
	mockCACertPEM string
	invalidPEM    = "not a valid PEM certificate"
)

type mockCertUser struct {
	cert       string
	key        string
	caCert     string
	certPath   string
	keyPath    string
	caCertPath string
	label      string
	proxy      string
}

func (m *mockCertUser) ClientCert() string     { return m.cert }
func (m *mockCertUser) ClientKey() string      { return m.key }
func (m *mockCertUser) CACert() string         { return m.caCert }
func (m *mockCertUser) ClientCertPath() string { return m.certPath }
func (m *mockCertUser) ClientKeyPath() string  { return m.keyPath }
func (m *mockCertUser) CACertPath() string     { return m.caCertPath }
func (m *mockCertUser) Label() string          { return m.label }
func (m *mockCertUser) Proxy() string          { return m.proxy }

func TestGetCertificate(t *testing.T) {
	tests := []struct {
		name        string
		certUser    CertUser
		expectError bool
		errorType   error
	}{
		{
			name: "valid cert and key",
			certUser: &mockCertUser{
				cert:   mockCertPEM,
				key:    mockKeyPEM,
				caCert: mockCACertPEM,
			},
			expectError: false,
		},
		{
			name: "missing both cert and key",
			certUser: &mockCertUser{
				cert: "",
				key:  "",
			},
			expectError: true,
			errorType:   errors.ErrCertKeyNotFound,
		},
		{
			name: "invalid cert format",
			certUser: &mockCertUser{
				cert: invalidPEM,
				key:  mockKeyPEM,
			},
			expectError: true,
		},
		{
			name: "invalid key format",
			certUser: &mockCertUser{
				cert: mockCertPEM,
				key:  invalidPEM,
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cert, err := GetCertificate(tt.certUser)
			if tt.expectError {
				assert.Error(t, err)
				if tt.errorType != nil {
					assert.ErrorIs(t, err, tt.errorType)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, cert)
			}
		})
	}
}

func TestGetHTTPClient(t *testing.T) {
	tests := []struct {
		name        string
		certUser    CertUser
		expectError bool
	}{
		{
			name: "successful client creation",
			certUser: &mockCertUser{
				cert:   mockCertPEM,
				key:    mockKeyPEM,
				caCert: mockCACertPEM,
				label:  "test",
			},
			expectError: false,
		},
		{
			name: "client creation without certificates",
			certUser: &mockCertUser{
				label: "test",
			},
			expectError: false,
		},
		{
			name: "client creation with proxy",
			certUser: &mockCertUser{
				label: "pulp",
				proxy: "http://proxy.example.com:8080",
			},
			expectError: false,
		},
		{
			name: "client creation with invalid certificate",
			certUser: &mockCertUser{
				cert:   invalidPEM,
				key:    mockKeyPEM,
				caCert: mockCACertPEM,
				label:  "test",
			},
			expectError: true,
		},
		{
			name: "client creation with invalid key",
			certUser: &mockCertUser{
				cert:   mockCertPEM,
				key:    invalidPEM,
				caCert: mockCACertPEM,
				label:  "test",
			},
			expectError: true,
		},
		{
			name: "client creation with invalid ca",
			certUser: &mockCertUser{
				cert:   mockCertPEM,
				key:    mockKeyPEM,
				caCert: invalidPEM,
				label:  "test",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := GetHTTPClient(tt.certUser)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, client)
				assert.Equal(t, 90*time.Second, client.Timeout)
				assert.NotNil(t, client.Transport)
			}
		})
	}
}
