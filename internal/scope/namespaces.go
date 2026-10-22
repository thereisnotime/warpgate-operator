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

// Package scope limits the namespaces the operator watches.
package scope

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// +kubebuilder:rbac:groups="",resources=namespaces,verbs=get;list;watch

// ErrNoNamespaces is returned when the scope is set but selects no namespace.
var ErrNoNamespaces = errors.New("no namespaces match --watch-namespaces or --watch-namespace-selector")

// ErrNamespacesChanged is returned by Watcher when the selected namespaces change.
var ErrNamespacesChanged = errors.New("the namespaces matching --watch-namespace-selector changed")

// Namespaces selects namespaces by name and by label.
type Namespaces struct {
	Names    []string
	Selector labels.Selector
}

// Parse reads a comma-separated list of names and a label selector.
func Parse(names, selector string) (Namespaces, error) {
	var ns Namespaces
	for name := range strings.SplitSeq(names, ",") {
		if name = strings.TrimSpace(name); name != "" {
			ns.Names = append(ns.Names, name)
		}
	}
	if strings.TrimSpace(selector) != "" {
		sel, err := labels.Parse(selector)
		if err != nil {
			return Namespaces{}, fmt.Errorf("parsing namespace selector: %w", err)
		}
		ns.Selector = sel
	}
	return ns, nil
}

// Enabled reports whether the operator is limited to some namespaces.
func (n Namespaces) Enabled() bool {
	return len(n.Names) > 0 || n.Selector != nil
}

// Resolve returns the sorted names plus the namespaces matching the selector.
func (n Namespaces) Resolve(ctx context.Context, r client.Reader) ([]string, error) {
	out := slices.Clone(n.Names)
	if n.Selector != nil {
		var list corev1.NamespaceList
		if err := r.List(ctx, &list, client.MatchingLabelsSelector{Selector: n.Selector}); err != nil {
			return nil, fmt.Errorf("listing namespaces: %w", err)
		}
		for _, ns := range list.Items {
			out = append(out, ns.Name)
		}
	}
	slices.Sort(out)
	return slices.Compact(out), nil
}

// CacheNamespaces resolves the namespaces for cache.Options.DefaultNamespaces.
// It returns nil when the scope is not set, so the cache watches all namespaces.
func (n Namespaces) CacheNamespaces(ctx context.Context, r client.Reader) ([]string, map[string]cache.Config, error) {
	if !n.Enabled() {
		return nil, nil, nil
	}
	names, err := n.Resolve(ctx, r)
	if err != nil {
		return nil, nil, err
	}
	if len(names) == 0 {
		return nil, nil, ErrNoNamespaces
	}
	cfg := make(map[string]cache.Config, len(names))
	for _, name := range names {
		cfg[name] = cache.Config{}
	}
	return names, cfg, nil
}

// Watcher stops the manager when the namespaces matching the selector change,
// so the operator restarts with a cache for the new set.
type Watcher struct {
	Namespaces Namespaces
	Reader     client.Reader
	Current    []string
	Interval   time.Duration
}

// NeedLeaderElection is false: every replica's cache must follow the namespaces.
func (w *Watcher) NeedLeaderElection() bool {
	return false
}

// Start polls the namespaces until they change or ctx is done.
func (w *Watcher) Start(ctx context.Context) error {
	ticker := time.NewTicker(w.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			names, err := w.Namespaces.Resolve(ctx, w.Reader)
			if err != nil {
				logf.FromContext(ctx).Error(err, "Failed to check the watched namespaces")
				continue
			}
			if !slices.Equal(names, w.Current) {
				return ErrNamespacesChanged
			}
		}
	}
}
