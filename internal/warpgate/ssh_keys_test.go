package warpgate

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetSSHOwnKeys(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/@warpgate/admin/api/ssh/own-keys" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode([]SSHKey{
			{Kind: "Ed25519", PublicKeyBase64: "AAAAC3NzaC1l..."},
			{Kind: "RSA", PublicKeyBase64: "AAAAB3NzaC1y..."},
		})
	}))
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	keys, err := c.GetSSHOwnKeys()
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 2 {
		t.Errorf("expected 2 keys, got %d", len(keys))
	}
	if keys[0].Kind != "Ed25519" {
		t.Errorf("expected Ed25519, got %s", keys[0].Kind)
	}
}
