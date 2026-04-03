package warpgate

import (
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewClient(t *testing.T) {
	c := NewClient(Config{
		Host:     "https://warpgate.example.com",
		Username: "u", Password: "p",
	})
	if c.baseURL != "https://warpgate.example.com/@warpgate/admin/api" {
		t.Errorf("unexpected baseURL: %s", c.baseURL)
	}
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

func TestAPIErrorString(t *testing.T) {
	err := &APIError{StatusCode: 403, Body: "forbidden"}
	expected := "warpgate API error (status 403): forbidden"
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}
}
