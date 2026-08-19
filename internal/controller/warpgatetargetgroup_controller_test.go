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
})
