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
	"errors"
	"slices"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func namespace(name string, labels map[string]string) *corev1.Namespace {
	return &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels}}
}

func newClient(t *testing.T, funcs *interceptor.Funcs, objects ...client.Object) client.WithWatch {
	t.Helper()
	s := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	b := fake.NewClientBuilder().WithScheme(s).WithObjects(objects...)
	if funcs != nil {
		b = b.WithInterceptorFuncs(*funcs)
	}
	return b.Build()
}

var errList = errors.New("list failed")

func failingList() *interceptor.Funcs {
	return &interceptor.Funcs{
		List: func(context.Context, client.WithWatch, client.ObjectList, ...client.ListOption) error {
			return errList
		},
	}
}

func mustParse(t *testing.T, names, selector string) Namespaces {
	t.Helper()
	ns, err := Parse(names, selector)
	if err != nil {
		t.Fatal(err)
	}
	return ns
}

func TestParse(t *testing.T) {
	ns := mustParse(t, " a, ,b ,", "")
	if !slices.Equal(ns.Names, []string{"a", "b"}) || ns.Selector != nil || !ns.Enabled() {
		t.Errorf("unexpected scope: %+v", ns)
	}

	ns = mustParse(t, "", "  ")
	if ns.Enabled() {
		t.Errorf("empty flags should not limit namespaces: %+v", ns)
	}

	ns = mustParse(t, "", "team=a,env in (prod)")
	if ns.Selector == nil || !ns.Enabled() {
		t.Errorf("selector not parsed: %+v", ns)
	}

	if _, err := Parse("", "a b"); err == nil {
		t.Error("expected an error for an invalid selector")
	}
}

func TestResolve(t *testing.T) {
	c := newClient(t, nil,
		namespace("watched", map[string]string{"warpgate": "true"}),
		namespace("also-watched", map[string]string{"warpgate": "true"}),
		namespace("other", nil),
	)
	got, err := mustParse(t, "named,watched", "warpgate=true").Resolve(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"also-watched", "named", "watched"}; !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}

	got, err = mustParse(t, "b,a", "").Resolve(context.Background(), newClient(t, failingList()))
	if err != nil || !slices.Equal(got, []string{"a", "b"}) {
		t.Errorf("names alone must not list namespaces: %v, %v", got, err)
	}

	if _, err := mustParse(t, "", "a=b").Resolve(context.Background(), newClient(t, failingList())); !errors.Is(err, errList) {
		t.Errorf("expected the list error, got %v", err)
	}
}

func TestCacheNamespaces(t *testing.T) {
	ctx := context.Background()
	c := newClient(t, nil, namespace("watched", map[string]string{"warpgate": "true"}))

	names, cfg, err := mustParse(t, "", "").CacheNamespaces(ctx, c)
	if names != nil || cfg != nil || err != nil {
		t.Errorf("no scope should watch all namespaces: %v %v %v", names, cfg, err)
	}

	names, cfg, err = mustParse(t, "named", "warpgate=true").CacheNamespaces(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(names, []string{"named", "watched"}) || len(cfg) != 2 {
		t.Errorf("unexpected namespaces: %v %v", names, cfg)
	}
	if _, ok := cfg["watched"]; !ok {
		t.Errorf("missing cache config for watched: %v", cfg)
	}

	if _, _, err := mustParse(t, "", "none=true").CacheNamespaces(ctx, c); !errors.Is(err, ErrNoNamespaces) {
		t.Errorf("expected ErrNoNamespaces, got %v", err)
	}

	if _, _, err := mustParse(t, "", "a=b").CacheNamespaces(ctx, newClient(t, failingList())); !errors.Is(err, errList) {
		t.Errorf("expected the list error, got %v", err)
	}
}

func TestWatcher(t *testing.T) {
	selected := map[string]string{"warpgate": "true"}

	t.Run("does not need leader election", func(t *testing.T) {
		if (&Watcher{}).NeedLeaderElection() {
			t.Error("every replica must watch the namespaces")
		}
	})

	t.Run("returns when the namespaces change", func(t *testing.T) {
		c := newClient(t, nil, namespace("a", selected))
		w := &Watcher{Namespaces: mustParse(t, "", "warpgate=true"), Reader: c, Current: []string{"a"}, Interval: time.Millisecond}
		if err := c.Create(context.Background(), namespace("b", selected)); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := w.Start(ctx); !errors.Is(err, ErrNamespacesChanged) {
			t.Errorf("expected ErrNamespacesChanged, got %v", err)
		}
	})

	t.Run("keeps running while unchanged or failing", func(t *testing.T) {
		for name, c := range map[string]client.Reader{
			"unchanged": newClient(t, nil, namespace("a", selected)),
			"failing":   newClient(t, failingList()),
		} {
			w := &Watcher{Namespaces: mustParse(t, "", "warpgate=true"), Reader: c, Current: []string{"a"}, Interval: time.Millisecond}
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			if err := w.Start(ctx); err != nil {
				t.Errorf("%s: expected nil after the context ends, got %v", name, err)
			}
			cancel()
		}
	})
}
