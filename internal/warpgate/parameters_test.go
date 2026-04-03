package warpgate

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetParameters(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/@warpgate/admin/api/parameters" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		json.NewEncoder(w).Encode(Parameters{
			AllowOwnCredentialManagement: true,
			SSHClientAuthPublicKey:       true,
			SSHClientAuthPassword:        true,
		})
	}))
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	params, err := c.GetParameters()
	if err != nil {
		t.Fatal(err)
	}
	if !params.AllowOwnCredentialManagement {
		t.Error("expected AllowOwnCredentialManagement=true")
	}
}

func TestUpdateParameters(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PUT" {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		var params Parameters
		json.NewDecoder(r.Body).Decode(&params)
		if !params.MinimizePasswordLogin {
			t.Error("expected MinimizePasswordLogin=true")
		}
		w.WriteHeader(http.StatusCreated) // API returns 201 with no body
	}))
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	err := c.UpdateParameters(Parameters{
		AllowOwnCredentialManagement: true,
		MinimizePasswordLogin:        true,
	})
	if err != nil {
		t.Fatal(err)
	}
}
