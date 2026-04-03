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

	c := NewTestClient(srv.URL)
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

	c := NewTestClient(srv.URL)
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

	c := NewTestClient(srv.URL)
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

	c := NewTestClient(srv.URL)
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

	c := NewTestClient(srv.URL)
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

	c := NewTestClient(srv.URL)
	roles, err := c.ListRoles(testRoleName)
	if err != nil {
		t.Fatal(err)
	}
	if len(roles) != 1 {
		t.Errorf("expected 1 role, got %d", len(roles))
	}
}

func TestCreateRole_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"duplicate"}`))
	}))
	defer srv.Close()

	c := NewTestClient(srv.URL)
	_, err := c.CreateRole(RoleCreateRequest{Name: "dup"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGetRole_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	}))
	defer srv.Close()

	c := NewTestClient(srv.URL)
	_, err := c.GetRole("missing")
	if err == nil {
		t.Fatal("expected error")
	}
	if !IsNotFound(err) {
		t.Errorf("expected 404, got %v", err)
	}
}

func TestUpdateRole_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal"}`))
	}))
	defer srv.Close()

	c := NewTestClient(srv.URL)
	_, err := c.UpdateRole("r1", RoleCreateRequest{Name: "x"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestListRoles_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"forbidden"}`))
	}))
	defer srv.Close()

	c := NewTestClient(srv.URL)
	_, err := c.ListRoles("")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGetRoleByName_ListError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal"}`))
	}))
	defer srv.Close()

	c := NewTestClient(srv.URL)
	_, err := c.GetRoleByName("admin")
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

func TestListRoles_NoSearch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" {
			t.Errorf("expected no query, got %s", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode([]Role{{ID: "r1", Name: "admin"}, {ID: "r2", Name: "user"}})
	}))
	defer srv.Close()

	c := NewTestClient(srv.URL)
	roles, err := c.ListRoles("")
	if err != nil {
		t.Fatal(err)
	}
	if len(roles) != 2 {
		t.Errorf("expected 2, got %d", len(roles))
	}
}
