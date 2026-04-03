package warpgate

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateTargetGroup(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req TargetGroupRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Color != "Primary" {
			t.Errorf("expected color Primary, got %s", req.Color)
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(TargetGroup{ID: "tg1", Name: req.Name, Color: req.Color})
	}))
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	tg, err := c.CreateTargetGroup(TargetGroupRequest{Name: "production", Color: "Primary"})
	if err != nil {
		t.Fatal(err)
	}
	if tg.ID != "tg1" || tg.Color != "Primary" {
		t.Errorf("unexpected: %+v", tg)
	}
}

func TestGetTargetGroup(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(TargetGroup{ID: "tg1", Name: "prod"})
	}))
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	tg, err := c.GetTargetGroup("tg1")
	if err != nil {
		t.Fatal(err)
	}
	if tg.Name != "prod" {
		t.Errorf("unexpected: %+v", tg)
	}
}

func TestUpdateTargetGroup(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(TargetGroup{ID: "tg1", Name: "updated", Color: "Danger"})
	}))
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	tg, err := c.UpdateTargetGroup("tg1", TargetGroupRequest{Name: "updated", Color: "Danger"})
	if err != nil {
		t.Fatal(err)
	}
	if tg.Color != "Danger" {
		t.Errorf("expected Danger, got %s", tg.Color)
	}
}

func TestDeleteTargetGroup(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	if err := c.DeleteTargetGroup("tg1"); err != nil {
		t.Fatal(err)
	}
}
