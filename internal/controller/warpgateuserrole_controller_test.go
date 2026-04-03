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

var _ = Describe("WarpgateUserRole Controller", func() {

	var (
		reconciler *WarpgateUserRoleReconciler
	)

	BeforeEach(func() {
		reconciler = &WarpgateUserRoleReconciler{
			Client: k8sClient,
			Scheme: k8sClient.Scheme(),
		}
	})

	Context("Create user-role binding", func() {
		var (
			mockServer *httptest.Server
			secretName string
			connName   string
			crName     string
			namespace  string
		)

		BeforeEach(func() {
			secretName = "userrole-test-token"
			connName = "userrole-test-conn"
			crName = "userrole-test-binding"
			namespace = testNamespace

			mux := http.NewServeMux()
			mockLogin(mux)
			mux.HandleFunc("/@warpgate/admin/api/users", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode([]map[string]any{
					{"id": "user-uuid-1", "username": "testuser"},
				})
			})
			mux.HandleFunc("/@warpgate/admin/api/roles", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode([]map[string]any{
					{"id": "role-uuid-1", "name": "testrole"},
				})
			})
			mux.HandleFunc("/@warpgate/admin/api/users/user-uuid-1/roles/role-uuid-1", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					w.WriteHeader(http.StatusCreated)
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
					Name:      secretName,
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"username": []byte("admin"), "password": []byte("test-pass"),
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
						Name: secretName,
						Key:  "token",
					},
					InsecureSkipVerify: true,
				},
			}
			Expect(k8sClient.Create(ctx, conn)).To(Succeed())

			cr := &warpgatev1alpha1.WarpgateUserRole{
				ObjectMeta: metav1.ObjectMeta{
					Name:      crName,
					Namespace: namespace,
				},
				Spec: warpgatev1alpha1.WarpgateUserRoleSpec{
					ConnectionRef: connName,
					Username:      "testuser",
					RoleName:      "testrole",
				},
			}
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
		})

		AfterEach(func() {
			mockServer.Close()

			cr := &warpgatev1alpha1.WarpgateUserRole{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: crName, Namespace: namespace}, cr); err == nil {
				controllerutil.RemoveFinalizer(cr, warpgateUserRoleFinalizer)
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
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: namespace}, secret); err == nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should resolve user and role, create binding, and set Ready=True", func() {
			nn := types.NamespacedName{Name: crName, Namespace: namespace}

			// First reconcile adds the finalizer.
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile resolves IDs and creates the binding.
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			var updated warpgatev1alpha1.WarpgateUserRole
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())
			Expect(updated.Status.UserID).To(Equal("user-uuid-1"))
			Expect(updated.Status.RoleID).To(Equal("role-uuid-1"))

			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(readyCond.Reason).To(Equal("Bound"))
		})
	})

	Context("Delete binding", func() {
		var (
			mockServer   *httptest.Server
			secretName   string
			connName     string
			crName       string
			namespace    string
			deleteCalled bool
		)

		BeforeEach(func() {
			secretName = "userrole-del-token"
			connName = "userrole-del-conn"
			crName = "userrole-del-binding"
			namespace = testNamespace
			deleteCalled = false

			mux := http.NewServeMux()
			mockLogin(mux)
			mux.HandleFunc("/@warpgate/admin/api/users", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode([]map[string]any{
					{"id": "user-uuid-2", "username": "deluser"},
				})
			})
			mux.HandleFunc("/@warpgate/admin/api/roles", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode([]map[string]any{
					{"id": "role-uuid-2", "name": "delrole"},
				})
			})
			mux.HandleFunc("/@warpgate/admin/api/users/user-uuid-2/roles/role-uuid-2", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					w.WriteHeader(http.StatusCreated)
					return
				}
				if r.Method == http.MethodDelete {
					deleteCalled = true
					w.WriteHeader(http.StatusNoContent)
					return
				}
			})
			mockServer = httptest.NewServer(mux)

			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"username": []byte("admin"), "password": []byte("test-pass"),
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
						Name: secretName,
						Key:  "token",
					},
					InsecureSkipVerify: true,
				},
			}
			Expect(k8sClient.Create(ctx, conn)).To(Succeed())

			cr := &warpgatev1alpha1.WarpgateUserRole{
				ObjectMeta: metav1.ObjectMeta{
					Name:      crName,
					Namespace: namespace,
				},
				Spec: warpgatev1alpha1.WarpgateUserRoleSpec{
					ConnectionRef: connName,
					Username:      "deluser",
					RoleName:      "delrole",
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
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: namespace}, secret); err == nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should call DELETE on the Warpgate API and remove the finalizer", func() {
			nn := types.NamespacedName{Name: crName, Namespace: namespace}

			// Add finalizer + create binding.
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// Delete the CR.
			var cr warpgatev1alpha1.WarpgateUserRole
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

	Context("Role not found", func() {
		var (
			mockServer *httptest.Server
			secretName string
			connName   string
			crName     string
			namespace  string
		)

		BeforeEach(func() {
			secretName = "userrole-norole-token"
			connName = "userrole-norole-conn"
			crName = "userrole-norole-binding"
			namespace = testNamespace

			mux := http.NewServeMux()
			mockLogin(mux)
			mux.HandleFunc("/@warpgate/admin/api/users", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode([]map[string]any{
					{"id": "user-uuid-nr", "username": "realuser"},
				})
			})
			mux.HandleFunc("/@warpgate/admin/api/roles", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode([]map[string]any{})
			})
			mockServer = httptest.NewServer(mux)

			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: namespace},
				Data:       map[string][]byte{"username": []byte("admin"), "password": []byte("test-pass")},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			conn := &warpgatev1alpha1.WarpgateConnection{
				ObjectMeta: metav1.ObjectMeta{Name: connName, Namespace: namespace},
				Spec: warpgatev1alpha1.WarpgateConnectionSpec{
					Host:               mockServer.URL,
					TokenSecretRef:     warpgatev1alpha1.SecretKeyRef{Name: secretName, Key: "token"},
					InsecureSkipVerify: true,
				},
			}
			Expect(k8sClient.Create(ctx, conn)).To(Succeed())

			cr := &warpgatev1alpha1.WarpgateUserRole{
				ObjectMeta: metav1.ObjectMeta{Name: crName, Namespace: namespace},
				Spec: warpgatev1alpha1.WarpgateUserRoleSpec{
					ConnectionRef: connName,
					Username:      "realuser",
					RoleName:      "ghost-role",
				},
			}
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
		})

		AfterEach(func() {
			mockServer.Close()
			cr := &warpgatev1alpha1.WarpgateUserRole{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: crName, Namespace: namespace}, cr); err == nil {
				controllerutil.RemoveFinalizer(cr, warpgateUserRoleFinalizer)
				_ = k8sClient.Update(ctx, cr)
				_ = k8sClient.Delete(ctx, cr)
			}
			conn := &warpgatev1alpha1.WarpgateConnection{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: connName, Namespace: namespace}, conn); err == nil {
				_ = k8sClient.Delete(ctx, conn)
			}
			secret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: namespace}, secret); err == nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should set Ready=False with RoleNotFound when the role doesn't exist", func() {
			nn := types.NamespacedName{Name: crName, Namespace: namespace}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			var updated warpgatev1alpha1.WarpgateUserRole
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())

			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("RoleNotFound"))
		})
	})

	Context("Binding creation failed", func() {
		var (
			mockServer *httptest.Server
			secretName string
			connName   string
			crName     string
			namespace  string
		)

		BeforeEach(func() {
			secretName = "userrole-bindfail-token"
			connName = "userrole-bindfail-conn"
			crName = "userrole-bindfail-binding"
			namespace = testNamespace

			mux := http.NewServeMux()
			mockLogin(mux)
			mux.HandleFunc("/@warpgate/admin/api/users", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode([]map[string]any{
					{"id": "user-uuid-bf", "username": "bfuser"},
				})
			})
			mux.HandleFunc("/@warpgate/admin/api/roles", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode([]map[string]any{
					{"id": "role-uuid-bf", "name": "bfrole"},
				})
			})
			mux.HandleFunc("/@warpgate/admin/api/users/user-uuid-bf/roles/role-uuid-bf", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			})
			mockServer = httptest.NewServer(mux)

			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: namespace},
				Data:       map[string][]byte{"username": []byte("admin"), "password": []byte("test-pass")},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			conn := &warpgatev1alpha1.WarpgateConnection{
				ObjectMeta: metav1.ObjectMeta{Name: connName, Namespace: namespace},
				Spec: warpgatev1alpha1.WarpgateConnectionSpec{
					Host:               mockServer.URL,
					TokenSecretRef:     warpgatev1alpha1.SecretKeyRef{Name: secretName, Key: "token"},
					InsecureSkipVerify: true,
				},
			}
			Expect(k8sClient.Create(ctx, conn)).To(Succeed())

			cr := &warpgatev1alpha1.WarpgateUserRole{
				ObjectMeta: metav1.ObjectMeta{Name: crName, Namespace: namespace},
				Spec: warpgatev1alpha1.WarpgateUserRoleSpec{
					ConnectionRef: connName,
					Username:      "bfuser",
					RoleName:      "bfrole",
				},
			}
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
		})

		AfterEach(func() {
			mockServer.Close()
			cr := &warpgatev1alpha1.WarpgateUserRole{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: crName, Namespace: namespace}, cr); err == nil {
				controllerutil.RemoveFinalizer(cr, warpgateUserRoleFinalizer)
				_ = k8sClient.Update(ctx, cr)
				_ = k8sClient.Delete(ctx, cr)
			}
			conn := &warpgatev1alpha1.WarpgateConnection{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: connName, Namespace: namespace}, conn); err == nil {
				_ = k8sClient.Delete(ctx, conn)
			}
			secret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: namespace}, secret); err == nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should set Ready=False with BindingFailed when the API returns an error", func() {
			nn := types.NamespacedName{Name: crName, Namespace: namespace}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			var updated warpgatev1alpha1.WarpgateUserRole
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())

			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("BindingFailed"))
		})
	})

	Context("Resource not found", func() {
		It("should return no error when the resource doesn't exist", func() {
			nn := types.NamespacedName{Name: "userrole-nonexistent", Namespace: testNamespace}
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))
		})
	})

	Context("Delete binding with empty UserID or RoleID", func() {
		var (
			mockServer *httptest.Server
			secretName string
			connName   string
			crName     string
			namespace  string
		)

		BeforeEach(func() {
			secretName = "userrole-delempty-token"
			connName = "userrole-delempty-conn"
			crName = "userrole-delempty-binding"
			namespace = testNamespace

			mux := http.NewServeMux()
			mockLogin(mux)
			mux.HandleFunc("/@warpgate/admin/api/users", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode([]map[string]any{})
			})
			mux.HandleFunc("/@warpgate/admin/api/roles", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode([]map[string]any{})
			})
			mockServer = httptest.NewServer(mux)

			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: namespace},
				Data:       map[string][]byte{"username": []byte("admin"), "password": []byte("test-pass")},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			conn := &warpgatev1alpha1.WarpgateConnection{
				ObjectMeta: metav1.ObjectMeta{Name: connName, Namespace: namespace},
				Spec: warpgatev1alpha1.WarpgateConnectionSpec{
					Host:               mockServer.URL,
					TokenSecretRef:     warpgatev1alpha1.SecretKeyRef{Name: secretName, Key: "token"},
					InsecureSkipVerify: true,
				},
			}
			Expect(k8sClient.Create(ctx, conn)).To(Succeed())

			// Create a CR with a finalizer but no resolved IDs in status.
			cr := &warpgatev1alpha1.WarpgateUserRole{
				ObjectMeta: metav1.ObjectMeta{
					Name:       crName,
					Namespace:  namespace,
					Finalizers: []string{warpgateUserRoleFinalizer},
				},
				Spec: warpgatev1alpha1.WarpgateUserRoleSpec{
					ConnectionRef: connName,
					Username:      "emptyuser",
					RoleName:      "emptyrole",
				},
			}
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
		})

		AfterEach(func() {
			mockServer.Close()
			conn := &warpgatev1alpha1.WarpgateConnection{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: connName, Namespace: namespace}, conn); err == nil {
				_ = k8sClient.Delete(ctx, conn)
			}
			secret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: namespace}, secret); err == nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should skip Warpgate API delete and just remove the finalizer when IDs are empty", func() {
			nn := types.NamespacedName{Name: crName, Namespace: namespace}

			// Delete the CR.
			var cr warpgatev1alpha1.WarpgateUserRole
			Expect(k8sClient.Get(ctx, nn, &cr)).To(Succeed())
			Expect(k8sClient.Delete(ctx, &cr)).To(Succeed())

			// Reconcile deletion: UserID and RoleID are empty, so skip API call.
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// CR should be gone.
			err = k8sClient.Get(ctx, nn, &cr)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))
		})
	})

	Context("setUserRoleCondition updates existing condition reason without changing status", func() {
		It("should update the reason/message when the status stays the same", func() {
			ur := &warpgatev1alpha1.WarpgateUserRole{
				Status: warpgatev1alpha1.WarpgateUserRoleStatus{
					Conditions: []metav1.Condition{
						{
							Type:    "Ready",
							Status:  metav1.ConditionFalse,
							Reason:  "FirstReason",
							Message: "first message",
						},
					},
				},
			}

			// Call with same status (False) but different reason.
			setUserRoleCondition(ur, metav1.ConditionFalse, "SecondReason", "second message")

			Expect(ur.Status.Conditions).To(HaveLen(1))
			Expect(ur.Status.Conditions[0].Reason).To(Equal("SecondReason"))
			Expect(ur.Status.Conditions[0].Message).To(Equal("second message"))
			// Status should remain False.
			Expect(ur.Status.Conditions[0].Status).To(Equal(metav1.ConditionFalse))
		})

		It("should replace the whole condition when status changes", func() {
			ur := &warpgatev1alpha1.WarpgateUserRole{
				Status: warpgatev1alpha1.WarpgateUserRoleStatus{
					Conditions: []metav1.Condition{
						{
							Type:    "Ready",
							Status:  metav1.ConditionFalse,
							Reason:  "WasFalse",
							Message: "was false",
						},
					},
				},
			}

			setUserRoleCondition(ur, metav1.ConditionTrue, "NowTrue", "now true")

			Expect(ur.Status.Conditions).To(HaveLen(1))
			Expect(ur.Status.Conditions[0].Status).To(Equal(metav1.ConditionTrue))
			Expect(ur.Status.Conditions[0].Reason).To(Equal("NowTrue"))
		})
	})

	Context("Client error", func() {
		var (
			crName    string
			namespace string
		)

		BeforeEach(func() {
			crName = "userrole-clienterr-binding"
			namespace = testNamespace

			cr := &warpgatev1alpha1.WarpgateUserRole{
				ObjectMeta: metav1.ObjectMeta{
					Name:      crName,
					Namespace: namespace,
				},
				Spec: warpgatev1alpha1.WarpgateUserRoleSpec{
					ConnectionRef: "nonexistent-conn",
					Username:      "someuser",
					RoleName:      "somerole",
				},
			}
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
		})

		AfterEach(func() {
			cr := &warpgatev1alpha1.WarpgateUserRole{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: crName, Namespace: namespace}, cr); err == nil {
				controllerutil.RemoveFinalizer(cr, warpgateUserRoleFinalizer)
				_ = k8sClient.Update(ctx, cr)
				_ = k8sClient.Delete(ctx, cr)
			}
		})

		It("should set Ready=False with ClientError when the connection doesn't exist", func() {
			nn := types.NamespacedName{Name: crName, Namespace: namespace}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred()) // returns requeue, not error

			var updated warpgatev1alpha1.WarpgateUserRole
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())

			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("ClientError"))
		})
	})

	Context("User not found", func() {
		var (
			mockServer *httptest.Server
			secretName string
			connName   string
			crName     string
			namespace  string
		)

		BeforeEach(func() {
			secretName = "userrole-nouser-token"
			connName = "userrole-nouser-conn"
			crName = "userrole-nouser-binding"
			namespace = testNamespace

			mux := http.NewServeMux()
			mockLogin(mux)
			mux.HandleFunc("/@warpgate/admin/api/users", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				// Return empty list so user resolution fails.
				_ = json.NewEncoder(w).Encode([]map[string]any{})
			})
			mockServer = httptest.NewServer(mux)

			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"username": []byte("admin"), "password": []byte("test-pass"),
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
						Name: secretName,
						Key:  "token",
					},
					InsecureSkipVerify: true,
				},
			}
			Expect(k8sClient.Create(ctx, conn)).To(Succeed())

			cr := &warpgatev1alpha1.WarpgateUserRole{
				ObjectMeta: metav1.ObjectMeta{
					Name:      crName,
					Namespace: namespace,
				},
				Spec: warpgatev1alpha1.WarpgateUserRoleSpec{
					ConnectionRef: connName,
					Username:      "ghost",
					RoleName:      "testrole",
				},
			}
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
		})

		AfterEach(func() {
			mockServer.Close()

			cr := &warpgatev1alpha1.WarpgateUserRole{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: crName, Namespace: namespace}, cr); err == nil {
				controllerutil.RemoveFinalizer(cr, warpgateUserRoleFinalizer)
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
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: namespace}, secret); err == nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should set Ready=False with UserNotFound when the user doesn't exist", func() {
			nn := types.NamespacedName{Name: crName, Namespace: namespace}

			// First reconcile adds the finalizer.
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile fails on user resolution.
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred()) // returns requeue, not error

			var updated warpgatev1alpha1.WarpgateUserRole
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())

			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("UserNotFound"))
			Expect(readyCond.Message).To(ContainSubstring("ghost"))
		})
	})
})
