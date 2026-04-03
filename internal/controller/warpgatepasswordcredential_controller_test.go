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
	"encoding/json"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	warpgatev1alpha1 "github.com/thereisnotime/warpgate-operator/api/v1alpha1"
)

var _ = Describe("WarpgatePasswordCredential Controller", func() {

	var (
		reconciler *WarpgatePasswordCredentialReconciler
	)

	BeforeEach(func() {
		reconciler = &WarpgatePasswordCredentialReconciler{
			Client: k8sClient,
			Scheme: k8sClient.Scheme(),
		}
	})

	Context("Create password credential", func() {
		var (
			mockServer     *httptest.Server
			tokenSecret    string
			passwordSecret string
			connName       string
			crName         string
			namespace      string
		)

		BeforeEach(func() {
			tokenSecret = "pwcred-test-token"
			passwordSecret = "pwcred-test-password"
			connName = "pwcred-test-conn"
			crName = "pwcred-test-cred"
			namespace = "default"

			mux := http.NewServeMux()
			mux.HandleFunc("/@warpgate/admin/api/users", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode([]map[string]any{
					{"id": "user-uuid-pw1", "username": "pwuser"},
				})
			})
			mux.HandleFunc("/@warpgate/admin/api/users/user-uuid-pw1/credentials/passwords", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusCreated)
					_ = json.NewEncoder(w).Encode(map[string]any{
						"id":       "cred-uuid-pw1",
						"password": "hashed",
					})
					return
				}
				http.NotFound(w, r)
			})
			mux.HandleFunc("/@warpgate/admin/api/users/user-uuid-pw1/credentials/passwords/cred-uuid-pw1", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodDelete {
					w.WriteHeader(http.StatusNoContent)
					return
				}
				http.NotFound(w, r)
			})
			mockServer = httptest.NewServer(mux)

			// Token secret for WarpgateConnection.
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tokenSecret,
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"token": []byte("test-token"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			// Password secret referenced by the CR.
			pwSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      passwordSecret,
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"password": []byte("s3cretpass"),
				},
			}
			Expect(k8sClient.Create(ctx, pwSecret)).To(Succeed())

			conn := &warpgatev1alpha1.WarpgateConnection{
				ObjectMeta: metav1.ObjectMeta{
					Name:      connName,
					Namespace: namespace,
				},
				Spec: warpgatev1alpha1.WarpgateConnectionSpec{
					Host: mockServer.URL,
					TokenSecretRef: warpgatev1alpha1.SecretKeyRef{
						Name: tokenSecret,
						Key:  "token",
					},
					InsecureSkipVerify: true,
				},
			}
			Expect(k8sClient.Create(ctx, conn)).To(Succeed())

			cr := &warpgatev1alpha1.WarpgatePasswordCredential{
				ObjectMeta: metav1.ObjectMeta{
					Name:      crName,
					Namespace: namespace,
				},
				Spec: warpgatev1alpha1.WarpgatePasswordCredentialSpec{
					ConnectionRef: connName,
					Username:      "pwuser",
					PasswordSecretRef: warpgatev1alpha1.SecretKeyRef{
						Name: passwordSecret,
						Key:  "password",
					},
				},
			}
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
		})

		AfterEach(func() {
			mockServer.Close()

			cr := &warpgatev1alpha1.WarpgatePasswordCredential{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: crName, Namespace: namespace}, cr); err == nil {
				controllerutil.RemoveFinalizer(cr, passwordCredentialFinalizer)
				_ = k8sClient.Update(ctx, cr)
				_ = k8sClient.Delete(ctx, cr)
			}
			conn := &warpgatev1alpha1.WarpgateConnection{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: connName, Namespace: namespace}, conn); err == nil {
				controllerutil.RemoveFinalizer(conn, warpgateFinalizer)
				_ = k8sClient.Update(ctx, conn)
				_ = k8sClient.Delete(ctx, conn)
			}
			for _, name := range []string{tokenSecret, passwordSecret} {
				secret := &corev1.Secret{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, secret); err == nil {
					_ = k8sClient.Delete(ctx, secret)
				}
			}
		})

		It("should create the password credential and set Ready=True with CredentialID", func() {
			nn := types.NamespacedName{Name: crName, Namespace: namespace}

			// Single reconcile: adds finalizer, resolves user, creates credential, updates status.
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			var updated warpgatev1alpha1.WarpgatePasswordCredential
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())
			Expect(updated.Status.UserID).To(Equal("user-uuid-pw1"))
			Expect(updated.Status.CredentialID).To(Equal("cred-uuid-pw1"))

			readyCond := findCondition(updated.Status.Conditions, "Ready")
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(readyCond.Reason).To(Equal("Reconciled"))
		})
	})

	Context("Delete credential", func() {
		var (
			mockServer     *httptest.Server
			tokenSecret    string
			passwordSecret string
			connName       string
			crName         string
			namespace      string
			deleteCalled   bool
		)

		BeforeEach(func() {
			tokenSecret = "pwcred-del-token"
			passwordSecret = "pwcred-del-password"
			connName = "pwcred-del-conn"
			crName = "pwcred-del-cred"
			namespace = "default"
			deleteCalled = false

			mux := http.NewServeMux()
			mux.HandleFunc("/@warpgate/admin/api/users", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode([]map[string]any{
					{"id": "user-uuid-pw2", "username": "pwdeluser"},
				})
			})
			mux.HandleFunc("/@warpgate/admin/api/users/user-uuid-pw2/credentials/passwords", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusCreated)
					_ = json.NewEncoder(w).Encode(map[string]any{
						"id":       "cred-uuid-pw2",
						"password": "hashed",
					})
					return
				}
			})
			mux.HandleFunc("/@warpgate/admin/api/users/user-uuid-pw2/credentials/passwords/cred-uuid-pw2", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodDelete {
					deleteCalled = true
					w.WriteHeader(http.StatusNoContent)
					return
				}
			})
			mockServer = httptest.NewServer(mux)

			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tokenSecret,
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"token": []byte("test-token"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			pwSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      passwordSecret,
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"password": []byte("s3cretpass"),
				},
			}
			Expect(k8sClient.Create(ctx, pwSecret)).To(Succeed())

			conn := &warpgatev1alpha1.WarpgateConnection{
				ObjectMeta: metav1.ObjectMeta{
					Name:      connName,
					Namespace: namespace,
				},
				Spec: warpgatev1alpha1.WarpgateConnectionSpec{
					Host: mockServer.URL,
					TokenSecretRef: warpgatev1alpha1.SecretKeyRef{
						Name: tokenSecret,
						Key:  "token",
					},
					InsecureSkipVerify: true,
				},
			}
			Expect(k8sClient.Create(ctx, conn)).To(Succeed())

			cr := &warpgatev1alpha1.WarpgatePasswordCredential{
				ObjectMeta: metav1.ObjectMeta{
					Name:      crName,
					Namespace: namespace,
				},
				Spec: warpgatev1alpha1.WarpgatePasswordCredentialSpec{
					ConnectionRef: connName,
					Username:      "pwdeluser",
					PasswordSecretRef: warpgatev1alpha1.SecretKeyRef{
						Name: passwordSecret,
						Key:  "password",
					},
				},
			}
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
		})

		AfterEach(func() {
			mockServer.Close()

			conn := &warpgatev1alpha1.WarpgateConnection{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: connName, Namespace: namespace}, conn); err == nil {
				controllerutil.RemoveFinalizer(conn, warpgateFinalizer)
				_ = k8sClient.Update(ctx, conn)
				_ = k8sClient.Delete(ctx, conn)
			}
			for _, name := range []string{tokenSecret, passwordSecret} {
				secret := &corev1.Secret{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, secret); err == nil {
					_ = k8sClient.Delete(ctx, secret)
				}
			}
		})

		It("should call DELETE on the Warpgate API and remove the finalizer", func() {
			nn := types.NamespacedName{Name: crName, Namespace: namespace}

			// Single reconcile: adds finalizer, resolves user, creates credential.
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// Delete the CR.
			var cr warpgatev1alpha1.WarpgatePasswordCredential
			Expect(k8sClient.Get(ctx, nn, &cr)).To(Succeed())
			Expect(k8sClient.Delete(ctx, &cr)).To(Succeed())

			// Reconcile deletion.
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			Expect(deleteCalled).To(BeTrue())

			// CR should be gone.
			err = k8sClient.Get(ctx, nn, &cr)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))
		})
	})
})
