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

var _ = Describe("WarpgateUser Controller", func() {
	const (
		userNamespace = "user-test-ns"
		connName      = "user-test-conn"
		secretName    = "user-test-token"
		tokenKey      = "token"
		tokenValue    = "test-api-token"
	)

	var (
		reconciler *WarpgateUserReconciler
		ns         *corev1.Namespace
	)

	BeforeEach(func() {
		reconciler = &WarpgateUserReconciler{
			Client: k8sClient,
			Scheme: scheme.Scheme,
		}

		ns = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: userNamespace,
			},
		}
		_ = k8sClient.Create(ctx, ns)
	})

	setupMockAndConnection := func(mux *http.ServeMux, suffix string) *httptest.Server {
		srv := httptest.NewServer(mux)

		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName + suffix,
				Namespace: userNamespace,
			},
			StringData: map[string]string{
				tokenKey: tokenValue,
			},
		}
		Expect(k8sClient.Create(ctx, secret)).To(Succeed())

		conn := &warpgatev1alpha1.WarpgateConnection{
			ObjectMeta: metav1.ObjectMeta{
				Name:      connName + suffix,
				Namespace: userNamespace,
			},
			Spec: warpgatev1alpha1.WarpgateConnectionSpec{
				Host:               srv.URL,
				TokenSecretRef:     warpgatev1alpha1.SecretKeyRef{Name: secretName + suffix, Key: tokenKey},
				InsecureSkipVerify: true,
			},
		}
		Expect(k8sClient.Create(ctx, conn)).To(Succeed())

		return srv
	}

	boolPtr := func(b bool) *bool { return &b }

	Context("Create user", func() {
		It("should create the user in Warpgate and set ExternalID and Ready condition", func() {
			mux := http.NewServeMux()
			mux.HandleFunc("/@warpgate/admin/api/users", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(warpgate.User{ID: "user-create-001", Username: "testuser"})
				}
			})
			// The password credential endpoint — user has generatePassword defaulting to true.
			mux.HandleFunc("/@warpgate/admin/api/users/user-create-001/credentials/passwords", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(warpgate.PasswordCredential{ID: "cred-create-001", Password: "generated-pw"})
				}
			})
			srv := setupMockAndConnection(mux, "-create")
			defer srv.Close()

			user := &warpgatev1alpha1.WarpgateUser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-create-user",
					Namespace: userNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateUserSpec{
					ConnectionRef:    connName + "-create",
					Username:         "testuser",
					Description:      "a test user",
					GeneratePassword: boolPtr(false), // keep this simple — no password gen
				},
			}
			Expect(k8sClient.Create(ctx, user)).To(Succeed())

			nn := types.NamespacedName{Name: user.Name, Namespace: userNamespace}

			// Single reconcile: adds finalizer, creates user, updates status — all in one pass.
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			var updated warpgatev1alpha1.WarpgateUser
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())
			Expect(updated.Status.ExternalID).To(Equal("user-create-001"))

			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
		})
	})

	Context("Create user with auto-generated password", func() {
		It("should create the user, generate a password credential, and store it in a Secret", func() {
			mux := http.NewServeMux()
			mux.HandleFunc("/@warpgate/admin/api/users", func(w http.ResponseWriter, r *http.Request) {
				// Handle both POST /users (create) and other list calls.
				if r.Method == http.MethodPost {
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(warpgate.User{ID: "user-pwgen-001", Username: "pwgenuser"})
					return
				}
			})
			mux.HandleFunc("/@warpgate/admin/api/users/user-pwgen-001/credentials/passwords", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(warpgate.PasswordCredential{ID: "cred-pwgen-001", Password: "super-secret-pw"})
				}
			})
			// Update path for subsequent reconciles.
			mux.HandleFunc("/@warpgate/admin/api/users/user-pwgen-001", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPut {
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(warpgate.User{ID: "user-pwgen-001", Username: "pwgenuser"})
				}
			})
			srv := setupMockAndConnection(mux, "-pwgen")
			defer srv.Close()

			user := &warpgatev1alpha1.WarpgateUser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pwgen-user",
					Namespace: userNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateUserSpec{
					ConnectionRef:    connName + "-pwgen",
					Username:         "pwgenuser",
					GeneratePassword: boolPtr(true),
				},
			}
			Expect(k8sClient.Create(ctx, user)).To(Succeed())

			nn := types.NamespacedName{Name: user.Name, Namespace: userNamespace}

			// Single reconcile: adds finalizer, creates user + password credential — all in one pass.
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			var updated warpgatev1alpha1.WarpgateUser
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())
			Expect(updated.Status.ExternalID).To(Equal("user-pwgen-001"))
			Expect(updated.Status.PasswordCredentialID).To(Equal("cred-pwgen-001"))
			Expect(updated.Status.PasswordSecretRef).To(Equal("test-pwgen-user-password"))

			// Verify the password Secret was created.
			var pwSecret corev1.Secret
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "test-pwgen-user-password",
				Namespace: userNamespace,
			}, &pwSecret)).To(Succeed())
			Expect(string(pwSecret.Data["username"])).To(Equal("pwgenuser"))
			// The password is randomly generated by the controller, not the mock response.
			Expect(pwSecret.Data["password"]).NotTo(BeEmpty())

			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
		})
	})

	Context("Create user with generatePassword=false", func() {
		It("should not create a password credential or Secret", func() {
			mux := http.NewServeMux()
			passwordEndpointCalled := false
			mux.HandleFunc("/@warpgate/admin/api/users", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(warpgate.User{ID: "user-nopw-001", Username: "nopwuser"})
				}
			})
			mux.HandleFunc("/@warpgate/admin/api/users/user-nopw-001/credentials/passwords", func(w http.ResponseWriter, r *http.Request) {
				passwordEndpointCalled = true
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(warpgate.PasswordCredential{ID: "should-not-exist"})
			})
			srv := setupMockAndConnection(mux, "-nopw")
			defer srv.Close()

			user := &warpgatev1alpha1.WarpgateUser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-nopw-user",
					Namespace: userNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateUserSpec{
					ConnectionRef:    connName + "-nopw",
					Username:         "nopwuser",
					GeneratePassword: boolPtr(false),
				},
			}
			Expect(k8sClient.Create(ctx, user)).To(Succeed())

			nn := types.NamespacedName{Name: user.Name, Namespace: userNamespace}

			// Single reconcile: adds finalizer + creates user in one pass.
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			Expect(passwordEndpointCalled).To(BeFalse())

			var updated warpgatev1alpha1.WarpgateUser
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())
			Expect(updated.Status.ExternalID).To(Equal("user-nopw-001"))
			Expect(updated.Status.PasswordCredentialID).To(BeEmpty())
			Expect(updated.Status.PasswordSecretRef).To(BeEmpty())
		})
	})

	Context("Update user", func() {
		It("should update the user in Warpgate with credential policy", func() {
			var receivedBody map[string]any
			mux := http.NewServeMux()
			mux.HandleFunc("/@warpgate/admin/api/users", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(warpgate.User{ID: "user-upd-001", Username: "upduser"})
				}
			})
			mux.HandleFunc("/@warpgate/admin/api/users/user-upd-001", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPut {
					_ = json.NewDecoder(r.Body).Decode(&receivedBody)
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(warpgate.User{
						ID:       "user-upd-001",
						Username: "upduser",
						CredentialPolicy: &warpgate.CredentialPolicy{
							HTTP: []string{"Password"},
							SSH:  []string{"PublicKey"},
						},
					})
				}
			})
			srv := setupMockAndConnection(mux, "-upd")
			defer srv.Close()

			user := &warpgatev1alpha1.WarpgateUser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-upd-user",
					Namespace: userNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateUserSpec{
					ConnectionRef:    connName + "-upd",
					Username:         "upduser",
					GeneratePassword: boolPtr(false),
					CredentialPolicy: &warpgatev1alpha1.CredentialPolicySpec{
						HTTP: []string{"Password"},
						SSH:  []string{"PublicKey"},
					},
				},
			}
			Expect(k8sClient.Create(ctx, user)).To(Succeed())

			nn := types.NamespacedName{Name: user.Name, Namespace: userNamespace}

			// First reconcile: adds finalizer + creates user in one pass.
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			// Second reconcile: ExternalID is set, triggers update path (PUT).
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// Verify the update payload included credential_policy.
			Expect(receivedBody).NotTo(BeNil())
			cp, ok := receivedBody["credential_policy"].(map[string]any)
			Expect(ok).To(BeTrue())
			Expect(cp["http"]).To(ContainElement("Password"))
			Expect(cp["ssh"]).To(ContainElement("PublicKey"))

			var updated warpgatev1alpha1.WarpgateUser
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())
			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
		})
	})

	Context("Delete user", func() {
		It("should clean up password credential, Secret, and user in Warpgate", func() {
			userDeleteCalled := false
			credDeleteCalled := false
			mux := http.NewServeMux()
			mux.HandleFunc("/@warpgate/admin/api/users", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(warpgate.User{ID: "user-del-001", Username: "deluser"})
				}
			})
			mux.HandleFunc("/@warpgate/admin/api/users/user-del-001/credentials/passwords", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(warpgate.PasswordCredential{ID: "cred-del-001", Password: "deleteme"})
				}
			})
			mux.HandleFunc("/@warpgate/admin/api/users/user-del-001/credentials/passwords/cred-del-001", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodDelete {
					credDeleteCalled = true
					w.WriteHeader(http.StatusNoContent)
				}
			})
			mux.HandleFunc("/@warpgate/admin/api/users/user-del-001", func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case http.MethodDelete:
					userDeleteCalled = true
					w.WriteHeader(http.StatusNoContent)
				case http.MethodPut:
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(warpgate.User{ID: "user-del-001", Username: "deluser"})
				}
			})
			srv := setupMockAndConnection(mux, "-del")
			defer srv.Close()

			user := &warpgatev1alpha1.WarpgateUser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-del-user",
					Namespace: userNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateUserSpec{
					ConnectionRef:    connName + "-del",
					Username:         "deluser",
					GeneratePassword: boolPtr(true),
				},
			}
			Expect(k8sClient.Create(ctx, user)).To(Succeed())

			nn := types.NamespacedName{Name: user.Name, Namespace: userNamespace}

			// Single reconcile: adds finalizer, creates user + password credential — all in one pass.
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// Verify password secret exists.
			var pwSecret corev1.Secret
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "test-del-user-password",
				Namespace: userNamespace,
			}, &pwSecret)).To(Succeed())

			// Delete the user CR.
			Expect(k8sClient.Delete(ctx, user)).To(Succeed())

			// Reconcile deletion.
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			Expect(credDeleteCalled).To(BeTrue())
			Expect(userDeleteCalled).To(BeTrue())

			// The CR should be gone.
			var deleted warpgatev1alpha1.WarpgateUser
			err = k8sClient.Get(ctx, nn, &deleted)
			Expect(err).To(HaveOccurred())

			// The password Secret should also be gone (cleaned up by the finalizer).
			var deletedSecret corev1.Secret
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      "test-del-user-password",
				Namespace: userNamespace,
			}, &deletedSecret)
			Expect(err).To(HaveOccurred())
		})
	})

	Context("Create user with custom passwordLength", func() {
		It("should generate a password of the specified length", func() {
			mux := http.NewServeMux()
			mux.HandleFunc("/@warpgate/admin/api/users", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(warpgate.User{ID: "user-pwlen-001", Username: "pwlenuser"})
				}
			})
			mux.HandleFunc("/@warpgate/admin/api/users/user-pwlen-001/credentials/passwords", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(warpgate.PasswordCredential{ID: "cred-pwlen-001", Password: "pw"})
				}
			})
			mux.HandleFunc("/@warpgate/admin/api/users/user-pwlen-001", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPut {
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(warpgate.User{ID: "user-pwlen-001", Username: "pwlenuser"})
				}
			})
			srv := setupMockAndConnection(mux, "-pwlen")
			defer srv.Close()

			customLen := 64
			user := &warpgatev1alpha1.WarpgateUser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pwlen-user",
					Namespace: userNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateUserSpec{
					ConnectionRef:    connName + "-pwlen",
					Username:         "pwlenuser",
					GeneratePassword: boolPtr(true),
					PasswordLength:   &customLen,
				},
			}
			Expect(k8sClient.Create(ctx, user)).To(Succeed())

			nn := types.NamespacedName{Name: user.Name, Namespace: userNamespace}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			var updated warpgatev1alpha1.WarpgateUser
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())
			Expect(updated.Status.PasswordCredentialID).To(Equal("cred-pwlen-001"))
			Expect(updated.Status.PasswordSecretRef).To(Equal("test-pwlen-user-password"))

			// Verify the generated password in the Secret is the right length.
			var pwSecret corev1.Secret
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      "test-pwlen-user-password",
				Namespace: userNamespace,
			}, &pwSecret)).To(Succeed())
			Expect(string(pwSecret.Data["password"])).To(HaveLen(customLen))
		})
	})

	Context("Password secret already exists", func() {
		It("should not fail when the password Secret already exists (AlreadyExists is tolerated)", func() {
			mux := http.NewServeMux()
			mux.HandleFunc("/@warpgate/admin/api/users", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(warpgate.User{ID: "user-dupsec-001", Username: "dupsecuser"})
				}
			})
			mux.HandleFunc("/@warpgate/admin/api/users/user-dupsec-001/credentials/passwords", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(warpgate.PasswordCredential{ID: "cred-dupsec-001", Password: "pw"})
				}
			})
			srv := setupMockAndConnection(mux, "-dupsec")
			defer srv.Close()

			// Pre-create the password Secret that the controller would also try to create.
			existingSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dupsec-user-password",
					Namespace: userNamespace,
				},
				Data: map[string][]byte{
					"password": []byte("pre-existing"),
				},
			}
			Expect(k8sClient.Create(ctx, existingSecret)).To(Succeed())

			user := &warpgatev1alpha1.WarpgateUser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-dupsec-user",
					Namespace: userNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateUserSpec{
					ConnectionRef:    connName + "-dupsec",
					Username:         "dupsecuser",
					GeneratePassword: boolPtr(true),
				},
			}
			Expect(k8sClient.Create(ctx, user)).To(Succeed())

			nn := types.NamespacedName{Name: user.Name, Namespace: userNamespace}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			var updated warpgatev1alpha1.WarpgateUser
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())
			Expect(updated.Status.PasswordCredentialID).To(Equal("cred-dupsec-001"))

			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
		})
	})

	Context("Create user API error", func() {
		It("should set Ready=False with CreateFailed when the API returns an error", func() {
			mux := http.NewServeMux()
			mux.HandleFunc("/@warpgate/admin/api/users", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte(`{"error":"internal"}`))
					return
				}
			})
			srv := setupMockAndConnection(mux, "-createfail")
			defer srv.Close()

			user := &warpgatev1alpha1.WarpgateUser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-createfail-user",
					Namespace: userNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateUserSpec{
					ConnectionRef:    connName + "-createfail",
					Username:         "failuser",
					GeneratePassword: boolPtr(false),
				},
			}
			Expect(k8sClient.Create(ctx, user)).To(Succeed())

			nn := types.NamespacedName{Name: user.Name, Namespace: userNamespace}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).To(HaveOccurred())

			var updated warpgatev1alpha1.WarpgateUser
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())
			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("CreateFailed"))
		})
	})

	Context("Update user 404 triggers recreate", func() {
		It("should clear ExternalID and requeue when update returns 404", func() {
			mux := http.NewServeMux()
			mux.HandleFunc("/@warpgate/admin/api/users", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(warpgate.User{ID: "user-404-001", Username: "user404"})
				}
			})
			mux.HandleFunc("/@warpgate/admin/api/users/user-404-001", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPut {
					w.WriteHeader(http.StatusNotFound)
					_, _ = w.Write([]byte(`{"error":"not found"}`))
					return
				}
			})
			srv := setupMockAndConnection(mux, "-user404")
			defer srv.Close()

			user := &warpgatev1alpha1.WarpgateUser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-user404",
					Namespace: userNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateUserSpec{
					ConnectionRef:    connName + "-user404",
					Username:         "user404",
					GeneratePassword: boolPtr(false),
				},
			}
			Expect(k8sClient.Create(ctx, user)).To(Succeed())

			nn := types.NamespacedName{Name: user.Name, Namespace: userNamespace}

			// First reconcile: create.
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile: update returns 404.
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(Equal(reconcile.Result{}))

			var updated warpgatev1alpha1.WarpgateUser
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())
			Expect(updated.Status.ExternalID).To(BeEmpty())

			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("NotFound"))
		})
	})

	Context("Update user non-404 error", func() {
		It("should set Ready=False with UpdateFailed for non-404 API errors", func() {
			mux := http.NewServeMux()
			mux.HandleFunc("/@warpgate/admin/api/users", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(warpgate.User{ID: "user-500-001", Username: "user500"})
				}
			})
			mux.HandleFunc("/@warpgate/admin/api/users/user-500-001", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPut {
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte(`{"error":"internal"}`))
					return
				}
			})
			srv := setupMockAndConnection(mux, "-user500")
			defer srv.Close()

			user := &warpgatev1alpha1.WarpgateUser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-user500",
					Namespace: userNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateUserSpec{
					ConnectionRef:    connName + "-user500",
					Username:         "user500",
					GeneratePassword: boolPtr(false),
				},
			}
			Expect(k8sClient.Create(ctx, user)).To(Succeed())

			nn := types.NamespacedName{Name: user.Name, Namespace: userNamespace}

			// First reconcile: create.
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile: update fails.
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).To(HaveOccurred())

			var updated warpgatev1alpha1.WarpgateUser
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())
			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("UpdateFailed"))
		})
	})

	Context("Missing connection", func() {
		It("should return an error when the WarpgateConnection does not exist", func() {
			user := &warpgatev1alpha1.WarpgateUser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-missing-conn-user",
					Namespace: userNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateUserSpec{
					ConnectionRef:    "nonexistent-user-connection",
					Username:         "ghost",
					GeneratePassword: boolPtr(false),
				},
			}
			Expect(k8sClient.Create(ctx, user)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: user.Name, Namespace: userNamespace},
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("nonexistent-user-connection"))

			var updated warpgatev1alpha1.WarpgateUser
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: user.Name, Namespace: userNamespace}, &updated)).To(Succeed())
			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("ClientError"))
		})
	})
})

// Ensure the reconciler satisfies the interface.
var _ reconcile.Reconciler = &WarpgateUserReconciler{}
