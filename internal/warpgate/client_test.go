package warpgate

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewClient(t *testing.T) {
	c := NewClient(Config{
		Host:  "https://warpgate.example.com",
		Token: "test-token",
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

func TestAuthHeader(t *testing.T) {
	var gotHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("X-Warpgate-Token")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Token: "my-secret-token"})
	_ = c.Get("/test", nil)

	if gotHeader != "my-secret-token" {
		t.Errorf("expected X-Warpgate-Token=my-secret-token, got %q", gotHeader)
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

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
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

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
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

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
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

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
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

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
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

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
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
		Host:               "https://localhost",
		Token:              "tok",
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

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
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
		baseURL:    "://bad-url",
		token:      "tok",
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
		Host:               "https://localhost",
		Token:              "tok",
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

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
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

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	var result map[string]string
	// Even with a non-nil result pointer, empty body should not error.
	err := c.Get("/empty", &result)
	if err != nil {
		t.Fatalf("expected no error for empty body with 204, got: %v", err)
	}
}

func TestConnectionRefused(t *testing.T) {
	c := NewClient(Config{
		Host:  "http://127.0.0.1:1",
		Token: "tok",
	})
	err := c.Get("/test", nil)
	if err == nil {
		t.Fatal("expected connection error, got nil")
	}
	if _, ok := err.(*APIError); ok {
		t.Error("expected a transport-level error, not *APIError")
	}
}

func TestAPIErrorString(t *testing.T) {
	err := &APIError{StatusCode: 403, Body: "forbidden"}
	expected := "warpgate API error (status 403): forbidden"
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}
}
