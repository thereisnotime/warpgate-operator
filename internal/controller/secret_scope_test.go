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
	"context"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	warpgatev1alpha1 "github.com/thereisnotime/warpgate-operator/api/v1alpha1"
	"github.com/thereisnotime/warpgate-operator/internal/scope"
)

var _ = Describe("Secret selector", func() {
	const (
		authSecret = "secretscope-auth"
		connName   = "secretscope-conn"
		managed    = "warpgate.warp.tech/managed"
	)

	var (
		srv         *httptest.Server
		scoped      client.Client
		cacheCancel context.CancelFunc
		nn          = types.NamespacedName{Name: connName, Namespace: testNamespace}
	)

	BeforeEach(func() {
		srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[]`))
		}))

		// Same wiring as cmd/main.go: a cache limited by the selector, and the
		// wrapper around the client that reads from it.
		secrets, err := scope.ParseSecrets(managed + "=true")
		Expect(err).NotTo(HaveOccurred())
		c, err := cache.New(cfg, cache.Options{Scheme: scheme.Scheme, ByObject: secrets.ByObject()})
		Expect(err).NotTo(HaveOccurred())
		var cacheCtx context.Context
		cacheCtx, cacheCancel = context.WithCancel(ctx)
		go func() { _ = c.Start(cacheCtx) }()
		Expect(c.WaitForCacheSync(cacheCtx)).To(BeTrue())
		cached, err := client.New(cfg, client.Options{Scheme: scheme.Scheme, Cache: &client.CacheOptions{Reader: c}})
		Expect(err).NotTo(HaveOccurred())
		scoped = secrets.Client(cached, k8sClient)

		Expect(k8sClient.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: authSecret, Namespace: testNamespace},
			Data:       map[string][]byte{"token": []byte("t")},
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
			&warpgatev1alpha1.WarpgateRole{ObjectMeta: metav1.ObjectMeta{Name: "secretscope-role", Namespace: testNamespace}},
			&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: authSecret, Namespace: testNamespace}},
			&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "secretscope-created", Namespace: testNamespace}},
		)
		cacheCancel()
	})

	readyCondition := func(obj client.Object, conds func() []metav1.Condition) *metav1.Condition {
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(obj), obj)).To(Succeed())
		return findReadyCondition(conds())
	}

	It("reports SecretNotSelected for an unlabelled auth Secret, then connects once it is labelled", func() {
		reconciler := &WarpgateConnectionReconciler{Client: scoped, Scheme: scheme.Scheme}
		var conn warpgatev1alpha1.WarpgateConnection
		conn.Name, conn.Namespace = connName, testNamespace

		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())
		cond := readyCondition(&conn, func() []metav1.Condition { return conn.Status.Conditions })
		Expect(cond).NotTo(BeNil())
		Expect(cond.Reason).To(Equal("SecretNotSelected"))
		Expect(cond.Message).To(ContainSubstring(managed + "=true"))

		var secret corev1.Secret
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: authSecret, Namespace: testNamespace}, &secret)).To(Succeed())
		secret.Labels = map[string]string{managed: "true"}
		Expect(k8sClient.Update(ctx, &secret)).To(Succeed())

		Eventually(func(g Gomega) {
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(k8sClient.Get(ctx, nn, &conn)).To(Succeed())
			cond := findReadyCondition(conn.Status.Conditions)
			g.Expect(cond).NotTo(BeNil())
			g.Expect(cond.Reason).To(Equal("Connected"))
		}).Should(Succeed())
	})

	It("reports SecretNotSelected from resource controllers using the connection", func() {
		role := &warpgatev1alpha1.WarpgateRole{
			ObjectMeta: metav1.ObjectMeta{Name: "secretscope-role", Namespace: testNamespace},
			Spec:       warpgatev1alpha1.WarpgateRoleSpec{ConnectionRef: connName, Name: "secretscope"},
		}
		Expect(k8sClient.Create(ctx, role)).To(Succeed())

		reconciler := &WarpgateRoleReconciler{Client: scoped, Scheme: scheme.Scheme}
		_, _ = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(role)})
		cond := readyCondition(role, func() []metav1.Condition { return role.Status.Conditions })
		Expect(cond).NotTo(BeNil())
		Expect(cond.Reason).To(Equal("SecretNotSelected"))
	})

	It("labels the Secrets it creates so they stay visible", func() {
		Expect(scoped.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "secretscope-created", Namespace: testNamespace},
		})).To(Succeed())
		Eventually(func() error {
			return scoped.Get(ctx, types.NamespacedName{Name: "secretscope-created", Namespace: testNamespace}, &corev1.Secret{})
		}).Should(Succeed())
	})
})
