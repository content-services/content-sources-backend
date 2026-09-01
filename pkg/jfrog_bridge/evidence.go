package jfrog_bridge

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// EvidenceCreator signs and deploys DSSE evidence to JFrog.
type EvidenceCreator interface {
	CreateAndDeploy(ctx context.Context, predicate []byte, repoPath, sha256hex string) error
}

type evidenceCreator struct {
	httpClient  *http.Client
	catalogURL  string
	catalogRepo string
	token       string
	privateKey  *ecdsa.PrivateKey
	keyAlias    string
	maxRetries  int
}

type prepareSubject struct {
	SubjectType string `json:"subject_type"`
	RepoPath    string `json:"repo_path,omitempty"`
	SHA256      string `json:"sha256,omitempty"`
}

type prepareRequest struct {
	Subject       prepareSubject         `json:"subject"`
	Predicate     map[string]interface{} `json:"predicate"`
	PredicateType string                 `json:"predicate_type"`
	KeyAlias      string                 `json:"key_alias,omitempty"`
}

type prepareResponse struct {
	DSSEPayload     string `json:"dsse_payload"`
	DSSEPayloadType string `json:"dsse_payload_type"`
	PAE             string `json:"pre_authentication_encoding"`
	PostURL         string `json:"post_url"`
}

type dsseEnvelope struct {
	PayloadType string          `json:"payloadType"`
	Payload     string          `json:"payload"`
	Signatures  []dsseSignature `json:"signatures"`
}

type dsseSignature struct {
	KeyID string `json:"keyid"`
	Sig   string `json:"sig"`
}

func newEvidenceCreator(cfg bridgeConfig) (EvidenceCreator, error) {
	ec := &evidenceCreator{
		httpClient: &http.Client{
			Timeout: time.Duration(cfg.RequestTimeout) * time.Second,
		},
		catalogURL:  strings.TrimSuffix(cfg.CatalogURL, "/"),
		catalogRepo: cfg.CatalogRepo,
		token:       cfg.CatalogToken,
		keyAlias:    cfg.SigningKeyAlias,
		maxRetries:  cfg.MaxRetries,
	}

	if cfg.SigningKeyPEM != "" {
		key, err := parseECDSAPrivateKey(cfg.SigningKeyPEM)
		if err != nil {
			return nil, fmt.Errorf("parse signing key: %w", err)
		}
		ec.privateKey = key
	}

	return ec, nil
}

func (e *evidenceCreator) CreateAndDeploy(ctx context.Context, predicate []byte, repoPath, sha256hex string) error {
	if e.privateKey == nil {
		return fmt.Errorf("signing key not configured")
	}

	// Parse predicate JSON so it is embedded as an object, not a string
	var predicateObj map[string]interface{}
	if err := json.Unmarshal(predicate, &predicateObj); err != nil {
		return fmt.Errorf("parse predicate JSON: %w", err)
	}

	prepReq := prepareRequest{
		Subject: prepareSubject{
			SubjectType: "artifact",
			RepoPath:    fmt.Sprintf("%s/%s", e.catalogRepo, repoPath),
			SHA256:      sha256hex,
		},
		Predicate:     predicateObj,
		PredicateType: "https://in-toto.io/attestation/vulns",
		KeyAlias:      e.keyAlias,
	}

	prepBody, err := json.Marshal(prepReq)
	if err != nil {
		return fmt.Errorf("marshal prepare request: %w", err)
	}

	prepURL := fmt.Sprintf("%s/evidence/api/v1/evidence/prepare?include_pae=true", e.catalogURL)
	prepResp, err := e.doPost(ctx, prepURL, prepBody)
	if err != nil {
		return fmt.Errorf("prepare evidence: %w", err)
	}

	var prep prepareResponse
	if err := json.Unmarshal(prepResp, &prep); err != nil {
		return fmt.Errorf("parse prepare response: %w", err)
	}

	// Build PAE if not returned by the API
	pae := prep.PAE
	if pae == "" {
		payloadBytes, err := base64.StdEncoding.DecodeString(prep.DSSEPayload)
		if err != nil {
			return fmt.Errorf("decode dsse_payload: %w", err)
		}
		payloadType := prep.DSSEPayloadType
		if payloadType == "" {
			payloadType = "application/vnd.in-toto+json"
		}
		pae = fmt.Sprintf("DSSEv1 %d %s %d %s",
			len(payloadType), payloadType, len(payloadBytes), string(payloadBytes))
	}

	// Sign the PAE with ECDSA P-256
	paeHash := sha256.Sum256([]byte(pae))
	sigBytes, err := ecdsa.SignASN1(rand.Reader, e.privateKey, paeHash[:])
	if err != nil {
		return fmt.Errorf("sign PAE: %w", err)
	}

	payloadType := prep.DSSEPayloadType
	if payloadType == "" {
		payloadType = "application/vnd.in-toto+json"
	}

	envelope := dsseEnvelope{
		PayloadType: payloadType,
		Payload:     prep.DSSEPayload,
		Signatures: []dsseSignature{
			{
				KeyID: e.keyAlias,
				Sig:   base64.StdEncoding.EncodeToString(sigBytes),
			},
		},
	}

	envBody, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("marshal envelope: %w", err)
	}

	postURL := prep.PostURL
	if !strings.HasPrefix(postURL, "http") {
		postURL = e.catalogURL + postURL
	}

	respBody, err := e.doPost(ctx, postURL, envBody)
	if err != nil {
		return fmt.Errorf("deploy evidence: %w", err)
	}

	var result struct {
		Verified bool   `json:"verified"`
		ID       string `json:"id"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("parse evidence response: %w", err)
	}
	if !result.Verified {
		return fmt.Errorf("evidence not verified by JFrog")
	}

	return nil
}

func (e *evidenceCreator) doPost(ctx context.Context, url string, body []byte) ([]byte, error) {
	var result []byte
	err := retryWithBackoff(ctx, e.maxRetries, func() error {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		if e.token != "" {
			req.Header.Set("Authorization", "Bearer "+e.token)
		}

		resp, err := e.httpClient.Do(req)
		if err != nil {
			return err
		}
		respBody, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return fmt.Errorf("read response: %w", readErr)
		}

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			result = respBody
			return nil
		}
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	})
	return result, err
}

func parseECDSAPrivateKey(pemStr string) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, fmt.Errorf("no PEM block found in signing key")
	}

	key, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		// Try PKCS8 format
		pkcs8Key, err2 := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err2 != nil {
			return nil, fmt.Errorf("parse EC private key: %w (pkcs8: %v)", err, err2)
		}
		ecKey, ok := pkcs8Key.(*ecdsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("PKCS8 key is not ECDSA")
		}
		if ecKey.Curve != elliptic.P256() {
			return nil, fmt.Errorf("signing key must be ECDSA P-256, got %s", ecKey.Curve.Params().Name)
		}
		return ecKey, nil
	}
	if key.Curve != elliptic.P256() {
		return nil, fmt.Errorf("signing key must be ECDSA P-256, got %s", key.Curve.Params().Name)
	}
	return key, nil
}
