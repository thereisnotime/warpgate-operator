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

package scope

import (
	"context"
	"fmt"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const managed = "warpgate.warp.tech/managed"

func secret(name string, l map[string]string) *corev1.Secret {
	return &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "ns", Labels: l}}
}

func mustParseSecrets(t *testing.T, selector string) *Secrets {
	t.Helper()
	s, err := ParseSecrets(selector)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestParseSecrets(t *testing.T) {
	for _, empty := range []string{"", "  "} {
		if s := mustParseSecrets(t, empty); s != nil {
			t.Errorf("%q should not limit Secrets: %+v", empty, s)
		}
	}

	for selector, want := range map[string]labels.Set{
		managed + "=true":                  {managed: "true"},
		managed + "==yes":                  {managed: "yes"},
		"team in (b,a)":                    {"team": "a"},
		"warpgate":                         {"warpgate": "true"},
		managed + "=true,team!=x,!ignored": {managed: "true"},
	} {
		s := mustParseSecrets(t, selector)
		if s == nil || !labels.Equals(s.Labels, want) || !s.Selector.Matches(s.Labels) {
			t.Errorf("%q: got %+v, want labels %v", selector, s, want)
		}
		if s.String() != s.Selector.String() {
			t.Errorf("%q: String() = %q", selector, s.String())
		}
	}

	for _, bad := range []string{"a b", "team!=x", "!ignored", "a=b,a=c"} {
		if _, err := ParseSecrets(bad); err == nil {
			t.Errorf("%q: expected an error", bad)
		}
	}
}

func TestSecretsByObject(t *testing.T) {
	var none *Secrets
	if none.ByObject() != nil || none.String() != "" {
		t.Error("no selector should leave the cache alone")
	}
	by := mustParseSecrets(t, managed+"=true").ByObject()
	if len(by) != 1 {
		t.Fatalf("unexpected cache options: %v", by)
	}
	for obj, opts := range by {
		if _, ok := obj.(*corev1.Secret); !ok {
			t.Errorf("expected a Secret key, got %T", obj)
		}
		if opts.Label == nil || opts.Label.String() != managed+"=true" || opts.Namespaces != nil {
			t.Errorf("unexpected options: %+v", opts)
		}
	}
}

// newScopedClient mimics the manager: cached only holds labelled Secrets,
// apiServer holds every object.
func newScopedClient(t *testing.T, s *Secrets, objects ...client.Object) client.Client {
	t.Helper()
	apiServer := newClient(t, nil, objects...)
	var visible []client.Object
	for _, obj := range objects {
		if _, ok := obj.(*corev1.Secret); !ok || s.Selector.Matches(labels.Set(obj.GetLabels())) {
			visible = append(visible, obj.DeepCopyObject().(client.Object))
		}
	}
	return s.Client(newClient(t, nil, visible...), apiServer)
}

func TestSecretsClientGet(t *testing.T) {
	ctx := context.Background()
	s := mustParseSecrets(t, managed+"=true")
	c := newScopedClient(t, s,
		secret("labelled", map[string]string{managed: "true"}),
		secret("unlabelled", nil),
		namespace("other", nil),
	)

	if err := c.Get(ctx, client.ObjectKey{Namespace: "ns", Name: "labelled"}, &corev1.Secret{}); err != nil {
		t.Errorf("labelled Secret should be visible: %v", err)
	}

	err := c.Get(ctx, client.ObjectKey{Namespace: "ns", Name: "unlabelled"}, &corev1.Secret{})
	if !IsNotSelected(err) || !apierrors.IsNotFound(err) {
		t.Fatalf("expected a NotSelectedError that is also NotFound, got %v", err)
	}
	if !strings.Contains(err.Error(), "ns/unlabelled") || !strings.Contains(err.Error(), managed+"=true") {
		t.Errorf("message should name the Secret and the label: %v", err)
	}
	if !IsNotSelected(fmt.Errorf("wrapped: %w", err)) {
		t.Error("IsNotSelected should see through wrapping")
	}

	err = c.Get(ctx, client.ObjectKey{Namespace: "ns", Name: "missing"}, &corev1.Secret{})
	if !apierrors.IsNotFound(err) || IsNotSelected(err) {
		t.Errorf("a missing Secret should stay a plain NotFound, got %v", err)
	}

	err = c.Get(ctx, client.ObjectKey{Name: "absent"}, &corev1.Namespace{})
	if !apierrors.IsNotFound(err) || IsNotSelected(err) {
		t.Errorf("other kinds should pass through, got %v", err)
	}

	var none *Secrets
	plain := newClient(t, nil)
	if none.Client(plain, plain) != client.Client(plain) {
		t.Error("no selector should return the client unchanged")
	}
}

func TestSecretsClientWrites(t *testing.T) {
	ctx := context.Background()
	s := mustParseSecrets(t, managed+"=true")
	c := newScopedClient(t, s)

	created := secret("created", map[string]string{"app": "x"})
	if err := c.Create(ctx, created); err != nil {
		t.Fatal(err)
	}
	var got corev1.Secret
	if err := c.Get(ctx, client.ObjectKeyFromObject(created), &got); err != nil {
		t.Fatal(err)
	}
	if got.Labels[managed] != "true" || got.Labels["app"] != "x" {
		t.Errorf("created Secret should carry the selector labels and keep its own: %v", got.Labels)
	}

	got.Labels = nil
	if err := c.Update(ctx, &got); err != nil {
		t.Fatal(err)
	}
	if got.Labels[managed] != "true" {
		t.Errorf("updated Secret lost the selector labels: %v", got.Labels)
	}

	base := got.DeepCopy()
	got.Labels = map[string]string{}
	if err := c.Patch(ctx, &got, client.MergeFrom(base)); err != nil {
		t.Fatal(err)
	}
	if got.Labels[managed] != "true" {
		t.Errorf("patched Secret lost the selector labels: %v", got.Labels)
	}

	err := c.Create(ctx, secret("created", nil))
	if !apierrors.IsAlreadyExists(err) || !strings.Contains(err.Error(), "add the label") {
		t.Errorf("expected an AlreadyExists error with a hint, got %v", err)
	}

	ns := namespace("labels-untouched", nil)
	if err := c.Create(ctx, ns); err != nil {
		t.Fatal(err)
	}
	if len(ns.Labels) != 0 {
		t.Errorf("only Secrets should be labelled: %v", ns.Labels)
	}
}
