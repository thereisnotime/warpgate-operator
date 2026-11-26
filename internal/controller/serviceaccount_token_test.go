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
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	warpgatev1alpha1 "github.com/thereisnotime/warpgate-operator/api/v1alpha1"
	"github.com/thereisnotime/warpgate-operator/internal/warpgate"
)

// fakeWarpgate records every target create/update and the token each one carried.
type fakeWarpgate struct {
	mu     sync.Mutex
	srv    *httptest.Server
	tokens []string // auth.token of every POST/PUT, in order
	// failWith, when set, makes create/update fail with this status and echo the
	// request body back, like an API that quotes the offending input.
	failWith int
}

func newFakeWarpgate() *fakeWarpgate {
	f := &fakeWarpgate{}
	mux := http.NewServeMux()
	mockLogin(mux)
	handle := func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Options struct {
				Auth struct {
					Token string `json:"token"`
				} `json:"auth"`
			} `json:"options"`
		}
		_ = json.Unmarshal(body, &req)
		f.mu.Lock()
		f.tokens = append(f.tokens, req.Options.Auth.Token)
		failWith := f.failWith
		f.mu.Unlock()
		if failWith != 0 {
			w.WriteHeader(failWith)
			_, _ = w.Write(body)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "k8s-target-1", "name": "k8s", "options": json.RawMessage(`{"kind":"Kubernetes"}`)})
	}
	mux.HandleFunc("/@warpgate/admin/api/targets", handle)
	mux.HandleFunc("/@warpgate/admin/api/targets/", handle)
	f.srv = httptest.NewServer(mux)
	return f
}

func (f *fakeWarpgate) sent() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.tokens...)
}

func (f *fakeWarpgate) setFail(status int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failWith = status
}

// jwtClaims decodes a JWT payload without verifying it: enough to check what the API
// server issued.
func jwtClaims(token string) map[string]any {
	parts := strings.Split(token, ".")
	ExpectWithOffset(1, parts).To(HaveLen(3))
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	var claims map[string]any
	ExpectWithOffset(1, json.Unmarshal(payload, &claims)).To(Succeed())
	return claims
}

// interceptedClient is an admin client for the test API server with funcs applied.
func interceptedClient(funcs interceptor.Funcs) client.Client {
	wc, err := client.NewWithWatch(cfg, client.Options{Scheme: k8sClient.Scheme()})
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	return interceptor.NewClient(wc, funcs)
}

// annotations builds the two opt-in annotations.
func annotations(audiences, clusterURLs string) map[string]string {
	return map[string]string{
		warpgatev1alpha1.ServiceAccountTokenAudiencesAnnotation:   audiences,
		warpgatev1alpha1.ServiceAccountTokenClusterURLsAnnotation: clusterURLs,
	}
}

// testClock is a clock tests can move forward.
type testClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *testClock) Set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = t
}

