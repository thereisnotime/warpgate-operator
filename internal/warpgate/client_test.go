package warpgate

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	t.Run("session auth", func(t *testing.T) {
		c := NewClient(Config{
			Host:     "https://warpgate.example.com",
			Username: "u", Password: "p",
		})
		if c.baseURL != "https://warpgate.example.com/@warpgate/admin/api" {
			t.Errorf("unexpected baseURL: %s", c.baseURL)
		}
		if c.authed {
			t.Error("session client should not be pre-authed")
		}
		if c.token != "" {
			t.Error("session client should have empty token")
		}
	})

	t.Run("bearer token", func(t *testing.T) {
		c := NewClient(Config{
			Host:  "https://warpgate.example.com",
			Token: "my-secret-token",
		})
		if c.baseURL != "https://warpgate.example.com/@warpgate/admin/api" {
			t.Errorf("unexpected baseURL: %s", c.baseURL)
		}
		if !c.authed {
			t.Error("token client should be pre-authed")
		}
		if c.token != "my-secret-token" {
			t.Errorf("unexpected token: %s", c.token)
		}
	})
}

func TestNewClientTrailingSlash(t *testing.T) {
	c := NewClient(Config{
		Host: "https://warpgate.example.com/",
	})
	if c.baseURL != "https://warpgate.example.com/@warpgate/admin/api" {
		t.Errorf("unexpected baseURL: %s", c.baseURL)
	}
}

func TestSessionLogin(t *testing.T) {
	var loginCalled bool
	mux := http.NewServeMux()
	mux.HandleFunc("/@warpgate/api/auth/login", func(w http.ResponseWriter, r *http.Request) {
		loginCalled = true
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["username"] != "admin" || body["password"] != "secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "warpgate", Value: "session-123", Path: "/"})
		w.WriteHeader(http.StatusCreated)
	})
	mux.HandleFunc("/@warpgate/admin/api/roles", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]string{})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Username: "admin", Password: "secret"})
	err := c.Get("/roles", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !loginCalled {
		t.Error("expected login to be called")
	}
}

func TestLoginFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"bad credentials"}`))
	}))
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Username: "bad", Password: "bad"})
	err := c.Get("/roles", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestContentTypeHeaders(t *testing.T) {
	var gotContentType, gotAccept string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		gotAccept = r.Header.Get("Accept")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewTestClient(srv.URL)
	_ = c.Post("/test", map[string]string{"key": "val"}, nil)

	if gotContentType != "application/json" {
		t.Errorf("expected Content-Type=application/json, got %q", gotContentType)
	}
	if gotAccept != "application/json" {
		t.Errorf("expected Accept=application/json, got %q", gotAccept)
	}
}

func TestGetUnmarshal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "123", "name": "test"})
	}))
	defer srv.Close()

	c := NewTestClient(srv.URL)
	var result map[string]string
	err := c.Get("/roles", &result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["id"] != "123" || result["name"] != "test" {
		t.Errorf("unexpected result: %v", result)
	}
}

func TestPostWithBody(t *testing.T) {
	var gotBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "new-id"})
	}))
	defer srv.Close()

	c := NewTestClient(srv.URL)
	var result map[string]string
	err := c.Post("/roles", map[string]string{"name": "admin"}, &result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotBody["name"] != "admin" {
		t.Errorf("request body not sent correctly: %v", gotBody)
	}
	if result["id"] != "new-id" {
		t.Errorf("response not parsed correctly: %v", result)
	}
}

func TestPut(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewTestClient(srv.URL)
	err := c.Put("/role/123", map[string]string{"name": "updated"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDelete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := NewTestClient(srv.URL)
	err := c.Delete("/role/123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAPIErrorParsing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	}))
	defer srv.Close()

	c := NewTestClient(srv.URL)
	err := c.Get("/role/nonexistent", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != 404 {
		t.Errorf("expected status 404, got %d", apiErr.StatusCode)
	}
	if apiErr.Body != `{"error":"not found"}` {
		t.Errorf("unexpected body: %s", apiErr.Body)
	}
}

func TestIsNotFound(t *testing.T) {
	if IsNotFound(nil) {
		t.Error("nil should not be not-found")
	}
	if IsNotFound(&APIError{StatusCode: 500}) {
		t.Error("500 should not be not-found")
	}
	if !IsNotFound(&APIError{StatusCode: 404}) {
		t.Error("404 should be not-found")
	}
}

func TestIsConflict(t *testing.T) {
	if IsConflict(nil) {
		t.Error("nil should not be conflict")
	}
	if IsConflict(&APIError{StatusCode: 500}) {
		t.Error("500 should not be conflict")
	}
	if !IsConflict(&APIError{StatusCode: 409}) {
		t.Error("409 should be conflict")
	}
}

func TestInsecureSkipVerify(t *testing.T) {
	c := NewClient(Config{
		Host:     "https://localhost",
		Username: "u", Password: "p",
		InsecureSkipVerify: true,
	})
	transport := c.httpClient.Transport.(*http.Transport)
	if transport.TLSClientConfig == nil || !transport.TLSClientConfig.InsecureSkipVerify {
		t.Error("expected InsecureSkipVerify to be true")
	}
}

func TestNonJSONErrorBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("Internal Server Error: something went wrong"))
	}))
	defer srv.Close()

	c := NewTestClient(srv.URL)
	err := c.Get("/failing-endpoint", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != 500 {
		t.Errorf("expected status 500, got %d", apiErr.StatusCode)
	}
	if apiErr.Body != "Internal Server Error: something went wrong" {
		t.Errorf("unexpected body: %s", apiErr.Body)
	}
}

func TestInvalidURLRequestCreation(t *testing.T) {
	c := &Client{
		baseURL:  "://bad-url",
		username: "u", password: "p", authed: true,
		httpClient: &http.Client{},
	}
	err := c.Get("/test", nil)
	if err == nil {
		t.Fatal("expected error for invalid URL, got nil")
	}
	if _, ok := err.(*APIError); ok {
		t.Error("expected a non-APIError (request creation failure), got *APIError")
	}
}

func TestSecureClientNoTLSConfig(t *testing.T) {
	c := NewClient(Config{
		Host:     "https://localhost",
		Username: "u", Password: "p",
		InsecureSkipVerify: false,
	})
	transport := c.httpClient.Transport.(*http.Transport)
	if transport.TLSClientConfig != nil {
		t.Error("expected no TLSClientConfig when InsecureSkipVerify is false")
	}
}

func TestUnmarshalErrorOnBadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{not valid json"))
	}))
	defer srv.Close()

	c := NewTestClient(srv.URL)
	var result map[string]string
	err := c.Get("/bad-json", &result)
	if err == nil {
		t.Fatal("expected unmarshal error, got nil")
	}
	// Should not be an APIError since status was 200.
	if _, ok := err.(*APIError); ok {
		t.Error("expected unmarshal error, not *APIError")
	}
}

func TestEmptyBodyNoUnmarshalError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := NewTestClient(srv.URL)
	var result map[string]string
	// Even with a non-nil result pointer, empty body should not error.
	err := c.Get("/empty", &result)
	if err != nil {
		t.Fatalf("expected no error for empty body with 204, got: %v", err)
	}
}

func TestConnectionRefused(t *testing.T) {
	c := NewClient(Config{
		Host:     "http://127.0.0.1:1",
		Username: "u", Password: "p",
	})
	err := c.Get("/test", nil)
	if err == nil {
		t.Fatal("expected connection error, got nil")
	}
	if _, ok := err.(*APIError); ok {
		t.Error("expected a transport-level error, not *APIError")
	}
}

func TestDoRequestMarshalError(t *testing.T) {
	c := NewTestClient("http://localhost")
	// math.Inf cannot be marshaled to JSON
	_, err := c.doRequest("POST", "/test", math.Inf(1))
	if err == nil {
		t.Fatal("expected marshal error, got nil")
	}
	if !strings.Contains(err.Error(), "marshaling request body") {
		t.Errorf("expected marshaling error, got: %v", err)
	}
}

type brokenReadCloser struct{}

func (b *brokenReadCloser) Read([]byte) (int, error) {
	return 0, errors.New("read exploded")
}
func (b *brokenReadCloser) Close() error { return nil }

type brokenBodyTransport struct {
	statusCode int
}

func (t *brokenBodyTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: t.statusCode,
		Body:       &brokenReadCloser{},
		Header:     make(http.Header),
	}, nil
}

func TestDoReadAllError(t *testing.T) {
	c := &Client{
		baseURL:  "http://localhost",
		username: "u", password: "p", authed: true,
		httpClient: &http.Client{
			Transport: &brokenBodyTransport{statusCode: 200},
		},
	}
	err := c.do("GET", "/test", nil, nil)
	if err == nil {
		t.Fatal("expected read error, got nil")
	}
	if !strings.Contains(err.Error(), "reading response body") {
		t.Errorf("expected reading response body error, got: %v", err)
	}
}

func TestDoNilResultWithBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"some":"data"}`))
	}))
	defer srv.Close()

	c := NewTestClient(srv.URL)
	// result is nil — should not attempt unmarshal and should not error
	err := c.do("GET", "/test", nil, nil)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestLoginSkippedWhenAlreadyAuthed(t *testing.T) {
	// Verify that login() is a no-op when already authenticated.
	c := NewTestClient("http://localhost")
	if !c.authed {
		t.Fatal("expected test client to be pre-authed")
	}
	err := c.login()
	if err != nil {
		t.Fatalf("expected nil from login() when already authed, got: %v", err)
	}
}

