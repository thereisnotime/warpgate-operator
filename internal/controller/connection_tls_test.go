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
	"encoding/pem"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	warpgatev1alpha1 "github.com/thereisnotime/warpgate-operator/api/v1alpha1"
)

// deleteIgnoringFinalizers removes test objects that may carry a finalizer.
func deleteIgnoringFinalizers(objs ...client.Object) {
	for _, obj := range objs {
		if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(obj), obj); err != nil {
			continue
		}
		obj.SetFinalizers(nil)
		_ = k8sClient.Update(ctx, obj)
		_ = k8sClient.Delete(ctx, obj)
	}
}

var _ = Describe("Connection TLS verification", func() {
	boolPtr := func(b bool) *bool { return &b }

	Context("WarpgateConnection with caSecretRef", func() {
		const (
			authSecret = "conntls-auth"
			caSecret   = "conntls-ca"
			connName   = "conntls-conn"
		)

		var (
			srv        *httptest.Server
			reconciler *WarpgateConnectionReconciler
		)

		BeforeEach(func() {
			srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`[]`))
			}))
			reconciler = &WarpgateConnectionReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
			Expect(k8sClient.Create(ctx, &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: authSecret, Namespace: testNamespace},
				Data:       map[string][]byte{"token": []byte("t")},
			})).To(Succeed())
		})

		AfterEach(func() {
			srv.Close()
			deleteIgnoringFinalizers(
				&warpgatev1alpha1.WarpgateConnection{ObjectMeta: metav1.ObjectMeta{Name: connName, Namespace: testNamespace}},
				&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: authSecret, Namespace: testNamespace}},
				&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: caSecret, Namespace: testNamespace}},
			)
		})

		createConn := func(caData []byte) types.NamespacedName {
			if caData != nil {
				Expect(k8sClient.Create(ctx, &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: caSecret, Namespace: testNamespace},
					Data:       map[string][]byte{"ca.crt": caData},
				})).To(Succeed())
			}
			Expect(k8sClient.Create(ctx, &warpgatev1alpha1.WarpgateConnection{
				ObjectMeta: metav1.ObjectMeta{Name: connName, Namespace: testNamespace},
				Spec: warpgatev1alpha1.WarpgateConnectionSpec{
					Host:          srv.URL,
					AuthSecretRef: warpgatev1alpha1.AuthSecretRef{Name: authSecret},
					CASecretRef:   &warpgatev1alpha1.SecretKeyRef{Name: caSecret},
				},
			})).To(Succeed())
			return types.NamespacedName{Name: connName, Namespace: testNamespace}
		}

		readyReason := func(nn types.NamespacedName) string {
			var conn warpgatev1alpha1.WarpgateConnection
			Expect(k8sClient.Get(ctx, nn, &conn)).To(Succeed())
			cond := findReadyCondition(conn.Status.Conditions)
			Expect(cond).NotTo(BeNil())
			return cond.Reason
		}

		It("connects when the CA signs the server certificate", func() {
			caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
			nn := createConn(caPEM)

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(readyReason(nn)).To(Equal("Connected"))

			_, err = getWarpgateClient(ctx, k8sClient, testNamespace, connName)
			Expect(err).NotTo(HaveOccurred())
		})

		It("fails when the CA secret is missing", func() {
			nn := createConn(nil)

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(readyReason(nn)).To(Equal("ConnectionFailed"))

			_, err = getWarpgateClient(ctx, k8sClient, testNamespace, connName)
			Expect(err).To(MatchError(ContainSubstring("CA secret")))
		})

		It("fails when the CA secret holds no PEM certificate", func() {
			nn := createConn([]byte("not a certificate"))

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(readyReason(nn)).To(Equal("ConnectionFailed"))
		})
	})

	Context("auto-created connection from a WarpgateInstance", func() {
		const (
			instName   = "conntls-inst"
			pwSecret   = "conntls-inst-pw"
			tlsSecret  = "conntls-inst-tls"
			connName   = instName + "-connection"
			authSecret = instName + "-admin-auth"
		)

		var reconciler *WarpgateInstanceReconciler

		BeforeEach(func() {
			reconciler = &WarpgateInstanceReconciler{Client: k8sClient, Scheme: scheme.Scheme}
			Expect(k8sClient.Create(ctx, &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: pwSecret, Namespace: testNamespace},
				Data:       map[string][]byte{"password": []byte("pw")},
			})).To(Succeed())
		})

		AfterEach(func() {
			deleteIgnoringFinalizers(
				&warpgatev1alpha1.WarpgateConnection{ObjectMeta: metav1.ObjectMeta{Name: connName, Namespace: testNamespace}},
				&warpgatev1alpha1.WarpgateInstance{ObjectMeta: metav1.ObjectMeta{Name: instName, Namespace: testNamespace}},
				&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: pwSecret, Namespace: testNamespace}},
				&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: tlsSecret, Namespace: testNamespace}},
				&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: authSecret, Namespace: testNamespace}},
			)
		})

		newInstance := func(tls *warpgatev1alpha1.InstanceTLSSpec) *warpgatev1alpha1.WarpgateInstance {
			return &warpgatev1alpha1.WarpgateInstance{
				ObjectMeta: metav1.ObjectMeta{Name: instName, Namespace: testNamespace},
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					Version:                "0.21.1",
					AdminPasswordSecretRef: warpgatev1alpha1.SecretKeyRef{Name: pwSecret},
					TLS:                    tls,
				},
			}
		}

		createTLSSecret := func(data map[string][]byte) {
			Expect(k8sClient.Create(ctx, &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: tlsSecret, Namespace: testNamespace},
				Data:       data,
			})).To(Succeed())
		}

		It("verifies against ca.crt from tls.secretName", func() {
			createTLSSecret(map[string][]byte{"ca.crt": []byte("ca"), "tls.crt": []byte("crt"), "tls.key": []byte("key")})
			inst := newInstance(&warpgatev1alpha1.InstanceTLSSpec{SecretName: tlsSecret, CertManager: boolPtr(false)})
			Expect(k8sClient.Create(ctx, inst)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(inst)})
			Expect(err).NotTo(HaveOccurred())

			var conn warpgatev1alpha1.WarpgateConnection
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: connName, Namespace: testNamespace}, &conn)).To(Succeed())
			Expect(conn.Spec.InsecureSkipVerify).To(BeFalse())
			Expect(conn.Spec.CASecretRef).To(Equal(&warpgatev1alpha1.SecretKeyRef{Name: tlsSecret, Key: "ca.crt"}))
		})

		It("falls back to tls.crt when the TLS secret has no ca.crt", func() {
			createTLSSecret(map[string][]byte{"tls.crt": []byte("crt"), "tls.key": []byte("key")})
			insecure, caRef, err := reconciler.connectionTLS(ctx, newInstance(&warpgatev1alpha1.InstanceTLSSpec{SecretName: tlsSecret}))
			Expect(err).NotTo(HaveOccurred())
			Expect(insecure).To(BeFalse())
			Expect(caRef).To(Equal(&warpgatev1alpha1.SecretKeyRef{Name: tlsSecret, Key: "tls.crt"}))
		})

		It("errors when the TLS secret has no certificate at all", func() {
			createTLSSecret(map[string][]byte{"tls.key": []byte("key")})
			_, _, err := reconciler.connectionTLS(ctx, newInstance(&warpgatev1alpha1.InstanceTLSSpec{SecretName: tlsSecret}))
			Expect(err).To(MatchError(ContainSubstring("neither ca.crt nor tls.crt")))
		})

		It("errors when the TLS secret does not exist", func() {
			_, _, err := reconciler.connectionTLS(ctx, newInstance(&warpgatev1alpha1.InstanceTLSSpec{SecretName: tlsSecret}))
			Expect(err).To(HaveOccurred())
		})

		It("skips verification for the self-generated certificate by default", func() {
			for _, tls := range []*warpgatev1alpha1.InstanceTLSSpec{nil, {CertManager: boolPtr(true)}, {CertManager: boolPtr(false)}} {
				insecure, caRef, err := reconciler.connectionTLS(ctx, newInstance(tls))
				Expect(err).NotTo(HaveOccurred())
				Expect(insecure).To(BeTrue())
				Expect(caRef).To(BeNil())
			}
		})

		It("skips verification when verifyConnection is false, even with a TLS secret", func() {
			insecure, caRef, err := reconciler.connectionTLS(ctx, newInstance(&warpgatev1alpha1.InstanceTLSSpec{
				SecretName: tlsSecret, VerifyConnection: boolPtr(false),
			}))
			Expect(err).NotTo(HaveOccurred())
			Expect(insecure).To(BeTrue())
			Expect(caRef).To(BeNil())
		})

		It("refuses to create an unverified connection when verifyConnection is true and no CA is known", func() {
			inst := newInstance(&warpgatev1alpha1.InstanceTLSSpec{CertManager: boolPtr(false), VerifyConnection: boolPtr(true)})
			Expect(k8sClient.Create(ctx, inst)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(inst)})
			Expect(err).To(MatchError(ContainSubstring("verifyConnection")))

			var updated warpgatev1alpha1.WarpgateInstance
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(inst), &updated)).To(Succeed())
			cond := findReadyCondition(updated.Status.Conditions)
			Expect(cond).NotTo(BeNil())
			Expect(cond.Reason).To(Equal("ConnectionFailed"))

			var conn warpgatev1alpha1.WarpgateConnection
			err = k8sClient.Get(ctx, types.NamespacedName{Name: connName, Namespace: testNamespace}, &conn)
			Expect(err).To(HaveOccurred())
		})
	})
})
