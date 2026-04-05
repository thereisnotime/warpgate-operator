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

var _ = Describe("WarpgateTicket Controller", func() {

	var (
		reconciler *WarpgateTicketReconciler
	)

	BeforeEach(func() {
		reconciler = &WarpgateTicketReconciler{
			Client: k8sClient,
			Scheme: k8sClient.Scheme(),
		}
	})

	Context("Create ticket", func() {
		var (
			mockServer  *httptest.Server
			tokenSecret string
			connName    string
			crName      string
			namespace   string
		)

		BeforeEach(func() {
			tokenSecret = "ticket-test-token"
			connName = "ticket-test-conn"
			crName = "ticket-test-tkt"
			namespace = testNamespace

			mux := http.NewServeMux()
			mockLogin(mux)
			mux.HandleFunc("/@warpgate/admin/api/tickets", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusCreated)
					_ = json.NewEncoder(w).Encode(map[string]any{
						"ticket": map[string]any{
							"id":       "t1",
							"username": "ticketuser",
							"target":   "tickettarget",
						},
						"secret": "s3cret-ticket-value",
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
					Host:               mockServer.URL,
					AuthSecretRef:      warpgatev1alpha1.AuthSecretRef{Name: tokenSecret},
					InsecureSkipVerify: true,
				},
			}
			Expect(k8sClient.Create(ctx, conn)).To(Succeed())

			cr := &warpgatev1alpha1.WarpgateTicket{
				ObjectMeta: metav1.ObjectMeta{
					Name:      crName,
					Namespace: namespace,
				},
				Spec: warpgatev1alpha1.WarpgateTicketSpec{
					ConnectionRef: connName,
					Username:      "ticketuser",
					TargetName:    "tickettarget",
					Description:   "test ticket",
				},
			}
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
		})

		AfterEach(func() {
			mockServer.Close()

			cr := &warpgatev1alpha1.WarpgateTicket{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: crName, Namespace: namespace}, cr); err == nil {
				controllerutil.RemoveFinalizer(cr, ticketFinalizer)
				_ = k8sClient.Update(ctx, cr)
				_ = k8sClient.Delete(ctx, cr)
			}
			// Clean up the auto-created ticket secret.
			ticketSecret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: crName + "-secret", Namespace: namespace}, ticketSecret); err == nil {
				_ = k8sClient.Delete(ctx, ticketSecret)
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

		It("should create the ticket, store the secret, and set Ready=True", func() {
			nn := types.NamespacedName{Name: crName, Namespace: namespace}

			// Single reconcile: adds finalizer, creates ticket, stores secret, updates status.
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			var updated warpgatev1alpha1.WarpgateTicket
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())
			Expect(updated.Status.TicketID).To(Equal("t1"))
			Expect(updated.Status.SecretRef).To(Equal(crName + "-secret"))

			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(readyCond.Reason).To(Equal("Reconciled"))

			// Verify the auto-created Secret contains the ticket secret value.
			var ticketSecret corev1.Secret
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      crName + "-secret",
				Namespace: namespace,
			}, &ticketSecret)).To(Succeed())
			Expect(string(ticketSecret.Data["secret"])).To(Equal("s3cret-ticket-value"))
		})
	})

	Context("Create ticket fails", func() {
		var (
			mockServer  *httptest.Server
			tokenSecret string
			connName    string
			crName      string
			namespace   string
		)

		BeforeEach(func() {
			tokenSecret = "ticket-fail-token"
			connName = "ticket-fail-conn"
			crName = "ticket-fail-tkt"
			namespace = testNamespace

			mux := http.NewServeMux()
			mockLogin(mux)
			mux.HandleFunc("/@warpgate/admin/api/tickets", func(w http.ResponseWriter, r *http.Request) {
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

			conn := &warpgatev1alpha1.WarpgateConnection{
				ObjectMeta: metav1.ObjectMeta{Name: connName, Namespace: namespace},
				Spec: warpgatev1alpha1.WarpgateConnectionSpec{
					Host:               mockServer.URL,
					AuthSecretRef:      warpgatev1alpha1.AuthSecretRef{Name: tokenSecret},
					InsecureSkipVerify: true,
				},
			}
			Expect(k8sClient.Create(ctx, conn)).To(Succeed())

			cr := &warpgatev1alpha1.WarpgateTicket{
				ObjectMeta: metav1.ObjectMeta{Name: crName, Namespace: namespace},
				Spec: warpgatev1alpha1.WarpgateTicketSpec{
					ConnectionRef: connName,
					Username:      "failuser",
					TargetName:    "failtarget",
				},
			}
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
		})

		AfterEach(func() {
			mockServer.Close()
			cr := &warpgatev1alpha1.WarpgateTicket{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: crName, Namespace: namespace}, cr); err == nil {
				controllerutil.RemoveFinalizer(cr, ticketFinalizer)
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

		It("should set Ready=False with CreateFailed when the API returns an error", func() {
			nn := types.NamespacedName{Name: crName, Namespace: namespace}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).To(HaveOccurred())

			var updated warpgatev1alpha1.WarpgateTicket
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())

			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("CreateFailed"))
		})
	})

	Context("Resource not found", func() {
		It("should return no error when the resource doesn't exist", func() {
			nn := types.NamespacedName{Name: "ticket-nonexistent", Namespace: testNamespace}
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))
		})
	})

	Context("Client error", func() {
		var (
			crName    string
			namespace string
		)

		BeforeEach(func() {
			crName = "ticket-clienterr-tkt"
			namespace = testNamespace

			cr := &warpgatev1alpha1.WarpgateTicket{
				ObjectMeta: metav1.ObjectMeta{
					Name:      crName,
					Namespace: namespace,
				},
				Spec: warpgatev1alpha1.WarpgateTicketSpec{
					ConnectionRef: "nonexistent-conn",
					Username:      "someuser",
					TargetName:    "sometarget",
				},
			}
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
		})

		AfterEach(func() {
			cr := &warpgatev1alpha1.WarpgateTicket{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: crName, Namespace: namespace}, cr); err == nil {
				controllerutil.RemoveFinalizer(cr, ticketFinalizer)
				_ = k8sClient.Update(ctx, cr)
				_ = k8sClient.Delete(ctx, cr)
			}
		})

		It("should set Ready=False with ClientError when the connection doesn't exist", func() {
			nn := types.NamespacedName{Name: crName, Namespace: namespace}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).To(HaveOccurred())

			var updated warpgatev1alpha1.WarpgateTicket
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())

			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("ClientError"))
		})
	})

	Context("Ticket already exists (TicketID set)", func() {
		var (
			mockServer  *httptest.Server
			tokenSecret string
			connName    string
			crName      string
			namespace   string
		)

		BeforeEach(func() {
			tokenSecret = "ticket-noop-token"
			connName = "ticket-noop-conn"
			crName = "ticket-noop-tkt"
			namespace = testNamespace

			mux := http.NewServeMux()
			mockLogin(mux)
			// No POST handler needed - ticket should NOT be created again.
			mockServer = httptest.NewServer(mux)

			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: tokenSecret, Namespace: namespace},
				Data:       map[string][]byte{"username": []byte("admin"), "password": []byte("test-pass")},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			conn := &warpgatev1alpha1.WarpgateConnection{
				ObjectMeta: metav1.ObjectMeta{Name: connName, Namespace: namespace},
				Spec: warpgatev1alpha1.WarpgateConnectionSpec{
					Host:               mockServer.URL,
					AuthSecretRef:      warpgatev1alpha1.AuthSecretRef{Name: tokenSecret},
					InsecureSkipVerify: true,
				},
			}
			Expect(k8sClient.Create(ctx, conn)).To(Succeed())

			cr := &warpgatev1alpha1.WarpgateTicket{
				ObjectMeta: metav1.ObjectMeta{
					Name:       crName,
					Namespace:  namespace,
					Finalizers: []string{ticketFinalizer},
				},
				Spec: warpgatev1alpha1.WarpgateTicketSpec{
					ConnectionRef: connName,
					Username:      "existinguser",
					TargetName:    "existingtarget",
				},
			}
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())

			// Manually set the TicketID and SecretRef in status to simulate an already-created ticket.
			cr.Status.TicketID = "already-exists-id"
			cr.Status.SecretRef = crName + "-secret"
			Expect(k8sClient.Status().Update(ctx, cr)).To(Succeed())
		})

		AfterEach(func() {
			mockServer.Close()
			cr := &warpgatev1alpha1.WarpgateTicket{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: crName, Namespace: namespace}, cr); err == nil {
				controllerutil.RemoveFinalizer(cr, ticketFinalizer)
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

		It("should be a no-op and just mark Ready=True without creating a new ticket", func() {
			nn := types.NamespacedName{Name: crName, Namespace: namespace}

			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).NotTo(BeZero())

			var updated warpgatev1alpha1.WarpgateTicket
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())
			Expect(updated.Status.TicketID).To(Equal("already-exists-id"))

			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionTrue))
			Expect(readyCond.Reason).To(Equal("Reconciled"))
		})
	})

	Context("Delete ticket with empty TicketID", func() {
		var (
			mockServer  *httptest.Server
			tokenSecret string
			connName    string
			crName      string
			namespace   string
		)

		BeforeEach(func() {
			tokenSecret = "ticket-delempty-token"
			connName = "ticket-delempty-conn"
			crName = "ticket-delempty-tkt"
			namespace = testNamespace

			mux := http.NewServeMux()
			mockLogin(mux)
			mockServer = httptest.NewServer(mux)

			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: tokenSecret, Namespace: namespace},
				Data:       map[string][]byte{"username": []byte("admin"), "password": []byte("test-pass")},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			conn := &warpgatev1alpha1.WarpgateConnection{
				ObjectMeta: metav1.ObjectMeta{Name: connName, Namespace: namespace},
				Spec: warpgatev1alpha1.WarpgateConnectionSpec{
					Host:               mockServer.URL,
					AuthSecretRef:      warpgatev1alpha1.AuthSecretRef{Name: tokenSecret},
					InsecureSkipVerify: true,
				},
			}
			Expect(k8sClient.Create(ctx, conn)).To(Succeed())

			cr := &warpgatev1alpha1.WarpgateTicket{
				ObjectMeta: metav1.ObjectMeta{
					Name:       crName,
					Namespace:  namespace,
					Finalizers: []string{ticketFinalizer},
				},
				Spec: warpgatev1alpha1.WarpgateTicketSpec{
					ConnectionRef: connName,
					Username:      "emptyuser",
					TargetName:    "emptytarget",
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
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: tokenSecret, Namespace: namespace}, secret); err == nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should skip Warpgate API delete and just remove the finalizer", func() {
			nn := types.NamespacedName{Name: crName, Namespace: namespace}

			// Delete the CR (finalizer holds it).
			var cr warpgatev1alpha1.WarpgateTicket
			Expect(k8sClient.Get(ctx, nn, &cr)).To(Succeed())
			Expect(k8sClient.Delete(ctx, &cr)).To(Succeed())

			// Reconcile deletion: TicketID is empty so no API call, no SecretRef so no secret cleanup.
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// CR should be gone.
			err = k8sClient.Get(ctx, nn, &cr)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))
		})
	})

	Context("Delete ticket with populated IDs and missing secret", func() {
		var (
			mockServer   *httptest.Server
			tokenSecret  string
			connName     string
			crName       string
			namespace    string
			deleteCalled bool
		)

		BeforeEach(func() {
			tokenSecret = "ticket-delmissec-token"
			connName = "ticket-delmissec-conn"
			crName = "ticket-delmissec-tkt"
			namespace = testNamespace
			deleteCalled = false

			mux := http.NewServeMux()
			mockLogin(mux)
			mux.HandleFunc("/@warpgate/admin/api/tickets/pre-ticket-id", func(w http.ResponseWriter, r *http.Request) {
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

			conn := &warpgatev1alpha1.WarpgateConnection{
				ObjectMeta: metav1.ObjectMeta{Name: connName, Namespace: namespace},
				Spec: warpgatev1alpha1.WarpgateConnectionSpec{
					Host:               mockServer.URL,
					AuthSecretRef:      warpgatev1alpha1.AuthSecretRef{Name: tokenSecret},
					InsecureSkipVerify: true,
				},
			}
			Expect(k8sClient.Create(ctx, conn)).To(Succeed())

			cr := &warpgatev1alpha1.WarpgateTicket{
				ObjectMeta: metav1.ObjectMeta{
					Name:       crName,
					Namespace:  namespace,
					Finalizers: []string{ticketFinalizer},
				},
				Spec: warpgatev1alpha1.WarpgateTicketSpec{
					ConnectionRef: connName,
					Username:      "missecuser",
					TargetName:    "missectarget",
				},
			}
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())

			// Set status to simulate a previously-created ticket with a secret that no longer exists.
			cr.Status.TicketID = "pre-ticket-id"
			cr.Status.SecretRef = crName + "-secret"
			Expect(k8sClient.Status().Update(ctx, cr)).To(Succeed())
			// Deliberately do NOT create the secret - it's "already gone."
		})

		AfterEach(func() {
			mockServer.Close()
			conn := &warpgatev1alpha1.WarpgateConnection{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: connName, Namespace: namespace}, conn); err == nil {
				_ = k8sClient.Delete(ctx, conn)
			}
			secret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: tokenSecret, Namespace: namespace}, secret); err == nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should delete the ticket via API and tolerate missing secret gracefully", func() {
			nn := types.NamespacedName{Name: crName, Namespace: namespace}

			var cr warpgatev1alpha1.WarpgateTicket
			Expect(k8sClient.Get(ctx, nn, &cr)).To(Succeed())
			Expect(k8sClient.Delete(ctx, &cr)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			Expect(deleteCalled).To(BeTrue())

			// CR should be gone.
			err = k8sClient.Get(ctx, nn, &cr)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))
		})
	})

	Context("Secret creation failure", func() {
		var (
			mockServer  *httptest.Server
			tokenSecret string
			connName    string
			crName      string
			namespace   string
		)

		BeforeEach(func() {
			tokenSecret = "ticket-secfail-token"
			connName = "ticket-secfail-conn"
			crName = "ticket-secfail-tkt"
			namespace = testNamespace

			mux := http.NewServeMux()
			mockLogin(mux)
			mux.HandleFunc("/@warpgate/admin/api/tickets", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusCreated)
					_ = json.NewEncoder(w).Encode(map[string]any{
						"ticket": map[string]any{
							"id": "tsf1",
						},
						"secret": "secfail-ticket-secret",
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

			conn := &warpgatev1alpha1.WarpgateConnection{
				ObjectMeta: metav1.ObjectMeta{Name: connName, Namespace: namespace},
				Spec: warpgatev1alpha1.WarpgateConnectionSpec{
					Host:               mockServer.URL,
					AuthSecretRef:      warpgatev1alpha1.AuthSecretRef{Name: tokenSecret},
					InsecureSkipVerify: true,
				},
			}
			Expect(k8sClient.Create(ctx, conn)).To(Succeed())

			// Pre-create the secret that the controller will try to create, causing a conflict.
			conflictSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      crName + "-secret",
					Namespace: namespace,
					// Different owner / no owner ref, so it's a conflict for the controller.
				},
				Data: map[string][]byte{"secret": []byte("pre-existing")},
			}
			Expect(k8sClient.Create(ctx, conflictSecret)).To(Succeed())

			cr := &warpgatev1alpha1.WarpgateTicket{
				ObjectMeta: metav1.ObjectMeta{Name: crName, Namespace: namespace},
				Spec: warpgatev1alpha1.WarpgateTicketSpec{
					ConnectionRef: connName,
					Username:      "secfailuser",
					TargetName:    "secfailtarget",
				},
			}
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())
		})

		AfterEach(func() {
			mockServer.Close()
			cr := &warpgatev1alpha1.WarpgateTicket{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: crName, Namespace: namespace}, cr); err == nil {
				controllerutil.RemoveFinalizer(cr, ticketFinalizer)
				_ = k8sClient.Update(ctx, cr)
				_ = k8sClient.Delete(ctx, cr)
			}
			conflictSecret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: crName + "-secret", Namespace: namespace}, conflictSecret); err == nil {
				_ = k8sClient.Delete(ctx, conflictSecret)
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

		It("should set Ready=False with SecretCreateFailed when the secret already exists", func() {
			nn := types.NamespacedName{Name: crName, Namespace: namespace}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).To(HaveOccurred())

			var updated warpgatev1alpha1.WarpgateTicket
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())

			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("SecretCreateFailed"))
		})
	})

	Context("Delete ticket", func() {
		var (
			mockServer   *httptest.Server
			tokenSecret  string
			connName     string
			crName       string
			namespace    string
			deleteCalled bool
		)

		BeforeEach(func() {
			tokenSecret = "ticket-del-token"
			connName = "ticket-del-conn"
			crName = "ticket-del-tkt"
			namespace = testNamespace
			deleteCalled = false

			mux := http.NewServeMux()
			mockLogin(mux)
			mux.HandleFunc("/@warpgate/admin/api/tickets", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusCreated)
					_ = json.NewEncoder(w).Encode(map[string]any{
						"ticket": map[string]any{
							"id": "t2",
						},
						"secret": "del-ticket-secret",
					})
					return
				}
			})
			mux.HandleFunc("/@warpgate/admin/api/tickets/t2", func(w http.ResponseWriter, r *http.Request) {
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

			conn := &warpgatev1alpha1.WarpgateConnection{
				ObjectMeta: metav1.ObjectMeta{
					Name:      connName,
					Namespace: namespace,
				},
				Spec: warpgatev1alpha1.WarpgateConnectionSpec{
					Host:               mockServer.URL,
					AuthSecretRef:      warpgatev1alpha1.AuthSecretRef{Name: tokenSecret},
					InsecureSkipVerify: true,
				},
			}
			Expect(k8sClient.Create(ctx, conn)).To(Succeed())

			cr := &warpgatev1alpha1.WarpgateTicket{
				ObjectMeta: metav1.ObjectMeta{
					Name:      crName,
					Namespace: namespace,
				},
				Spec: warpgatev1alpha1.WarpgateTicketSpec{
					ConnectionRef: connName,
					Username:      "deluser",
					TargetName:    "deltarget",
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

		It("should delete the ticket and its Secret, and remove the finalizer", func() {
			nn := types.NamespacedName{Name: crName, Namespace: namespace}

			// Single reconcile: adds finalizer, creates ticket and secret.
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// Verify the ticket secret was created.
			var ticketSecret corev1.Secret
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      crName + "-secret",
				Namespace: namespace,
			}, &ticketSecret)).To(Succeed())

			// Delete the CR.
			var cr warpgatev1alpha1.WarpgateTicket
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

			// Ticket secret should also be gone.
			err = k8sClient.Get(ctx, types.NamespacedName{
				Name:      crName + "-secret",
				Namespace: namespace,
			}, &ticketSecret)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))
		})
	})

	Context("Delete ticket fails with API error", func() {
		var (
			mockServer  *httptest.Server
			tokenSecret string
			connName    string
			crName      string
			namespace   string
		)

		BeforeEach(func() {
			tokenSecret = "ticket-delfail-token"
			connName = "ticket-delfail-conn"
			crName = "ticket-delfail-tkt"
			namespace = testNamespace

			mux := http.NewServeMux()
			mockLogin(mux)
			mux.HandleFunc("/@warpgate/admin/api/tickets/fail-ticket-id", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodDelete {
					w.WriteHeader(http.StatusInternalServerError)
					_, _ = w.Write([]byte(`{"error":"internal server error"}`))
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

			conn := &warpgatev1alpha1.WarpgateConnection{
				ObjectMeta: metav1.ObjectMeta{Name: connName, Namespace: namespace},
				Spec: warpgatev1alpha1.WarpgateConnectionSpec{
					Host:               mockServer.URL,
					AuthSecretRef:      warpgatev1alpha1.AuthSecretRef{Name: tokenSecret},
					InsecureSkipVerify: true,
				},
			}
			Expect(k8sClient.Create(ctx, conn)).To(Succeed())

			cr := &warpgatev1alpha1.WarpgateTicket{
				ObjectMeta: metav1.ObjectMeta{
					Name:       crName,
					Namespace:  namespace,
					Finalizers: []string{ticketFinalizer},
				},
				Spec: warpgatev1alpha1.WarpgateTicketSpec{
					ConnectionRef: connName,
					Username:      "delfailuser",
					TargetName:    "delfailtarget",
				},
			}
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())

			// Simulate a previously-created ticket.
			cr.Status.TicketID = "fail-ticket-id"
			Expect(k8sClient.Status().Update(ctx, cr)).To(Succeed())
		})

		AfterEach(func() {
			mockServer.Close()
			cr := &warpgatev1alpha1.WarpgateTicket{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: crName, Namespace: namespace}, cr); err == nil {
				controllerutil.RemoveFinalizer(cr, ticketFinalizer)
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

		It("should return an error when the Warpgate delete API fails with non-404", func() {
			nn := types.NamespacedName{Name: crName, Namespace: namespace}

			// Mark the CR for deletion (finalizer holds it).
			var cr warpgatev1alpha1.WarpgateTicket
			Expect(k8sClient.Get(ctx, nn, &cr)).To(Succeed())
			Expect(k8sClient.Delete(ctx, &cr)).To(Succeed())

			// Reconcile: should attempt to delete via API, get a 500, and return error.
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("500"))

			// CR should still exist (finalizer not removed due to error).
			Expect(k8sClient.Get(ctx, nn, &cr)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(&cr, ticketFinalizer)).To(BeTrue())
		})
	})

	Context("Delete ticket when API returns 404 (already gone)", func() {
		var (
			mockServer  *httptest.Server
			tokenSecret string
			connName    string
			crName      string
			namespace   string
		)

		BeforeEach(func() {
			tokenSecret = "ticket-del404-token"
			connName = "ticket-del404-conn"
			crName = "ticket-del404-tkt"
			namespace = testNamespace

			mux := http.NewServeMux()
			mockLogin(mux)
			mux.HandleFunc("/@warpgate/admin/api/tickets/gone-ticket-id", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodDelete {
					w.WriteHeader(http.StatusNotFound)
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

			conn := &warpgatev1alpha1.WarpgateConnection{
				ObjectMeta: metav1.ObjectMeta{Name: connName, Namespace: namespace},
				Spec: warpgatev1alpha1.WarpgateConnectionSpec{
					Host:               mockServer.URL,
					AuthSecretRef:      warpgatev1alpha1.AuthSecretRef{Name: tokenSecret},
					InsecureSkipVerify: true,
				},
			}
			Expect(k8sClient.Create(ctx, conn)).To(Succeed())

			cr := &warpgatev1alpha1.WarpgateTicket{
				ObjectMeta: metav1.ObjectMeta{
					Name:       crName,
					Namespace:  namespace,
					Finalizers: []string{ticketFinalizer},
				},
				Spec: warpgatev1alpha1.WarpgateTicketSpec{
					ConnectionRef: connName,
					Username:      "del404user",
					TargetName:    "del404target",
				},
			}
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())

			cr.Status.TicketID = "gone-ticket-id"
			Expect(k8sClient.Status().Update(ctx, cr)).To(Succeed())
		})

		AfterEach(func() {
			mockServer.Close()
			conn := &warpgatev1alpha1.WarpgateConnection{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: connName, Namespace: namespace}, conn); err == nil {
				_ = k8sClient.Delete(ctx, conn)
			}
			secret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: tokenSecret, Namespace: namespace}, secret); err == nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should treat a 404 as success and remove the finalizer", func() {
			nn := types.NamespacedName{Name: crName, Namespace: namespace}

			var cr warpgatev1alpha1.WarpgateTicket
			Expect(k8sClient.Get(ctx, nn, &cr)).To(Succeed())
			Expect(k8sClient.Delete(ctx, &cr)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// CR should be gone.
			err = k8sClient.Get(ctx, nn, &cr)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))
		})
	})

	Context("Client error on deletion path", func() {
		var (
			crName    string
			namespace string
		)

		BeforeEach(func() {
			crName = "ticket-delclientfail-tkt"
			namespace = testNamespace

			cr := &warpgatev1alpha1.WarpgateTicket{
				ObjectMeta: metav1.ObjectMeta{
					Name:       crName,
					Namespace:  namespace,
					Finalizers: []string{ticketFinalizer},
				},
				Spec: warpgatev1alpha1.WarpgateTicketSpec{
					ConnectionRef: "nonexistent-del-conn",
					Username:      "someuser",
					TargetName:    "sometarget",
				},
			}
			Expect(k8sClient.Create(ctx, cr)).To(Succeed())

			cr.Status.TicketID = "some-ticket-id"
			Expect(k8sClient.Status().Update(ctx, cr)).To(Succeed())
		})

		AfterEach(func() {
			cr := &warpgatev1alpha1.WarpgateTicket{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: crName, Namespace: namespace}, cr); err == nil {
				controllerutil.RemoveFinalizer(cr, ticketFinalizer)
				_ = k8sClient.Update(ctx, cr)
				_ = k8sClient.Delete(ctx, cr)
			}
		})

		It("should return an error when the connection cannot be resolved during deletion", func() {
			nn := types.NamespacedName{Name: crName, Namespace: namespace}

			var cr warpgatev1alpha1.WarpgateTicket
			Expect(k8sClient.Get(ctx, nn, &cr)).To(Succeed())
			Expect(k8sClient.Delete(ctx, &cr)).To(Succeed())

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).To(HaveOccurred())

			// CR should still exist (couldn't build client for deletion).
			Expect(k8sClient.Get(ctx, nn, &cr)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(&cr, ticketFinalizer)).To(BeTrue())
		})
	})
})
