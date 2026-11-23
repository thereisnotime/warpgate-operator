package warpgate

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// connCounter counts connections an httptest server accepts and closes.
type connCounter struct {
	opened atomic.Int64
	closed atomic.Int64
}

func (cc *connCounter) hook(_ net.Conn, state http.ConnState) {
	switch state {
	case http.StateNew:
		cc.opened.Add(1)
	case http.StateClosed, http.StateHijacked:
		cc.closed.Add(1)
	}
}

func newCountingServer(t *testing.T, h http.Handler) (*httptest.Server, *connCounter) {
	t.Helper()
	cc := &connCounter{}
	srv := httptest.NewUnstartedServer(h)
	srv.Config.ConnState = cc.hook
	srv.Start()
	t.Cleanup(srv.Close)
	return srv, cc
}

func rolesHandler(w http.ResponseWriter, _ *http.Request) {
	_ = json.NewEncoder(w).Encode([]map[string]string{})
}

func TestClientReusesConnections(t *testing.T) {
	srv, cc := newCountingServer(t, http.HandlerFunc(rolesHandler))

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	for range 200 {
		if err := c.Get("/roles", nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if got := cc.opened.Load(); got > 2 {
		t.Errorf("expected keep-alive reuse (<=2 connections), server saw %d", got)
	}
}

func TestClientCloseIdleConnections(t *testing.T) {
	srv, cc := newCountingServer(t, http.HandlerFunc(rolesHandler))

	c := NewClient(Config{Host: srv.URL, Token: "tok"})
	if err := c.Get("/roles", nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	c.CloseIdleConnections()

	deadline := time.Now().Add(2 * time.Second)
	for cc.closed.Load() < cc.opened.Load() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if opened, closed := cc.opened.Load(), cc.closed.Load(); closed != opened {
		t.Errorf("expected all %d connections closed, %d closed", opened, closed)
	}
}

func TestNewClientTransportDefaults(t *testing.T) {
	c := NewClient(Config{Host: "https://warpgate.example.com", Token: "tok"})
	if c.httpClient.Timeout != DefaultTimeout {
		t.Errorf("expected client timeout %v, got %v", DefaultTimeout, c.httpClient.Timeout)
	}
	tr, ok := c.httpClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", c.httpClient.Transport)
	}
	if tr == http.DefaultTransport {
		t.Error("client must not share http.DefaultTransport")
	}
	if tr.IdleConnTimeout == 0 {
		t.Error("expected a non-zero IdleConnTimeout")
	}
	if tr.Proxy == nil {
		t.Error("expected proxy settings from the environment to be kept")
	}

	c = NewClient(Config{Host: "https://warpgate.example.com", Token: "tok", Timeout: time.Second})
	if c.httpClient.Timeout != time.Second {
		t.Errorf("expected custom timeout, got %v", c.httpClient.Timeout)
	}
}

func TestClientTimeout(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) })

	c := NewClient(Config{Host: srv.URL, Token: "tok", Timeout: 100 * time.Millisecond})
	start := time.Now()
	err := c.Get("/roles", nil)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("request took %v, timeout not applied", elapsed)
	}
}

func TestClientWithContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(rolesHandler))
	t.Cleanup(srv.Close)

	base := NewClient(Config{Host: srv.URL, Token: "tok"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := base.WithContext(ctx).Get("/roles", nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	// The base client is unaffected.
	if err := base.Get("/roles", nil); err != nil {
		t.Fatalf("unexpected error on base client: %v", err)
	}
}

// sessionServer simulates Warpgate cookie sessions that can be expired on demand.
type sessionServer struct {
	mu      sync.Mutex
	current string
	logins  atomic.Int64
}

func (s *sessionServer) expire() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.current = ""
}

func (s *sessionServer) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/@warpgate/api/auth/login", func(w http.ResponseWriter, _ *http.Request) {
		n := s.logins.Add(1)
		s.mu.Lock()
		s.current = "session-" + string(rune('a'+n))
		val := s.current
		s.mu.Unlock()
		http.SetCookie(w, &http.Cookie{Name: "warpgate-http-session", Value: val, Path: "/"})
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/@warpgate/admin/api/roles", func(w http.ResponseWriter, r *http.Request) {
		ck, err := r.Cookie("warpgate-http-session")
		s.mu.Lock()
		ok := err == nil && s.current != "" && ck.Value == s.current
		s.mu.Unlock()
		if !ok {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		rolesHandler(w, r)
	})
	return mux
}

func TestSessionClientConcurrentLoginOnce(t *testing.T) {
	ss := &sessionServer{}
	srv := httptest.NewServer(ss.handler())
	t.Cleanup(srv.Close)

	c := NewClient(Config{Host: srv.URL, Username: "admin", Password: "pw"})
	var wg sync.WaitGroup
	errs := make(chan error, 32)
	for range 32 {
		wg.Go(func() {
			if err := c.WithContext(context.Background()).Get("/roles", nil); err != nil {
				errs <- err
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("unexpected error: %v", err)
	}
	if got := ss.logins.Load(); got != 1 {
		t.Errorf("expected exactly one login for concurrent callers, got %d", got)
	}
}

func TestSessionClientReloginAfterExpiry(t *testing.T) {
	ss := &sessionServer{}
	srv := httptest.NewServer(ss.handler())
	t.Cleanup(srv.Close)

	c := NewClient(Config{Host: srv.URL, Username: "admin", Password: "pw"})
	if err := c.Get("/roles", nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ss.expire()
	if err := c.Get("/roles", nil); err != nil {
		t.Fatalf("expected transparent re-login after session expiry, got: %v", err)
	}
	if got := ss.logins.Load(); got != 2 {
		t.Errorf("expected 2 logins, got %d", got)
	}
}

func TestTokenClientDoesNotRetryOn401(t *testing.T) {
	var calls atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)

	c := NewClient(Config{Host: srv.URL, Token: "bad"})
	err := c.Get("/roles", nil)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 APIError, got %v", err)
	}
	if got := calls.Load(); got != 1 {
		t.Errorf("expected a single request for token auth, got %d", got)
	}
}

func TestSessionClientPostBodyResentOnRetry(t *testing.T) {
	ss := &sessionServer{}
	mux := http.NewServeMux()
	mux.Handle("/@warpgate/api/auth/login", ss.handler())
	var bodies []string
	var mu sync.Mutex
	mux.HandleFunc("/@warpgate/admin/api/roles", func(w http.ResponseWriter, r *http.Request) {
		var in map[string]string
		_ = json.NewDecoder(r.Body).Decode(&in)
		mu.Lock()
		bodies = append(bodies, in["name"])
		mu.Unlock()
		ck, err := r.Cookie("warpgate-http-session")
		ss.mu.Lock()
		ok := err == nil && ss.current != "" && ck.Value == ss.current
		ss.mu.Unlock()
		if !ok {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "1", "name": in["name"]})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := NewClient(Config{Host: srv.URL, Username: "admin", Password: "pw"})
	if _, err := c.login(); err != nil {
		t.Fatalf("login: %v", err)
	}
	ss.expire()

	var out map[string]string
	if err := c.Post("/roles", map[string]string{"name": "ops"}, &out); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out["name"] != "ops" {
		t.Errorf("unexpected response: %v", out)
	}
	if len(bodies) != 2 || bodies[0] != "ops" || bodies[1] != "ops" {
		t.Errorf("expected the body to be sent on both attempts, got %v", bodies)
	}
}
