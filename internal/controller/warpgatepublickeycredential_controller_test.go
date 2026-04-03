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

var _ = Describe("WarpgatePublicKeyCredential Controller", func() {

	var (
		reconciler *WarpgatePublicKeyCredentialReconciler
	)

	BeforeEach(func() {
		reconciler = &WarpgatePublicKeyCredentialReconciler{
			Client: k8sClient,
			Scheme: k8sClient.Scheme(),
		}
	})

	Context("Create public key credential", func() {
		var (
			mockServer  *httptest.Server
			tokenSecret string
			connName    string
			crName      string
			namespace   string
		)

		BeforeEach(func() {
			tokenSecret = "pkcred-test-token"
			connName = "pkcred-test-conn"
			crName = "pkcred-test-cred"
			namespace = testNamespace

			mux := http.NewServeMux()
			mux.HandleFunc("/@warpgate/admin/api/users", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode([]map[string]any{
					{"id": "user-uuid-pk1", "username": "pkuser"},
				})
			})
			mux.HandleFunc("/@warpgate/admin/api/users/user-uuid-pk1/credentials/public-keys", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusCreated)
					_ = json.NewEncoder(w).Encode(map[string]any{
						"id":                 "cred-uuid-pk1",
						"label":              "test-key",
						"openssh_public_key": "ssh-ed25519 AAAAC3... test@host",
					})
					return
				}
				http.NotFound(w, r)
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

			cr := &warpgatev1alpha1.WarpgatePublicKeyCredential{
				ObjectMeta: metav1.ObjectMeta{
					Name:      crName,
					Namespace: namespace,
				},
				Spec: warpgatev1alpha1.WarpgatePublicKeyCredentialSpec{
					ConnectionRef:    connName,
					Username:         "pkuser",
					Label:            "test-key",
					OpenSSHPublicKey: "ssh-ed25519 AAAAC3... test@host",
				},
			}
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
		})

		AfterEach(func() {
			mockServer.Close()

			cr := &warpgatev1alpha1.WarpgatePublicKeyCredential{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: crName, Namespace: namespace}, cr); err == nil {
				controllerutil.RemoveFinalizer(cr, publicKeyCredentialFinalizer)
				_ = k8sClient.Update(ctx, cr)
				_ = k8sClient.Delete(ctx, cr)
			}
			conn := &warpgatev1alpha1.WarpgateConnection{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: connName, Namespace: namespace}, conn); err == nil {
				controllerutil.RemoveFinalizer(conn, warpgateFinalizer)
				_ = k8sClient.Update(ctx, conn)
				_ = k8sClient.Delete(ctx, conn)
			}
			secret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: tokenSecret, Namespace: namespace}, secret); err == nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should create the public key credential and set Ready=True with CredentialID", func() {
			nn := types.NamespacedName{Name: crName, Namespace: namespace}

			// Single reconcile: adds finalizer, resolves user, creates credential, updates status.
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			var updated warpgatev1alpha1.WarpgatePublicKeyCredential
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())
			Expect(updated.Status.UserID).To(Equal("user-uuid-pk1"))
			Expect(updated.Status.CredentialID).To(Equal("cred-uuid-pk1"))

			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(readyCond.Reason).To(Equal("Reconciled"))
		})
	})

	Context("Update public key credential", func() {
		var (
			mockServer  *httptest.Server
			tokenSecret string
			connName    string
			crName      string
			namespace   string
			updateSeen  bool
		)

		BeforeEach(func() {
			tokenSecret = "pkcred-upd-token"
			connName = "pkcred-upd-conn"
			crName = "pkcred-upd-cred"
			namespace = testNamespace
			updateSeen = false

			mux := http.NewServeMux()
			mux.HandleFunc("/@warpgate/admin/api/users", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode([]map[string]any{
					{"id": "user-uuid-pk2", "username": "pkupduser"},
				})
			})
			mux.HandleFunc("/@warpgate/admin/api/users/user-uuid-pk2/credentials/public-keys", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusCreated)
					_ = json.NewEncoder(w).Encode(map[string]any{
						"id":                 "cred-uuid-pk2",
						"label":              "old-label",
						"openssh_public_key": "ssh-ed25519 AAAAC3... test@host",
					})
					return
				}
				http.NotFound(w, r)
			})
			mux.HandleFunc("/@warpgate/admin/api/users/user-uuid-pk2/credentials/public-keys/cred-uuid-pk2", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPut {
					updateSeen = true
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(map[string]any{
						"id":                 "cred-uuid-pk2",
						"label":              "new-label",
						"openssh_public_key": "ssh-ed25519 AAAAC3... test@host",
					})
					return
				}
				if r.Method == http.MethodDelete {
					w.WriteHeader(http.StatusNoContent)
					return
				}
				http.NotFound(w, r)
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

			cr := &warpgatev1alpha1.WarpgatePublicKeyCredential{
				ObjectMeta: metav1.ObjectMeta{
					Name:      crName,
					Namespace: namespace,
				},
				Spec: warpgatev1alpha1.WarpgatePublicKeyCredentialSpec{
					ConnectionRef:    connName,
					Username:         "pkupduser",
					Label:            "old-label",
					OpenSSHPublicKey: "ssh-ed25519 AAAAC3... test@host",
				},
			}
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
		})

		AfterEach(func() {
			mockServer.Close()

			cr := &warpgatev1alpha1.WarpgatePublicKeyCredential{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: crName, Namespace: namespace}, cr); err == nil {
				controllerutil.RemoveFinalizer(cr, publicKeyCredentialFinalizer)
				_ = k8sClient.Update(ctx, cr)
				_ = k8sClient.Delete(ctx, cr)
			}
			conn := &warpgatev1alpha1.WarpgateConnection{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: connName, Namespace: namespace}, conn); err == nil {
				controllerutil.RemoveFinalizer(conn, warpgateFinalizer)
				_ = k8sClient.Update(ctx, conn)
				_ = k8sClient.Delete(ctx, conn)
			}
			secret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: tokenSecret, Namespace: namespace}, secret); err == nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should update an existing credential via PUT and stay Ready=True", func() {
			nn := types.NamespacedName{Name: crName, Namespace: namespace}

			// Single reconcile: adds finalizer, resolves user, creates credential (POST).
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// Verify it was created.
			var created warpgatev1alpha1.WarpgatePublicKeyCredential
			Expect(k8sClient.Get(ctx, nn, &created)).To(Succeed())
			Expect(created.Status.CredentialID).To(Equal("cred-uuid-pk2"))

			// Update the label in the spec to trigger an update path.
			created.Spec.Label = "new-label"
			Expect(k8sClient.Update(ctx, &created)).To(Succeed())

			// Second reconcile: should hit the update (PUT) path.
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			Expect(updateSeen).To(BeTrue())

			var updated warpgatev1alpha1.WarpgatePublicKeyCredential
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())

			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
		})
	})

	Context("Delete credential", func() {
		var (
			mockServer   *httptest.Server
			tokenSecret  string
			connName     string
			crName       string
			namespace    string
			deleteCalled bool
		)

		BeforeEach(func() {
			tokenSecret = "pkcred-del-token"
			connName = "pkcred-del-conn"
			crName = "pkcred-del-cred"
			namespace = testNamespace
			deleteCalled = false

			mux := http.NewServeMux()
			mux.HandleFunc("/@warpgate/admin/api/users", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode([]map[string]any{
					{"id": "user-uuid-pk3", "username": "pkdeluser"},
				})
			})
			mux.HandleFunc("/@warpgate/admin/api/users/user-uuid-pk3/credentials/public-keys", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusCreated)
					_ = json.NewEncoder(w).Encode(map[string]any{
						"id":                 "cred-uuid-pk3",
						"label":              "del-key",
						"openssh_public_key": "ssh-ed25519 AAAAC3... test@host",
					})
					return
				}
			})
			mux.HandleFunc("/@warpgate/admin/api/users/user-uuid-pk3/credentials/public-keys/cred-uuid-pk3", func(w http.ResponseWriter, r *http.Request) {
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

			cr := &warpgatev1alpha1.WarpgatePublicKeyCredential{
				ObjectMeta: metav1.ObjectMeta{
					Name:      crName,
					Namespace: namespace,
				},
				Spec: warpgatev1alpha1.WarpgatePublicKeyCredentialSpec{
					ConnectionRef:    connName,
					Username:         "pkdeluser",
					Label:            "del-key",
					OpenSSHPublicKey: "ssh-ed25519 AAAAC3... test@host",
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
			secret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: tokenSecret, Namespace: namespace}, secret); err == nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should call DELETE on the Warpgate API and remove the finalizer", func() {
			nn := types.NamespacedName{Name: crName, Namespace: namespace}

			// Single reconcile: adds finalizer, resolves user, creates credential.
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// Delete the CR.
			var cr warpgatev1alpha1.WarpgatePublicKeyCredential
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
