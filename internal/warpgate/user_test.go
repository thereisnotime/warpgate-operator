package warpgate

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateUser(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/@warpgate/admin/api/users" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(User{ID: "u1", Username: "alice"})
	}))
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	user, err := c.CreateUser(UserCreateRequest{Username: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	if user.ID != "u1" {
		t.Errorf("unexpected user: %+v", user)
	}
}

func TestUpdateUserWithCredentialPolicy(t *testing.T) {
	var gotBody UserUpdateRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_ = json.NewEncoder(w).Encode(User{ID: "u1", Username: "alice", CredentialPolicy: gotBody.CredentialPolicy})
	}))
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	user, err := c.UpdateUser("u1", UserUpdateRequest{
		Username: "alice",
		CredentialPolicy: &CredentialPolicy{
			HTTP: []string{"Password", "Sso"},
			SSH:  []string{"PublicKey"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(gotBody.CredentialPolicy.HTTP) != 2 {
		t.Errorf("expected 2 HTTP cred types, got %d", len(gotBody.CredentialPolicy.HTTP))
	}
	if user.CredentialPolicy == nil || len(user.CredentialPolicy.SSH) != 1 {
		t.Errorf("credential policy not returned correctly: %+v", user.CredentialPolicy)
	}
}

func TestGetUserByUsername(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]User{
			{ID: "u1", Username: "alice"},
			{ID: "u2", Username: "bob"},
		})
	}))
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	user, err := c.GetUserByUsername("bob")
	if err != nil {
		t.Fatal(err)
	}
	if user.ID != "u2" {
		t.Errorf("expected u2, got %s", user.ID)
	}
}

func TestDeleteUser(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" || r.URL.Path != "/@warpgate/admin/api/users/u1" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	if err := c.DeleteUser("u1"); err != nil {
		t.Fatal(err)
	}
}

func TestListUsers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]User{{ID: "u1", Username: "alice"}})
	}))
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	users, err := c.ListUsers("")
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 {
		t.Errorf("expected 1 user, got %d", len(users))
	}
}

func TestGetUser(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/@warpgate/admin/api/users/u1" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(User{ID: "u1", Username: "alice", Description: "test user"})
	}))
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	user, err := c.GetUser("u1")
	if err != nil {
		t.Fatal(err)
	}
	if user.Username != "alice" {
		t.Errorf("expected alice, got %s", user.Username)
	}
}

func TestGetUser_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	}))
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	_, err := c.GetUser("missing")
	if err == nil {
		t.Fatal("expected error")
	}
	if !IsNotFound(err) {
		t.Errorf("expected 404, got %v", err)
	}
}

func TestGetUserByUsername_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]User{})
	}))
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	_, err := c.GetUserByUsername("ghost")
	if err == nil {
		t.Fatal("expected error")
	}
	if !IsNotFound(err) {
		t.Errorf("expected 404, got %v", err)
	}
}

func TestGetUserByUsername_ListError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal"}`))
	}))
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	_, err := c.GetUserByUsername("alice")
	if err == nil {
		t.Fatal("expected error")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != 500 {
		t.Errorf("expected status 500, got %d", apiErr.StatusCode)
	}
}

func TestCreateUser_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"duplicate"}`))
	}))
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	_, err := c.CreateUser(UserCreateRequest{Username: "dup"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestUpdateUser_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal"}`))
	}))
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	_, err := c.UpdateUser("u1", UserUpdateRequest{Username: "alice"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestListUsers_WithSearch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "search=bob" {
			t.Errorf("expected search=bob, got %s", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode([]User{{ID: "u2", Username: "bob"}})
	}))
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	users, err := c.ListUsers("bob")
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 {
		t.Errorf("expected 1, got %d", len(users))
	}
}

func TestListUsers_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"forbidden"}`))
	}))
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	_, err := c.ListUsers("")
	if err == nil {
		t.Fatal("expected error")
	}
}
