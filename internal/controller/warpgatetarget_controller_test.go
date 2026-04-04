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
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	warpgatev1alpha1 "github.com/thereisnotime/warpgate-operator/api/v1alpha1"
)

var _ = Describe("WarpgateTarget Controller", func() {

	var (
		reconciler *WarpgateTargetReconciler
	)

	BeforeEach(func() {
		reconciler = &WarpgateTargetReconciler{
			Client: k8sClient,
			Scheme: k8sClient.Scheme(),
		}
	})

	// helper: create a token secret and WarpgateConnection pointing at the mock server
	setupConnection := func(namespace, secretName, connName, serverURL string) {
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
				Host:               serverURL,
				AuthSecretRef:      warpgatev1alpha1.AuthSecretRef{Name: secretName},
				InsecureSkipVerify: true,
			},
		}
		Expect(k8sClient.Create(ctx, conn)).To(Succeed())
	}

	// helper: tear down connection resources
	cleanupConnection := func(namespace, secretName, connName string) {
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
	}

	// helper: tear down a target CR
	cleanupTarget := func(namespace, name string) {
		target := &warpgatev1alpha1.WarpgateTarget{}
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, target); err == nil {
			controllerutil.RemoveFinalizer(target, targetFinalizerName)
			_ = k8sClient.Update(ctx, target)
			_ = k8sClient.Delete(ctx, target)
		}
	}

	Context("Create SSH target", func() {
		var (
			mockServer *httptest.Server
			namespace  = testNamespace
			secretName = "wg-token-target-ssh"
			connName   = "wg-conn-target-ssh"
			targetName = "target-ssh-test"
		)

		BeforeEach(func() {
			mux := http.NewServeMux()
			mockLogin(mux)
			mux.HandleFunc("/@warpgate/admin/api/targets", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusCreated)
					_ = json.NewEncoder(w).Encode(map[string]any{
						"id":      "target-ssh-123",
						"name":    "my-ssh-target",
						"options": json.RawMessage(`{"kind":"Ssh"}`),
					})
					return
				}
				http.NotFound(w, r)
			})
			mockServer = httptest.NewServer(mux)
			setupConnection(namespace, secretName, connName, mockServer.URL)
		})

		AfterEach(func() {
			mockServer.Close()
			cleanupTarget(namespace, targetName)
			cleanupConnection(namespace, secretName, connName)
		})

		It("should create the target and set ExternalID and Ready=True", func() {
			target := &warpgatev1alpha1.WarpgateTarget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      targetName,
					Namespace: namespace,
				},
				Spec: warpgatev1alpha1.WarpgateTargetSpec{
					ConnectionRef: connName,
					Name:          "my-ssh-target",
					SSH: &warpgatev1alpha1.SSHTargetSpec{
						Host:     "10.0.0.1",
						Port:     22,
						Username: "admin",
						AuthKind: "PublicKey",
					},
				},
			}
			Expect(k8sClient.Create(ctx, target)).To(Succeed())

			nn := types.NamespacedName{Name: targetName, Namespace: namespace}

			// Single reconcile: adds finalizer + creates target — all in one pass.
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			var updated warpgatev1alpha1.WarpgateTarget
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())
			Expect(updated.Status.ExternalID).To(Equal("target-ssh-123"))

			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(readyCond.Reason).To(Equal("SSH"))
		})
	})

	Context("Create HTTP target", func() {
		var (
			mockServer *httptest.Server
			namespace  = testNamespace
			secretName = "wg-token-target-http"
			connName   = "wg-conn-target-http"
			targetName = "target-http-test"
		)

		BeforeEach(func() {
			mux := http.NewServeMux()
			mockLogin(mux)
			mux.HandleFunc("/@warpgate/admin/api/targets", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					// Read the request body and verify options marshalled correctly.
					body, _ := io.ReadAll(r.Body)
					var req map[string]json.RawMessage
					Expect(json.Unmarshal(body, &req)).To(Succeed())

					var opts map[string]any
					Expect(json.Unmarshal(req["options"], &opts)).To(Succeed())
					Expect(opts["kind"]).To(Equal("Http"))
					Expect(opts["url"]).To(Equal("https://internal-app.example.com"))

					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusCreated)
					_ = json.NewEncoder(w).Encode(map[string]any{
						"id":      "target-http-456",
						"name":    "my-http-target",
						"options": json.RawMessage(`{"kind":"Http"}`),
					})
					return
				}
				http.NotFound(w, r)
			})
			mockServer = httptest.NewServer(mux)
			setupConnection(namespace, secretName, connName, mockServer.URL)
		})

		AfterEach(func() {
			mockServer.Close()
			cleanupTarget(namespace, targetName)
			cleanupConnection(namespace, secretName, connName)
		})

		It("should create an HTTP target with correct options", func() {
			target := &warpgatev1alpha1.WarpgateTarget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      targetName,
					Namespace: namespace,
				},
				Spec: warpgatev1alpha1.WarpgateTargetSpec{
					ConnectionRef: connName,
					Name:          "my-http-target",
					HTTP: &warpgatev1alpha1.HTTPTargetSpec{
						URL: "https://internal-app.example.com",
						Headers: map[string]string{
							"X-Custom": "value",
						},
						ExternalHost: "app.example.com",
					},
				},
			}
			Expect(k8sClient.Create(ctx, target)).To(Succeed())

			nn := types.NamespacedName{Name: targetName, Namespace: namespace}

			// Single reconcile: adds finalizer + creates target — all in one pass.
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			var updated warpgatev1alpha1.WarpgateTarget
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())
			Expect(updated.Status.ExternalID).To(Equal("target-http-456"))

			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(readyCond.Reason).To(Equal("HTTP"))
		})
	})

	Context("Update target", func() {
		var (
			mockServer *httptest.Server
			namespace  = testNamespace
			secretName = "wg-token-target-update"
			connName   = "wg-conn-target-update"
			targetName = "target-update-test"
			callCount  int
			mu         sync.Mutex
		)

		BeforeEach(func() {
			callCount = 0

			mux := http.NewServeMux()
			mockLogin(mux)
			mux.HandleFunc("/@warpgate/admin/api/targets", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusCreated)
					_ = json.NewEncoder(w).Encode(map[string]any{
						"id":      "target-upd-789",
						"name":    "my-update-target",
						"options": json.RawMessage(`{"kind":"Ssh"}`),
					})
					return
				}
				http.NotFound(w, r)
			})
			mux.HandleFunc("/@warpgate/admin/api/targets/", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPut {
					mu.Lock()
					callCount++
					mu.Unlock()

					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(map[string]any{
						"id":      "target-upd-789",
						"name":    "my-update-target",
						"options": json.RawMessage(`{"kind":"Ssh"}`),
					})
					return
				}
				http.NotFound(w, r)
			})
			mockServer = httptest.NewServer(mux)
			setupConnection(namespace, secretName, connName, mockServer.URL)
		})

		AfterEach(func() {
			mockServer.Close()
			cleanupTarget(namespace, targetName)
			cleanupConnection(namespace, secretName, connName)
		})

		It("should update an existing target and keep Ready=True", func() {
			target := &warpgatev1alpha1.WarpgateTarget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      targetName,
					Namespace: namespace,
				},
				Spec: warpgatev1alpha1.WarpgateTargetSpec{
					ConnectionRef: connName,
					Name:          "my-update-target",
					SSH: &warpgatev1alpha1.SSHTargetSpec{
						Host:     "10.0.0.2",
						Port:     22,
						Username: "admin",
						AuthKind: "PublicKey",
					},
				},
			}
			Expect(k8sClient.Create(ctx, target)).To(Succeed())

			nn := types.NamespacedName{Name: targetName, Namespace: namespace}

			// Reconcile 1: adds finalizer + creates the target (POST) in one pass.
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// Reconcile 2: ExternalID is set, triggers update path (PUT).
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			mu.Lock()
			Expect(callCount).To(Equal(1))
			mu.Unlock()

			var updated warpgatev1alpha1.WarpgateTarget
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())
			Expect(updated.Status.ExternalID).To(Equal("target-upd-789"))

			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
		})
	})

	Context("Delete target", func() {
		var (
			mockServer   *httptest.Server
			namespace    = testNamespace
			secretName   = "wg-token-target-del"
			connName     = "wg-conn-target-del"
			targetName   = "target-delete-test"
			deleteCalled bool
			mu           sync.Mutex
		)

		BeforeEach(func() {
			deleteCalled = false

			mux := http.NewServeMux()
			mockLogin(mux)
			mux.HandleFunc("/@warpgate/admin/api/targets", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusCreated)
					_ = json.NewEncoder(w).Encode(map[string]any{
						"id":      "target-del-abc",
						"name":    "my-del-target",
						"options": json.RawMessage(`{"kind":"Ssh"}`),
					})
					return
				}
				http.NotFound(w, r)
			})
			mux.HandleFunc("/@warpgate/admin/api/targets/", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodDelete {
					mu.Lock()
					deleteCalled = true
					mu.Unlock()
					w.WriteHeader(http.StatusNoContent)
					return
				}
				http.NotFound(w, r)
			})
			mockServer = httptest.NewServer(mux)
			setupConnection(namespace, secretName, connName, mockServer.URL)
		})

		AfterEach(func() {
			mockServer.Close()
			cleanupTarget(namespace, targetName)
			cleanupConnection(namespace, secretName, connName)
		})

		It("should delete the target in Warpgate and remove the finalizer", func() {
			target := &warpgatev1alpha1.WarpgateTarget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      targetName,
					Namespace: namespace,
				},
				Spec: warpgatev1alpha1.WarpgateTargetSpec{
					ConnectionRef: connName,
					Name:          "my-del-target",
					SSH: &warpgatev1alpha1.SSHTargetSpec{
						Host:     "10.0.0.3",
						Port:     22,
						Username: "admin",
						AuthKind: "PublicKey",
					},
				},
			}
			Expect(k8sClient.Create(ctx, target)).To(Succeed())

			nn := types.NamespacedName{Name: targetName, Namespace: namespace}

			// Single reconcile: adds finalizer + creates the target in one pass.
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// Verify target was created.
			var created warpgatev1alpha1.WarpgateTarget
			Expect(k8sClient.Get(ctx, nn, &created)).To(Succeed())
			Expect(created.Status.ExternalID).To(Equal("target-del-abc"))

			// Delete the CR (finalizer will hold it).
			Expect(k8sClient.Delete(ctx, &created)).To(Succeed())

			// Reconcile 3: processes deletion.
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// The Warpgate API DELETE should have been called.
			mu.Lock()
			Expect(deleteCalled).To(BeTrue())
			mu.Unlock()

			// The resource should be fully gone.
			var deleted warpgatev1alpha1.WarpgateTarget
			err = k8sClient.Get(ctx, nn, &deleted)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))
		})
	})

	Context("SSH target with password from secret", func() {
		var (
			mockServer   *httptest.Server
			namespace    = testNamespace
			secretName   = "wg-token-target-sshpw"
			connName     = "wg-conn-target-sshpw"
			targetName   = "target-sshpw-test"
			pwSecretName = "ssh-password-secret"
			capturedBody []byte
			mu           sync.Mutex
		)

		BeforeEach(func() {
			capturedBody = nil

			mux := http.NewServeMux()
			mockLogin(mux)
			mux.HandleFunc("/@warpgate/admin/api/targets", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					mu.Lock()
					capturedBody, _ = io.ReadAll(r.Body)
					mu.Unlock()

					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusCreated)
					_ = json.NewEncoder(w).Encode(map[string]any{
						"id":      "target-sshpw-999",
						"name":    "my-sshpw-target",
						"options": json.RawMessage(`{"kind":"Ssh"}`),
					})
					return
				}
				http.NotFound(w, r)
			})
			mockServer = httptest.NewServer(mux)
			setupConnection(namespace, secretName, connName, mockServer.URL)

			// Create the password secret that the target will reference.
			pwSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      pwSecretName,
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"password": []byte("super-secret-pw"),
				},
			}
			Expect(k8sClient.Create(ctx, pwSecret)).To(Succeed())
		})

		AfterEach(func() {
			mockServer.Close()
			cleanupTarget(namespace, targetName)
			cleanupConnection(namespace, secretName, connName)

			pwSecret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: pwSecretName, Namespace: namespace}, pwSecret); err == nil {
				_ = k8sClient.Delete(ctx, pwSecret)
			}
		})

		It("should read the password from the referenced secret and include it in the API call", func() {
			target := &warpgatev1alpha1.WarpgateTarget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      targetName,
					Namespace: namespace,
				},
				Spec: warpgatev1alpha1.WarpgateTargetSpec{
					ConnectionRef: connName,
					Name:          "my-sshpw-target",
					SSH: &warpgatev1alpha1.SSHTargetSpec{
						Host:     "10.0.0.4",
						Port:     22,
						Username: "admin",
						AuthKind: "Password",
						PasswordSecretRef: &warpgatev1alpha1.SecretKeyRef{
							Name: pwSecretName,
							Key:  "password",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, target)).To(Succeed())

			nn := types.NamespacedName{Name: targetName, Namespace: namespace}

			// Single reconcile: adds finalizer + creates target in one pass.
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			var updated warpgatev1alpha1.WarpgateTarget
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())
			Expect(updated.Status.ExternalID).To(Equal("target-sshpw-999"))

			// Verify the password was sent in the API request body.
			mu.Lock()
			defer mu.Unlock()
			Expect(capturedBody).NotTo(BeNil())

			var req map[string]json.RawMessage
			Expect(json.Unmarshal(capturedBody, &req)).To(Succeed())

			var opts map[string]any
			Expect(json.Unmarshal(req["options"], &opts)).To(Succeed())
			Expect(opts["kind"]).To(Equal("Ssh"))

			auth, ok := opts["auth"].(map[string]any)
			Expect(ok).To(BeTrue())
			Expect(auth["kind"]).To(Equal("Password"))
			Expect(auth["password"]).To(Equal("super-secret-pw"))
		})
	})

	Context("Create MySQL target", func() {
		var (
			mockServer   *httptest.Server
			namespace    = testNamespace
			secretName   = "wg-token-target-mysql"
			connName     = "wg-conn-target-mysql"
			targetName   = "target-mysql-test"
			pwSecretName = "mysql-password-secret"
			capturedBody []byte
			mu           sync.Mutex
		)

		BeforeEach(func() {
			capturedBody = nil

			mux := http.NewServeMux()
			mockLogin(mux)
			mux.HandleFunc("/@warpgate/admin/api/targets", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					mu.Lock()
					capturedBody, _ = io.ReadAll(r.Body)
					mu.Unlock()

					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusCreated)
					_ = json.NewEncoder(w).Encode(map[string]any{
						"id":      "target-mysql-001",
						"name":    "my-mysql-target",
						"options": json.RawMessage(`{"kind":"MySql"}`),
					})
					return
				}
				http.NotFound(w, r)
			})
			mockServer = httptest.NewServer(mux)
			setupConnection(namespace, secretName, connName, mockServer.URL)

			pwSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      pwSecretName,
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"password": []byte("mysql-root-pw"),
				},
			}
			Expect(k8sClient.Create(ctx, pwSecret)).To(Succeed())
		})

		AfterEach(func() {
			mockServer.Close()
			cleanupTarget(namespace, targetName)
			cleanupConnection(namespace, secretName, connName)
			pwSecret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: pwSecretName, Namespace: namespace}, pwSecret); err == nil {
				_ = k8sClient.Delete(ctx, pwSecret)
			}
		})

		It("should create a MySQL target with password and TLS options", func() {
			target := &warpgatev1alpha1.WarpgateTarget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      targetName,
					Namespace: namespace,
				},
				Spec: warpgatev1alpha1.WarpgateTargetSpec{
					ConnectionRef: connName,
					Name:          "my-mysql-target",
					MySQL: &warpgatev1alpha1.MySQLTargetSpec{
						Host:     "db.example.com",
						Port:     3306,
						Username: "root",
						PasswordSecretRef: &warpgatev1alpha1.SecretKeyRef{
							Name: pwSecretName,
							Key:  "password",
						},
						TLS: &warpgatev1alpha1.TLSConfigSpec{
							Mode:   "Required",
							Verify: true,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, target)).To(Succeed())

			nn := types.NamespacedName{Name: targetName, Namespace: namespace}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			var updated warpgatev1alpha1.WarpgateTarget
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())
			Expect(updated.Status.ExternalID).To(Equal("target-mysql-001"))

			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(readyCond.Reason).To(Equal("MySQL"))

			// Verify the API payload.
			mu.Lock()
			defer mu.Unlock()
			Expect(capturedBody).NotTo(BeNil())

			var req map[string]json.RawMessage
			Expect(json.Unmarshal(capturedBody, &req)).To(Succeed())

			var opts map[string]any
			Expect(json.Unmarshal(req["options"], &opts)).To(Succeed())
			Expect(opts["kind"]).To(Equal("MySql"))
			Expect(opts["host"]).To(Equal("db.example.com"))
			Expect(opts["port"]).To(BeNumerically("==", 3306))
			Expect(opts["username"]).To(Equal("root"))
			Expect(opts["password"]).To(Equal("mysql-root-pw"))

			tlsCfg, ok := opts["tls"].(map[string]any)
			Expect(ok).To(BeTrue())
			Expect(tlsCfg["mode"]).To(Equal("Required"))
			Expect(tlsCfg["verify"]).To(BeTrue())
		})
	})

	Context("Create PostgreSQL target", func() {
		var (
			mockServer   *httptest.Server
			namespace    = testNamespace
			secretName   = "wg-token-target-pg"
			connName     = "wg-conn-target-pg"
			targetName   = "target-pg-test"
			pwSecretName = "pg-password-secret"
			capturedBody []byte
			mu           sync.Mutex
		)

		BeforeEach(func() {
			capturedBody = nil

			mux := http.NewServeMux()
			mockLogin(mux)
			mux.HandleFunc("/@warpgate/admin/api/targets", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					mu.Lock()
					capturedBody, _ = io.ReadAll(r.Body)
					mu.Unlock()

					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusCreated)
					_ = json.NewEncoder(w).Encode(map[string]any{
						"id":      "target-pg-001",
						"name":    "my-pg-target",
						"options": json.RawMessage(`{"kind":"Postgres"}`),
					})
					return
				}
				http.NotFound(w, r)
			})
			mockServer = httptest.NewServer(mux)
			setupConnection(namespace, secretName, connName, mockServer.URL)

			pwSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      pwSecretName,
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"password": []byte("pg-secret-pw"),
				},
			}
			Expect(k8sClient.Create(ctx, pwSecret)).To(Succeed())
		})

		AfterEach(func() {
			mockServer.Close()
			cleanupTarget(namespace, targetName)
			cleanupConnection(namespace, secretName, connName)
			pwSecret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: pwSecretName, Namespace: namespace}, pwSecret); err == nil {
				_ = k8sClient.Delete(ctx, pwSecret)
			}
		})

		It("should create a PostgreSQL target with password and TLS options", func() {
			target := &warpgatev1alpha1.WarpgateTarget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      targetName,
					Namespace: namespace,
				},
				Spec: warpgatev1alpha1.WarpgateTargetSpec{
					ConnectionRef: connName,
					Name:          "my-pg-target",
					PostgreSQL: &warpgatev1alpha1.PostgreSQLTargetSpec{
						Host:     "pgdb.example.com",
						Port:     5432,
						Username: "postgres",
						PasswordSecretRef: &warpgatev1alpha1.SecretKeyRef{
							Name: pwSecretName,
							Key:  "password",
						},
						TLS: &warpgatev1alpha1.TLSConfigSpec{
							Mode:   "Preferred",
							Verify: false,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, target)).To(Succeed())

			nn := types.NamespacedName{Name: targetName, Namespace: namespace}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			var updated warpgatev1alpha1.WarpgateTarget
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())
			Expect(updated.Status.ExternalID).To(Equal("target-pg-001"))

			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(readyCond.Reason).To(Equal("PostgreSQL"))

			// Verify the API payload.
			mu.Lock()
			defer mu.Unlock()
			Expect(capturedBody).NotTo(BeNil())

			var req map[string]json.RawMessage
			Expect(json.Unmarshal(capturedBody, &req)).To(Succeed())

			var opts map[string]any
			Expect(json.Unmarshal(req["options"], &opts)).To(Succeed())
			Expect(opts["kind"]).To(Equal("Postgres"))
			Expect(opts["host"]).To(Equal("pgdb.example.com"))
			Expect(opts["port"]).To(BeNumerically("==", 5432))
			Expect(opts["username"]).To(Equal("postgres"))
			Expect(opts["password"]).To(Equal("pg-secret-pw"))

			tlsCfg, ok := opts["tls"].(map[string]any)
			Expect(ok).To(BeTrue())
			Expect(tlsCfg["mode"]).To(Equal("Preferred"))
			Expect(tlsCfg["verify"]).To(BeFalse())
		})
	})

	Context("No target type specified", func() {
		var (
			mockServer *httptest.Server
			namespace  = testNamespace
			secretName = "wg-token-target-notype"
			connName   = "wg-conn-target-notype"
			targetName = "target-notype-test"
		)

		BeforeEach(func() {
			mux := http.NewServeMux()
			mockLogin(mux)
			mockServer = httptest.NewServer(mux)
			setupConnection(namespace, secretName, connName, mockServer.URL)
		})

		AfterEach(func() {
			mockServer.Close()
			cleanupTarget(namespace, targetName)
			cleanupConnection(namespace, secretName, connName)
		})

		It("should set Ready=False with BuildError when no target type is set", func() {
			target := &warpgatev1alpha1.WarpgateTarget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      targetName,
					Namespace: namespace,
				},
				Spec: warpgatev1alpha1.WarpgateTargetSpec{
					ConnectionRef: connName,
					Name:          "empty-target",
					// No SSH, HTTP, MySQL, or PostgreSQL set
				},
			}
			Expect(k8sClient.Create(ctx, target)).To(Succeed())

			nn := types.NamespacedName{Name: targetName, Namespace: namespace}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred()) // error is swallowed and set as condition

			var updated warpgatev1alpha1.WarpgateTarget
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())
			Expect(updated.Status.ExternalID).To(BeEmpty())

			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("BuildError"))
			Expect(readyCond.Message).To(ContainSubstring("exactly one target type"))
		})
	})

	Context("Create target API error", func() {
		var (
			mockServer *httptest.Server
			namespace  = testNamespace
			secretName = "wg-token-target-createfail"
			connName   = "wg-conn-target-createfail"
			targetName = "target-createfail-test"
		)

		BeforeEach(func() {
			mux := http.NewServeMux()
			mockLogin(mux)
			mux.HandleFunc("/@warpgate/admin/api/targets", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte(`{"error":"internal"}`))
					return
				}
				http.NotFound(w, r)
			})
			mockServer = httptest.NewServer(mux)
			setupConnection(namespace, secretName, connName, mockServer.URL)
		})

		AfterEach(func() {
			mockServer.Close()
			cleanupTarget(namespace, targetName)
			cleanupConnection(namespace, secretName, connName)
		})

		It("should set Ready=False with CreateError when the API returns an error", func() {
			target := &warpgatev1alpha1.WarpgateTarget{
				ObjectMeta: metav1.ObjectMeta{Name: targetName, Namespace: namespace},
				Spec: warpgatev1alpha1.WarpgateTargetSpec{
					ConnectionRef: connName,
					Name:          "my-fail-target",
					SSH: &warpgatev1alpha1.SSHTargetSpec{
						Host:     "10.0.0.1",
						Port:     22,
						Username: "admin",
						AuthKind: "PublicKey",
					},
				},
			}
			Expect(k8sClient.Create(ctx, target)).To(Succeed())

			nn := types.NamespacedName{Name: targetName, Namespace: namespace}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			var updated warpgatev1alpha1.WarpgateTarget
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())

			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("CreateError"))
		})
	})

	Context("Update target non-404 error", func() {
		var (
			mockServer *httptest.Server
			namespace  = testNamespace
			secretName = "wg-token-target-updfail"
			connName   = "wg-conn-target-updfail"
			targetName = "target-updfail-test"
		)

		BeforeEach(func() {
			mux := http.NewServeMux()
			mockLogin(mux)
			mux.HandleFunc("/@warpgate/admin/api/targets", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusCreated)
					_ = json.NewEncoder(w).Encode(map[string]any{
						"id":      "target-updfail-xyz",
						"name":    "my-updfail-target",
						"options": json.RawMessage(`{"kind":"Ssh"}`),
					})
					return
				}
				http.NotFound(w, r)
			})
			mux.HandleFunc("/@warpgate/admin/api/targets/", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPut {
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte(`{"error":"internal"}`))
					return
				}
				http.NotFound(w, r)
			})
			mockServer = httptest.NewServer(mux)
			setupConnection(namespace, secretName, connName, mockServer.URL)
		})

		AfterEach(func() {
			mockServer.Close()
			cleanupTarget(namespace, targetName)
			cleanupConnection(namespace, secretName, connName)
		})

		It("should set Ready=False with UpdateError for non-404 API errors", func() {
			target := &warpgatev1alpha1.WarpgateTarget{
				ObjectMeta: metav1.ObjectMeta{Name: targetName, Namespace: namespace},
				Spec: warpgatev1alpha1.WarpgateTargetSpec{
					ConnectionRef: connName,
					Name:          "my-updfail-target",
					SSH: &warpgatev1alpha1.SSHTargetSpec{
						Host:     "10.0.0.5",
						Port:     22,
						Username: "admin",
						AuthKind: "PublicKey",
					},
				},
			}
			Expect(k8sClient.Create(ctx, target)).To(Succeed())

			nn := types.NamespacedName{Name: targetName, Namespace: namespace}

			// Reconcile 1: create succeeds.
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// Reconcile 2: update fails with 500.
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			var updated warpgatev1alpha1.WarpgateTarget
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())

			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("UpdateError"))
		})
	})

	Context("Missing connection for target", func() {
		var (
			namespace  = testNamespace
			targetName = "target-noconn-test"
		)

		AfterEach(func() {
			cleanupTarget(namespace, targetName)
		})

		It("should set Ready=False with ClientError when connection doesn't exist", func() {
			target := &warpgatev1alpha1.WarpgateTarget{
				ObjectMeta: metav1.ObjectMeta{Name: targetName, Namespace: namespace},
				Spec: warpgatev1alpha1.WarpgateTargetSpec{
					ConnectionRef: "nonexistent-conn",
					Name:          "orphan-target",
					SSH: &warpgatev1alpha1.SSHTargetSpec{
						Host:     "10.0.0.1",
						Port:     22,
						Username: "admin",
						AuthKind: "PublicKey",
					},
				},
			}
			Expect(k8sClient.Create(ctx, target)).To(Succeed())

			nn := types.NamespacedName{Name: targetName, Namespace: namespace}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			var updated warpgatev1alpha1.WarpgateTarget
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())

			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("ClientError"))
		})
	})

	Context("SSH target with missing password secret", func() {
		var (
			mockServer *httptest.Server
			namespace  = testNamespace
			secretName = "wg-token-target-nosshpw"
			connName   = "wg-conn-target-nosshpw"
			targetName = "target-nosshpw-test"
		)

		BeforeEach(func() {
			mux := http.NewServeMux()
			mockLogin(mux)
			mockServer = httptest.NewServer(mux)
			setupConnection(namespace, secretName, connName, mockServer.URL)
		})

		AfterEach(func() {
			mockServer.Close()
			cleanupTarget(namespace, targetName)
			cleanupConnection(namespace, secretName, connName)
		})

		It("should set Ready=False with BuildError when the password secret doesn't exist", func() {
			target := &warpgatev1alpha1.WarpgateTarget{
				ObjectMeta: metav1.ObjectMeta{Name: targetName, Namespace: namespace},
				Spec: warpgatev1alpha1.WarpgateTargetSpec{
					ConnectionRef: connName,
					Name:          "nosshpw-target",
					SSH: &warpgatev1alpha1.SSHTargetSpec{
						Host:     "10.0.0.1",
						Port:     22,
						Username: "admin",
						AuthKind: "Password",
						PasswordSecretRef: &warpgatev1alpha1.SecretKeyRef{
							Name: "nonexistent-sshpw-secret",
							Key:  "password",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, target)).To(Succeed())

			nn := types.NamespacedName{Name: targetName, Namespace: namespace}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			var updated warpgatev1alpha1.WarpgateTarget
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())

			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("BuildError"))
			Expect(readyCond.Message).To(ContainSubstring("nonexistent-sshpw-secret"))
		})
	})

	Context("SSH target with missing key in password secret", func() {
		var (
			mockServer   *httptest.Server
			namespace    = testNamespace
			secretName   = "wg-token-target-badkey"
			connName     = "wg-conn-target-badkey"
			targetName   = "target-badkey-test"
			pwSecretName = "ssh-badkey-secret"
		)

		BeforeEach(func() {
			mux := http.NewServeMux()
			mockLogin(mux)
			mockServer = httptest.NewServer(mux)
			setupConnection(namespace, secretName, connName, mockServer.URL)

			pwSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      pwSecretName,
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"wrong-key": []byte("some-pw"),
				},
			}
			Expect(k8sClient.Create(ctx, pwSecret)).To(Succeed())
		})

		AfterEach(func() {
			mockServer.Close()
			cleanupTarget(namespace, targetName)
			cleanupConnection(namespace, secretName, connName)
			pwSecret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: pwSecretName, Namespace: namespace}, pwSecret); err == nil {
				_ = k8sClient.Delete(ctx, pwSecret)
			}
		})

		It("should set Ready=False with BuildError when the key is missing from the secret", func() {
			target := &warpgatev1alpha1.WarpgateTarget{
				ObjectMeta: metav1.ObjectMeta{Name: targetName, Namespace: namespace},
				Spec: warpgatev1alpha1.WarpgateTargetSpec{
					ConnectionRef: connName,
					Name:          "badkey-target",
					SSH: &warpgatev1alpha1.SSHTargetSpec{
						Host:     "10.0.0.1",
						Port:     22,
						Username: "admin",
						AuthKind: "Password",
						PasswordSecretRef: &warpgatev1alpha1.SecretKeyRef{
							Name: pwSecretName,
							Key:  "password",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, target)).To(Succeed())

			nn := types.NamespacedName{Name: targetName, Namespace: namespace}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			var updated warpgatev1alpha1.WarpgateTarget
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())

			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("BuildError"))
			Expect(readyCond.Message).To(ContainSubstring(`key "password" not found`))
		})
	})

	Context("readSecretValue default key", func() {
		var (
			mockServer   *httptest.Server
			namespace    = testNamespace
			secretName   = "wg-token-target-defkey"
			connName     = "wg-conn-target-defkey"
			targetName   = "target-defkey-test"
			pwSecretName = "ssh-defkey-secret"
		)

		BeforeEach(func() {
			mux := http.NewServeMux()
			mockLogin(mux)
			mux.HandleFunc("/@warpgate/admin/api/targets", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusCreated)
					_ = json.NewEncoder(w).Encode(map[string]any{
						"id":      "target-defkey-001",
						"name":    "defkey-target",
						"options": json.RawMessage(`{"kind":"Ssh"}`),
					})
					return
				}
				http.NotFound(w, r)
			})
			mockServer = httptest.NewServer(mux)
			setupConnection(namespace, secretName, connName, mockServer.URL)

			// Secret with "password" key (the default).
			pwSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      pwSecretName,
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"password": []byte("default-key-pw"),
				},
			}
			Expect(k8sClient.Create(ctx, pwSecret)).To(Succeed())
		})

		AfterEach(func() {
			mockServer.Close()
			cleanupTarget(namespace, targetName)
			cleanupConnection(namespace, secretName, connName)
			pwSecret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: pwSecretName, Namespace: namespace}, pwSecret); err == nil {
				_ = k8sClient.Delete(ctx, pwSecret)
			}
		})

		It("should use default 'password' key when Key is empty in SecretKeyRef", func() {
			target := &warpgatev1alpha1.WarpgateTarget{
				ObjectMeta: metav1.ObjectMeta{Name: targetName, Namespace: namespace},
				Spec: warpgatev1alpha1.WarpgateTargetSpec{
					ConnectionRef: connName,
					Name:          "defkey-target",
					SSH: &warpgatev1alpha1.SSHTargetSpec{
						Host:     "10.0.0.1",
						Port:     22,
						Username: "admin",
						AuthKind: "Password",
						PasswordSecretRef: &warpgatev1alpha1.SecretKeyRef{
							Name: pwSecretName,
							// Key is empty, should default to "password".
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, target)).To(Succeed())

			nn := types.NamespacedName{Name: targetName, Namespace: namespace}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			var updated warpgatev1alpha1.WarpgateTarget
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())
			Expect(updated.Status.ExternalID).To(Equal("target-defkey-001"))

			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
		})
	})

	Context("Resource not found", func() {
		It("should return no error when the resource doesn't exist", func() {
			nn := types.NamespacedName{Name: "target-nonexistent", Namespace: testNamespace}
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))
		})
	})

	Context("Target not found on update", func() {
		var (
			mockServer *httptest.Server
			namespace  = testNamespace
			secretName = "wg-token-target-notfound"
			connName   = "wg-conn-target-notfound"
			targetName = "target-notfound-test"
		)

		BeforeEach(func() {
			mux := http.NewServeMux()
			mockLogin(mux)
			mux.HandleFunc("/@warpgate/admin/api/targets", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusCreated)
					_ = json.NewEncoder(w).Encode(map[string]any{
						"id":      "target-nf-000",
						"name":    "my-notfound-target",
						"options": json.RawMessage(`{"kind":"Ssh"}`),
					})
					return
				}
				http.NotFound(w, r)
			})
			mux.HandleFunc("/@warpgate/admin/api/targets/", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPut {
					// Simulate target deleted out-of-band.
					w.WriteHeader(http.StatusNotFound)
					_, _ = w.Write([]byte(`{"error":"not found"}`))
					return
				}
				http.NotFound(w, r)
			})
			mockServer = httptest.NewServer(mux)
			setupConnection(namespace, secretName, connName, mockServer.URL)
		})

		AfterEach(func() {
			mockServer.Close()
			cleanupTarget(namespace, targetName)
			cleanupConnection(namespace, secretName, connName)
		})

		It("should clear ExternalID and requeue when PUT returns 404", func() {
			target := &warpgatev1alpha1.WarpgateTarget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      targetName,
					Namespace: namespace,
				},
				Spec: warpgatev1alpha1.WarpgateTargetSpec{
					ConnectionRef: connName,
					Name:          "my-notfound-target",
					SSH: &warpgatev1alpha1.SSHTargetSpec{
						Host:     "10.0.0.5",
						Port:     22,
						Username: "admin",
						AuthKind: "PublicKey",
					},
				},
			}
			Expect(k8sClient.Create(ctx, target)).To(Succeed())

			nn := types.NamespacedName{Name: targetName, Namespace: namespace}

			// Reconcile 1: adds finalizer + creates the target (POST succeeds) in one pass.
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// Verify ExternalID was set.
			var created warpgatev1alpha1.WarpgateTarget
			Expect(k8sClient.Get(ctx, nn, &created)).To(Succeed())
			Expect(created.Status.ExternalID).To(Equal("target-nf-000"))

			// Reconcile 2: update (PUT returns 404) — should clear ExternalID and requeue.
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(Equal(reconcile.Result{}))

			var updated warpgatev1alpha1.WarpgateTarget
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())
			Expect(updated.Status.ExternalID).To(BeEmpty())

			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("NotFound"))
			Expect(strings.Contains(readyCond.Message, "recreating")).To(BeTrue())
		})
	})

	Context("Delete target with empty ExternalID", func() {
		var (
			mockServer *httptest.Server
			namespace  = testNamespace
			secretName = "wg-token-target-delempty"
			connName   = "wg-conn-target-delempty"
			targetName = "target-delempty-test"
		)

		BeforeEach(func() {
			mux := http.NewServeMux()
			mockLogin(mux)
			// No targets endpoints needed — the delete should skip the API call.
			mockServer = httptest.NewServer(mux)
			setupConnection(namespace, secretName, connName, mockServer.URL)
		})

		AfterEach(func() {
			mockServer.Close()
			cleanupTarget(namespace, targetName)
			cleanupConnection(namespace, secretName, connName)
		})

		It("should remove the finalizer without calling Warpgate API delete", func() {
			target := &warpgatev1alpha1.WarpgateTarget{
				ObjectMeta: metav1.ObjectMeta{Name: targetName, Namespace: namespace},
				Spec: warpgatev1alpha1.WarpgateTargetSpec{
					ConnectionRef: connName,
					Name:          "delempty-target",
					SSH: &warpgatev1alpha1.SSHTargetSpec{
						Host:     "10.0.0.99",
						Port:     22,
						Username: "admin",
						AuthKind: "PublicKey",
					},
				},
			}
			Expect(k8sClient.Create(ctx, target)).To(Succeed())

			nn := types.NamespacedName{Name: targetName, Namespace: namespace}

			// Manually add the finalizer without creating in Warpgate (no ExternalID).
			var fetched warpgatev1alpha1.WarpgateTarget
			Expect(k8sClient.Get(ctx, nn, &fetched)).To(Succeed())
			controllerutil.AddFinalizer(&fetched, targetFinalizerName)
			Expect(k8sClient.Update(ctx, &fetched)).To(Succeed())

			// Delete the CR.
			Expect(k8sClient.Delete(ctx, &fetched)).To(Succeed())

			// Reconcile the deletion — should skip the API delete and just remove the finalizer.
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// The CR should be fully gone.
			var deleted warpgatev1alpha1.WarpgateTarget
			err = k8sClient.Get(ctx, nn, &deleted)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))
		})
	})

	Context("Create HTTP target with TLS config", func() {
		var (
			mockServer *httptest.Server
			namespace  = testNamespace
			secretName = "wg-token-target-httptls"
			connName   = "wg-conn-target-httptls"
			targetName = "target-httptls-test"
		)

		BeforeEach(func() {
			mux := http.NewServeMux()
			mockLogin(mux)
			mux.HandleFunc("/@warpgate/admin/api/targets", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusCreated)
					_ = json.NewEncoder(w).Encode(map[string]any{
						"id":      "target-httptls-001",
						"name":    "my-httptls-target",
						"options": json.RawMessage(`{"kind":"Http"}`),
					})
					return
				}
				http.NotFound(w, r)
			})
			mockServer = httptest.NewServer(mux)
			setupConnection(namespace, secretName, connName, mockServer.URL)
		})

		AfterEach(func() {
			mockServer.Close()
			cleanupTarget(namespace, targetName)
			cleanupConnection(namespace, secretName, connName)
		})

		It("should create an HTTP target with TLS options set", func() {
			target := &warpgatev1alpha1.WarpgateTarget{
				ObjectMeta: metav1.ObjectMeta{Name: targetName, Namespace: namespace},
				Spec: warpgatev1alpha1.WarpgateTargetSpec{
					ConnectionRef: connName,
					Name:          "my-httptls-target",
					HTTP: &warpgatev1alpha1.HTTPTargetSpec{
						URL:          "https://internal-app.example.com",
						ExternalHost: "app.example.com",
						TLS: &warpgatev1alpha1.TLSConfigSpec{
							Mode:   "Required",
							Verify: true,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, target)).To(Succeed())

			nn := types.NamespacedName{Name: targetName, Namespace: namespace}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			var updated warpgatev1alpha1.WarpgateTarget
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())
			Expect(updated.Status.ExternalID).To(Equal("target-httptls-001"))

			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(readyCond.Reason).To(Equal("HTTP"))
		})
	})

	Context("Create MySQL target without password or TLS", func() {
		var (
			mockServer *httptest.Server
			namespace  = testNamespace
			secretName = "wg-token-target-mysqlbare"
			connName   = "wg-conn-target-mysqlbare"
			targetName = "target-mysqlbare-test"
		)

		BeforeEach(func() {
			mux := http.NewServeMux()
			mockLogin(mux)
			mux.HandleFunc("/@warpgate/admin/api/targets", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusCreated)
					_ = json.NewEncoder(w).Encode(map[string]any{
						"id":      "target-mysqlbare-001",
						"name":    "my-mysqlbare-target",
						"options": json.RawMessage(`{"kind":"MySql"}`),
					})
					return
				}
				http.NotFound(w, r)
			})
			mockServer = httptest.NewServer(mux)
			setupConnection(namespace, secretName, connName, mockServer.URL)
		})

		AfterEach(func() {
			mockServer.Close()
			cleanupTarget(namespace, targetName)
			cleanupConnection(namespace, secretName, connName)
		})

		It("should create a MySQL target without password secret ref or TLS", func() {
			target := &warpgatev1alpha1.WarpgateTarget{
				ObjectMeta: metav1.ObjectMeta{Name: targetName, Namespace: namespace},
				Spec: warpgatev1alpha1.WarpgateTargetSpec{
					ConnectionRef: connName,
					Name:          "my-mysqlbare-target",
					MySQL: &warpgatev1alpha1.MySQLTargetSpec{
						Host:     "db.example.com",
						Port:     3306,
						Username: "root",
						// No PasswordSecretRef, no TLS
					},
				},
			}
			Expect(k8sClient.Create(ctx, target)).To(Succeed())

			nn := types.NamespacedName{Name: targetName, Namespace: namespace}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			var updated warpgatev1alpha1.WarpgateTarget
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())
			Expect(updated.Status.ExternalID).To(Equal("target-mysqlbare-001"))

			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(readyCond.Reason).To(Equal("MySQL"))
		})
	})

	Context("Create PostgreSQL target without password or TLS", func() {
		var (
			mockServer *httptest.Server
			namespace  = testNamespace
			secretName = "wg-token-target-pgbare"
			connName   = "wg-conn-target-pgbare"
			targetName = "target-pgbare-test"
		)

		BeforeEach(func() {
			mux := http.NewServeMux()
			mockLogin(mux)
			mux.HandleFunc("/@warpgate/admin/api/targets", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusCreated)
					_ = json.NewEncoder(w).Encode(map[string]any{
						"id":      "target-pgbare-001",
						"name":    "my-pgbare-target",
						"options": json.RawMessage(`{"kind":"Postgres"}`),
					})
					return
				}
				http.NotFound(w, r)
			})
			mockServer = httptest.NewServer(mux)
			setupConnection(namespace, secretName, connName, mockServer.URL)
		})

		AfterEach(func() {
			mockServer.Close()
			cleanupTarget(namespace, targetName)
			cleanupConnection(namespace, secretName, connName)
		})

		It("should create a PostgreSQL target without password secret ref or TLS", func() {
			target := &warpgatev1alpha1.WarpgateTarget{
				ObjectMeta: metav1.ObjectMeta{Name: targetName, Namespace: namespace},
				Spec: warpgatev1alpha1.WarpgateTargetSpec{
					ConnectionRef: connName,
					Name:          "my-pgbare-target",
					PostgreSQL: &warpgatev1alpha1.PostgreSQLTargetSpec{
						Host:     "pgdb.example.com",
						Port:     5432,
						Username: "postgres",
						// No PasswordSecretRef, no TLS
					},
				},
			}
			Expect(k8sClient.Create(ctx, target)).To(Succeed())

			nn := types.NamespacedName{Name: targetName, Namespace: namespace}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			var updated warpgatev1alpha1.WarpgateTarget
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())
			Expect(updated.Status.ExternalID).To(Equal("target-pgbare-001"))

			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(readyCond.Reason).To(Equal("PostgreSQL"))
		})
	})

	Context("Delete target with Warpgate API error", func() {
		var (
			mockServer *httptest.Server
			namespace  = testNamespace
			secretName = "wg-token-target-delerr"
			connName   = "wg-conn-target-delerr"
			targetName = "target-delerr-test"
		)

		BeforeEach(func() {
			mux := http.NewServeMux()
			mockLogin(mux)
			mux.HandleFunc("/@warpgate/admin/api/targets", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusCreated)
					_ = json.NewEncoder(w).Encode(map[string]any{
						"id":      "target-delerr-abc",
						"name":    "my-delerr-target",
						"options": json.RawMessage(`{"kind":"Ssh"}`),
					})
					return
				}
				http.NotFound(w, r)
			})
			mux.HandleFunc("/@warpgate/admin/api/targets/", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodDelete {
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte(`{"error":"internal"}`))
					return
				}
				http.NotFound(w, r)
			})
			mockServer = httptest.NewServer(mux)
			setupConnection(namespace, secretName, connName, mockServer.URL)
		})

		AfterEach(func() {
			mockServer.Close()
			cleanupTarget(namespace, targetName)
			cleanupConnection(namespace, secretName, connName)
		})

		It("should requeue when Warpgate API delete returns a non-404 error", func() {
			target := &warpgatev1alpha1.WarpgateTarget{
				ObjectMeta: metav1.ObjectMeta{Name: targetName, Namespace: namespace},
				Spec: warpgatev1alpha1.WarpgateTargetSpec{
					ConnectionRef: connName,
					Name:          "my-delerr-target",
					SSH: &warpgatev1alpha1.SSHTargetSpec{
						Host:     "10.0.0.50",
						Port:     22,
						Username: "admin",
						AuthKind: "PublicKey",
					},
				},
			}
			Expect(k8sClient.Create(ctx, target)).To(Succeed())

			nn := types.NamespacedName{Name: targetName, Namespace: namespace}

			// Reconcile 1: adds finalizer + creates target.
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// Verify ExternalID was set.
			var created warpgatev1alpha1.WarpgateTarget
			Expect(k8sClient.Get(ctx, nn, &created)).To(Succeed())
			Expect(created.Status.ExternalID).To(Equal("target-delerr-abc"))

			// Delete the CR.
			Expect(k8sClient.Delete(ctx, &created)).To(Succeed())

			// Reconcile the deletion — should requeue due to API error.
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(targetRequeueAfter))

			// The CR should still exist (finalizer not removed).
			var still warpgatev1alpha1.WarpgateTarget
			Expect(k8sClient.Get(ctx, nn, &still)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(&still, targetFinalizerName)).To(BeTrue())
		})
	})

	Context("MySQL target with missing password secret", func() {
		var (
			mockServer *httptest.Server
			namespace  = testNamespace
			secretName = "wg-token-target-mysqlnopw"
			connName   = "wg-conn-target-mysqlnopw"
			targetName = "target-mysqlnopw-test"
		)

		BeforeEach(func() {
			mux := http.NewServeMux()
			mockLogin(mux)
			mockServer = httptest.NewServer(mux)
			setupConnection(namespace, secretName, connName, mockServer.URL)
		})

		AfterEach(func() {
			mockServer.Close()
			cleanupTarget(namespace, targetName)
			cleanupConnection(namespace, secretName, connName)
		})

		It("should set Ready=False with BuildError when the MySQL password secret doesn't exist", func() {
			target := &warpgatev1alpha1.WarpgateTarget{
				ObjectMeta: metav1.ObjectMeta{Name: targetName, Namespace: namespace},
				Spec: warpgatev1alpha1.WarpgateTargetSpec{
					ConnectionRef: connName,
					Name:          "mysqlnopw-target",
					MySQL: &warpgatev1alpha1.MySQLTargetSpec{
						Host:     "db.example.com",
						Port:     3306,
						Username: "root",
						PasswordSecretRef: &warpgatev1alpha1.SecretKeyRef{
							Name: "nonexistent-mysql-secret",
							Key:  "password",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, target)).To(Succeed())

			nn := types.NamespacedName{Name: targetName, Namespace: namespace}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			var updated warpgatev1alpha1.WarpgateTarget
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())

			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("BuildError"))
			Expect(readyCond.Message).To(ContainSubstring("MySQL password"))
		})
	})

	Context("PostgreSQL target with missing password secret", func() {
		var (
			mockServer *httptest.Server
			namespace  = testNamespace
			secretName = "wg-token-target-pgnopw"
			connName   = "wg-conn-target-pgnopw"
			targetName = "target-pgnopw-test"
		)

		BeforeEach(func() {
			mux := http.NewServeMux()
			mockLogin(mux)
			mockServer = httptest.NewServer(mux)
			setupConnection(namespace, secretName, connName, mockServer.URL)
		})

		AfterEach(func() {
			mockServer.Close()
			cleanupTarget(namespace, targetName)
			cleanupConnection(namespace, secretName, connName)
		})

		It("should set Ready=False with BuildError when the PostgreSQL password secret doesn't exist", func() {
			target := &warpgatev1alpha1.WarpgateTarget{
				ObjectMeta: metav1.ObjectMeta{Name: targetName, Namespace: namespace},
				Spec: warpgatev1alpha1.WarpgateTargetSpec{
					ConnectionRef: connName,
					Name:          "pgnopw-target",
					PostgreSQL: &warpgatev1alpha1.PostgreSQLTargetSpec{
						Host:     "pgdb.example.com",
						Port:     5432,
						Username: "postgres",
						PasswordSecretRef: &warpgatev1alpha1.SecretKeyRef{
							Name: "nonexistent-pg-secret",
							Key:  "password",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, target)).To(Succeed())

			nn := types.NamespacedName{Name: targetName, Namespace: namespace}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			var updated warpgatev1alpha1.WarpgateTarget
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())

			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("BuildError"))
			Expect(readyCond.Message).To(ContainSubstring("PostgreSQL password"))
		})
	})
})
