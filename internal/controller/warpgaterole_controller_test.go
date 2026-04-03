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
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	warpgatev1alpha1 "github.com/thereisnotime/warpgate-operator/api/v1alpha1"
	"github.com/thereisnotime/warpgate-operator/internal/warpgate"
)

var _ = Describe("WarpgateRole Controller", func() {
	const (
		roleNamespace = "role-test-ns"
		connName      = "role-test-conn"
		secretName    = "role-test-token"
		usernameKey   = "username"
		usernameValue = "admin"
		passwordKey   = "password"
		passwordValue = "test-pass"
	)

	var (
		reconciler *WarpgateRoleReconciler
		ns         *corev1.Namespace
	)

	BeforeEach(func() {
		reconciler = &WarpgateRoleReconciler{
			Client: k8sClient,
			Scheme: scheme.Scheme,
		}

		ns = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: roleNamespace,
			},
		}
		_ = k8sClient.Create(ctx, ns)
	})

	// setupMockAndConnection creates the httptest server, the token Secret, and the
	// WarpgateConnection CR. Returns the server (caller must defer Close) and a cleanup func.
	setupMockAndConnection := func(mux *http.ServeMux, suffix string) *httptest.Server {
		srv := httptest.NewServer(mux)

		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName + suffix,
				Namespace: roleNamespace,
			},
			StringData: map[string]string{
				usernameKey: usernameValue,
				passwordKey: passwordValue,
			},
		}
		Expect(k8sClient.Create(ctx, secret)).To(Succeed())

		conn := &warpgatev1alpha1.WarpgateConnection{
			ObjectMeta: metav1.ObjectMeta{
				Name:      connName + suffix,
				Namespace: roleNamespace,
			},
			Spec: warpgatev1alpha1.WarpgateConnectionSpec{
				Host:                 srv.URL,
				CredentialsSecretRef: warpgatev1alpha1.CredentialsSecretRef{Name: secretName + suffix},
				InsecureSkipVerify:   true,
			},
		}
		Expect(k8sClient.Create(ctx, conn)).To(Succeed())

		return srv
	}

	Context("Create role", func() {
		It("should create the role in Warpgate and set ExternalID and Ready condition", func() {
			mux := http.NewServeMux()
			mockLogin(mux)
			mux.HandleFunc("/@warpgate/admin/api/roles", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(warpgate.Role{ID: "role-create-001", Name: "test-create-role"})
				}
			})
			srv := setupMockAndConnection(mux, "-create")
			defer srv.Close()

			role := &warpgatev1alpha1.WarpgateRole{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-create-role",
					Namespace: roleNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateRoleSpec{
					ConnectionRef: connName + "-create",
					Name:          "test-create-role",
					Description:   "a test role",
				},
			}
			Expect(k8sClient.Create(ctx, role)).To(Succeed())

			// Single reconcile: adds finalizer, creates role, updates status — all in one pass.
			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: role.Name, Namespace: roleNamespace},
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify the status.
			var updated warpgatev1alpha1.WarpgateRole
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: role.Name, Namespace: roleNamespace}, &updated)).To(Succeed())
			Expect(updated.Status.ExternalID).To(Equal("role-create-001"))

			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(readyCond.Reason).To(Equal("Reconciled"))
		})
	})

	Context("Update role", func() {
		It("should update the role in Warpgate after creation", func() {
			mux := http.NewServeMux()
			mockLogin(mux)
			mux.HandleFunc("/@warpgate/admin/api/roles", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(warpgate.Role{ID: "role-update-001", Name: "test-update-role"})
				}
			})
			mux.HandleFunc("/@warpgate/admin/api/role/", func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case http.MethodGet:
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(warpgate.Role{ID: "role-update-001", Name: "test-update-role"})
				case http.MethodPut:
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(warpgate.Role{ID: "role-update-001", Name: "test-update-role-updated"})
				}
			})
			srv := setupMockAndConnection(mux, "-update")
			defer srv.Close()

			role := &warpgatev1alpha1.WarpgateRole{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-update-role",
					Namespace: roleNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateRoleSpec{
					ConnectionRef: connName + "-update",
					Name:          "test-update-role",
				},
			}
			Expect(k8sClient.Create(ctx, role)).To(Succeed())

			nn := types.NamespacedName{Name: role.Name, Namespace: roleNamespace}

			// First reconcile: adds finalizer + creates role in one pass.
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			// Second reconcile: ExternalID is set, triggers update path (PUT).
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			var updated warpgatev1alpha1.WarpgateRole
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())

			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
		})
	})

	Context("Delete role", func() {
		It("should delete the role in Warpgate and remove the finalizer", func() {
			deleteCalled := false
			mux := http.NewServeMux()
			mockLogin(mux)
			mux.HandleFunc("/@warpgate/admin/api/roles", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(warpgate.Role{ID: "role-delete-001", Name: "test-delete-role"})
				}
			})
			mux.HandleFunc("/@warpgate/admin/api/role/", func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case http.MethodDelete:
					deleteCalled = true
					w.WriteHeader(http.StatusNoContent)
				case http.MethodPut:
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(warpgate.Role{ID: "role-delete-001", Name: "test-delete-role"})
				}
			})
			srv := setupMockAndConnection(mux, "-delete")
			defer srv.Close()

			role := &warpgatev1alpha1.WarpgateRole{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-delete-role",
					Namespace: roleNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateRoleSpec{
					ConnectionRef: connName + "-delete",
					Name:          "test-delete-role",
				},
			}
			Expect(k8sClient.Create(ctx, role)).To(Succeed())

			nn := types.NamespacedName{Name: role.Name, Namespace: roleNamespace}

			// Single reconcile: adds finalizer + creates role in one pass.
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// Delete the CR.
			Expect(k8sClient.Delete(ctx, role)).To(Succeed())

			// Reconcile deletion.
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			Expect(deleteCalled).To(BeTrue())

			// The CR should be fully gone now.
			var deleted warpgatev1alpha1.WarpgateRole
			err = k8sClient.Get(ctx, nn, &deleted)
			Expect(err).To(HaveOccurred())
		})
	})

	Context("Warpgate returns 404 on update", func() {
		It("should clear ExternalID and requeue", func() {
			callCount := 0
			mux := http.NewServeMux()
			mockLogin(mux)
			mux.HandleFunc("/@warpgate/admin/api/roles", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(warpgate.Role{ID: "role-404-001", Name: "test-404-role"})
				}
			})
			mux.HandleFunc("/@warpgate/admin/api/role/", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPut {
					callCount++
					if callCount >= 1 {
						w.WriteHeader(http.StatusNotFound)
						_, _ = w.Write([]byte(`"not found"`))
						return
					}
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(warpgate.Role{ID: "role-404-001", Name: "test-404-role"})
				}
			})
			srv := setupMockAndConnection(mux, "-404")
			defer srv.Close()

			role := &warpgatev1alpha1.WarpgateRole{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-404-role",
					Namespace: roleNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateRoleSpec{
					ConnectionRef: connName + "-404",
					Name:          "test-404-role",
				},
			}
			Expect(k8sClient.Create(ctx, role)).To(Succeed())

			nn := types.NamespacedName{Name: role.Name, Namespace: roleNamespace}

			// Single reconcile: adds finalizer + creates role in one pass.
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// Next reconcile hits update path, which returns 404.
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(Equal(reconcile.Result{}))

			var updated warpgatev1alpha1.WarpgateRole
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())
			Expect(updated.Status.ExternalID).To(BeEmpty())

			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("NotFound"))
		})
	})

	Context("Create role API error", func() {
		It("should set Ready=False with CreateFailed when the API returns an error", func() {
			mux := http.NewServeMux()
			mockLogin(mux)
			mux.HandleFunc("/@warpgate/admin/api/roles", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte(`{"error":"internal"}`))
					return
				}
			})
			srv := setupMockAndConnection(mux, "-createfail")
			defer srv.Close()

			role := &warpgatev1alpha1.WarpgateRole{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-createfail-role",
					Namespace: roleNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateRoleSpec{
					ConnectionRef: connName + "-createfail",
					Name:          "test-createfail-role",
				},
			}
			Expect(k8sClient.Create(ctx, role)).To(Succeed())

			nn := types.NamespacedName{Name: role.Name, Namespace: roleNamespace}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).To(HaveOccurred())

			var updated warpgatev1alpha1.WarpgateRole
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())

			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("CreateFailed"))
		})
	})

	Context("Update role non-404 error", func() {
		It("should set Ready=False with UpdateFailed for non-404 API errors", func() {
			mux := http.NewServeMux()
			mockLogin(mux)
			mux.HandleFunc("/@warpgate/admin/api/roles", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(warpgate.Role{ID: "role-updfail-001", Name: "test-updfail-role"})
				}
			})
			mux.HandleFunc("/@warpgate/admin/api/role/", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPut {
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte(`{"error":"internal"}`))
					return
				}
			})
			srv := setupMockAndConnection(mux, "-updfail")
			defer srv.Close()

			role := &warpgatev1alpha1.WarpgateRole{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-updfail-role",
					Namespace: roleNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateRoleSpec{
					ConnectionRef: connName + "-updfail",
					Name:          "test-updfail-role",
				},
			}
			Expect(k8sClient.Create(ctx, role)).To(Succeed())

			nn := types.NamespacedName{Name: role.Name, Namespace: roleNamespace}

			// First reconcile: create role.
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile: update fails with 500.
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).To(HaveOccurred())

			var updated warpgatev1alpha1.WarpgateRole
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())

			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("UpdateFailed"))
		})
	})

	Context("Delete role with empty ExternalID", func() {
		It("should remove the finalizer without calling Warpgate API delete", func() {
			mux := http.NewServeMux()
			mockLogin(mux)
			// No role endpoints needed — the delete should skip the API call entirely.
			srv := setupMockAndConnection(mux, "-delempty")
			defer srv.Close()

			role := &warpgatev1alpha1.WarpgateRole{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-delempty-role",
					Namespace: roleNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateRoleSpec{
					ConnectionRef: connName + "-delempty",
					Name:          "test-delempty-role",
				},
			}
			Expect(k8sClient.Create(ctx, role)).To(Succeed())

			nn := types.NamespacedName{Name: role.Name, Namespace: roleNamespace}

			// Manually add the finalizer without creating in Warpgate (no ExternalID).
			var fetched warpgatev1alpha1.WarpgateRole
			Expect(k8sClient.Get(ctx, nn, &fetched)).To(Succeed())
			controllerutil.AddFinalizer(&fetched, roleFinalizer)
			Expect(k8sClient.Update(ctx, &fetched)).To(Succeed())

			// Delete the CR.
			Expect(k8sClient.Delete(ctx, &fetched)).To(Succeed())

			// Reconcile the deletion — should skip the API delete and just remove the finalizer.
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// The CR should be fully gone now.
			var deleted warpgatev1alpha1.WarpgateRole
			err = k8sClient.Get(ctx, nn, &deleted)
			Expect(err).To(HaveOccurred())
		})
	})

	Context("Delete role with Warpgate API error", func() {
		It("should return an error when Warpgate API delete fails with non-404", func() {
			mux := http.NewServeMux()
			mockLogin(mux)
			mux.HandleFunc("/@warpgate/admin/api/roles", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(warpgate.Role{ID: "role-delerr-001", Name: "test-delerr-role"})
				}
			})
			mux.HandleFunc("/@warpgate/admin/api/role/", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodDelete {
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte(`{"error":"internal"}`))
					return
				}
			})
			srv := setupMockAndConnection(mux, "-delerr")
			defer srv.Close()

			role := &warpgatev1alpha1.WarpgateRole{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-delerr-role",
					Namespace: roleNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateRoleSpec{
					ConnectionRef: connName + "-delerr",
					Name:          "test-delerr-role",
				},
			}
			Expect(k8sClient.Create(ctx, role)).To(Succeed())

			nn := types.NamespacedName{Name: role.Name, Namespace: roleNamespace}

			// Create the role first.
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// Verify ExternalID was set.
			var created warpgatev1alpha1.WarpgateRole
			Expect(k8sClient.Get(ctx, nn, &created)).To(Succeed())
			Expect(created.Status.ExternalID).To(Equal("role-delerr-001"))

			// Delete the CR.
			Expect(k8sClient.Delete(ctx, &created)).To(Succeed())

			// Reconcile the deletion — should return error since API delete failed.
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).To(HaveOccurred())

			// The CR should still exist (finalizer not removed).
			var still warpgatev1alpha1.WarpgateRole
			Expect(k8sClient.Get(ctx, nn, &still)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(&still, roleFinalizer)).To(BeTrue())
		})
	})

	Context("Delete role already gone in Warpgate", func() {
		It("should remove the finalizer when Warpgate returns 404 on delete", func() {
			mux := http.NewServeMux()
			mockLogin(mux)
			mux.HandleFunc("/@warpgate/admin/api/roles", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(warpgate.Role{ID: "role-del404-001", Name: "test-del404-role"})
				}
			})
			mux.HandleFunc("/@warpgate/admin/api/role/", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodDelete {
					w.WriteHeader(http.StatusNotFound)
					_, _ = w.Write([]byte(`"not found"`))
					return
				}
			})
			srv := setupMockAndConnection(mux, "-del404")
			defer srv.Close()

			role := &warpgatev1alpha1.WarpgateRole{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-del404-role",
					Namespace: roleNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateRoleSpec{
					ConnectionRef: connName + "-del404",
					Name:          "test-del404-role",
				},
			}
			Expect(k8sClient.Create(ctx, role)).To(Succeed())

			nn := types.NamespacedName{Name: role.Name, Namespace: roleNamespace}

			// Create the role first.
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// Delete the CR.
			var created warpgatev1alpha1.WarpgateRole
			Expect(k8sClient.Get(ctx, nn, &created)).To(Succeed())
			Expect(k8sClient.Delete(ctx, &created)).To(Succeed())

			// Reconcile the deletion — 404 from Warpgate should be ignored, finalizer removed.
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// The CR should be fully gone now.
			var deleted warpgatev1alpha1.WarpgateRole
			err = k8sClient.Get(ctx, nn, &deleted)
			Expect(err).To(HaveOccurred())
		})
	})

	Context("Missing connection", func() {
		It("should return an error when the WarpgateConnection does not exist", func() {
			role := &warpgatev1alpha1.WarpgateRole{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-missing-conn-role",
					Namespace: roleNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateRoleSpec{
					ConnectionRef: "nonexistent-connection",
					Name:          "ghost-role",
				},
			}
			Expect(k8sClient.Create(ctx, role)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: role.Name, Namespace: roleNamespace},
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("nonexistent-connection"))

			// Status should reflect the error.
			var updated warpgatev1alpha1.WarpgateRole
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: role.Name, Namespace: roleNamespace}, &updated)).To(Succeed())
			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("ClientError"))
		})
	})
})

// Ensure the reconciler satisfies the interface.
var _ reconcile.Reconciler = &WarpgateRoleReconciler{}
