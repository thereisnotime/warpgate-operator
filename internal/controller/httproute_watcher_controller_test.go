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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	warpgatev1alpha1 "github.com/thereisnotime/warpgate-operator/api/v1alpha1"
)

// newFakeClient builds a fake client with the warpgate scheme registered.
func newFakeClient(objects ...client.Object) client.Client {
	s := runtime.NewScheme()
	_ = scheme.AddToScheme(s)
	_ = warpgatev1alpha1.AddToScheme(s)
	return fake.NewClientBuilder().WithScheme(s).WithObjects(objects...).Build()
}

// newHTTPRoute builds an unstructured HTTPRoute for use in tests.
func newHTTPRoute(name string, annotations map[string]string, hostnames []string) *unstructured.Unstructured {
	route := &unstructured.Unstructured{}
	route.SetGroupVersionKind(httprouteGVK)
	route.SetNamespace("default")
	route.SetName(name)
	route.SetAnnotations(annotations)
	if len(hostnames) > 0 {
		_ = unstructured.SetNestedStringSlice(route.Object, hostnames, "spec", "hostnames")
	}
	return route
}

var _ = Describe("HTTPRouteWatcher Controller", func() {

	const ns = "default"

	Context("Annotations present — target is created", func() {
		It("creates a WarpgateTarget when all three annotations are set", func() {
			routeName := "app-route"
			route := newHTTPRoute(routeName, map[string]string{
				AnnotationBindDomain:  "app.warpgate.example.com",
				AnnotationTargetGroup: "web-targets",
				AnnotationConnRef:     "my-connection",
			}, []string{"app.example.com"})

			fakeClient := newFakeClient()
			// The fake client supports unstructured.
			Expect(fakeClient.Create(ctx, route)).To(Succeed())

			reconciler := &HTTPRouteWatcherReconciler{
				Client: fakeClient,
				Scheme: fakeClient.Scheme(),
			}

			nn := types.NamespacedName{Name: routeName, Namespace: ns}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// The managed WarpgateTarget should exist.
			targetName := managedTargetName(ns, routeName)
			var target warpgatev1alpha1.WarpgateTarget
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: targetName, Namespace: ns}, &target)).To(Succeed())

			Expect(target.Labels[LabelManagedBy]).To(Equal(LabelManagedByValue))
			Expect(target.Labels[LabelSourceHTTPRoute]).To(Equal(ns + "/" + routeName))
			Expect(target.Spec.ConnectionRef).To(Equal("my-connection"))
			Expect(target.Spec.HTTP).NotTo(BeNil())
			Expect(target.Spec.HTTP.URL).To(Equal("https://app.example.com"))
			Expect(target.Spec.HTTP.ExternalHost).To(Equal("app.warpgate.example.com"))
			Expect(target.Annotations[AnnotationTargetGroup]).To(Equal("web-targets"))
		})
	})

	Context("Annotation removed — target is deleted", func() {
		It("deletes the managed WarpgateTarget when the bind-domain annotation is removed", func() {
			routeName := "app-route-noanno"

			// Pre-create a managed target that should be cleaned up.
			targetName := managedTargetName(ns, routeName)
			existingTarget := &warpgatev1alpha1.WarpgateTarget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      targetName,
					Namespace: ns,
					Labels: map[string]string{
						LabelManagedBy:       LabelManagedByValue,
						LabelSourceHTTPRoute: ns + "/" + routeName,
					},
				},
				Spec: warpgatev1alpha1.WarpgateTargetSpec{
					ConnectionRef: "my-connection",
					Name:          targetName,
					HTTP:          &warpgatev1alpha1.HTTPTargetSpec{URL: "https://app.example.com"},
				},
			}

			// HTTPRoute exists but annotations are now absent.
			route := newHTTPRoute(routeName, nil, []string{"app.example.com"})

			fakeClient := newFakeClient(existingTarget)
			Expect(fakeClient.Create(ctx, route)).To(Succeed())

			reconciler := &HTTPRouteWatcherReconciler{
				Client: fakeClient,
				Scheme: fakeClient.Scheme(),
			}

			nn := types.NamespacedName{Name: routeName, Namespace: ns}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// The target should be gone.
			var target warpgatev1alpha1.WarpgateTarget
			err = fakeClient.Get(ctx, types.NamespacedName{Name: targetName, Namespace: ns}, &target)
			Expect(err).To(HaveOccurred())
			Expect(client.IgnoreNotFound(err)).To(Succeed())
		})

		It("deletes the managed WarpgateTarget when the connection-ref annotation is missing", func() {
			routeName := "app-route-noconnref"
			targetName := managedTargetName(ns, routeName)
			existingTarget := &warpgatev1alpha1.WarpgateTarget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      targetName,
					Namespace: ns,
					Labels: map[string]string{
						LabelManagedBy:       LabelManagedByValue,
						LabelSourceHTTPRoute: ns + "/" + routeName,
					},
				},
				Spec: warpgatev1alpha1.WarpgateTargetSpec{
					ConnectionRef: "my-connection",
					Name:          targetName,
					HTTP:          &warpgatev1alpha1.HTTPTargetSpec{URL: "https://app.example.com"},
				},
			}

			// bind-domain and group present, but connection-ref missing.
			route := newHTTPRoute(routeName, map[string]string{
				AnnotationBindDomain:  "app.warpgate.example.com",
				AnnotationTargetGroup: "web-targets",
			}, []string{"app.example.com"})

			fakeClient := newFakeClient(existingTarget)
			Expect(fakeClient.Create(ctx, route)).To(Succeed())

			reconciler := &HTTPRouteWatcherReconciler{
				Client: fakeClient,
				Scheme: fakeClient.Scheme(),
			}

			nn := types.NamespacedName{Name: routeName, Namespace: ns}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			var target warpgatev1alpha1.WarpgateTarget
			err = fakeClient.Get(ctx, types.NamespacedName{Name: targetName, Namespace: ns}, &target)
			Expect(err).To(HaveOccurred())
			Expect(client.IgnoreNotFound(err)).To(Succeed())
		})
	})

	Context("No annotations — no action", func() {
		It("does not create a WarpgateTarget when none of the annotations are set", func() {
			routeName := "app-route-empty"
			route := newHTTPRoute(routeName, nil, []string{"app.example.com"})

			fakeClient := newFakeClient()
			Expect(fakeClient.Create(ctx, route)).To(Succeed())

			reconciler := &HTTPRouteWatcherReconciler{
				Client: fakeClient,
				Scheme: fakeClient.Scheme(),
			}

			nn := types.NamespacedName{Name: routeName, Namespace: ns}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// No WarpgateTarget should have been created.
			var targetList warpgatev1alpha1.WarpgateTargetList
			Expect(fakeClient.List(ctx, &targetList, client.InNamespace(ns))).To(Succeed())
			Expect(targetList.Items).To(BeEmpty())
		})
	})

	Context("HTTPRoute not found — managed target is cleaned up", func() {
		It("deletes an orphaned managed WarpgateTarget when the HTTPRoute is gone", func() {
			routeName := "gone-route"
			targetName := managedTargetName(ns, routeName)
			existingTarget := &warpgatev1alpha1.WarpgateTarget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      targetName,
					Namespace: ns,
					Labels: map[string]string{
						LabelManagedBy:       LabelManagedByValue,
						LabelSourceHTTPRoute: ns + "/" + routeName,
					},
				},
				Spec: warpgatev1alpha1.WarpgateTargetSpec{
					ConnectionRef: "my-connection",
					Name:          targetName,
					HTTP:          &warpgatev1alpha1.HTTPTargetSpec{URL: "https://gone.example.com"},
				},
			}

			fakeClient := newFakeClient(existingTarget)
			reconciler := &HTTPRouteWatcherReconciler{
				Client: fakeClient,
				Scheme: fakeClient.Scheme(),
			}

			// Route doesn't exist.
			nn := types.NamespacedName{Name: routeName, Namespace: ns}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			var target warpgatev1alpha1.WarpgateTarget
			err = fakeClient.Get(ctx, types.NamespacedName{Name: targetName, Namespace: ns}, &target)
			Expect(err).To(HaveOccurred())
			Expect(client.IgnoreNotFound(err)).To(Succeed())
		})
	})

	Context("Target already exists — update on spec change", func() {
		It("updates the WarpgateTarget when the URL changes", func() {
			routeName := "app-route-update"
			targetName := managedTargetName(ns, routeName)

			// Pre-existing target with the old URL.
			existingTarget := &warpgatev1alpha1.WarpgateTarget{
				ObjectMeta: metav1.ObjectMeta{
					Name:      targetName,
					Namespace: ns,
					Labels: map[string]string{
						LabelManagedBy:       LabelManagedByValue,
						LabelSourceHTTPRoute: ns + "/" + routeName,
					},
				},
				Spec: warpgatev1alpha1.WarpgateTargetSpec{
					ConnectionRef: "my-connection",
					Name:          targetName,
					HTTP:          &warpgatev1alpha1.HTTPTargetSpec{URL: "https://old.example.com"},
				},
			}

			// HTTPRoute now points to a new hostname.
			route := newHTTPRoute(routeName, map[string]string{
				AnnotationBindDomain:  "app.warpgate.example.com",
				AnnotationTargetGroup: "web-targets",
				AnnotationConnRef:     "my-connection",
			}, []string{"new.example.com"})

			fakeClient := newFakeClient(existingTarget)
			Expect(fakeClient.Create(ctx, route)).To(Succeed())

			reconciler := &HTTPRouteWatcherReconciler{
				Client: fakeClient,
				Scheme: fakeClient.Scheme(),
			}

			nn := types.NamespacedName{Name: routeName, Namespace: ns}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			var updated warpgatev1alpha1.WarpgateTarget
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: targetName, Namespace: ns}, &updated)).To(Succeed())
			Expect(updated.Spec.HTTP.URL).To(Equal("https://new.example.com"))
		})
	})

	Context("No hostnames in spec", func() {
		It("returns an error when spec.hostnames is empty", func() {
			routeName := "app-route-nohosts"
			route := newHTTPRoute(routeName, map[string]string{
				AnnotationBindDomain:  "app.warpgate.example.com",
				AnnotationTargetGroup: "web-targets",
				AnnotationConnRef:     "my-connection",
			}, nil)

			fakeClient := newFakeClient()
			Expect(fakeClient.Create(ctx, route)).To(Succeed())

			reconciler := &HTTPRouteWatcherReconciler{
				Client: fakeClient,
				Scheme: fakeClient.Scheme(),
			}

			nn := types.NamespacedName{Name: routeName, Namespace: ns}
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no spec.hostnames"))
		})
	})

	Context("managedTargetName helper", func() {
		It("produces a valid name from namespace and route name", func() {
			name := managedTargetName("my-ns", "my-route")
			Expect(name).To(Equal("my-ns-my-route"))
		})

		It("truncates names longer than 63 characters", func() {
			long := managedTargetName("verylongnamespace", "verylongroutenamethatmakesthistoolongforku8snames")
			Expect(len(long)).To(BeNumerically("<=", 63))
		})
	})
})
