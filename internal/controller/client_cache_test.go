/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	warpgatev1alpha1 "github.com/thereisnotime/warpgate-operator/api/v1alpha1"
	"github.com/thereisnotime/warpgate-operator/internal/warpgate"
)

// countingServer is a fake Warpgate API that counts the TCP connections it
// accepts and closes, and records the API token of the last request.
type countingServer struct {
	*httptest.Server
	opened    atomic.Int64
	closed    atomic.Int64
	mu        sync.Mutex
	lastToken string
}

func newCountingServer() *countingServer {
	cs := &countingServer{}
	cs.Server = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cs.mu.Lock()
		cs.lastToken = r.Header.Get("X-Warpgate-Token")
		cs.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	cs.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		switch state {
		case http.StateNew:
			cs.opened.Add(1)
		case http.StateClosed, http.StateHijacked:
			cs.closed.Add(1)
		}
	}
	cs.Start()
	return cs
}

func (cs *countingServer) token() string {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.lastToken
}

func TestClientCacheReusesAndReplaces(t *testing.T) {
	cc := newClientCache()
	key := types.NamespacedName{Namespace: "ns", Name: "conn"}
	cfg := warpgate.Config{Host: "https://wg.example.com", Token: "a"}

	first := cc.get(key, cfg)
	if again := cc.get(key, cfg); again != first {
		t.Fatal("expected the same client for an unchanged config")
	}

	for name, changed := range map[string]warpgate.Config{
		"token":    {Host: cfg.Host, Token: "b"},
		"host":     {Host: "https://other.example.com", Token: "a"},
		"insecure": {Host: cfg.Host, Token: "a", InsecureSkipVerify: true},
		"ca":       {Host: cfg.Host, Token: "a", CACert: []byte("pem")},
		"password": {Host: cfg.Host, Username: "u", Password: "p2"},
	} {
		cc.get(key, cfg)
		if got := cc.get(key, changed); got == first {
			t.Errorf("%s change: expected a new client", name)
		}
		first = cc.get(key, cfg)
	}

	other := types.NamespacedName{Namespace: "ns2", Name: "conn"}
	if cc.get(other, cfg) == cc.get(key, cfg) {
		t.Error("expected separate clients per connection")
	}

	cc.evict(key)
	if cc.has(key) {
		t.Error("expected entry to be evicted")
	}
	cc.evict(key) // no-op
}

var _ = Describe("Warpgate client cache", func() {
	const (
		authSecret = "clientcache-auth"
		connName   = "clientcache-conn"
	)

	var (
		srv *countingServer
		nn  types.NamespacedName
	)

	BeforeEach(func() {
		srv = newCountingServer()
		nn = types.NamespacedName{Name: connName, Namespace: testNamespace}
		Expect(k8sClient.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: authSecret, Namespace: testNamespace},
			Data:       map[string][]byte{"token": []byte("token-1")},
		})).To(Succeed())
		Expect(k8sClient.Create(ctx, &warpgatev1alpha1.WarpgateConnection{
			ObjectMeta: metav1.ObjectMeta{Name: connName, Namespace: testNamespace},
			Spec: warpgatev1alpha1.WarpgateConnectionSpec{
				Host:          srv.URL,
				AuthSecretRef: warpgatev1alpha1.AuthSecretRef{Name: authSecret},
			},
		})).To(Succeed())
	})

	AfterEach(func() {
		srv.Close()
		deleteIgnoringFinalizers(
			&warpgatev1alpha1.WarpgateConnection{ObjectMeta: metav1.ObjectMeta{Name: connName, Namespace: testNamespace}},
			&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: authSecret, Namespace: testNamespace}},
		)
		warpgateClients.evict(nn)
	})

	It("keeps the number of connections bounded across many reconciles", func() {
		reconciler := &WarpgateConnectionReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
		for range 50 {
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
		}
		for range 200 {
			wg, err := getWarpgateClient(ctx, k8sClient, testNamespace, connName)
			Expect(err).NotTo(HaveOccurred())
			_, err = wg.ListRoles("")
			Expect(err).NotTo(HaveOccurred())
		}
		// Before the cache every call opened (and leaked) a new keep-alive connection.
		Expect(srv.opened.Load()).To(BeNumerically("<=", 2))
	})

	It("builds a new client when the auth secret changes and closes the old one's connections", func() {
		wg, err := getWarpgateClient(ctx, k8sClient, testNamespace, connName)
		Expect(err).NotTo(HaveOccurred())
		_, err = wg.ListRoles("")
		Expect(err).NotTo(HaveOccurred())
		Expect(srv.token()).To(Equal("token-1"))
		Expect(srv.opened.Load()).To(BeEquivalentTo(1))

		var secret corev1.Secret
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: authSecret, Namespace: testNamespace}, &secret)).To(Succeed())
		secret.Data["token"] = []byte("token-2")
		Expect(k8sClient.Update(ctx, &secret)).To(Succeed())

		wg, err = getWarpgateClient(ctx, k8sClient, testNamespace, connName)
		Expect(err).NotTo(HaveOccurred())
		_, err = wg.ListRoles("")
		Expect(err).NotTo(HaveOccurred())
		Expect(srv.token()).To(Equal("token-2"))
		Expect(srv.opened.Load()).To(BeEquivalentTo(2))
		// The replaced client's idle connection is closed, not left behind.
		Eventually(srv.closed.Load, 2*time.Second, 10*time.Millisecond).Should(BeEquivalentTo(1))
	})

	It("builds a new client when the connection host changes", func() {
		wg, err := getWarpgateClient(ctx, k8sClient, testNamespace, connName)
		Expect(err).NotTo(HaveOccurred())
		_, err = wg.ListRoles("")
		Expect(err).NotTo(HaveOccurred())

		srv2 := newCountingServer()
		defer srv2.Close()

		var conn warpgatev1alpha1.WarpgateConnection
		Expect(k8sClient.Get(ctx, nn, &conn)).To(Succeed())
		conn.Spec.Host = srv2.URL
		Expect(k8sClient.Update(ctx, &conn)).To(Succeed())

		wg, err = getWarpgateClient(ctx, k8sClient, testNamespace, connName)
		Expect(err).NotTo(HaveOccurred())
		_, err = wg.ListRoles("")
		Expect(err).NotTo(HaveOccurred())
		Expect(srv2.opened.Load()).To(BeEquivalentTo(1))
		Eventually(srv.closed.Load, 2*time.Second, 10*time.Millisecond).Should(BeEquivalentTo(1))
	})

	It("evicts the client when the connection is deleted", func() {
		reconciler := &WarpgateConnectionReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())
		Expect(warpgateClients.has(nn)).To(BeTrue())

		deleteIgnoringFinalizers(&warpgatev1alpha1.WarpgateConnection{ObjectMeta: metav1.ObjectMeta{Name: connName, Namespace: testNamespace}})
		Eventually(func() bool {
			var conn warpgatev1alpha1.WarpgateConnection
			return k8sClient.Get(ctx, nn, &conn) != nil
		}, 5*time.Second, 50*time.Millisecond).Should(BeTrue())

		_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())
		Expect(warpgateClients.has(nn)).To(BeFalse())
		Eventually(srv.closed.Load, 2*time.Second, 10*time.Millisecond).Should(Equal(srv.opened.Load()))
	})
})
