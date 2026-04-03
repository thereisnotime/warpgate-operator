package warpgate

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateUserRole(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/@warpgate/admin/api/users/u1/roles/r1" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	if err := c.CreateUserRole("u1", "r1"); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteUserRole(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	if err := c.DeleteUserRole("u1", "r1"); err != nil {
		t.Fatal(err)
	}
}

func TestListUserRoles(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]Role{{ID: "r1", Name: "admin"}, {ID: "r2", Name: "user"}})
	}))
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	roles, err := c.ListUserRoles("u1")
	if err != nil {
		t.Fatal(err)
	}
	if len(roles) != 2 {
		t.Errorf("expected 2 roles, got %d", len(roles))
	}
}

func TestCreateTargetRole(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/@warpgate/admin/api/targets/t1/roles/r1" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	if err := c.CreateTargetRole("t1", "r1"); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteTargetRole(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	if err := c.DeleteTargetRole("t1", "r1"); err != nil {
		t.Fatal(err)
	}
}

func TestListTargetRoles(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]Role{{ID: "r1", Name: "admin"}})
	}))
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	roles, err := c.ListTargetRoles("t1")
	if err != nil {
		t.Fatal(err)
	}
	if len(roles) != 1 {
		t.Errorf("expected 1 role, got %d", len(roles))
	}
}
