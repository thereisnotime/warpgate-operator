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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	warpgatev1alpha1 "github.com/thereisnotime/warpgate-operator/api/v1alpha1"
)

const testNamespace = "default"

var _ = Describe("WarpgateConnection Controller", func() {

	var (
		reconciler *WarpgateConnectionReconciler
	)

	BeforeEach(func() {
		reconciler = &WarpgateConnectionReconciler{
			Client: k8sClient,
			Scheme: k8sClient.Scheme(),
		}
	})

	Context("Successful connection", func() {
		var (
			mockServer *httptest.Server
			secretName string
			connName   string
			namespace  string
		)

		BeforeEach(func() {
			secretName = "wg-token-success"
			connName = "wg-conn-success"
			namespace = testNamespace

			mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/@warpgate/api/auth/login" && r.Method == "POST" {
					http.SetCookie(w, &http.Cookie{Name: "warpgate", Value: "test-session", Path: "/"})
					w.WriteHeader(http.StatusCreated)
					return
				}
				if r.URL.Path == "/@warpgate/admin/api/roles" {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_ = json.NewEncoder(w).Encode([]map[string]any{
						{"id": "role-1", "name": "admin"},
					})
					return
				}
				http.NotFound(w, r)
			}))

			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"username": []byte("admin"),
					"password": []byte("test-pass"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			conn := &warpgatev1alpha1.WarpgateConnection{
				ObjectMeta: metav1.ObjectMeta{
					Name:      connName,
					Namespace: namespace,
				},
				Spec: warpgatev1alpha1.WarpgateConnectionSpec{
					Host:               mockServer.URL,
					AuthSecretRef:      warpgatev1alpha1.AuthSecretRef{Name: secretName},
					InsecureSkipVerify: true,
				},
			}
			Expect(k8sClient.Create(ctx, conn)).To(Succeed())
		})

		AfterEach(func() {
			mockServer.Close()

			conn := &warpgatev1alpha1.WarpgateConnection{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: connName, Namespace: namespace}, conn); err == nil {
				// Remove finalizer before deleting so cleanup doesn't block
				controllerutil.RemoveFinalizer(conn, warpgateFinalizer)
				_ = k8sClient.Update(ctx, conn)
				_ = k8sClient.Delete(ctx, conn)
			}

			secret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: namespace}, secret); err == nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should set Ready=True when Warpgate responds successfully", func() {
			nn := types.NamespacedName{Name: connName, Namespace: namespace}

			// Single reconcile: adds finalizer + validates connection — all in one pass.
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			var updated warpgatev1alpha1.WarpgateConnection
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())

			Expect(updated.Status.Conditions).NotTo(BeEmpty())
			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(readyCond.Reason).To(Equal("Connected"))
		})
	})

	Context("Missing secret", func() {
		var (
			connName  string
			namespace string
		)

		BeforeEach(func() {
			connName = "wg-conn-nosecret"
			namespace = testNamespace

			conn := &warpgatev1alpha1.WarpgateConnection{
				ObjectMeta: metav1.ObjectMeta{
					Name:      connName,
					Namespace: namespace,
				},
				Spec: warpgatev1alpha1.WarpgateConnectionSpec{
					Host:          "https://warpgate.example.com",
					AuthSecretRef: warpgatev1alpha1.AuthSecretRef{Name: "nonexistent-secret"},
				},
			}
			Expect(k8sClient.Create(ctx, conn)).To(Succeed())
		})

		AfterEach(func() {
			conn := &warpgatev1alpha1.WarpgateConnection{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: connName, Namespace: namespace}, conn); err == nil {
				controllerutil.RemoveFinalizer(conn, warpgateFinalizer)
				_ = k8sClient.Update(ctx, conn)
				_ = k8sClient.Delete(ctx, conn)
			}
		})

		It("should set Ready=False with ConnectionFailed when the secret doesn't exist", func() {
			nn := types.NamespacedName{Name: connName, Namespace: namespace}

			// Single reconcile: adds finalizer + tries to build client (fails on missing secret).
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			var updated warpgatev1alpha1.WarpgateConnection
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())

			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("ConnectionFailed"))
			Expect(readyCond.Message).To(ContainSubstring("nonexistent-secret"))
		})
	})

	Context("Invalid host", func() {
		var (
			secretName string
			connName   string
			namespace  string
		)

		BeforeEach(func() {
			secretName = "wg-token-badhost"
			connName = "wg-conn-badhost"
			namespace = testNamespace

			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"username": []byte("admin"),
					"password": []byte("some-pass"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			conn := &warpgatev1alpha1.WarpgateConnection{
				ObjectMeta: metav1.ObjectMeta{
					Name:      connName,
					Namespace: namespace,
				},
				Spec: warpgatev1alpha1.WarpgateConnectionSpec{
					Host:          "http://127.0.0.1:1", // unreachable port
					AuthSecretRef: warpgatev1alpha1.AuthSecretRef{Name: secretName},
				},
			}
			Expect(k8sClient.Create(ctx, conn)).To(Succeed())
		})

		AfterEach(func() {
			conn := &warpgatev1alpha1.WarpgateConnection{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: connName, Namespace: namespace}, conn); err == nil {
				controllerutil.RemoveFinalizer(conn, warpgateFinalizer)
				_ = k8sClient.Update(ctx, conn)
				_ = k8sClient.Delete(ctx, conn)
			}

			secret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: namespace}, secret); err == nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should set Ready=False with ConnectionFailed when the host is unreachable", func() {
			nn := types.NamespacedName{Name: connName, Namespace: namespace}

			// Single reconcile: adds finalizer + tries to connect (fails on unreachable host).
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			var updated warpgatev1alpha1.WarpgateConnection
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())

			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("ConnectionFailed"))
		})
	})

	Context("Deletion with finalizer", func() {
		var (
			mockServer *httptest.Server
			secretName string
			connName   string
			namespace  string
		)

		BeforeEach(func() {
			secretName = "wg-token-delete"
			connName = "wg-conn-delete"
			namespace = testNamespace

			mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/@warpgate/api/auth/login" && r.Method == "POST" {
					http.SetCookie(w, &http.Cookie{Name: "warpgate", Value: "test-session", Path: "/"})
					w.WriteHeader(http.StatusCreated)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode([]map[string]any{})
			}))

			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"username": []byte("admin"),
					"password": []byte("delete-test-pass"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			conn := &warpgatev1alpha1.WarpgateConnection{
				ObjectMeta: metav1.ObjectMeta{
					Name:      connName,
					Namespace: namespace,
				},
				Spec: warpgatev1alpha1.WarpgateConnectionSpec{
					Host:               mockServer.URL,
					AuthSecretRef:      warpgatev1alpha1.AuthSecretRef{Name: secretName},
					InsecureSkipVerify: true,
				},
			}
			Expect(k8sClient.Create(ctx, conn)).To(Succeed())
		})

		AfterEach(func() {
			mockServer.Close()

			secret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: namespace}, secret); err == nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should add a finalizer on first reconcile and remove it on deletion", func() {
			nn := types.NamespacedName{Name: connName, Namespace: namespace}

			// First reconcile should add the finalizer.
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			var conn warpgatev1alpha1.WarpgateConnection
			Expect(k8sClient.Get(ctx, nn, &conn)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(&conn, warpgateFinalizer)).To(BeTrue())

			// Delete the resource (it will be held by the finalizer).
			Expect(k8sClient.Delete(ctx, &conn)).To(Succeed())

			// Reconcile again to process the deletion and remove the finalizer.
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// The resource should now be fully gone.
			var deleted warpgatev1alpha1.WarpgateConnection
			err = k8sClient.Get(ctx, nn, &deleted)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))
		})
	})

	Context("Missing username key in secret", func() {
		var (
			secretName string
			connName   string
			namespace  string
		)

		BeforeEach(func() {
			secretName = "wg-token-missingkey"
			connName = "wg-conn-missingkey"
			namespace = testNamespace

			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"password": []byte("some-pass"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			conn := &warpgatev1alpha1.WarpgateConnection{
				ObjectMeta: metav1.ObjectMeta{
					Name:      connName,
					Namespace: namespace,
				},
				Spec: warpgatev1alpha1.WarpgateConnectionSpec{
					Host:          "https://warpgate.example.com",
					AuthSecretRef: warpgatev1alpha1.AuthSecretRef{Name: secretName},
				},
			}
			Expect(k8sClient.Create(ctx, conn)).To(Succeed())
		})

		AfterEach(func() {
			conn := &warpgatev1alpha1.WarpgateConnection{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: connName, Namespace: namespace}, conn); err == nil {
				controllerutil.RemoveFinalizer(conn, warpgateFinalizer)
				_ = k8sClient.Update(ctx, conn)
				_ = k8sClient.Delete(ctx, conn)
			}

			secret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: namespace}, secret); err == nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should set Ready=False when the username key is missing from the secret", func() {
			nn := types.NamespacedName{Name: connName, Namespace: namespace}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			var updated warpgatev1alpha1.WarpgateConnection
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())

			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("ConnectionFailed"))
			Expect(readyCond.Message).To(ContainSubstring(`key "username" not found`))
		})
	})

	Context("Missing password key in secret", func() {
		var (
			secretName string
			connName   string
			namespace  string
		)

		BeforeEach(func() {
			secretName = "wg-token-nopass"
			connName = "wg-conn-nopass"
			namespace = testNamespace

			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"username": []byte("admin"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			conn := &warpgatev1alpha1.WarpgateConnection{
				ObjectMeta: metav1.ObjectMeta{
					Name:      connName,
					Namespace: namespace,
				},
				Spec: warpgatev1alpha1.WarpgateConnectionSpec{
					Host:          "https://warpgate.example.com",
					AuthSecretRef: warpgatev1alpha1.AuthSecretRef{Name: secretName},
				},
			}
			Expect(k8sClient.Create(ctx, conn)).To(Succeed())
		})

		AfterEach(func() {
			conn := &warpgatev1alpha1.WarpgateConnection{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: connName, Namespace: namespace}, conn); err == nil {
				controllerutil.RemoveFinalizer(conn, warpgateFinalizer)
				_ = k8sClient.Update(ctx, conn)
				_ = k8sClient.Delete(ctx, conn)
			}

			secret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: namespace}, secret); err == nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should set Ready=False when the password key is missing from the secret", func() {
			nn := types.NamespacedName{Name: connName, Namespace: namespace}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			var updated warpgatev1alpha1.WarpgateConnection
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())

			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("ConnectionFailed"))
			Expect(readyCond.Message).To(ContainSubstring(`key "password" not found`))
		})
	})

	Context("Custom PasswordKey fallback in buildClient", func() {
		var (
			mockServer *httptest.Server
			secretName string
			connName   string
			namespace  string
		)

		BeforeEach(func() {
			secretName = "wg-token-pwkey"
			connName = "wg-conn-pwkey"
			namespace = testNamespace

			mux := http.NewServeMux()
			mockLogin(mux)
			mux.HandleFunc("/@warpgate/admin/api/roles", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode([]map[string]any{})
			})
			mockServer = httptest.NewServer(mux)

			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"username":  []byte("admin"),
					"password":  []byte("default-pass"),
					"my-secret": []byte("custom-pass"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())
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
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: namespace}, secret); err == nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should default PasswordKey to 'password' when left empty", func() {
			conn := &warpgatev1alpha1.WarpgateConnection{
				ObjectMeta: metav1.ObjectMeta{
					Name:      connName,
					Namespace: namespace,
				},
				Spec: warpgatev1alpha1.WarpgateConnectionSpec{
					Host: mockServer.URL,
					AuthSecretRef: warpgatev1alpha1.AuthSecretRef{
						Name:        secretName,
						PasswordKey: "", // should fall back to "password"
					},
					InsecureSkipVerify: true,
				},
			}
			Expect(k8sClient.Create(ctx, conn)).To(Succeed())

			nn := types.NamespacedName{Name: connName, Namespace: namespace}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			var updated warpgatev1alpha1.WarpgateConnection
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())

			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
		})

		It("should fail when custom PasswordKey is set but the key is missing from the secret", func() {
			conn := &warpgatev1alpha1.WarpgateConnection{
				ObjectMeta: metav1.ObjectMeta{
					Name:      connName,
					Namespace: namespace,
				},
				Spec: warpgatev1alpha1.WarpgateConnectionSpec{
					Host: mockServer.URL,
					AuthSecretRef: warpgatev1alpha1.AuthSecretRef{
						Name:        secretName,
						PasswordKey: "nonexistent-key",
					},
					InsecureSkipVerify: true,
				},
			}
			Expect(k8sClient.Create(ctx, conn)).To(Succeed())

			nn := types.NamespacedName{Name: connName, Namespace: namespace}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			var updated warpgatev1alpha1.WarpgateConnection
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())

			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("ConnectionFailed"))
			Expect(readyCond.Message).To(ContainSubstring(`key "nonexistent-key" not found`))
		})
	})

	Context("Status update failure after Ready=True", func() {
		var (
			mockServer *httptest.Server
			secretName string
			connName   string
			namespace  string
		)

		BeforeEach(func() {
			secretName = "wg-token-statusfail-true"
			connName = "wg-conn-statusfail-true"
			namespace = testNamespace

			mux := http.NewServeMux()
			mockLogin(mux)
			mux.HandleFunc("/@warpgate/admin/api/roles", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode([]map[string]any{})
			})
			mockServer = httptest.NewServer(mux)

			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"username": []byte("admin"),
					"password": []byte("test-pass"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			conn := &warpgatev1alpha1.WarpgateConnection{
				ObjectMeta: metav1.ObjectMeta{
					Name:      connName,
					Namespace: namespace,
				},
				Spec: warpgatev1alpha1.WarpgateConnectionSpec{
					Host:               mockServer.URL,
					AuthSecretRef:      warpgatev1alpha1.AuthSecretRef{Name: secretName},
					InsecureSkipVerify: true,
				},
			}
			Expect(k8sClient.Create(ctx, conn)).To(Succeed())
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
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: namespace}, secret); err == nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should return an error when status update fails after setting Ready=True", func() {
			nn := types.NamespacedName{Name: connName, Namespace: namespace}

			// Use a reconciler with a status client that will fail writes.
			failReconciler := &WarpgateConnectionReconciler{
				Client: &statusFailClient{Client: k8sClient},
				Scheme: k8sClient.Scheme(),
			}
			_, err := failReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("status update injected failure"))
		})
	})

	Context("Status update failure after Ready=False", func() {
		var (
			secretName string
			connName   string
			namespace  string
		)

		BeforeEach(func() {
			secretName = "wg-token-statusfail-false"
			connName = "wg-conn-statusfail-false"
			namespace = testNamespace

			// Secret exists but the connection host is unreachable, so buildClient succeeds
			// but ListRoles fails, leading to Ready=False.
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"username": []byte("admin"),
					"password": []byte("test-pass"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			conn := &warpgatev1alpha1.WarpgateConnection{
				ObjectMeta: metav1.ObjectMeta{
					Name:      connName,
					Namespace: namespace,
				},
				Spec: warpgatev1alpha1.WarpgateConnectionSpec{
					Host:          "http://127.0.0.1:1",
					AuthSecretRef: warpgatev1alpha1.AuthSecretRef{Name: secretName},
				},
			}
			Expect(k8sClient.Create(ctx, conn)).To(Succeed())
		})

		AfterEach(func() {
			conn := &warpgatev1alpha1.WarpgateConnection{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: connName, Namespace: namespace}, conn); err == nil {
				controllerutil.RemoveFinalizer(conn, warpgateFinalizer)
				_ = k8sClient.Update(ctx, conn)
				_ = k8sClient.Delete(ctx, conn)
			}
			secret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: namespace}, secret); err == nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should return an error when status update fails after setting Ready=False (ListRoles path)", func() {
			nn := types.NamespacedName{Name: connName, Namespace: namespace}

			failReconciler := &WarpgateConnectionReconciler{
				Client: &statusFailClient{Client: k8sClient},
				Scheme: k8sClient.Scheme(),
			}
			_, err := failReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("status update injected failure"))
		})
	})

	Context("Status update failure after buildClient fails", func() {
		var (
			connName  string
			namespace string
		)

		BeforeEach(func() {
			connName = "wg-conn-statusfail-build"
			namespace = testNamespace

			conn := &warpgatev1alpha1.WarpgateConnection{
				ObjectMeta: metav1.ObjectMeta{
					Name:      connName,
					Namespace: namespace,
				},
				Spec: warpgatev1alpha1.WarpgateConnectionSpec{
					Host:          "https://warpgate.example.com",
					AuthSecretRef: warpgatev1alpha1.AuthSecretRef{Name: "nonexistent-secret-statusfail"},
				},
			}
			Expect(k8sClient.Create(ctx, conn)).To(Succeed())
		})

		AfterEach(func() {
			conn := &warpgatev1alpha1.WarpgateConnection{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: connName, Namespace: namespace}, conn); err == nil {
				controllerutil.RemoveFinalizer(conn, warpgateFinalizer)
				_ = k8sClient.Update(ctx, conn)
				_ = k8sClient.Delete(ctx, conn)
			}
		})

		It("should return the status update error when buildClient fails and status update also fails", func() {
			nn := types.NamespacedName{Name: connName, Namespace: namespace}

			failReconciler := &WarpgateConnectionReconciler{
				Client: &statusFailClient{Client: k8sClient},
				Scheme: k8sClient.Scheme(),
			}
			_, err := failReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("status update injected failure"))
		})
	})

	Context("Resource not found", func() {
		It("should return no error when the resource doesn't exist", func() {
			nn := types.NamespacedName{
				Name:      "does-not-exist",
				Namespace: testNamespace,
			}

			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))
		})
	})

	Context("Token-based auth via buildClient", func() {
		var (
			mockServer *httptest.Server
			secretName string
			connName   string
			namespace  string
			gotToken   string
		)

		BeforeEach(func() {
			secretName = "wg-secret-tokenauth"
			connName = "wg-conn-tokenauth"
			namespace = testNamespace
			gotToken = ""

			mux := http.NewServeMux()
			mux.HandleFunc("/@warpgate/admin/api/roles", func(w http.ResponseWriter, r *http.Request) {
				gotToken = r.Header.Get("X-Warpgate-Token")
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode([]map[string]any{})
			})
			mockServer = httptest.NewServer(mux)

			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"token": []byte("my-super-token"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			conn := &warpgatev1alpha1.WarpgateConnection{
				ObjectMeta: metav1.ObjectMeta{
					Name:      connName,
					Namespace: namespace,
				},
				Spec: warpgatev1alpha1.WarpgateConnectionSpec{
					Host:               mockServer.URL,
					AuthSecretRef:      warpgatev1alpha1.AuthSecretRef{Name: secretName},
					InsecureSkipVerify: true,
				},
			}
			Expect(k8sClient.Create(ctx, conn)).To(Succeed())
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
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: namespace}, secret); err == nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should send the X-Warpgate-Token header and set Ready=True", func() {
			nn := types.NamespacedName{Name: connName, Namespace: namespace}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			Expect(gotToken).To(Equal("my-super-token"))

			var updated warpgatev1alpha1.WarpgateConnection
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())

			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(readyCond.Reason).To(Equal("Connected"))
		})
	})

	Context("buildClient with missing token and missing username", func() {
		var (
			secretName string
			connName   string
			namespace  string
		)

		BeforeEach(func() {
			secretName = "wg-secret-empty"
			connName = "wg-conn-empty"
			namespace = testNamespace

			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"password": []byte("only-password"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			conn := &warpgatev1alpha1.WarpgateConnection{
				ObjectMeta: metav1.ObjectMeta{
					Name:      connName,
					Namespace: namespace,
				},
				Spec: warpgatev1alpha1.WarpgateConnectionSpec{
					Host:          "https://warpgate.example.com",
					AuthSecretRef: warpgatev1alpha1.AuthSecretRef{Name: secretName},
				},
			}
			Expect(k8sClient.Create(ctx, conn)).To(Succeed())
		})

		AfterEach(func() {
			conn := &warpgatev1alpha1.WarpgateConnection{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: connName, Namespace: namespace}, conn); err == nil {
				controllerutil.RemoveFinalizer(conn, warpgateFinalizer)
				_ = k8sClient.Update(ctx, conn)
				_ = k8sClient.Delete(ctx, conn)
			}
			secret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: namespace}, secret); err == nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should set Ready=False when the secret has neither token nor username", func() {
			nn := types.NamespacedName{Name: connName, Namespace: namespace}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			var updated warpgatev1alpha1.WarpgateConnection
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())

			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("ConnectionFailed"))
			Expect(readyCond.Message).To(ContainSubstring(`key "username" not found`))
		})
	})

	Context("API returns 500 on validation", func() {
		var (
			mockServer *httptest.Server
			secretName string
			connName   string
			namespace  string
		)

		BeforeEach(func() {
			secretName = "wg-token-500"
			connName = "wg-conn-500"
			namespace = testNamespace

			mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/@warpgate/api/auth/login" && r.Method == "POST" {
					http.SetCookie(w, &http.Cookie{Name: "warpgate", Value: "test-session", Path: "/"})
					w.WriteHeader(http.StatusCreated)
					return
				}
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error":"internal server error"}`))
			}))

			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"username": []byte("admin"),
					"password": []byte("test-pass"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			conn := &warpgatev1alpha1.WarpgateConnection{
				ObjectMeta: metav1.ObjectMeta{
					Name:      connName,
					Namespace: namespace,
				},
				Spec: warpgatev1alpha1.WarpgateConnectionSpec{
					Host:               mockServer.URL,
					AuthSecretRef:      warpgatev1alpha1.AuthSecretRef{Name: secretName},
					InsecureSkipVerify: true,
				},
			}
			Expect(k8sClient.Create(ctx, conn)).To(Succeed())
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
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: namespace}, secret); err == nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should set Ready=False with ConnectionFailed when the API returns 500", func() {
			nn := types.NamespacedName{Name: connName, Namespace: namespace}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			var updated warpgatev1alpha1.WarpgateConnection
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())

			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("ConnectionFailed"))
			Expect(readyCond.Message).To(ContainSubstring("Connection check failed"))
		})
	})

	Context("buildClient with custom UsernameKey via reconcile", func() {
		var (
			mockServer *httptest.Server
			secretName string
			connName   string
			namespace  string
		)

		BeforeEach(func() {
			secretName = "wg-secret-custukey"
			connName = "wg-conn-custukey"
			namespace = testNamespace

			mux := http.NewServeMux()
			mockLogin(mux)
			mux.HandleFunc("/@warpgate/admin/api/roles", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode([]map[string]any{})
			})
			mockServer = httptest.NewServer(mux)

			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"wg-user": []byte("admin"),
					"wg-pass": []byte("test-pass"),
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
					AuthSecretRef: warpgatev1alpha1.AuthSecretRef{
						Name:        secretName,
						UsernameKey: "wg-user",
						PasswordKey: "wg-pass",
					},
					InsecureSkipVerify: true,
				},
			}
			Expect(k8sClient.Create(ctx, conn)).To(Succeed())
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
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: namespace}, secret); err == nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should use custom UsernameKey and PasswordKey from AuthSecretRef and set Ready=True", func() {
			nn := types.NamespacedName{Name: connName, Namespace: namespace}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			var updated warpgatev1alpha1.WarpgateConnection
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())

			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(readyCond.Reason).To(Equal("Connected"))
		})
	})

	Context("buildClient falls back to password when token key is empty", func() {
		var (
			mockServer *httptest.Server
			secretName string
			connName   string
			namespace  string
		)

		BeforeEach(func() {
			secretName = "wg-secret-emptytoken"
			connName = "wg-conn-emptytoken"
			namespace = testNamespace

			mux := http.NewServeMux()
			mockLogin(mux)
			mux.HandleFunc("/@warpgate/admin/api/roles", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode([]map[string]any{})
			})
			mockServer = httptest.NewServer(mux)

			// Secret has a "token" key, but it's empty -- should fall back to password auth.
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"token":    []byte(""),
					"username": []byte("admin"),
					"password": []byte("test-pass"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			conn := &warpgatev1alpha1.WarpgateConnection{
				ObjectMeta: metav1.ObjectMeta{
					Name:      connName,
					Namespace: namespace,
				},
				Spec: warpgatev1alpha1.WarpgateConnectionSpec{
					Host:               mockServer.URL,
					AuthSecretRef:      warpgatev1alpha1.AuthSecretRef{Name: secretName},
					InsecureSkipVerify: true,
				},
			}
			Expect(k8sClient.Create(ctx, conn)).To(Succeed())
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
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: namespace}, secret); err == nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should fall back to username/password when token key exists but is empty", func() {
			nn := types.NamespacedName{Name: connName, Namespace: namespace}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			var updated warpgatev1alpha1.WarpgateConnection
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())

			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(readyCond.Reason).To(Equal("Connected"))
		})
	})

	Context("buildClient with custom TokenKey via reconcile", func() {
		var (
			mockServer *httptest.Server
			secretName string
			connName   string
			namespace  string
			gotToken   string
		)

		BeforeEach(func() {
			secretName = "wg-secret-custtkey"
			connName = "wg-conn-custtkey"
			namespace = testNamespace
			gotToken = ""

			mux := http.NewServeMux()
			mux.HandleFunc("/@warpgate/admin/api/roles", func(w http.ResponseWriter, r *http.Request) {
				gotToken = r.Header.Get("X-Warpgate-Token")
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode([]map[string]any{})
			})
			mockServer = httptest.NewServer(mux)

			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"my-api-key": []byte("custom-token-via-reconcile"),
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
					AuthSecretRef: warpgatev1alpha1.AuthSecretRef{
						Name:     secretName,
						TokenKey: "my-api-key",
					},
					InsecureSkipVerify: true,
				},
			}
			Expect(k8sClient.Create(ctx, conn)).To(Succeed())
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
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: namespace}, secret); err == nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should use the custom TokenKey from AuthSecretRef and set Ready=True", func() {
			nn := types.NamespacedName{Name: connName, Namespace: namespace}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			Expect(gotToken).To(Equal("custom-token-via-reconcile"))

			var updated warpgatev1alpha1.WarpgateConnection
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())

			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(readyCond.Reason).To(Equal("Connected"))
		})
	})
})

// findReadyCondition returns the "Ready" condition, or nil if not found.
func findReadyCondition(conditions []metav1.Condition) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == "Ready" {
			return &conditions[i]
		}
	}
	return nil
}

// statusFailClient wraps a real client but makes Status().Update() always fail.
type statusFailClient struct {
	client.Client
}

type statusFailWriter struct {
	client.SubResourceClient
}

func (s *statusFailClient) Status() client.SubResourceWriter {
	return &statusFailWriter{}
}

func (w *statusFailWriter) Update(_ context.Context, _ client.Object, _ ...client.SubResourceUpdateOption) error {
	return fmt.Errorf("status update injected failure")
}

func (w *statusFailWriter) Patch(_ context.Context, _ client.Object, _ client.Patch, _ ...client.SubResourcePatchOption) error {
	return fmt.Errorf("status patch injected failure")
}

func (w *statusFailWriter) Create(_ context.Context, _ client.Object, _ client.Object, _ ...client.SubResourceCreateOption) error {
	return fmt.Errorf("status create injected failure")
}
