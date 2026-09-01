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
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	warpgatev1alpha1 "github.com/thereisnotime/warpgate-operator/api/v1alpha1"
	"github.com/thereisnotime/warpgate-operator/internal/warpgate"
)

var _ = Describe("WarpgateTargetGroup Controller", func() {
	const (
		groupNamespace = "targetgroup-test-ns"
		connName       = "targetgroup-test-conn"
		secretName     = "targetgroup-test-token"
		usernameKey    = "username"
		usernameValue  = "admin"
		passwordKey    = "password"
		passwordValue  = "test-pass"
	)

	var (
		reconciler *WarpgateTargetGroupReconciler
		ns         *corev1.Namespace
	)

	BeforeEach(func() {
		reconciler = &WarpgateTargetGroupReconciler{
			Client: k8sClient,
			Scheme: scheme.Scheme,
		}

		ns = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: groupNamespace,
			},
		}
		_ = k8sClient.Create(ctx, ns)
	})

	setupMockAndConnection := func(mux *http.ServeMux, suffix string) *httptest.Server {
		srv := httptest.NewServer(mux)

		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName + suffix,
				Namespace: groupNamespace,
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
				Namespace: groupNamespace,
			},
			Spec: warpgatev1alpha1.WarpgateConnectionSpec{
				Host:               srv.URL,
				AuthSecretRef:      warpgatev1alpha1.AuthSecretRef{Name: secretName + suffix},
				InsecureSkipVerify: true,
			},
		}
		Expect(k8sClient.Create(ctx, conn)).To(Succeed())

		return srv
	}

	Context("Create target group", func() {
		It("should create the target group in Warpgate and set ExternalID and Ready condition", func() {
			mux := http.NewServeMux()
			mockLogin(mux)
			mux.HandleFunc("/@warpgate/admin/api/target-groups", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(warpgate.TargetGroup{
						ID:    "tg-create-001",
						Name:  "production",
						Color: "Danger",
					})
				}
			})
			srv := setupMockAndConnection(mux, "-create")
			defer srv.Close()

			group := &warpgatev1alpha1.WarpgateTargetGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-create-group",
					Namespace: groupNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateTargetGroupSpec{
					ConnectionRef: connName + "-create",
					Name:          "production",
					Description:   "Production servers",
					Color:         "Danger",
				},
			}
			Expect(k8sClient.Create(ctx, group)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: group.Name, Namespace: groupNamespace},
			})
			Expect(err).NotTo(HaveOccurred())

			var updated warpgatev1alpha1.WarpgateTargetGroup
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: group.Name, Namespace: groupNamespace}, &updated)).To(Succeed())
			Expect(updated.Status.ExternalID).To(Equal("tg-create-001"))

			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(readyCond.Reason).To(Equal("Reconciled"))
		})
	})

	Context("Update target group", func() {
		It("should update the target group in Warpgate after creation", func() {
			mux := http.NewServeMux()
			mockLogin(mux)
			mux.HandleFunc("/@warpgate/admin/api/target-groups", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(warpgate.TargetGroup{ID: "tg-update-001", Name: "dev"})
				}
			})
			mux.HandleFunc("/@warpgate/admin/api/target-groups/", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPut {
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(warpgate.TargetGroup{ID: "tg-update-001", Name: "dev", Color: "Info"})
				}
			})
			srv := setupMockAndConnection(mux, "-update")
			defer srv.Close()

			group := &warpgatev1alpha1.WarpgateTargetGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-update-group",
					Namespace: groupNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateTargetGroupSpec{
					ConnectionRef: connName + "-update",
					Name:          "dev",
				},
			}
			Expect(k8sClient.Create(ctx, group)).To(Succeed())

			nn := types.NamespacedName{Name: group.Name, Namespace: groupNamespace}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			var updated warpgatev1alpha1.WarpgateTargetGroup
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())
			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
		})
	})

	Context("Delete target group", func() {
		It("should delete the target group in Warpgate and remove the finalizer", func() {
			deleteCalled := false
			mux := http.NewServeMux()
			mockLogin(mux)
			mux.HandleFunc("/@warpgate/admin/api/target-groups", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(warpgate.TargetGroup{ID: "tg-delete-001", Name: "tmp"})
				}
			})
			mux.HandleFunc("/@warpgate/admin/api/target-groups/", func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case http.MethodDelete:
					deleteCalled = true
					w.WriteHeader(http.StatusNoContent)
				case http.MethodPut:
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(warpgate.TargetGroup{ID: "tg-delete-001", Name: "tmp"})
				}
			})
			srv := setupMockAndConnection(mux, "-delete")
			defer srv.Close()

			group := &warpgatev1alpha1.WarpgateTargetGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-delete-group",
					Namespace: groupNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateTargetGroupSpec{
					ConnectionRef: connName + "-delete",
					Name:          "tmp",
				},
			}
			Expect(k8sClient.Create(ctx, group)).To(Succeed())

			nn := types.NamespacedName{Name: group.Name, Namespace: groupNamespace}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Delete(ctx, group)).To(Succeed())
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(deleteCalled).To(BeTrue())

			var deleted warpgatev1alpha1.WarpgateTargetGroup
			err = k8sClient.Get(ctx, nn, &deleted)
			Expect(err).To(HaveOccurred())
		})
	})

	Context("Warpgate returns 404 on update", func() {
		It("should clear ExternalID and requeue", func() {
			mux := http.NewServeMux()
			mockLogin(mux)
			mux.HandleFunc("/@warpgate/admin/api/target-groups", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(warpgate.TargetGroup{ID: "tg-404-001", Name: "gone"})
				}
			})
			mux.HandleFunc("/@warpgate/admin/api/target-groups/", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPut {
					w.WriteHeader(http.StatusNotFound)
					_, _ = w.Write([]byte(`"not found"`))
				}
			})
			srv := setupMockAndConnection(mux, "-404")
			defer srv.Close()

			group := &warpgatev1alpha1.WarpgateTargetGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-404-group",
					Namespace: groupNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateTargetGroupSpec{
					ConnectionRef: connName + "-404",
					Name:          "gone",
				},
			}
			Expect(k8sClient.Create(ctx, group)).To(Succeed())

			nn := types.NamespacedName{Name: group.Name, Namespace: groupNamespace}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(Equal(reconcile.Result{}))

			var updated warpgatev1alpha1.WarpgateTargetGroup
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())
			Expect(updated.Status.ExternalID).To(BeEmpty())

			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("NotFound"))
		})
	})

	Context("Connection not found", func() {
		It("should set ClientError condition when the connectionRef does not exist", func() {
			group := &warpgatev1alpha1.WarpgateTargetGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-noconn-group",
					Namespace: groupNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateTargetGroupSpec{
					ConnectionRef: "nonexistent-warpgate-conn",
					Name:          "orphan-group",
				},
			}
			Expect(k8sClient.Create(ctx, group)).To(Succeed())

			nn := types.NamespacedName{Name: group.Name, Namespace: groupNamespace}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).To(HaveOccurred())

			var updated warpgatev1alpha1.WarpgateTargetGroup
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())
			Expect(updated.Status.ExternalID).To(BeEmpty())

			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("ClientError"))
		})
	})

	Context("Warpgate API error on create", func() {
		It("should set CreateFailed condition and return error when the API returns 500 on create", func() {
			mux := http.NewServeMux()
			mockLogin(mux)
			mux.HandleFunc("/@warpgate/admin/api/target-groups", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte(`{"error":"internal server error"}`))
				}
			})
			srv := setupMockAndConnection(mux, "-createfail")
			defer srv.Close()

			group := &warpgatev1alpha1.WarpgateTargetGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-createfail-group",
					Namespace: groupNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateTargetGroupSpec{
					ConnectionRef: connName + "-createfail",
					Name:          "failing-group",
					Color:         "Danger",
				},
			}
			Expect(k8sClient.Create(ctx, group)).To(Succeed())

			nn := types.NamespacedName{Name: group.Name, Namespace: groupNamespace}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).To(HaveOccurred())

			var updated warpgatev1alpha1.WarpgateTargetGroup
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())
			Expect(updated.Status.ExternalID).To(BeEmpty())

			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("CreateFailed"))
		})
	})

	Context("Empty connectionRef", func() {
		It("should set ClientError condition when connectionRef is empty", func() {
			// Webhooks would normally reject this, but the controller should handle it
			// gracefully if the resource somehow gets through with an empty connectionRef.
			group := &warpgatev1alpha1.WarpgateTargetGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-emptyconn-group",
					Namespace: groupNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateTargetGroupSpec{
					ConnectionRef: "",
					Name:          "empty-conn-group",
				},
			}
			Expect(k8sClient.Create(ctx, group)).To(Succeed())

			nn := types.NamespacedName{Name: group.Name, Namespace: groupNamespace}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).To(HaveOccurred())

			var updated warpgatev1alpha1.WarpgateTargetGroup
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())
			Expect(updated.Status.ExternalID).To(BeEmpty())

			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("ClientError"))
		})
	})

	Context("Invalid color rejected by CRD validation", func() {
		It("should reject a WarpgateTargetGroup with an unsupported color value at the API level", func() {
			// The CRD enforces a color enum; the k8s API server must reject invalid values
			// before they reach the controller, so no reconcile is needed for this path.
			group := &warpgatev1alpha1.WarpgateTargetGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-badcolor-group",
					Namespace: groupNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateTargetGroupSpec{
					ConnectionRef: connName + "-badcolor",
					Name:          "badcolor-group",
					Color:         "Purple", // Not in the supported enum.
				},
			}
			err := k8sClient.Create(ctx, group)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Unsupported value"))
			Expect(err.Error()).To(ContainSubstring("Purple"))
		})
	})

	Context("Target group idempotency (already exists on Warpgate side)", func() {
		It("should update not create when ExternalID is already set, and remain Ready across multiple reconciles", func() {
			createCount := 0
			updateCount := 0
			mux := http.NewServeMux()
			mockLogin(mux)
			mux.HandleFunc("/@warpgate/admin/api/target-groups", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					createCount++
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(warpgate.TargetGroup{
						ID:    "tg-idem-001",
						Name:  "idempotent-group",
						Color: "Success",
					})
				}
			})
			mux.HandleFunc("/@warpgate/admin/api/target-groups/", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPut {
					updateCount++
					// Return the existing group unchanged — simulating an already-synced state.
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(warpgate.TargetGroup{
						ID:    "tg-idem-001",
						Name:  "idempotent-group",
						Color: "Success",
					})
				}
			})
			srv := setupMockAndConnection(mux, "-idem")
			defer srv.Close()

			group := &warpgatev1alpha1.WarpgateTargetGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-idem-group",
					Namespace: groupNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateTargetGroupSpec{
					ConnectionRef: connName + "-idem",
					Name:          "idempotent-group",
					Color:         "Success",
				},
			}
			Expect(k8sClient.Create(ctx, group)).To(Succeed())

			nn := types.NamespacedName{Name: group.Name, Namespace: groupNamespace}

			// Reconcile 1: no ExternalID — should POST (create).
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			var afterCreate warpgatev1alpha1.WarpgateTargetGroup
			Expect(k8sClient.Get(ctx, nn, &afterCreate)).To(Succeed())
			Expect(afterCreate.Status.ExternalID).To(Equal("tg-idem-001"))
			Expect(createCount).To(Equal(1))

			// Reconcile 2 and 3: ExternalID is set — should PUT (update), not POST again.
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			Expect(createCount).To(Equal(1)) // POST called exactly once.
			Expect(updateCount).To(Equal(2)) // PUT called for the two subsequent reconciles.

			var updated warpgatev1alpha1.WarpgateTargetGroup
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())
			Expect(updated.Status.ExternalID).To(Equal("tg-idem-001"))

			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(readyCond.Reason).To(Equal("Reconciled"))
		})
	})
})
