package warpgate

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const apiBasePath = "/@warpgate/admin/api"

// Config holds the configuration for a Warpgate API client.
type Config struct {
	Host               string
	Token              string
	InsecureSkipVerify bool
}

// APIError represents an error response from the Warpgate API.
type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("warpgate API error (status %d): %s", e.StatusCode, e.Body)
}

// Client is a Warpgate REST API client.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewClient creates a new Warpgate API client from the given config.
func NewClient(cfg Config) *Client {
	host := strings.TrimRight(cfg.Host, "/")

	transport := &http.Transport{}
	if cfg.InsecureSkipVerify {
		transport.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: true, // #nosec G402 -- user-configured InsecureSkipVerify
		}
	}

	return &Client{
		baseURL: host + apiBasePath,
		token:   cfg.Token,
		httpClient: &http.Client{
			Transport: transport,
		},
	}
}

func (c *Client) doRequest(method, path string, body any) (*http.Response, error) {
	url := c.baseURL + path

	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshaling request body: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("X-Warpgate-Token", c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}

	return resp, nil
}

func (c *Client) do(method, path string, body any, result any) error {
	resp, err := c.doRequest(method, path, body)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		return &APIError{
			StatusCode: resp.StatusCode,
			Body:       string(respBody),
		}
	}

	if result != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("unmarshaling response: %w", err)
		}
	}

	return nil
}

// Get performs a GET request.
func (c *Client) Get(path string, result any) error {
	return c.do(http.MethodGet, path, nil, result)
}

// Post performs a POST request.
func (c *Client) Post(path string, body any, result any) error {
	return c.do(http.MethodPost, path, body, result)
}

// Put performs a PUT request.
func (c *Client) Put(path string, body any, result any) error {
	return c.do(http.MethodPut, path, body, result)
}

// Delete performs a DELETE request.
func (c *Client) Delete(path string) error {
	return c.do(http.MethodDelete, path, nil, nil)
}

// IsNotFound returns true if the error is a 404 API error.
func IsNotFound(err error) bool {
	apiErr, ok := err.(*APIError)
	return ok && apiErr.StatusCode == http.StatusNotFound
}
