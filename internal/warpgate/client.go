package warpgate

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"sync"
	"time"
)

const apiBasePath = "/@warpgate/admin/api"

// DefaultTimeout bounds every request made by a Client (including reading the
// response body), so a hung Warpgate instance can't block a reconcile worker forever.
const DefaultTimeout = 30 * time.Second

// Config holds the configuration for a Warpgate API client.
type Config struct {
	Host               string
	Token              string // API token (recommended, bypasses OTP)
	Username           string // For session auth fallback
	Password           string // For session auth fallback
	InsecureSkipVerify bool
	// CACert is a PEM bundle of CAs trusted for the Warpgate server certificate.
	// When set (and InsecureSkipVerify is false), it replaces the system roots.
	CACert []byte
	// Timeout is the per-request timeout. Zero means DefaultTimeout.
	Timeout time.Duration
}

// APIError represents an error response from the Warpgate API.
type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("warpgate API error (status %d): %s", e.StatusCode, e.Body)
}

// session tracks the cookie-session login state. It is shared by a Client and
// every view returned by WithContext, and is safe for concurrent use.
type session struct {
	mu     sync.Mutex
	authed bool
	// gen increments on every successful login so a stale 401 can't throw
	// away a session that another goroutine just established.
	gen uint64
}

func newSession(authed bool) *session {
	return &session{authed: authed}
}

func (s *session) isAuthed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.authed
}

// Client is a Warpgate REST API client supporting token-based and session-based cookie authentication.
//
// A Client is safe for concurrent use and is meant to be long-lived: it owns an
// http.Transport with a keep-alive connection pool, so creating one per request
// or per reconcile leaks connections. Use WithContext to bind a request context.
type Client struct {
	host       string
	baseURL    string
	token      string // API token
	username   string
	password   string
	httpClient *http.Client
	session    *session
	ctx        context.Context
}

// NewClient creates a new Warpgate API client from the given config.
// If Token is set, token-based auth is used (recommended, bypasses OTP).
// Otherwise, Username/Password session auth is used as a fallback.
func NewClient(cfg Config) *Client {
	host := strings.TrimRight(cfg.Host, "/")

	c := &Client{
		host:       host,
		baseURL:    host + apiBasePath,
		token:      cfg.Token,
		httpClient: newHTTPClient(cfg),
	}

	if cfg.Token != "" {
		// API token auth — no login needed.
		c.session = newSession(true)
	} else {
		c.username = cfg.Username
		c.password = cfg.Password
		c.session = newSession(false)
	}

	return c
}

// newHTTPClient builds an http.Client on a clone of http.DefaultTransport, so
// it keeps the proxy settings, dial/TLS handshake timeouts and idle connection
// limits (IdleConnTimeout, MaxIdleConns) of the default transport.
func newHTTPClient(cfg Config) *http.Client {
	jar, _ := cookiejar.New(nil)

	transport := http.DefaultTransport.(*http.Transport).Clone()
	switch {
	case cfg.InsecureSkipVerify:
		transport.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: true, // #nosec G402 -- user-configured InsecureSkipVerify
		}
	case len(cfg.CACert) > 0:
		pool := x509.NewCertPool()
		pool.AppendCertsFromPEM(cfg.CACert)
		transport.TLSClientConfig = &tls.Config{
			RootCAs:    pool,
			MinVersion: tls.VersionTLS12,
		}
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}

	return &http.Client{
		Transport: transport,
		Jar:       jar,
		Timeout:   timeout,
	}
}

// NewTestClient creates a pre-authenticated client for unit tests with httptest servers.
// It skips the login flow so mock servers don't need a login endpoint.
func NewTestClient(host string) *Client {
	host = strings.TrimRight(host, "/")
	return &Client{
		host:       host,
		baseURL:    host + apiBasePath,
		session:    newSession(true),
		httpClient: newHTTPClient(Config{}),
	}
}

// WithContext returns a view of c whose requests use ctx. The view shares the
// connection pool and login session with c.
func (c *Client) WithContext(ctx context.Context) *Client {
	c2 := *c
	c2.ctx = ctx
	return &c2
}

// CloseIdleConnections closes any idle keep-alive connections held by the client.
// Call it when a client is replaced or no longer needed.
func (c *Client) CloseIdleConnections() {
	c.httpClient.CloseIdleConnections()
}

func (c *Client) context() context.Context {
	if c.ctx != nil {
		return c.ctx
	}
	return context.Background()
}

// login authenticates with Warpgate and stores the session cookie. Concurrent
// callers wait for a single login. It returns the session generation in use.
func (c *Client) login() (uint64, error) {
	s := c.session
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.authed {
		return s.gen, nil
	}

	loginURL := c.host + "/@warpgate/api/auth/login"
	body, err := json.Marshal(map[string]string{
		"username": c.username,
		"password": c.password,
	})
	if err != nil {
		return 0, fmt.Errorf("marshaling login body: %w", err)
	}

	req, err := http.NewRequestWithContext(c.context(), http.MethodPost, loginURL, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("creating login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("login request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return 0, &APIError{
			StatusCode: resp.StatusCode,
			Body:       string(respBody),
		}
	}
	// Drain so the connection can go back to the pool.
	_, _ = io.Copy(io.Discard, resp.Body)

	s.authed = true
	s.gen++
	return s.gen, nil
}

// invalidateSession marks the session as logged out, unless a newer login
// already replaced the one that produced the 401.
func (c *Client) invalidateSession(gen uint64) {
	s := c.session
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.gen == gen {
		s.authed = false
	}
}

func (c *Client) doRequest(method, path string, body any) (*http.Response, error) {
	var data []byte
	if body != nil {
		var err error
		data, err = json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshaling request body: %w", err)
		}
	}

	resp, gen, err := c.send(method, path, body != nil, data)
	if err != nil {
		return nil, err
	}

	// A shared client outlives the Warpgate session cookie. With session auth,
	// a 401 most likely means the session expired: log in again and retry once.
	if resp.StatusCode == http.StatusUnauthorized && c.token == "" {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		c.invalidateSession(gen)
		resp, _, err = c.send(method, path, body != nil, data)
		if err != nil {
			return nil, err
		}
	}

	return resp, nil
}

func (c *Client) send(method, path string, hasBody bool, data []byte) (*http.Response, uint64, error) {
	gen, err := c.login()
	if err != nil {
		return nil, 0, fmt.Errorf("authenticating: %w", err)
	}

	var reqBody io.Reader
	if hasBody {
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(c.context(), method, c.baseURL+path, reqBody)
	if err != nil {
		return nil, 0, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	if c.token != "" {
		req.Header.Set("X-Warpgate-Token", c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("executing request: %w", err)
	}

	return resp, gen, nil
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

// IsConflict returns true if the error is a 409 API error.
func IsConflict(err error) bool {
	apiErr, ok := err.(*APIError)
	return ok && apiErr.StatusCode == http.StatusConflict
}
