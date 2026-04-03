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
			namespace = testNamespace

			mux := http.NewServeMux()
			mockLogin(mux)
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
					"username": []byte("admin"), "password": []byte("test-pass"),
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
					Host:                 mockServer.URL,
					CredentialsSecretRef: warpgatev1alpha1.CredentialsSecretRef{Name: tokenSecret},
					InsecureSkipVerify:   true,
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

			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(readyCond.Reason).To(Equal("Reconciled"))
		})
	})

	Context("Missing password secret", func() {
		var (
			mockServer  *httptest.Server
			tokenSecret string
			connName    string
			crName      string
			namespace   string
		)

		BeforeEach(func() {
			tokenSecret = "pwcred-nosec-token"
			connName = "pwcred-nosec-conn"
			crName = "pwcred-nosec-cred"
			namespace = testNamespace

			mux := http.NewServeMux()
			mockLogin(mux)
			mux.HandleFunc("/@warpgate/admin/api/users", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode([]map[string]any{
					{"id": "user-uuid-nosec", "username": "nosecuser"},
				})
			})
			mockServer = httptest.NewServer(mux)

			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: tokenSecret, Namespace: namespace},
				Data:       map[string][]byte{"username": []byte("admin"), "password": []byte("test-pass")},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			conn := &warpgatev1alpha1.WarpgateConnection{
				ObjectMeta: metav1.ObjectMeta{Name: connName, Namespace: namespace},
				Spec: warpgatev1alpha1.WarpgateConnectionSpec{
					Host:                 mockServer.URL,
					CredentialsSecretRef: warpgatev1alpha1.CredentialsSecretRef{Name: tokenSecret},
					InsecureSkipVerify:   true,
				},
			}
			Expect(k8sClient.Create(ctx, conn)).To(Succeed())

			cr := &warpgatev1alpha1.WarpgatePasswordCredential{
				ObjectMeta: metav1.ObjectMeta{Name: crName, Namespace: namespace},
				Spec: warpgatev1alpha1.WarpgatePasswordCredentialSpec{
					ConnectionRef: connName,
					Username:      "nosecuser",
					PasswordSecretRef: warpgatev1alpha1.SecretKeyRef{
						Name: "nonexistent-pw-secret",
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
				_ = k8sClient.Delete(ctx, conn)
			}
			secret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: tokenSecret, Namespace: namespace}, secret); err == nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should set Ready=False with SecretNotFound when the password secret doesn't exist", func() {
			nn := types.NamespacedName{Name: crName, Namespace: namespace}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).To(HaveOccurred())

			var updated warpgatev1alpha1.WarpgatePasswordCredential
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())

			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("SecretNotFound"))
		})
	})

	Context("Missing key in password secret", func() {
		var (
			mockServer     *httptest.Server
			tokenSecret    string
			passwordSecret string
			connName       string
			crName         string
			namespace      string
		)

		BeforeEach(func() {
			tokenSecret = "pwcred-nokey-token"
			passwordSecret = "pwcred-nokey-password"
			connName = "pwcred-nokey-conn"
			crName = "pwcred-nokey-cred"
			namespace = testNamespace

			mux := http.NewServeMux()
			mockLogin(mux)
			mux.HandleFunc("/@warpgate/admin/api/users", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode([]map[string]any{
					{"id": "user-uuid-nokey", "username": "nokeyuser"},
				})
			})
			mockServer = httptest.NewServer(mux)

			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: tokenSecret, Namespace: namespace},
				Data:       map[string][]byte{"username": []byte("admin"), "password": []byte("test-pass")},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			pwSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: passwordSecret, Namespace: namespace},
				Data:       map[string][]byte{"wrong-key": []byte("some-value")},
			}
			Expect(k8sClient.Create(ctx, pwSecret)).To(Succeed())

			conn := &warpgatev1alpha1.WarpgateConnection{
				ObjectMeta: metav1.ObjectMeta{Name: connName, Namespace: namespace},
				Spec: warpgatev1alpha1.WarpgateConnectionSpec{
					Host:                 mockServer.URL,
					CredentialsSecretRef: warpgatev1alpha1.CredentialsSecretRef{Name: tokenSecret},
					InsecureSkipVerify:   true,
				},
			}
			Expect(k8sClient.Create(ctx, conn)).To(Succeed())

			cr := &warpgatev1alpha1.WarpgatePasswordCredential{
				ObjectMeta: metav1.ObjectMeta{Name: crName, Namespace: namespace},
				Spec: warpgatev1alpha1.WarpgatePasswordCredentialSpec{
					ConnectionRef: connName,
					Username:      "nokeyuser",
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
				_ = k8sClient.Delete(ctx, conn)
			}
			for _, name := range []string{tokenSecret, passwordSecret} {
				secret := &corev1.Secret{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, secret); err == nil {
					_ = k8sClient.Delete(ctx, secret)
				}
			}
		})

		It("should set Ready=False with SecretKeyMissing when the key is missing", func() {
			nn := types.NamespacedName{Name: crName, Namespace: namespace}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).To(HaveOccurred())

			var updated warpgatev1alpha1.WarpgatePasswordCredential
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())

			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("SecretKeyMissing"))
		})
	})

	Context("User not found", func() {
		var (
			mockServer     *httptest.Server
			tokenSecret    string
			passwordSecret string
			connName       string
			crName         string
			namespace      string
		)

		BeforeEach(func() {
			tokenSecret = "pwcred-nouser-token"
			passwordSecret = "pwcred-nouser-password"
			connName = "pwcred-nouser-conn"
			crName = "pwcred-nouser-cred"
			namespace = testNamespace

			mux := http.NewServeMux()
			mockLogin(mux)
			mux.HandleFunc("/@warpgate/admin/api/users", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode([]map[string]any{})
			})
			mockServer = httptest.NewServer(mux)

			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: tokenSecret, Namespace: namespace},
				Data:       map[string][]byte{"username": []byte("admin"), "password": []byte("test-pass")},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			pwSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: passwordSecret, Namespace: namespace},
				Data:       map[string][]byte{"password": []byte("pw")},
			}
			Expect(k8sClient.Create(ctx, pwSecret)).To(Succeed())

			conn := &warpgatev1alpha1.WarpgateConnection{
				ObjectMeta: metav1.ObjectMeta{Name: connName, Namespace: namespace},
				Spec: warpgatev1alpha1.WarpgateConnectionSpec{
					Host:                 mockServer.URL,
					CredentialsSecretRef: warpgatev1alpha1.CredentialsSecretRef{Name: tokenSecret},
					InsecureSkipVerify:   true,
				},
			}
			Expect(k8sClient.Create(ctx, conn)).To(Succeed())

			cr := &warpgatev1alpha1.WarpgatePasswordCredential{
				ObjectMeta: metav1.ObjectMeta{Name: crName, Namespace: namespace},
				Spec: warpgatev1alpha1.WarpgatePasswordCredentialSpec{
					ConnectionRef: connName,
					Username:      "ghost-user",
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
				_ = k8sClient.Delete(ctx, conn)
			}
			for _, name := range []string{tokenSecret, passwordSecret} {
				secret := &corev1.Secret{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, secret); err == nil {
					_ = k8sClient.Delete(ctx, secret)
				}
			}
		})

		It("should set Ready=False with UserNotFound when the user doesn't exist", func() {
			nn := types.NamespacedName{Name: crName, Namespace: namespace}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).To(HaveOccurred())

			var updated warpgatev1alpha1.WarpgatePasswordCredential
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())

			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("UserNotFound"))
		})
	})

	Context("Resource not found", func() {
		It("should return no error when the resource doesn't exist", func() {
			nn := types.NamespacedName{Name: "pwcred-nonexistent", Namespace: testNamespace}
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))
		})
	})

	Context("Delete credential with empty credentialID", func() {
		var (
			mockServer     *httptest.Server
			tokenSecret    string
			passwordSecret string
			connName       string
			crName         string
			namespace      string
		)

		BeforeEach(func() {
			tokenSecret = "pwcred-delempty-token"
			passwordSecret = "pwcred-delempty-password"
			connName = "pwcred-delempty-conn"
			crName = "pwcred-delempty-cred"
			namespace = testNamespace

			mux := http.NewServeMux()
			mockLogin(mux)
			mockServer = httptest.NewServer(mux)

			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: tokenSecret, Namespace: namespace},
				Data:       map[string][]byte{"username": []byte("admin"), "password": []byte("test-pass")},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			pwSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: passwordSecret, Namespace: namespace},
				Data:       map[string][]byte{"password": []byte("pw")},
			}
			Expect(k8sClient.Create(ctx, pwSecret)).To(Succeed())

			conn := &warpgatev1alpha1.WarpgateConnection{
				ObjectMeta: metav1.ObjectMeta{Name: connName, Namespace: namespace},
				Spec: warpgatev1alpha1.WarpgateConnectionSpec{
					Host:                 mockServer.URL,
					CredentialsSecretRef: warpgatev1alpha1.CredentialsSecretRef{Name: tokenSecret},
					InsecureSkipVerify:   true,
				},
			}
			Expect(k8sClient.Create(ctx, conn)).To(Succeed())

			// Create CR with finalizer but no CredentialID or UserID in status.
			cr := &warpgatev1alpha1.WarpgatePasswordCredential{
				ObjectMeta: metav1.ObjectMeta{
					Name:       crName,
					Namespace:  namespace,
					Finalizers: []string{passwordCredentialFinalizer},
				},
				Spec: warpgatev1alpha1.WarpgatePasswordCredentialSpec{
					ConnectionRef: connName,
					Username:      "emptyuser",
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
				_ = k8sClient.Delete(ctx, conn)
			}
			for _, name := range []string{tokenSecret, passwordSecret} {
				secret := &corev1.Secret{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, secret); err == nil {
					_ = k8sClient.Delete(ctx, secret)
				}
			}
		})

		It("should skip Warpgate API delete and just remove the finalizer when IDs are empty", func() {
			nn := types.NamespacedName{Name: crName, Namespace: namespace}

			var cr warpgatev1alpha1.WarpgatePasswordCredential
			Expect(k8sClient.Get(ctx, nn, &cr)).To(Succeed())
			Expect(k8sClient.Delete(ctx, &cr)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			err = k8sClient.Get(ctx, nn, &cr)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))
		})
	})

	Context("Default password key", func() {
		var (
			mockServer     *httptest.Server
			tokenSecret    string
			passwordSecret string
			connName       string
			crName         string
			namespace      string
		)

		BeforeEach(func() {
			tokenSecret = "pwcred-defkey-token"
			passwordSecret = "pwcred-defkey-password"
			connName = "pwcred-defkey-conn"
			crName = "pwcred-defkey-cred"
			namespace = testNamespace

			mux := http.NewServeMux()
			mockLogin(mux)
			mux.HandleFunc("/@warpgate/admin/api/users", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode([]map[string]any{
					{"id": "user-uuid-defkey", "username": "defkeyuser"},
				})
			})
			mux.HandleFunc("/@warpgate/admin/api/users/user-uuid-defkey/credentials/passwords", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusCreated)
					_ = json.NewEncoder(w).Encode(map[string]any{
						"id":       "cred-uuid-defkey",
						"password": "hashed",
					})
					return
				}
				http.NotFound(w, r)
			})
			mockServer = httptest.NewServer(mux)

			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: tokenSecret, Namespace: namespace},
				Data:       map[string][]byte{"username": []byte("admin"), "password": []byte("test-pass")},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			// The password secret uses the default key "password".
			pwSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: passwordSecret, Namespace: namespace},
				Data:       map[string][]byte{"password": []byte("default-key-pw")},
			}
			Expect(k8sClient.Create(ctx, pwSecret)).To(Succeed())

			conn := &warpgatev1alpha1.WarpgateConnection{
				ObjectMeta: metav1.ObjectMeta{Name: connName, Namespace: namespace},
				Spec: warpgatev1alpha1.WarpgateConnectionSpec{
					Host:                 mockServer.URL,
					CredentialsSecretRef: warpgatev1alpha1.CredentialsSecretRef{Name: tokenSecret},
					InsecureSkipVerify:   true,
				},
			}
			Expect(k8sClient.Create(ctx, conn)).To(Succeed())

			cr := &warpgatev1alpha1.WarpgatePasswordCredential{
				ObjectMeta: metav1.ObjectMeta{Name: crName, Namespace: namespace},
				Spec: warpgatev1alpha1.WarpgatePasswordCredentialSpec{
					ConnectionRef: connName,
					Username:      "defkeyuser",
					PasswordSecretRef: warpgatev1alpha1.SecretKeyRef{
						Name: passwordSecret,
						// Key is empty, should default to "password".
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
				_ = k8sClient.Delete(ctx, conn)
			}
			for _, name := range []string{tokenSecret, passwordSecret} {
				secret := &corev1.Secret{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, secret); err == nil {
					_ = k8sClient.Delete(ctx, secret)
				}
			}
		})

		It("should use default 'password' key when Key is empty in PasswordSecretRef", func() {
			nn := types.NamespacedName{Name: crName, Namespace: namespace}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			var updated warpgatev1alpha1.WarpgatePasswordCredential
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())
			Expect(updated.Status.CredentialID).To(Equal("cred-uuid-defkey"))

			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
		})
	})

	Context("Create credential API failure", func() {
		var (
			mockServer     *httptest.Server
			tokenSecret    string
			passwordSecret string
			connName       string
			crName         string
			namespace      string
		)

		BeforeEach(func() {
			tokenSecret = "pwcred-createfail-token"
			passwordSecret = "pwcred-createfail-password"
			connName = "pwcred-createfail-conn"
			crName = "pwcred-createfail-cred"
			namespace = testNamespace

			mux := http.NewServeMux()
			mockLogin(mux)
			mux.HandleFunc("/@warpgate/admin/api/users", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode([]map[string]any{
					{"id": "user-uuid-cf", "username": "cfuser"},
				})
			})
			mux.HandleFunc("/@warpgate/admin/api/users/user-uuid-cf/credentials/passwords", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte(`{"error":"internal"}`))
					return
				}
				http.NotFound(w, r)
			})
			mockServer = httptest.NewServer(mux)

			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: tokenSecret, Namespace: namespace},
				Data:       map[string][]byte{"username": []byte("admin"), "password": []byte("test-pass")},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			pwSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: passwordSecret, Namespace: namespace},
				Data:       map[string][]byte{"password": []byte("pw")},
			}
			Expect(k8sClient.Create(ctx, pwSecret)).To(Succeed())

			conn := &warpgatev1alpha1.WarpgateConnection{
				ObjectMeta: metav1.ObjectMeta{Name: connName, Namespace: namespace},
				Spec: warpgatev1alpha1.WarpgateConnectionSpec{
					Host:                 mockServer.URL,
					CredentialsSecretRef: warpgatev1alpha1.CredentialsSecretRef{Name: tokenSecret},
					InsecureSkipVerify:   true,
				},
			}
			Expect(k8sClient.Create(ctx, conn)).To(Succeed())

			cr := &warpgatev1alpha1.WarpgatePasswordCredential{
				ObjectMeta: metav1.ObjectMeta{Name: crName, Namespace: namespace},
				Spec: warpgatev1alpha1.WarpgatePasswordCredentialSpec{
					ConnectionRef: connName,
					Username:      "cfuser",
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
				_ = k8sClient.Delete(ctx, conn)
			}
			for _, name := range []string{tokenSecret, passwordSecret} {
				secret := &corev1.Secret{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, secret); err == nil {
					_ = k8sClient.Delete(ctx, secret)
				}
			}
		})

		It("should set Ready=False with CreateFailed when the API returns an error", func() {
			nn := types.NamespacedName{Name: crName, Namespace: namespace}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).To(HaveOccurred())

			var updated warpgatev1alpha1.WarpgatePasswordCredential
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())

			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("CreateFailed"))
		})
	})

	Context("Client error", func() {
		var (
			crName    string
			namespace string
		)

		BeforeEach(func() {
			crName = "pwcred-clienterr-cred"
			namespace = testNamespace

			cr := &warpgatev1alpha1.WarpgatePasswordCredential{
				ObjectMeta: metav1.ObjectMeta{
					Name:      crName,
					Namespace: namespace,
				},
				Spec: warpgatev1alpha1.WarpgatePasswordCredentialSpec{
					ConnectionRef: "nonexistent-pw-conn",
					Username:      "someuser",
					PasswordSecretRef: warpgatev1alpha1.SecretKeyRef{
						Name: "some-secret",
						Key:  "password",
					},
				},
			}
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
		})

		AfterEach(func() {
			cr := &warpgatev1alpha1.WarpgatePasswordCredential{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: crName, Namespace: namespace}, cr); err == nil {
				controllerutil.RemoveFinalizer(cr, passwordCredentialFinalizer)
				_ = k8sClient.Update(ctx, cr)
				_ = k8sClient.Delete(ctx, cr)
			}
		})

		It("should set Ready=False with ClientError when the connection doesn't exist", func() {
			nn := types.NamespacedName{Name: crName, Namespace: namespace}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).To(HaveOccurred())

			var updated warpgatev1alpha1.WarpgatePasswordCredential
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())

			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("ClientError"))
		})
	})

	Context("Delete credential with populated credentialID", func() {
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
			tokenSecret = "pwcred-delpop-token"
			passwordSecret = "pwcred-delpop-password"
			connName = "pwcred-delpop-conn"
			crName = "pwcred-delpop-cred"
			namespace = testNamespace
			deleteCalled = false

			mux := http.NewServeMux()
			mockLogin(mux)
			mux.HandleFunc("/@warpgate/admin/api/users/pre-user-id/credentials/passwords/pre-cred-id", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodDelete {
					deleteCalled = true
					w.WriteHeader(http.StatusNoContent)
					return
				}
				http.NotFound(w, r)
			})
			mockServer = httptest.NewServer(mux)

			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: tokenSecret, Namespace: namespace},
				Data:       map[string][]byte{"username": []byte("admin"), "password": []byte("test-pass")},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			pwSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: passwordSecret, Namespace: namespace},
				Data:       map[string][]byte{"password": []byte("pw")},
			}
			Expect(k8sClient.Create(ctx, pwSecret)).To(Succeed())

			conn := &warpgatev1alpha1.WarpgateConnection{
				ObjectMeta: metav1.ObjectMeta{Name: connName, Namespace: namespace},
				Spec: warpgatev1alpha1.WarpgateConnectionSpec{
					Host:                 mockServer.URL,
					CredentialsSecretRef: warpgatev1alpha1.CredentialsSecretRef{Name: tokenSecret},
					InsecureSkipVerify:   true,
				},
			}
			Expect(k8sClient.Create(ctx, conn)).To(Succeed())

			// Create the CR with a finalizer and pre-populated status IDs, simulating
			// a credential that was previously created.
			cr := &warpgatev1alpha1.WarpgatePasswordCredential{
				ObjectMeta: metav1.ObjectMeta{
					Name:       crName,
					Namespace:  namespace,
					Finalizers: []string{passwordCredentialFinalizer},
				},
				Spec: warpgatev1alpha1.WarpgatePasswordCredentialSpec{
					ConnectionRef: connName,
					Username:      "someuser",
					PasswordSecretRef: warpgatev1alpha1.SecretKeyRef{
						Name: passwordSecret,
						Key:  "password",
					},
				},
			}
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())

			// Set the status fields to simulate an already-created credential.
			cr.Status.UserID = "pre-user-id"
			cr.Status.CredentialID = "pre-cred-id"
			Expect(k8sClient.Status().Update(ctx, cr)).To(Succeed())
		})

		AfterEach(func() {
			mockServer.Close()
			conn := &warpgatev1alpha1.WarpgateConnection{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: connName, Namespace: namespace}, conn); err == nil {
				_ = k8sClient.Delete(ctx, conn)
			}
			for _, name := range []string{tokenSecret, passwordSecret} {
				secret := &corev1.Secret{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, secret); err == nil {
					_ = k8sClient.Delete(ctx, secret)
				}
			}
		})

		It("should call the Warpgate DELETE API and remove the finalizer", func() {
			nn := types.NamespacedName{Name: crName, Namespace: namespace}

			// Mark for deletion.
			var cr warpgatev1alpha1.WarpgatePasswordCredential
			Expect(k8sClient.Get(ctx, nn, &cr)).To(Succeed())
			Expect(k8sClient.Delete(ctx, &cr)).To(Succeed())

			// Reconcile the deletion.
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			Expect(deleteCalled).To(BeTrue())

			// CR should be gone.
			err = k8sClient.Get(ctx, nn, &cr)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))
		})
	})

	Context("User resolution after username change", func() {
		var (
			mockServer     *httptest.Server
			tokenSecret    string
			passwordSecret string
			connName       string
			crName         string
			namespace      string
		)

		BeforeEach(func() {
			tokenSecret = "pwcred-userchg-token"
			passwordSecret = "pwcred-userchg-password"
			connName = "pwcred-userchg-conn"
			crName = "pwcred-userchg-cred"
			namespace = testNamespace

			mux := http.NewServeMux()
			mockLogin(mux)
			mux.HandleFunc("/@warpgate/admin/api/users", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode([]map[string]any{
					{"id": "user-uuid-new", "username": "newuser"},
				})
			})
			mux.HandleFunc("/@warpgate/admin/api/users/user-uuid-new/credentials/passwords", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusCreated)
					_ = json.NewEncoder(w).Encode(map[string]any{
						"id":       "cred-uuid-new",
						"password": "hashed",
					})
					return
				}
				http.NotFound(w, r)
			})
			mockServer = httptest.NewServer(mux)

			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: tokenSecret, Namespace: namespace},
				Data:       map[string][]byte{"username": []byte("admin"), "password": []byte("test-pass")},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			pwSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: passwordSecret, Namespace: namespace},
				Data:       map[string][]byte{"password": []byte("pw")},
			}
			Expect(k8sClient.Create(ctx, pwSecret)).To(Succeed())

			conn := &warpgatev1alpha1.WarpgateConnection{
				ObjectMeta: metav1.ObjectMeta{Name: connName, Namespace: namespace},
				Spec: warpgatev1alpha1.WarpgateConnectionSpec{
					Host:                 mockServer.URL,
					CredentialsSecretRef: warpgatev1alpha1.CredentialsSecretRef{Name: tokenSecret},
					InsecureSkipVerify:   true,
				},
			}
			Expect(k8sClient.Create(ctx, conn)).To(Succeed())

			// Create CR with a finalizer and old UserID, simulating a previous reconcile
			// where the username was different.
			cr := &warpgatev1alpha1.WarpgatePasswordCredential{
				ObjectMeta: metav1.ObjectMeta{
					Name:       crName,
					Namespace:  namespace,
					Finalizers: []string{passwordCredentialFinalizer},
				},
				Spec: warpgatev1alpha1.WarpgatePasswordCredentialSpec{
					ConnectionRef: connName,
					Username:      "newuser",
					PasswordSecretRef: warpgatev1alpha1.SecretKeyRef{
						Name: passwordSecret,
						Key:  "password",
					},
				},
			}
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())

			// Set old UserID to simulate username having changed.
			cr.Status.UserID = "user-uuid-old"
			Expect(k8sClient.Status().Update(ctx, cr)).To(Succeed())
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
				_ = k8sClient.Delete(ctx, conn)
			}
			for _, name := range []string{tokenSecret, passwordSecret} {
				secret := &corev1.Secret{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, secret); err == nil {
					_ = k8sClient.Delete(ctx, secret)
				}
			}
		})

		It("should resolve the new user and update the UserID in status", func() {
			nn := types.NamespacedName{Name: crName, Namespace: namespace}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			var updated warpgatev1alpha1.WarpgatePasswordCredential
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())
			// UserID should now reflect the new username resolution.
			Expect(updated.Status.UserID).To(Equal("user-uuid-new"))
			Expect(updated.Status.CredentialID).To(Equal("cred-uuid-new"))

			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
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
			namespace = testNamespace
			deleteCalled = false

			mux := http.NewServeMux()
			mockLogin(mux)
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
					"username": []byte("admin"), "password": []byte("test-pass"),
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
					Host:                 mockServer.URL,
					CredentialsSecretRef: warpgatev1alpha1.CredentialsSecretRef{Name: tokenSecret},
					InsecureSkipVerify:   true,
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