var _ = Describe("Kubernetes targets with serviceAccountToken", func() {
	const audience = "zone10-drinking"

	var (
		wg         *fakeWarpgate
		reconciler *WarpgateTargetReconciler
		clock      *testClock
		suffix     string
		connName   string
		authSecret string
		saName     string
		targetName string
		nn         types.NamespacedName
	)

	createSA := func(name string, annotations map[string]string) {
		sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNamespace, Annotations: annotations}}
		ExpectWithOffset(1, k8sClient.Create(ctx, sa)).To(Succeed())
	}

	deleteSA := func(name string) {
		sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNamespace}}
		ExpectWithOffset(1, client.IgnoreNotFound(k8sClient.Delete(ctx, sa))).To(Succeed())
		EventuallyWithOffset(1, func() bool {
			return apierrors.IsNotFound(k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: name}, &corev1.ServiceAccount{}))
		}).Should(BeTrue())
	}

	const clusterURL = "https://192.168.42.49:6443"

	// optIn allows the audiences and the default clusterURL.
	optIn := func(audiences ...string) map[string]string {
		return map[string]string{
			warpgatev1alpha1.ServiceAccountTokenAudiencesAnnotation:   strings.Join(audiences, ","),
			warpgatev1alpha1.ServiceAccountTokenClusterURLsAnnotation: clusterURL,
		}
	}

	createTarget := func(sat *warpgatev1alpha1.ServiceAccountTokenSpec) {
		target := &warpgatev1alpha1.WarpgateTarget{
			ObjectMeta: metav1.ObjectMeta{Name: targetName, Namespace: testNamespace},
			Spec: warpgatev1alpha1.WarpgateTargetSpec{
				ConnectionRef: connName,
				Name:          "k8s-" + suffix,
				Kubernetes: &warpgatev1alpha1.KubernetesTargetSpec{
					ClusterURL:          clusterURL,
					AuthKind:            "Token",
					ServiceAccountToken: sat,
				},
			},
		}
		ExpectWithOffset(1, k8sClient.Create(ctx, target)).To(Succeed())
	}

	reconcileOnce := func() reconcile.Result {
		res, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		ExpectWithOffset(1, err).NotTo(HaveOccurred())
		return res
	}

	getTarget := func() *warpgatev1alpha1.WarpgateTarget {
		var t warpgatev1alpha1.WarpgateTarget
		ExpectWithOffset(1, k8sClient.Get(ctx, nn, &t)).To(Succeed())
		return &t
	}

	ready := func() *metav1.Condition {
		c := findReadyCondition(getTarget().Status.Conditions)
		ExpectWithOffset(1, c).NotTo(BeNil())
		return c
	}

	BeforeEach(func() {
		suffix = fmt.Sprintf("%d", time.Now().UnixNano()%1_000_000_000)
		connName, authSecret = "sat-conn-"+suffix, "sat-auth-"+suffix
		saName, targetName = "sat-sa-"+suffix, "sat-target-"+suffix
		nn = types.NamespacedName{Namespace: testNamespace, Name: targetName}

		wg = newFakeWarpgate()
		Expect(k8sClient.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: authSecret, Namespace: testNamespace},
			Data:       map[string][]byte{"username": []byte("admin"), "password": []byte("pw")},
		})).To(Succeed())
		Expect(k8sClient.Create(ctx, &warpgatev1alpha1.WarpgateConnection{
			ObjectMeta: metav1.ObjectMeta{Name: connName, Namespace: testNamespace},
			Spec: warpgatev1alpha1.WarpgateConnectionSpec{
				Host:          wg.srv.URL,
				AuthSecretRef: warpgatev1alpha1.AuthSecretRef{Name: authSecret},
			},
		})).To(Succeed())

		clock = &testClock{t: time.Now()}
		reconciler = &WarpgateTargetReconciler{
			Client:            k8sClient,
			Scheme:            k8sClient.Scheme(),
			ReconcileInterval: time.Hour,
			Now:               clock.Now,
		}
	})

	AfterEach(func() {
		wg.srv.Close()
		var t warpgatev1alpha1.WarpgateTarget
		if err := k8sClient.Get(ctx, nn, &t); err == nil {
			controllerutil.RemoveFinalizer(&t, targetFinalizerName)
			_ = k8sClient.Update(ctx, &t)
			_ = k8sClient.Delete(ctx, &t)
		}
		var conn warpgatev1alpha1.WarpgateConnection
		if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: testNamespace, Name: connName}, &conn); err == nil {
			controllerutil.RemoveFinalizer(&conn, warpgateFinalizer)
			_ = k8sClient.Update(ctx, &conn)
			_ = k8sClient.Delete(ctx, &conn)
		}
		_ = k8sClient.Delete(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: authSecret, Namespace: testNamespace}})
		deleteSA(saName)
	})

	It("issues a token for the ServiceAccount, sends it to Warpgate and reports only its timing", func() {
		createSA(saName, optIn(audience, "other"))
		createTarget(&warpgatev1alpha1.ServiceAccountTokenSpec{Name: saName, Audiences: []string{audience}, ExpirationSeconds: new(int64(7200))})

		res := reconcileOnce()

		sent := wg.sent()
		Expect(sent).To(HaveLen(1))
		token := sent[0]
		Expect(token).NotTo(BeEmpty())

		claims := jwtClaims(token)
		Expect(claims["sub"]).To(Equal(fmt.Sprintf("system:serviceaccount:%s:%s", testNamespace, saName)))
		Expect(claims["aud"]).To(ConsistOf(audience), "only the requested audience, never the API server's own")
		lifetime := time.Duration(claims["exp"].(float64)-claims["iat"].(float64)) * time.Second
		Expect(lifetime).To(BeNumerically("~", 2*time.Hour, time.Minute))

		got := getTarget()
		Expect(got.Status.ExternalID).To(Equal("k8s-target-1"))
		Expect(got.Status.ServiceAccountToken).NotTo(BeNil())
		exp := got.Status.ServiceAccountToken.ExpirationTimestamp.Time
		renew := got.Status.ServiceAccountToken.RenewAfter.Time
		Expect(exp).To(BeTemporally("~", time.Unix(int64(claims["exp"].(float64)), 0), time.Second))
		Expect(renew).To(BeTemporally("~", clock.Now().Add(exp.Sub(clock.Now())/2), 2*time.Second))

		c := ready()
		Expect(c.Status).To(Equal(metav1.ConditionTrue))
		Expect(c.Reason).To(Equal("Kubernetes"))

		// Requeued in time to renew, not after the (longer) reconcile interval.
		Expect(res.RequeueAfter).To(BeNumerically("~", renew.Sub(clock.Now()), 2*time.Second))
		Expect(res.RequeueAfter).To(BeNumerically("<", reconciler.ReconcileInterval))

		// The token is nowhere in the stored object.
		raw, err := json.Marshal(got)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(raw)).NotTo(ContainSubstring(token))
	})

	It("reuses the token until renewAfter and renews it after", func() {
		createSA(saName, optIn(audience))
		createTarget(&warpgatev1alpha1.ServiceAccountTokenSpec{Name: saName, Audiences: []string{audience}, ExpirationSeconds: new(int64(7200))})

		reconcileOnce()
		first := getTarget().Status.ServiceAccountToken.RenewAfter.Time

		// Just before renewAfter: same token, no new TokenRequest.
		clock.Set(first.Add(-time.Second))
		reconcileOnce()
		Expect(wg.sent()).To(HaveLen(2))
		Expect(wg.sent()[1]).To(Equal(wg.sent()[0]))
		Expect(getTarget().Status.ServiceAccountToken.RenewAfter.Time).To(BeTemporally("==", first))

		// Past renewAfter (status has whole seconds, the in-memory time doesn't): a new token.
		clock.Set(first.Add(time.Second))
		reconcileOnce()
		sent := wg.sent()
		Expect(sent).To(HaveLen(3))
		Expect(sent[2]).NotTo(Equal(sent[0]))
		Expect(getTarget().Status.ServiceAccountToken.RenewAfter.Time).To(BeTemporally(">", first))
	})

	It("requests a new token as soon as the token source changes", func() {
		createSA(saName, optIn(audience, "second"))
		createTarget(&warpgatev1alpha1.ServiceAccountTokenSpec{Name: saName, Audiences: []string{audience}, ExpirationSeconds: new(int64(7200))})
		reconcileOnce()

		t := getTarget()
		t.Spec.Kubernetes.ServiceAccountToken.Audiences = []string{audience, "second"}
		Expect(k8sClient.Update(ctx, t)).To(Succeed())
		reconcileOnce()

		sent := wg.sent()
		Expect(sent).To(HaveLen(2))
		Expect(sent[1]).NotTo(Equal(sent[0]))
		Expect(jwtClaims(sent[1])["aud"]).To(ConsistOf(audience, "second"))

		// Changing the lifetime also counts.
		t = getTarget()
		t.Spec.Kubernetes.ServiceAccountToken.ExpirationSeconds = new(int64(10800))
		Expect(k8sClient.Update(ctx, t)).To(Succeed())
		reconcileOnce()
		Expect(wg.sent()[2]).NotTo(Equal(sent[1]))
	})

	It("requests a new token when the ServiceAccount is recreated", func() {
		createSA(saName, optIn(audience))
		createTarget(&warpgatev1alpha1.ServiceAccountTokenSpec{Name: saName, Audiences: []string{audience}, ExpirationSeconds: new(int64(7200))})
		reconcileOnce()

		deleteSA(saName)
		createSA(saName, optIn(audience))
		reconcileOnce()

		sent := wg.sent()
		Expect(sent).To(HaveLen(2))
		Expect(sent[1]).NotTo(Equal(sent[0]), "the old token belongs to the deleted ServiceAccount")
	})

	DescribeTable("refuses to request a token without consent",
		func(annotations map[string]string, createIt bool, audiences []string, wantReason string) {
			if createIt {
				createSA(saName, annotations)
			}
			createTarget(&warpgatev1alpha1.ServiceAccountTokenSpec{Name: saName, Audiences: audiences, ExpirationSeconds: new(int64(7200))})

			res := reconcileOnce()

			Expect(wg.sent()).To(BeEmpty(), "Warpgate must not be called")
			c := ready()
			Expect(c.Status).To(Equal(metav1.ConditionFalse))
			Expect(c.Reason).To(Equal(wantReason))
			Expect(res.RequeueAfter).To(Equal(tokenRetryInterval), "token failures retry sooner than the reconcile interval")
			Expect(getTarget().Status.ServiceAccountToken).To(BeNil())
			Expect(reconciler.tokenCache().len()).To(BeZero())
		},
		Entry("ServiceAccount missing", nil, false, []string{audience}, ReasonServiceAccountNotFound),
		Entry("no annotation", nil, true, []string{audience}, ReasonServiceAccountNotOptIn),
		Entry("annotation without the audience", annotations("other", clusterURL), true,
			[]string{audience}, ReasonTokenAudienceNotAllowed),
		Entry("only some audiences allowed", annotations(audience, clusterURL), true,
			[]string{audience, "extra"}, ReasonTokenAudienceNotAllowed),
		Entry("empty audience annotation", annotations(" , ", clusterURL), true,
			[]string{audience}, ReasonTokenAudienceNotAllowed),
		Entry("audience annotation matching only a prefix", annotations(audience+"-x", clusterURL), true,
			[]string{audience}, ReasonTokenAudienceNotAllowed),
		Entry("no cluster URL annotation", map[string]string{warpgatev1alpha1.ServiceAccountTokenAudiencesAnnotation: audience}, true,
			[]string{audience}, ReasonServiceAccountNotOptIn),
		Entry("cluster URL not allowed", annotations(audience, "https://other:6443"), true,
			[]string{audience}, ReasonClusterURLNotAllowed),
		Entry("cluster URL matching only a prefix", annotations(audience, "https://192.168.42.49"), true,
			[]string{audience}, ReasonClusterURLNotAllowed),
		Entry("cluster URL with a different path", annotations(audience, clusterURL+"/"), true,
			[]string{audience}, ReasonClusterURLNotAllowed),
		Entry("empty cluster URL annotation", annotations(audience, ""), true,
			[]string{audience}, ReasonClusterURLNotAllowed),
	)

	It("never requests a token without audiences", func() {
		// The CRD schema rejects an empty list even without the webhook...
		createSA(saName, optIn(audience))
		target := &warpgatev1alpha1.WarpgateTarget{
			ObjectMeta: metav1.ObjectMeta{Name: targetName, Namespace: testNamespace},
			Spec: warpgatev1alpha1.WarpgateTargetSpec{
				ConnectionRef: connName,
				Name:          "k8s-" + suffix,
				Kubernetes: &warpgatev1alpha1.KubernetesTargetSpec{
					ClusterURL: "https://k", AuthKind: "Token",
					ServiceAccountToken: &warpgatev1alpha1.ServiceAccountTokenSpec{Name: saName, Audiences: []string{}},
				},
			},
		}
		err := k8sClient.Create(ctx, target)
		Expect(apierrors.IsInvalid(err)).To(BeTrue(), "got %v", err)

		// ...and the controller refuses one on its own, before asking the API server
		// (which would issue a token for its own audiences).
		target.UID = "no-audiences"
		_, err = reconciler.serviceAccountToken(ctx, target)
		reason, ok := tokenErrorReason(err)
		Expect(ok).To(BeTrue())
		Expect(reason).To(Equal(ReasonTokenRequestError))
		Expect(reconciler.tokenCache().len()).To(BeZero())
	})

	Context("with the operator's RBAC limited like the Helm chart renders it", func() {
		var limited client.Client

		BeforeEach(func() {
			user, err := testEnv.AddUser(envtest.User{Name: "sat-operator-" + suffix}, nil)
			Expect(err).NotTo(HaveOccurred())
			limited, err = client.New(user.Config(), client.Options{Scheme: k8sClient.Scheme()})
			Expect(err).NotTo(HaveOccurred())

			// Same rules as charts/warpgate-operator/templates/serviceaccount-tokens-rbac.yaml.
			role := &rbacv1.Role{
				ObjectMeta: metav1.ObjectMeta{Name: "sat-" + suffix, Namespace: testNamespace},
				Rules: []rbacv1.PolicyRule{
					{APIGroups: []string{""}, Resources: []string{"serviceaccounts/token"}, ResourceNames: []string{saName}, Verbs: []string{"create"}},
					{APIGroups: []string{""}, Resources: []string{"serviceaccounts"}, ResourceNames: []string{saName}, Verbs: []string{"get"}},
				},
			}
			Expect(k8sClient.Create(ctx, role)).To(Succeed())
			Expect(k8sClient.Create(ctx, &rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{Name: "sat-" + suffix, Namespace: testNamespace},
				RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "Role", Name: role.Name},
				Subjects:   []rbacv1.Subject{{Kind: rbacv1.UserKind, Name: "sat-operator-" + suffix}},
			})).To(Succeed())
			DeferCleanup(func() {
				_ = k8sClient.Delete(ctx, &rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: "sat-" + suffix, Namespace: testNamespace}})
				_ = k8sClient.Delete(ctx, role)
			})

			// Everything else goes through the admin client; ServiceAccount reads and token
			// requests go through the limited user, as with the chart's Role.
			reconciler.APIReader = limited
			reconciler.Client = interceptedClient(interceptor.Funcs{
				SubResourceCreate: func(ctx context.Context, c client.Client, sub string, obj client.Object, subObj client.Object, opts ...client.SubResourceCreateOption) error {
					if sub == "token" {
						return limited.SubResource("token").Create(ctx, obj, subObj, opts...)
					}
					return c.SubResource(sub).Create(ctx, obj, subObj, opts...)
				},
			})
		})

		It("gets a token for the listed ServiceAccount", func() {
			createSA(saName, optIn(audience))
			createTarget(&warpgatev1alpha1.ServiceAccountTokenSpec{Name: saName, Audiences: []string{audience}, ExpirationSeconds: new(int64(7200))})
			reconcileOnce()
			Expect(wg.sent()).To(HaveLen(1))
			Expect(ready().Status).To(Equal(metav1.ConditionTrue))
		})

		It("is refused a token for any other ServiceAccount, even an opted-in one", func() {
			other := "sat-other-" + suffix
			createSA(other, optIn(audience))
			DeferCleanup(deleteSA, other)

			// With the chart's Role, the uncached read is what fails first.
			createTarget(&warpgatev1alpha1.ServiceAccountTokenSpec{Name: other, Audiences: []string{audience}, ExpirationSeconds: new(int64(7200))})
			reconcileOnce()

			Expect(wg.sent()).To(BeEmpty())
			c := ready()
			Expect(c.Status).To(Equal(metav1.ConditionFalse))
			Expect(c.Reason).To(Equal(ReasonTokenRequestForbidden))
			Expect(c.Message).To(ContainSubstring("serviceAccountTokens.allowed"))
		})

		It("is refused by the API server when only the read is allowed", func() {
			other := "sat-other-" + suffix
			createSA(other, optIn(audience))
			DeferCleanup(deleteSA, other)

			// Read through the admin client, so the token request itself hits RBAC.
			reconciler.APIReader = k8sClient
			createTarget(&warpgatev1alpha1.ServiceAccountTokenSpec{Name: other, Audiences: []string{audience}, ExpirationSeconds: new(int64(7200))})
			reconcileOnce()

			Expect(wg.sent()).To(BeEmpty())
			c := ready()
			Expect(c.Reason).To(Equal(ReasonTokenRequestForbidden))
			Expect(c.Message).To(ContainSubstring("serviceaccounts/token"))
		})
	})

	It("requests a new token when clusterURL changes, and only to an allowed URL", func() {
		second := "https://10.0.0.2:6443"
		createSA(saName, annotations(audience, clusterURL+","+second))
		createTarget(&warpgatev1alpha1.ServiceAccountTokenSpec{Name: saName, Audiences: []string{audience}, ExpirationSeconds: new(int64(7200))})
		reconcileOnce()

		t := getTarget()
		t.Spec.Kubernetes.ClusterURL = second
		Expect(k8sClient.Update(ctx, t)).To(Succeed())
		reconcileOnce()
		sent := wg.sent()
		Expect(sent).To(HaveLen(2))
		Expect(sent[1]).NotTo(Equal(sent[0]), "a token is never reused for another cluster")

		// Repointing to a URL the ServiceAccount doesn't allow sends nothing there.
		t = getTarget()
		t.Spec.Kubernetes.ClusterURL = "https://attacker.example:6443"
		Expect(k8sClient.Update(ctx, t)).To(Succeed())
		res := reconcileOnce()
		Expect(wg.sent()).To(HaveLen(2), "Warpgate must not receive a token for the new URL")
		Expect(ready().Reason).To(Equal(ReasonClusterURLNotAllowed))
		Expect(res.RequeueAfter).To(Equal(tokenRetryInterval))
	})

	It("retries a failed Warpgate update quickly when a token is involved", func() {
		createSA(saName, optIn(audience))
		createTarget(&warpgatev1alpha1.ServiceAccountTokenSpec{Name: saName, Audiences: []string{audience}, ExpirationSeconds: new(int64(7200))})
		reconcileOnce()

		wg.setFail(http.StatusInternalServerError)
		res := reconcileOnce()
		Expect(ready().Reason).To(Equal("UpdateError"))
		Expect(res.RequeueAfter).To(Equal(tokenRetryInterval), "not the 1h reconcile interval")
	})

	It("reports other TokenRequest failures as TokenRequestError", func() {
		createSA(saName, optIn(audience))
		createTarget(&warpgatev1alpha1.ServiceAccountTokenSpec{Name: saName, Audiences: []string{audience}, ExpirationSeconds: new(int64(7200))})
		reconciler.Client = interceptedClient(interceptor.Funcs{
			SubResourceCreate: func(ctx context.Context, c client.Client, sub string, obj client.Object, subObj client.Object, opts ...client.SubResourceCreateOption) error {
				return apierrors.NewServiceUnavailable("apiserver overloaded")
			},
		})
		reconcileOnce()
		Expect(wg.sent()).To(BeEmpty())
		Expect(ready().Reason).To(Equal(ReasonTokenRequestError))
	})

	It("rejects a TokenRequest response without a usable token", func() {
		createSA(saName, optIn(audience))
		createTarget(&warpgatev1alpha1.ServiceAccountTokenSpec{Name: saName, Audiences: []string{audience}, ExpirationSeconds: new(int64(7200))})
		reconciler.Client = interceptedClient(interceptor.Funcs{
			SubResourceCreate: func(ctx context.Context, c client.Client, sub string, obj client.Object, subObj client.Object, opts ...client.SubResourceCreateOption) error {
				return nil // leaves Status.Token empty and the expiry at the zero time
			},
		})
		reconcileOnce()
		Expect(wg.sent()).To(BeEmpty())
		Expect(ready().Reason).To(Equal(ReasonTokenRequestError))
		Expect(reconciler.tokenCache().len()).To(BeZero())
	})

	It("redacts the token from Warpgate errors in the condition", func() {
		createSA(saName, optIn(audience))
		createTarget(&warpgatev1alpha1.ServiceAccountTokenSpec{Name: saName, Audiences: []string{audience}, ExpirationSeconds: new(int64(7200))})
		wg.setFail(http.StatusBadRequest) // echoes the request body, token included

		reconcileOnce()

		sent := wg.sent()
		Expect(sent).To(HaveLen(1))
		c := ready()
		Expect(c.Reason).To(Equal("CreateError"))
		Expect(c.Message).To(ContainSubstring(redactedToken))
		Expect(c.Message).NotTo(ContainSubstring(sent[0]))
		raw, err := json.Marshal(getTarget())
		Expect(err).NotTo(HaveOccurred())
		Expect(string(raw)).NotTo(ContainSubstring(sent[0]))

		// The token stays cached, so the retry doesn't request another one.
		wg.setFail(0)
		reconcileOnce()
		Expect(wg.sent()[1]).To(Equal(sent[0]))
	})

	It("clears the token state when the target stops using a ServiceAccount", func() {
		createSA(saName, optIn(audience))
		createTarget(&warpgatev1alpha1.ServiceAccountTokenSpec{Name: saName, Audiences: []string{audience}, ExpirationSeconds: new(int64(7200))})
		reconcileOnce()
		Expect(reconciler.tokenCache().len()).To(Equal(1))

		secretName := "sat-static-" + suffix
		Expect(k8sClient.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: testNamespace},
			Data:       map[string][]byte{"token": []byte("static-token")},
		})).To(Succeed())
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: testNamespace}})
		})
		t := getTarget()
		t.Spec.Kubernetes.ServiceAccountToken = nil
		t.Spec.Kubernetes.TokenSecretRef = &warpgatev1alpha1.SecretKeyRef{Name: secretName}
		Expect(k8sClient.Update(ctx, t)).To(Succeed())

		res := reconcileOnce()
		Expect(wg.sent()[1]).To(Equal("static-token"))
		Expect(getTarget().Status.ServiceAccountToken).To(BeNil())
		Expect(reconciler.tokenCache().len()).To(BeZero())
		Expect(res.RequeueAfter).To(Equal(reconciler.ReconcileInterval))

		// With interval 0 (no periodic resync) a target without a token is not requeued.
		reconciler.ReconcileInterval = 0
		res = reconcileOnce()
		Expect(res.RequeueAfter).To(BeZero())
	})

	It("forgets the token when the target is deleted", func() {
		createSA(saName, optIn(audience))
		createTarget(&warpgatev1alpha1.ServiceAccountTokenSpec{Name: saName, Audiences: []string{audience}, ExpirationSeconds: new(int64(7200))})
		reconcileOnce()
		Expect(reconciler.tokenCache().len()).To(Equal(1))

		Expect(k8sClient.Delete(ctx, getTarget())).To(Succeed())
		reconcileOnce()
		Expect(reconciler.tokenCache().len()).To(BeZero())
	})

	It("reports a failed ServiceAccount read other than not-found as TokenRequestError", func() {
		createSA(saName, optIn(audience))
		createTarget(&warpgatev1alpha1.ServiceAccountTokenSpec{Name: saName, Audiences: []string{audience}, ExpirationSeconds: new(int64(7200))})
		reconciler.APIReader = interceptedClient(interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if _, ok := obj.(*corev1.ServiceAccount); ok {
					return apierrors.NewTimeoutError("etcd slow", 1)
				}
				return c.Get(ctx, key, obj, opts...)
			},
		})
		reconcileOnce()
		Expect(wg.sent()).To(BeEmpty())
		Expect(ready().Reason).To(Equal(ReasonTokenRequestError))
	})

	It("refuses both token sources at once (webhook bypassed)", func() {
		createSA(saName, optIn(audience))
		target := &warpgatev1alpha1.WarpgateTarget{
			ObjectMeta: metav1.ObjectMeta{Name: targetName, Namespace: testNamespace},
			Spec: warpgatev1alpha1.WarpgateTargetSpec{
				ConnectionRef: connName,
				Name:          "k8s-" + suffix,
				Kubernetes: &warpgatev1alpha1.KubernetesTargetSpec{
					ClusterURL: "https://k", AuthKind: "Token",
					TokenSecretRef:      &warpgatev1alpha1.SecretKeyRef{Name: "whatever"},
					ServiceAccountToken: &warpgatev1alpha1.ServiceAccountTokenSpec{Name: saName, Audiences: []string{audience}},
				},
			},
		}
		Expect(k8sClient.Create(ctx, target)).To(Succeed())
		reconcileOnce()
		c := ready()
		Expect(c.Reason).To(Equal("BuildError"))
		Expect(c.Message).To(ContainSubstring("mutually exclusive"))
		Expect(wg.sent()).To(BeEmpty())
		Expect(reconciler.tokenCache().len()).To(BeZero())
	})

	It("sets BuildError instead of crashing on a Token target with no source (webhook bypassed)", func() {
		target := &warpgatev1alpha1.WarpgateTarget{
			ObjectMeta: metav1.ObjectMeta{Name: targetName, Namespace: testNamespace},
			Spec: warpgatev1alpha1.WarpgateTargetSpec{
				ConnectionRef: connName,
				Name:          "k8s-" + suffix,
				Kubernetes:    &warpgatev1alpha1.KubernetesTargetSpec{ClusterURL: "https://k", AuthKind: "Token"},
			},
		}
		Expect(k8sClient.Create(ctx, target)).To(Succeed())
		Expect(func() { reconcileOnce() }).NotTo(Panic())
		Expect(ready().Reason).To(Equal("BuildError"))
		Expect(wg.sent()).To(BeEmpty())
	})
})

