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
