package warpgate

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateTicket(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/@warpgate/admin/api/tickets" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		var req TicketCreateRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Username != "alice" || req.TargetName != "ssh-server" {
			t.Errorf("unexpected request: %+v", req)
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(TicketAndSecret{
			Ticket: Ticket{ID: "tk1", Username: "alice", Target: "ssh-server"},
			Secret: "supersecret123",
		})
	}))
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	uses := 5
	result, err := c.CreateTicket(TicketCreateRequest{
		Username:     "alice",
		TargetName:   "ssh-server",
		NumberOfUses: &uses,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Secret != "supersecret123" {
		t.Errorf("expected secret, got %s", result.Secret)
	}
	if result.Ticket.ID != "tk1" {
		t.Errorf("unexpected ticket: %+v", result.Ticket)
	}
}

func TestDeleteTicket(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/@warpgate/admin/api/tickets/tk1" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	if err := c.DeleteTicket("tk1"); err != nil {
		t.Fatal(err)
	}
}

func TestCreateTicket_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad request"}`))
	}))
	defer srv.Close()

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	_, err := c.CreateTicket(TicketCreateRequest{Username: "x", TargetName: "y"})
	if err == nil {
		t.Fatal("expected error")
	}
}