var _ = Describe("serviceAccountToken helpers", func() {
	It("fingerprints the token source, ignoring audience order", func() {
		base := tokenFingerprint("sa", "uid-1", "https://k", []string{"a", "b"}, 7200)
		Expect(tokenFingerprint("sa", "uid-1", "https://k", []string{"b", "a"}, 7200)).To(Equal(base))
		Expect(tokenFingerprint("sa2", "uid-1", "https://k", []string{"a", "b"}, 7200)).NotTo(Equal(base))
		Expect(tokenFingerprint("sa", "uid-2", "https://k", []string{"a", "b"}, 7200)).NotTo(Equal(base))
		Expect(tokenFingerprint("sa", "uid-1", "https://other", []string{"a", "b"}, 7200)).NotTo(Equal(base))
		Expect(tokenFingerprint("sa", "uid-1", "https://k", []string{"a"}, 7200)).NotTo(Equal(base))
		Expect(tokenFingerprint("sa", "uid-1", "https://k", []string{"a", "b"}, 10800)).NotTo(Equal(base))
		// Separators keep concatenations apart.
		Expect(tokenFingerprint("sa", "uid-1", "u", []string{"ab", "c"}, 7200)).NotTo(Equal(tokenFingerprint("sa", "uid-1", "u", []string{"a", "bc"}, 7200)))
		Expect(tokenFingerprint("s", "auid", "u", nil, 1)).NotTo(Equal(tokenFingerprint("sa", "uid", "u", nil, 1)))
	})

	It("does not modify the caller's audience slice", func() {
		auds := []string{"b", "a"}
		_ = tokenFingerprint("sa", "uid", "u", auds, 7200)
		Expect(auds).To(Equal([]string{"b", "a"}))
	})

	It("parses the annotation lists exactly", func() {
		Expect(annotationList("a, b ,,c").UnsortedList()).To(ConsistOf("a", "b", "c"))
		Expect(annotationList("").Len()).To(BeZero())
		Expect(annotationList(" , ").Len()).To(BeZero())
		Expect(annotationList("https://x/zone09").Has("https://x/zone09")).To(BeTrue())
		Expect(annotationList("ab").Has("a")).To(BeFalse())
		Expect(annotationList("https://k:6443").Has("https://k:6443/")).To(BeFalse())
		Expect(annotationList("HTTPS://K").Has("https://k")).To(BeFalse(), "case-sensitive")
	})

	It("renews halfway through the granted lifetime", func() {
		issued := time.Date(2026, 11, 27, 10, 0, 0, 0, time.UTC)
		Expect(renewTime(issued, issued.Add(2*time.Hour))).To(Equal(issued.Add(time.Hour)))
		Expect(renewTime(issued, issued.Add(10*time.Minute))).To(Equal(issued.Add(5*time.Minute)), "a lifetime shortened by the API server")
	})

	It("redacts tokens from strings and errors", func() {
		Expect(redact("token abc in abc", "abc")).To(Equal("token [REDACTED] in [REDACTED]"))
		Expect(redact("nothing", "")).To(Equal("nothing"))
		err := errors.New("bad request: {\"token\":\"secret-value\"}")
		Expect(redactError(err, "secret-value").Error()).To(Equal("bad request: {\"token\":\"[REDACTED]\"}"))

		// Each JWT segment on its own, e.g. an API that quotes only the payload.
		jwt := "aaaaaaaaaaaa.bbbbbbbbbbbbbbbb.cccccccccccccccc"
		Expect(redact("payload bbbbbbbbbbbbbbbb sig cccccccccccccccc", jwt)).To(Equal("payload [REDACTED] sig [REDACTED]"))
		Expect(redact("a.b short", "a.b.c")).To(Equal("a.b short"), "fragments under 8 characters are left alone")

		// A Warpgate API error keeps its type and status, so IsNotFound still works.
		apiErr := &warpgate.APIError{StatusCode: http.StatusNotFound, Body: "no target with token " + jwt}
		red := redactError(apiErr, jwt)
		Expect(warpgate.IsNotFound(red)).To(BeTrue())
		Expect(red.Error()).NotTo(ContainSubstring("bbbbbbbbbbbbbbbb"))
		Expect(apiErr.Body).To(ContainSubstring(jwt), "the original error is not modified")
		Expect(redactError(err, "")).To(BeIdenticalTo(err))
		Expect(redactError(err, "absent")).To(BeIdenticalTo(err))
		Expect(redactError(nil, "x")).To(Succeed())
	})

	It("reuses a cached token only for the same source and before renewAfter", func() {
		c := newTokenCache()
		now := time.Now()
		c.put("uid", issuedToken{token: "t", renewAt: now.Add(time.Minute), expiresAt: now.Add(2 * time.Minute), fingerprint: "fp"}, now)
		_, ok := c.get("uid", "fp", now)
		Expect(ok).To(BeTrue())
		_, ok = c.get("uid", "other", now)
		Expect(ok).To(BeFalse())
		_, ok = c.get("uid", "fp", now.Add(time.Minute))
		Expect(ok).To(BeFalse(), "renewAfter itself is due")
		_, ok = c.get("missing", "fp", now)
		Expect(ok).To(BeFalse())
		c.drop("uid")
		Expect(c.len()).To(BeZero())
	})

	It("sweeps expired tokens when storing a new one", func() {
		c := newTokenCache()
		now := time.Now()
		// A target that vanished without a reconcile (finalizer removed by hand).
		c.put("gone", issuedToken{token: "old", expiresAt: now.Add(time.Hour), renewAt: now.Add(30 * time.Minute)}, now)
		c.put("live", issuedToken{token: "new", expiresAt: now.Add(3 * time.Hour), renewAt: now.Add(90 * time.Minute)}, now.Add(2*time.Hour))
		Expect(c.len()).To(Equal(1))
		_, ok := c.get("gone", "", now.Add(2*time.Hour))
		Expect(ok).To(BeFalse())
	})

	It("requeues in time for renewal and retries token failures sooner", func() {
		clock := &testClock{t: time.Now()}
		r := &WarpgateTargetReconciler{ReconcileInterval: time.Hour, Now: clock.Now}
		Expect(r.requeueAfter(nil)).To(Equal(time.Hour))
		Expect(r.requeueAfter(&issuedToken{renewAt: clock.Now().Add(10 * time.Minute)})).To(Equal(10 * time.Minute))
		Expect(r.requeueAfter(&issuedToken{renewAt: clock.Now().Add(2 * time.Hour)})).To(Equal(time.Hour))
		Expect(r.requeueAfter(&issuedToken{renewAt: clock.Now().Add(-time.Minute)})).To(Equal(time.Second), "never below a second")

		Expect(r.retryAfter(newTokenError(ReasonTokenRequestError, "x"), false)).To(Equal(tokenRetryInterval))
		Expect(r.retryAfter(errors.New("warpgate 500"), true)).To(Equal(tokenRetryInterval), "a target holding a token")
		Expect(r.retryAfter(errors.New("other"), false)).To(Equal(time.Hour))
		r.ReconcileInterval = 30 * time.Second
		Expect(r.retryAfter(newTokenError(ReasonTokenRequestError, "x"), false)).To(Equal(30 * time.Second))

		r.ReconcileInterval = 0
		Expect(r.requeueAfter(nil)).To(BeZero(), "interval 0 still means no periodic resync")
		Expect(r.requeueAfter(&issuedToken{renewAt: clock.Now().Add(10 * time.Minute)})).To(Equal(10 * time.Minute))
	})

	It("keeps the reason through wrapping", func() {
		err := fmt.Errorf("outer: %w", newTokenError(ReasonServiceAccountNotOptIn, "inner %d", 1))
		reason, ok := tokenErrorReason(err)
		Expect(ok).To(BeTrue())
		Expect(reason).To(Equal(ReasonServiceAccountNotOptIn))
		Expect(err.Error()).To(Equal("outer: inner 1"))
		_, ok = tokenErrorReason(errors.New("plain"))
		Expect(ok).To(BeFalse())

		cause := apierrors.NewForbidden(schema.GroupResource{Resource: "serviceaccounts/token"}, "sa", errors.New("denied"))
		wrapped := newTokenError(ReasonTokenRequestForbidden, "requesting: %w", cause)
		Expect(errors.Is(wrapped, cause)).To(BeTrue(), "the cause stays inspectable")
		Expect(apierrors.IsForbidden(wrapped)).To(BeTrue())
	})

	It("uses the default lifetime when none is set", func() {
		Expect(expirationSeconds(&warpgatev1alpha1.ServiceAccountTokenSpec{})).To(Equal(warpgatev1alpha1.ServiceAccountTokenDefaultExpirationSeconds))
		Expect(expirationSeconds(&warpgatev1alpha1.ServiceAccountTokenSpec{ExpirationSeconds: new(int64(10800))})).To(Equal(int64(10800)))
	})
})
