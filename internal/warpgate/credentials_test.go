package warpgate

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreatePasswordCredential(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/@warpgate/admin/api/users/u1/credentials/passwords" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(PasswordCredential{ID: "pc1", Password: "hashed"})
	}))
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	cred, err := c.CreatePasswordCredential("u1", "secret123")
	if err != nil {
		t.Fatal(err)
	}
	if cred.ID != "pc1" {
		t.Errorf("unexpected: %+v", cred)
	}
}

func TestDeletePasswordCredential(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/@warpgate/admin/api/users/u1/credentials/passwords/pc1" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	if err := c.DeletePasswordCredential("u1", "pc1"); err != nil {
		t.Fatal(err)
	}
}

func TestCreatePublicKeyCredential(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req PublicKeyCredentialRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Label != "laptop" {
			t.Errorf("expected label=laptop, got %s", req.Label)
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(PublicKeyCredential{ID: "pk1", Label: req.Label, OpenSSHPublicKey: req.OpenSSHPublicKey, DateAdded: "2024-01-01"})
	}))
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	cred, err := c.CreatePublicKeyCredential("u1", PublicKeyCredentialRequest{Label: "laptop", OpenSSHPublicKey: "ssh-ed25519 AAAA..."})
	if err != nil {
		t.Fatal(err)
	}
	if cred.DateAdded != "2024-01-01" {
		t.Errorf("expected computed date_added, got %s", cred.DateAdded)
	}
}

func TestListPublicKeyCredentials(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]PublicKeyCredential{{ID: "pk1", Label: "laptop"}, {ID: "pk2", Label: "desktop"}})
	}))
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	creds, err := c.ListPublicKeyCredentials("u1")
	if err != nil {
		t.Fatal(err)
	}
	if len(creds) != 2 {
		t.Errorf("expected 2, got %d", len(creds))
	}
}

func TestDeletePublicKeyCredential(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	if err := c.DeletePublicKeyCredential("u1", "pk1"); err != nil {
		t.Fatal(err)
	}
}

func TestCreateSsoCredential(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req SsoCredentialRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(SsoCredential{ID: "sso1", Provider: req.Provider, Email: req.Email})
	}))
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	cred, err := c.CreateSsoCredential("u1", SsoCredentialRequest{Provider: "google", Email: "alice@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if cred.Provider != "google" {
		t.Errorf("expected google, got %s", cred.Provider)
	}
}

func TestDeleteSsoCredential(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	if err := c.DeleteSsoCredential("u1", "sso1"); err != nil {
		t.Fatal(err)
	}
}

func TestUpdatePublicKeyCredential(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/@warpgate/admin/api/users/u1/credentials/public-keys/pk1" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		var req PublicKeyCredentialRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		_ = json.NewEncoder(w).Encode(PublicKeyCredential{ID: "pk1", Label: req.Label, OpenSSHPublicKey: req.OpenSSHPublicKey})
	}))
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	cred, err := c.UpdatePublicKeyCredential("u1", "pk1", PublicKeyCredentialRequest{Label: "updated", OpenSSHPublicKey: "ssh-ed25519 AAAA..."})
	if err != nil {
		t.Fatal(err)
	}
	if cred.Label != "updated" {
		t.Errorf("expected updated, got %s", cred.Label)
	}
}

func TestUpdatePublicKeyCredential_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	}))
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	_, err := c.UpdatePublicKeyCredential("u1", "pk-gone", PublicKeyCredentialRequest{Label: "x"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !IsNotFound(err) {
		t.Errorf("expected 404 error, got %v", err)
	}
}

func TestListSsoCredentials(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/@warpgate/admin/api/users/u1/credentials/sso" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode([]SsoCredential{
			{ID: "sso1", Provider: "google", Email: "a@b.com"},
			{ID: "sso2", Provider: "github", Email: "c@d.com"},
		})
	}))
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	creds, err := c.ListSsoCredentials("u1")
	if err != nil {
		t.Fatal(err)
	}
	if len(creds) != 2 {
		t.Errorf("expected 2, got %d", len(creds))
	}
}

func TestUpdateSsoCredential(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/@warpgate/admin/api/users/u1/credentials/sso/sso1" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		var req SsoCredentialRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		_ = json.NewEncoder(w).Encode(SsoCredential{ID: "sso1", Provider: req.Provider, Email: req.Email})
	}))
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	cred, err := c.UpdateSsoCredential("u1", "sso1", SsoCredentialRequest{Provider: "github", Email: "new@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if cred.Email != "new@example.com" {
		t.Errorf("expected new@example.com, got %s", cred.Email)
	}
}

func TestCreatePasswordCredential_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal"}`))
	}))
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	_, err := c.CreatePasswordCredential("u1", "pw")
	if err == nil {
		t.Fatal("expected error")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected APIError, got %T", err)
	}
	if apiErr.StatusCode != 500 {
		t.Errorf("expected 500, got %d", apiErr.StatusCode)
	}
}

func TestCreatePublicKeyCredential_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad key"}`))
	}))
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	_, err := c.CreatePublicKeyCredential("u1", PublicKeyCredentialRequest{Label: "x"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestListPublicKeyCredentials_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"forbidden"}`))
	}))
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	_, err := c.ListPublicKeyCredentials("u1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCreateSsoCredential_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"conflict"}`))
	}))
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	_, err := c.CreateSsoCredential("u1", SsoCredentialRequest{Provider: "google", Email: "x@y.com"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestListSsoCredentials_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal"}`))
	}))
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	_, err := c.ListSsoCredentials("u1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestUpdateSsoCredential_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	}))
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	_, err := c.UpdateSsoCredential("u1", "gone", SsoCredentialRequest{Provider: "x", Email: "y"})
	if err == nil {
		t.Fatal("expected error")
	}
}
