package warpgate

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateSSHTarget(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req TargetRequest
		_ = json.NewDecoder(r.Body).Decode(&req)

		kind, _ := ParseOptionsKind(req.Options)
		if kind != "Ssh" {
			t.Errorf("expected Ssh kind, got %s", kind)
		}

		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(Target{ID: "t1", Name: req.Name, Options: req.Options})
	}))
	defer srv.Close()

	opts, _ := MarshalOptions(SSHOptions{
		Kind:     "Ssh",
		Host:     "10.0.0.1",
		Port:     22,
		Username: "root",
		Auth:     SSHAuth{Kind: "Password", Password: "secret"},
	})

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	target, err := c.CreateTarget(TargetRequest{Name: "ssh-server", Options: opts})
	if err != nil {
		t.Fatal(err)
	}
	if target.ID != "t1" {
		t.Errorf("unexpected target: %+v", target)
	}
}

func TestCreateHTTPTarget(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req TargetRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		kind, _ := ParseOptionsKind(req.Options)
		if kind != "Http" {
			t.Errorf("expected Http kind, got %s", kind)
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(Target{ID: "t2", Name: req.Name, Options: req.Options})
	}))
	defer srv.Close()

	opts, _ := MarshalOptions(HTTPOptions{
		Kind:    "Http",
		URL:     "https://internal.example.com",
		TLS:     &TLSConfig{Mode: "Required", Verify: true},
		Headers: map[string]string{"X-Custom": "value"},
	})

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	target, err := c.CreateTarget(TargetRequest{Name: "http-app", Options: opts})
	if err != nil {
		t.Fatal(err)
	}
	if target.ID != "t2" {
		t.Errorf("unexpected target: %+v", target)
	}
}

func TestCreateMySQLTarget(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req TargetRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		kind, _ := ParseOptionsKind(req.Options)
		if kind != "MySql" {
			t.Errorf("expected MySql kind, got %s", kind)
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(Target{ID: "t3", Name: "mysql-db", Options: req.Options})
	}))
	defer srv.Close()

	opts, _ := MarshalOptions(MySQLOptions{
		Kind:     "MySql",
		Host:     "db.example.com",
		Port:     3306,
		Username: "app",
		Password: "dbpass",
	})

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	target, err := c.CreateTarget(TargetRequest{Name: "mysql-db", Options: opts})
	if err != nil {
		t.Fatal(err)
	}
	if target.ID != "t3" {
		t.Errorf("unexpected: %+v", target)
	}
}

func TestCreatePostgresTarget(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req TargetRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		kind, _ := ParseOptionsKind(req.Options)
		if kind != "Postgres" {
			t.Errorf("expected Postgres kind, got %s", kind)
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(Target{ID: "t4", Name: "pg-db", Options: req.Options})
	}))
	defer srv.Close()

	opts, _ := MarshalOptions(PostgresOptions{
		Kind:     "Postgres",
		Host:     "pg.example.com",
		Port:     5432,
		Username: "admin",
		TLS:      &TLSConfig{Mode: "Preferred", Verify: false},
	})

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	target, err := c.CreateTarget(TargetRequest{Name: "pg-db", Options: opts})
	if err != nil {
		t.Fatal(err)
	}
	if target.ID != "t4" {
		t.Errorf("unexpected: %+v", target)
	}
}

func TestCreateKubernetesTarget(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req TargetRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		kind, _ := ParseOptionsKind(req.Options)
		if kind != "Kubernetes" {
			t.Errorf("expected Kubernetes kind, got %s", kind)
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(Target{ID: "t5", Name: "k8s", Options: req.Options})
	}))
	defer srv.Close()

	opts, _ := MarshalOptions(KubernetesOptions{
		Kind:       "Kubernetes",
		ClusterURL: "https://k8s.example.com",
		Auth:       KubernetesAuth{Kind: "Token", Token: "k8s-token"},
	})

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	target, err := c.CreateTarget(TargetRequest{Name: "k8s", Options: opts})
	if err != nil {
		t.Fatal(err)
	}
	if target.ID != "t5" {
		t.Errorf("unexpected: %+v", target)
	}
}

