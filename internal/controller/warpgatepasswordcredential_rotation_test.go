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
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	warpgatev1alpha1 "github.com/thereisnotime/warpgate-operator/api/v1alpha1"
)

// rotationMock is a fake Warpgate that records password credential calls.
type rotationMock struct {
	mu        sync.Mutex
	next      int
	created   []string // passwords sent on create, in order
	deleted   []string // credential IDs deleted, in order
	deleteErr bool
}

func (m *rotationMock) snapshot() (created, deleted []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.created...), append([]string(nil), m.deleted...)
}

func (m *rotationMock) server(userID, username string) *httptest.Server {
	mux := http.NewServeMux()
	mockLogin(mux)
	mux.HandleFunc("/@warpgate/admin/api/users", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]any{{"id": userID, "username": username}})
	})
	base := "/@warpgate/admin/api/users/" + userID + "/credentials/passwords"
	mux.HandleFunc(base, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		m.mu.Lock()
		m.next++
		id := fmt.Sprintf("cred-rot-%d", m.next)
		m.created = append(m.created, body["password"])
		m.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": id, "password": "hashed"})
	})
	mux.HandleFunc(base+"/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.NotFound(w, r)
			return
		}
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.deleteErr {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		m.deleted = append(m.deleted, strings.TrimPrefix(r.URL.Path, base+"/"))
		w.WriteHeader(http.StatusNoContent)
	})
	return httptest.NewServer(mux)
}