func TestBearerTokenAuth(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("X-Warpgate-Token")
		_ = json.NewEncoder(w).Encode([]map[string]string{})
	}))
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Token: "test-token-abc"})
	err := c.Get("/roles", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotAuth != "test-token-abc" {
		t.Errorf("expected X-Warpgate-Token header 'test-token-abc', got %q", gotAuth)
	}
}

func TestBearerTokenSkipsLogin(t *testing.T) {
	var loginCalled bool
	mux := http.NewServeMux()
	mux.HandleFunc("/@warpgate/api/auth/login", func(w http.ResponseWriter, r *http.Request) {
		loginCalled = true
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/@warpgate/admin/api/roles", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]string{})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Token: "skip-login-token"})
	err := c.Get("/roles", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if loginCalled {
		t.Error("login endpoint should NOT be called when bearer token is set")
	}
}

func TestLoginHTTPPostFailure(t *testing.T) {
	// Use a host with an invalid scheme to make httpClient.Post fail
	// before getting a response (covers the "login request" error path).
	c := NewClient(Config{
		Host:     "http://[::1]:namedport", // invalid URL that will fail at Post time
		Username: "admin",
		Password: "secret",
	})
	err := c.login()
	if err == nil {
		t.Fatal("expected login request error, got nil")
	}
	if !strings.Contains(err.Error(), "login request") {
		t.Errorf("expected 'login request' in error, got: %v", err)
	}
}

func TestLoginHTTPErrorResponse(t *testing.T) {
	// Cover the branch where login gets a response with status >= 400
	// and reads the body into an APIError.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"account disabled"}`))
	}))
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Username: "admin", Password: "bad"})
	err := c.login()
	if err == nil {
		t.Fatal("expected error from login, got nil")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != http.StatusForbidden {
		t.Errorf("expected status 403, got %d", apiErr.StatusCode)
	}
	if !strings.Contains(apiErr.Body, "account disabled") {
		t.Errorf("expected body to contain 'account disabled', got: %s", apiErr.Body)
	}
}

func TestAPIErrorString(t *testing.T) {
	err := &APIError{StatusCode: 403, Body: "forbidden"}
	expected := "warpgate API error (status 403): forbidden"
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}
}

func tlsServerCAPEM(srv *httptest.Server) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
}

func TestCACertVerifiesServer(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Token: "t", CACert: tlsServerCAPEM(srv)})
	tr := c.httpClient.Transport.(*http.Transport)
	if tr.TLSClientConfig == nil || tr.TLSClientConfig.RootCAs == nil {
		t.Fatal("expected RootCAs to be set from CACert")
	}
	if tr.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("CACert must not disable verification")
	}
	var out []any
	if err := c.Get("/roles", &out); err != nil {
		t.Fatalf("expected request to succeed against the trusted CA, got: %v", err)
	}
}

func TestCACertRejectsOtherServer(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	// Trust an unrelated self-signed CA that did not sign the server's cert.
	c := NewClient(Config{Host: srv.URL, Token: "t", CACert: selfSignedPEM(t)})
	if err := c.Get("/roles", nil); err == nil {
		t.Fatal("expected certificate verification to fail for an untrusted server")
	}
}

func TestNoCACertUsesSystemRoots(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Token: "t"})
	if err := c.Get("/roles", nil); err == nil {
		t.Fatal("expected verification against system roots to reject the test server")
	}
}

func TestInsecureSkipVerifyWinsOverCACert(t *testing.T) {
	c := NewClient(Config{Host: "https://x", InsecureSkipVerify: true, CACert: selfSignedPEM(t)})
	tr := c.httpClient.Transport.(*http.Transport)
	if !tr.TLSClientConfig.InsecureSkipVerify || tr.TLSClientConfig.RootCAs != nil {
		t.Fatal("expected InsecureSkipVerify to take precedence over CACert")
	}
}

func selfSignedPEM(t *testing.T) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "unrelated"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}
