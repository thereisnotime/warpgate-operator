package warpgate

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

const (
	testRoleName   = "admin"
	testRoleR1Path = "/@warpgate/admin/api/role/r1"
)

func TestCreateRole(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/@warpgate/admin/api/roles" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		var req RoleCreateRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Name != testRoleName {
			t.Errorf("expected name=%s, got %s", testRoleName, req.Name)
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(Role{ID: "r1", Name: testRoleName, Description: "Admin role"})
	}))
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	role, err := c.CreateRole(RoleCreateRequest{Name: testRoleName, Description: "Admin role"})
	if err != nil {
		t.Fatal(err)
	}
	if role.ID != "r1" || role.Name != testRoleName {
		t.Errorf("unexpected role: %+v", role)
	}
}

func TestGetRole(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != testRoleR1Path {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(Role{ID: "r1", Name: testRoleName})
	}))
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	role, err := c.GetRole("r1")
	if err != nil {
		t.Fatal(err)
	}
	if role.ID != "r1" {
		t.Errorf("unexpected role: %+v", role)
	}
}

func TestGetRoleByName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]Role{
			{ID: "r1", Name: testRoleName},
			{ID: "r2", Name: "user"},
		})
	}))
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	role, err := c.GetRoleByName("user")
	if err != nil {
		t.Fatal(err)
	}
	if role.ID != "r2" {
		t.Errorf("expected r2, got %s", role.ID)
	}

	_, err = c.GetRoleByName("nonexistent")
	if !IsNotFound(err) {
		t.Errorf("expected not found, got %v", err)
	}
}

func TestUpdateRole(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != testRoleR1Path {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(Role{ID: "r1", Name: "updated"})
	}))
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	role, err := c.UpdateRole("r1", RoleCreateRequest{Name: "updated"})
	if err != nil {
		t.Fatal(err)
	}
	if role.Name != "updated" {
		t.Errorf("expected updated, got %s", role.Name)
	}
}

func TestDeleteRole(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != testRoleR1Path {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	if err := c.DeleteRole("r1"); err != nil {
		t.Fatal(err)
	}
}

func TestListRoles(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("search") != testRoleName {
			t.Errorf("expected search=%s, got %s", testRoleName, r.URL.Query().Get("search"))
		}
		_ = json.NewEncoder(w).Encode([]Role{{ID: "r1", Name: testRoleName}})
	}))
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	roles, err := c.ListRoles(testRoleName)
	if err != nil {
		t.Fatal(err)
	}
	if len(roles) != 1 {
		t.Errorf("expected 1 role, got %d", len(roles))
	}
}