var _ = Describe("WarpgatePasswordCredential Secret rotation", func() {
	const (
		tokenSecret = "pwrot-token"
		pwSecret    = "pwrot-password"
		connName    = "pwrot-conn"
		crName      = "pwrot-cred"
		otherCrName = "pwrot-other-cred"
	)

	var (
		mock       *rotationMock
		mockServer *httptest.Server
		reconciler *WarpgatePasswordCredentialReconciler
		nn         types.NamespacedName
	)

	BeforeEach(func() {
		mock = &rotationMock{}
		mockServer = mock.server("user-rot", "rotuser")
		reconciler = &WarpgatePasswordCredentialReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
		nn = types.NamespacedName{Name: crName, Namespace: testNamespace}

		Expect(k8sClient.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: tokenSecret, Namespace: testNamespace},
			Data:       map[string][]byte{"token": []byte("t")},
		})).To(Succeed())
		Expect(k8sClient.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: pwSecret, Namespace: testNamespace},
			Data:       map[string][]byte{"password": []byte("first-password")},
		})).To(Succeed())
		Expect(k8sClient.Create(ctx, &warpgatev1alpha1.WarpgateConnection{
			ObjectMeta: metav1.ObjectMeta{Name: connName, Namespace: testNamespace},
			Spec: warpgatev1alpha1.WarpgateConnectionSpec{
				Host:          mockServer.URL,
				AuthSecretRef: warpgatev1alpha1.AuthSecretRef{Name: tokenSecret},
			},
		})).To(Succeed())
		Expect(k8sClient.Create(ctx, &warpgatev1alpha1.WarpgatePasswordCredential{
			ObjectMeta: metav1.ObjectMeta{Name: crName, Namespace: testNamespace},
			Spec: warpgatev1alpha1.WarpgatePasswordCredentialSpec{
				ConnectionRef:     connName,
				Username:          "rotuser",
				PasswordSecretRef: warpgatev1alpha1.SecretKeyRef{Name: pwSecret},
			},
		})).To(Succeed())
	})

	AfterEach(func() {
		mockServer.Close()
		for _, name := range []string{crName, otherCrName} {
			cr := &warpgatev1alpha1.WarpgatePasswordCredential{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: testNamespace}, cr); err == nil {
				controllerutil.RemoveFinalizer(cr, passwordCredentialFinalizer)
				_ = k8sClient.Update(ctx, cr)
				_ = k8sClient.Delete(ctx, cr)
			}
		}
		_ = k8sClient.Delete(ctx, &warpgatev1alpha1.WarpgateConnection{
			ObjectMeta: metav1.ObjectMeta{Name: connName, Namespace: testNamespace},
		})
		for _, name := range []string{tokenSecret, pwSecret} {
			_ = k8sClient.Delete(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNamespace}})
		}
	})

	rotateSecret := func(password string) {
		var s corev1.Secret
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: pwSecret, Namespace: testNamespace}, &s)).To(Succeed())
		s.Data["password"] = []byte(password)
		Expect(k8sClient.Update(ctx, &s)).To(Succeed())
	}

	It("records the applied Secret version and does nothing while it is unchanged", func() {
		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())
		_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())

		created, deleted := mock.snapshot()
		Expect(created).To(Equal([]string{"first-password"}))
		Expect(deleted).To(BeEmpty())

		var cr warpgatev1alpha1.WarpgatePasswordCredential
		Expect(k8sClient.Get(ctx, nn, &cr)).To(Succeed())
		Expect(cr.Status.CredentialID).To(Equal("cred-rot-1"))
		Expect(cr.Status.AppliedSecretVersion).To(HavePrefix(pwSecret + "/password@"))
	})

	It("replaces the Warpgate credential when the Secret changes", func() {
		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())

		rotateSecret("second-password")
		_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())

		created, deleted := mock.snapshot()
		Expect(created).To(Equal([]string{"first-password", "second-password"}))
		Expect(deleted).To(Equal([]string{"cred-rot-1"}))

		var cr warpgatev1alpha1.WarpgatePasswordCredential
		Expect(k8sClient.Get(ctx, nn, &cr)).To(Succeed())
		Expect(cr.Status.CredentialID).To(Equal("cred-rot-2"))
		readyCond := findReadyCondition(cr.Status.Conditions)
		Expect(readyCond).NotTo(BeNil())
		Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
	})

	It("re-applies once for credentials created before the version was tracked", func() {
		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())

		var cr warpgatev1alpha1.WarpgatePasswordCredential
		Expect(k8sClient.Get(ctx, nn, &cr)).To(Succeed())
		cr.Status.AppliedSecretVersion = ""
		Expect(k8sClient.Status().Update(ctx, &cr)).To(Succeed())

		_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())
		created, deleted := mock.snapshot()
		Expect(created).To(HaveLen(2))
		Expect(deleted).To(Equal([]string{"cred-rot-1"}))
	})

	It("keeps the old credential ID and reports RotateFailed when the delete fails", func() {
		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())

		mock.mu.Lock()
		mock.deleteErr = true
		mock.mu.Unlock()
		rotateSecret("second-password")

		_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).To(HaveOccurred())

		created, _ := mock.snapshot()
		Expect(created).To(Equal([]string{"first-password"}))

		var cr warpgatev1alpha1.WarpgatePasswordCredential
		Expect(k8sClient.Get(ctx, nn, &cr)).To(Succeed())
		Expect(cr.Status.CredentialID).To(Equal("cred-rot-1"))
		readyCond := findReadyCondition(cr.Status.Conditions)
		Expect(readyCond).NotTo(BeNil())
		Expect(readyCond.Reason).To(Equal("RotateFailed"))
	})

	It("maps a Secret to the credentials that reference it", func() {
		Expect(k8sClient.Create(ctx, &warpgatev1alpha1.WarpgatePasswordCredential{
			ObjectMeta: metav1.ObjectMeta{Name: otherCrName, Namespace: testNamespace},
			Spec: warpgatev1alpha1.WarpgatePasswordCredentialSpec{
				ConnectionRef:     connName,
				Username:          "rotuser",
				PasswordSecretRef: warpgatev1alpha1.SecretKeyRef{Name: "some-other-secret"},
			},
		})).To(Succeed())

		reqs := reconciler.credentialsForSecret(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: pwSecret, Namespace: testNamespace},
		})
		Expect(reqs).To(Equal([]reconcile.Request{{NamespacedName: nn}}))

		Expect(reconciler.credentialsForSecret(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: pwSecret, Namespace: "kube-system"},
		})).To(BeEmpty())
	})

	It("pushes a rotated Secret to Warpgate through the Secret watch", func() {
		skip := true
		mgr, err := ctrl.NewManager(cfg, ctrl.Options{
			Scheme:     k8sClient.Scheme(),
			Metrics:    metricsserver.Options{BindAddress: "0"},
			Controller: config.Controller{SkipNameValidation: &skip},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect((&WarpgatePasswordCredentialReconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		}).SetupWithManager(mgr)).To(Succeed())

		mgrCtx, stop := context.WithCancel(ctx)
		done := make(chan struct{})
		go func() {
			defer GinkgoRecover()
			defer close(done)
			Expect(mgr.Start(mgrCtx)).To(Succeed())
		}()
		DeferCleanup(func() {
			stop()
			<-done
		})

		// Wait for the initial create to land in status.
		Eventually(func() string {
			var cr warpgatev1alpha1.WarpgatePasswordCredential
			if err := k8sClient.Get(ctx, nn, &cr); err != nil {
				return ""
			}
			return cr.Status.AppliedSecretVersion
		}, 20*time.Second, 200*time.Millisecond).ShouldNot(BeEmpty())
		created, _ := mock.snapshot()
		Expect(created).To(ContainElement("first-password"))
		Expect(created).NotTo(ContainElement("second-password"))

		// ReconcileInterval is zero, so nothing but the Secret watch can
		// trigger the next reconcile.
		rotateSecret("second-password")

		Eventually(func() []string {
			created, _ := mock.snapshot()
			return created
		}, 20*time.Second, 200*time.Millisecond).Should(ContainElement("second-password"))
		Eventually(func() []string {
			_, deleted := mock.snapshot()
			return deleted
		}, 5*time.Second, 200*time.Millisecond).ShouldNot(BeEmpty())
	})
})