func TestGetTarget(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		opts, _ := MarshalOptions(SSHOptions{Kind: "Ssh", Host: "h", Port: 22, Username: "u", Auth: SSHAuth{Kind: "PublicKey"}})
		_ = json.NewEncoder(w).Encode(Target{ID: "t1", Name: "test", Options: opts, AllowRoles: []string{"r1"}})
	}))
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	target, err := c.GetTarget("t1")
	if err != nil {
		t.Fatal(err)
	}
	if len(target.AllowRoles) != 1 {
		t.Errorf("expected 1 allow_role, got %d", len(target.AllowRoles))
	}
}

func TestListTargets(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]Target{{ID: "t1", Name: "srv1"}, {ID: "t2", Name: "srv2"}})
	}))
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	targets, err := c.ListTargets("")
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 2 {
		t.Errorf("expected 2, got %d", len(targets))
	}
}

func TestDeleteTarget(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	if err := c.DeleteTarget("t1"); err != nil {
		t.Fatal(err)
	}
}

func TestGetTargetByName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		opts, _ := MarshalOptions(SSHOptions{Kind: "Ssh", Host: "h", Port: 22, Username: "u", Auth: SSHAuth{Kind: "PublicKey"}})
		_ = json.NewEncoder(w).Encode([]Target{
			{ID: "t1", Name: "alpha", Options: opts},
			{ID: "t2", Name: "beta", Options: opts},
		})
	}))
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	target, err := c.GetTargetByName("beta")
	if err != nil {
		t.Fatal(err)
	}
	if target.ID != "t2" {
		t.Errorf("expected t2, got %s", target.ID)
	}
}

func TestGetTargetByName_ListError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal"}`))
	}))
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	_, err := c.GetTargetByName("myhost")
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

func TestGetTargetByName_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]Target{})
	}))
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	_, err := c.GetTargetByName("missing")
	if err == nil {
		t.Fatal("expected error")
	}
	if !IsNotFound(err) {
		t.Errorf("expected 404 error, got %v", err)
	}
}

func TestUpdateTarget(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/@warpgate/admin/api/targets/t1" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		var req TargetRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		_ = json.NewEncoder(w).Encode(Target{ID: "t1", Name: req.Name, Options: req.Options})
	}))
	defer srv.Close()

	opts, _ := MarshalOptions(SSHOptions{Kind: "Ssh", Host: "10.0.0.1", Port: 22, Username: "root", Auth: SSHAuth{Kind: "Password"}})
	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	target, err := c.UpdateTarget("t1", TargetRequest{Name: "updated-target", Options: opts})
	if err != nil {
		t.Fatal(err)
	}
	if target.Name != "updated-target" {
		t.Errorf("expected updated-target, got %s", target.Name)
	}
}

func TestUpdateTarget_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	}))
	defer srv.Close()

	opts, _ := MarshalOptions(SSHOptions{Kind: "Ssh", Host: "h", Port: 22, Username: "u", Auth: SSHAuth{Kind: "PublicKey"}})
	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	_, err := c.UpdateTarget("gone", TargetRequest{Name: "x", Options: opts})
	if err == nil {
		t.Fatal("expected error")
	}
	if !IsNotFound(err) {
		t.Errorf("expected 404, got %v", err)
	}
}

func TestListTargets_WithSearch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "search=myhost" {
			t.Errorf("expected search=myhost, got %s", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode([]Target{{ID: "t1", Name: "myhost"}})
	}))
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	targets, err := c.ListTargets("myhost")
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 {
		t.Errorf("expected 1, got %d", len(targets))
	}
}

func TestCreateTarget_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad request"}`))
	}))
	defer srv.Close()

	opts, _ := MarshalOptions(SSHOptions{Kind: "Ssh"})
	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	_, err := c.CreateTarget(TargetRequest{Name: "x", Options: opts})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGetTarget_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	}))
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	_, err := c.GetTarget("missing")
	if err == nil {
		t.Fatal("expected error")
	}
	if !IsNotFound(err) {
		t.Errorf("expected 404, got %v", err)
	}
}

func TestListTargets_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal"}`))
	}))
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	_, err := c.ListTargets("")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMarshalOptions_And_ParseOptionsKind(t *testing.T) {
	// Error path for ParseOptionsKind with invalid JSON.
	_, err := ParseOptionsKind(json.RawMessage(`{invalid`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestMarshalOptions_Error(t *testing.T) {
	// MarshalOptions with a channel (cannot be marshaled).
	_, err := MarshalOptions(make(chan int))
	if err == nil {
		t.Fatal("expected error for unmarshalable type")
	}
}
