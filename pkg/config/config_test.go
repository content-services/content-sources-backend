package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

const MockCertData = "./test_files/cert.crt"

func TestConfigureCertificateFile(t *testing.T) {
	c := Get()
	c.Certs.CertPath = MockCertData
	os.Setenv(RhCertEnv, "")
	cert, err := ConfigureCertificate()
	assert.Nil(t, err)
	assert.NotNil(t, cert)
}

func TestConfigureCertificateEnv(t *testing.T) {
	file, err := os.ReadFile(MockCertData)
	assert.Nil(t, err)
	os.Setenv(RhCertEnv, string(file))
	cert, err := ConfigureCertificate()
	assert.Nil(t, err)
	assert.NotNil(t, cert)
}

func TestBadCertsConfigureCertificate(t *testing.T) {
	c := Get()

	// Test bad path
	c.Certs.CertPath = "/tmp/foo"
	os.Setenv(RhCertEnv, "")
	cert, err := ConfigureCertificate()
	assert.Nil(t, cert)
	assert.Contains(t, err.Error(), "no such file")

	// Test bad cert in env variable, should ignore path if set
	os.Setenv(RhCertEnv, "not a real cert")
	cert, err = ConfigureCertificate()
	assert.Nil(t, cert)
	assert.Contains(t, err.Error(), "failed to find any PEM")
}

func TestNoCertConfigureCertificate(t *testing.T) {
	c := Get()
	os.Setenv(RhCertEnv, "")
	c.Certs.CertPath = ""
	cert, err := ConfigureCertificate()
	assert.Nil(t, cert)
	assert.Nil(t, err)

}
