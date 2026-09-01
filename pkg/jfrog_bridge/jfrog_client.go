package jfrog_bridge

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// JFrogClient abstracts HTTP calls to the JFrog Catalog Partners Artifactory.
type JFrogClient interface {
	UploadFile(ctx context.Context, repoPath string, data []byte, contentType string) error
	SetProperties(ctx context.Context, repoPath string, props map[string]string) error
	VerifyDelivery(ctx context.Context, repoPath string) error
	Ping(ctx context.Context) error
}

type httpJFrogClient struct {
	httpClient  *http.Client
	catalogURL  string
	catalogRepo string
	token       string
	maxRetries  int
}

func newJFrogClient(cfg bridgeConfig) JFrogClient {
	return &httpJFrogClient{
		httpClient: &http.Client{
			Timeout: time.Duration(cfg.RequestTimeout) * time.Second,
		},
		catalogURL:  strings.TrimSuffix(cfg.CatalogURL, "/"),
		catalogRepo: cfg.CatalogRepo,
		token:       cfg.CatalogToken,
		maxRetries:  cfg.MaxRetries,
	}
}

func (c *httpJFrogClient) UploadFile(ctx context.Context, repoPath string, data []byte, contentType string) error {
	uploadURL := fmt.Sprintf("%s/artifactory/%s/%s", c.catalogURL, c.catalogRepo, repoPath)
	err := retryWithBackoff(ctx, c.maxRetries, func() error {
		req, err := http.NewRequestWithContext(ctx, http.MethodPut, uploadURL, bytes.NewReader(data))
		if err != nil {
			return fmt.Errorf("create upload request: %w", err)
		}
		req.Header.Set("Content-Type", contentType)
		c.setAuth(req)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return err
		}
		resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}
		return fmt.Errorf("upload %s: HTTP %d", repoPath, resp.StatusCode)
	})
	if err != nil {
		return fmt.Errorf("upload file: %w", err)
	}
	return nil
}

func (c *httpJFrogClient) SetProperties(ctx context.Context, repoPath string, props map[string]string) error {
	var parts []string
	for k, v := range props {
		parts = append(parts, fmt.Sprintf("%s=%s", k, url.QueryEscape(v)))
	}
	propStr := strings.Join(parts, ";")

	reqURL := fmt.Sprintf("%s/artifactory/api/storage/%s/%s?properties=%s",
		c.catalogURL, c.catalogRepo, repoPath, propStr)

	err := retryWithBackoff(ctx, c.maxRetries, func() error {
		req, err := http.NewRequestWithContext(ctx, http.MethodPut, reqURL, nil)
		if err != nil {
			return fmt.Errorf("create properties request: %w", err)
		}
		c.setAuth(req)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return err
		}
		resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}
		return fmt.Errorf("set properties: HTTP %d", resp.StatusCode)
	})
	if err != nil {
		return fmt.Errorf("set properties: %w", err)
	}
	return nil
}

func (c *httpJFrogClient) VerifyDelivery(ctx context.Context, repoPath string) error {
	url := fmt.Sprintf("%s/artifactory/api/storage/%s/%s",
		c.catalogURL, c.catalogRepo, repoPath)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("create verify request: %w", err)
	}
	c.setAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("verify delivery: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("verify delivery: HTTP %d", resp.StatusCode)
	}
	return nil
}

func (c *httpJFrogClient) Ping(ctx context.Context) error {
	url := fmt.Sprintf("%s/artifactory/api/system/ping", c.catalogURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("create ping request: %w", err)
	}
	c.setAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("ping: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxJFrogPingBytes))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ping: HTTP %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func (c *httpJFrogClient) setAuth(req *http.Request) {
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
}
