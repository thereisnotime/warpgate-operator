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
	"fmt"
	"maps"
	"net/http"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Secrets limits the Secrets the operator caches and reads to those matching
// a label selector. The zero value (nil *Secrets) means no limit.
type Secrets struct {
	// Selector is what a Secret's labels must match to be visible.
	Selector labels.Selector
	// Labels is put on every Secret the operator creates or updates, so its
	// own Secrets always match Selector.
	Labels labels.Set
}

// ParseSecrets reads --secret-selector. It returns nil for an empty selector.
// The selector must be satisfiable by a fixed set of labels (key=value,
// key in (...), key, plus any negative terms), because the operator labels
// the Secrets it creates with it.
func ParseSecrets(selector string) (*Secrets, error) {
	if strings.TrimSpace(selector) == "" {
		return nil, nil
	}
	sel, err := labels.Parse(selector)
	if err != nil {
		return nil, fmt.Errorf("parsing secret selector: %w", err)
	}
	reqs, _ := sel.Requirements()
	if len(reqs) == 0 {
		return nil, nil
	}
	set := labels.Set{}
	for _, req := range reqs {
		switch req.Operator() {
		case selection.Equals, selection.DoubleEquals, selection.In:
			set[req.Key()] = req.Values().List()[0]
		case selection.Exists:
			set[req.Key()] = "true"
		}
	}
	if len(set) == 0 || !sel.Matches(set) {
		return nil, fmt.Errorf("secret selector %q must require at least one label the operator can set "+
			"on the Secrets it creates (for example warpgate.warp.tech/managed=true)", selector)
	}
	return &Secrets{Selector: sel, Labels: set}, nil
}

// String returns the selector, or "" when there is no limit.
func (s *Secrets) String() string {
	if s == nil {
		return ""
	}
	return s.Selector.String()
}

// ByObject returns the cache options that limit the Secret informer to the
// selector. Namespaces stay at the cache defaults. It returns nil without a
// selector, which leaves the cache watching every Secret.
func (s *Secrets) ByObject() map[client.Object]cache.ByObject {
	if s == nil {
		return nil
	}
	return map[client.Object]cache.ByObject{
		&corev1.Secret{}: {Label: s.Selector},
	}
}

// NotSelectedError reports a Secret that exists but is outside the selector.
// It is a NotFound API status, so callers treating a missing Secret as
// NotFound keep working, and its message tells the user what to fix.
type NotSelectedError struct {
	*apierrors.StatusError
}

// IsNotSelected reports whether err (or an error it wraps) is a NotSelectedError.
func IsNotSelected(err error) bool {
	var target *NotSelectedError
	return errors.As(err, &target)
}

func newNotSelected(key client.ObjectKey, s *Secrets) *NotSelectedError {
	return &NotSelectedError{&apierrors.StatusError{ErrStatus: metav1.Status{
		Status: metav1.StatusFailure,
		Code:   http.StatusNotFound,
		Reason: metav1.StatusReasonNotFound,
		Details: &metav1.StatusDetails{
			Kind: "secrets",
			Name: key.Name,
		},
		Message: fmt.Sprintf("Secret %s exists but does not match the operator's --secret-selector %q; "+
			"add the label(s) %s so the operator can read it", key, s.Selector.String(), s.Labels.String()),
	}}}
}

// Client wraps c so that, with a selector:
//   - Get of a Secret the cache does not have checks the Secret's metadata
//     with apiReader and returns a NotSelectedError if it exists unlabelled;
//   - Create, Update and Patch of a Secret add the selector's labels.
//
// Without a selector it returns c unchanged.
func (s *Secrets) Client(c client.Client, apiReader client.Reader) client.Client {
	if s == nil {
		return c
	}
	return &secretClient{Client: c, apiReader: apiReader, secrets: s}
}

type secretClient struct {
	client.Client
	apiReader client.Reader
	secrets   *Secrets
}

func (c *secretClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	err := c.Client.Get(ctx, key, obj, opts...)
	if _, ok := obj.(*corev1.Secret); !ok || !apierrors.IsNotFound(err) {
		return err
	}
	// Metadata only: the Secret's data is never read outside the selector.
	meta := &metav1.PartialObjectMetadata{}
	meta.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("Secret"))
	if getErr := c.apiReader.Get(ctx, key, meta); getErr == nil && !c.secrets.Selector.Matches(labels.Set(meta.Labels)) {
		return newNotSelected(key, c.secrets)
	}
	return err
}

func (c *secretClient) label(obj client.Object) {
	if _, ok := obj.(*corev1.Secret); !ok {
		return
	}
	l := obj.GetLabels()
	if l == nil {
		l = map[string]string{}
	}
	maps.Copy(l, c.secrets.Labels)
	obj.SetLabels(l)
}

func (c *secretClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	c.label(obj)
	err := c.Client.Create(ctx, obj, opts...)
	if _, ok := obj.(*corev1.Secret); ok && apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("%w (if it was created before --secret-selector was set, add the label(s) %s to it)",
			err, c.secrets.Labels.String())
	}
	return err
}

func (c *secretClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	c.label(obj)
	return c.Client.Update(ctx, obj, opts...)
}

func (c *secretClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	c.label(obj)
	return c.Client.Patch(ctx, obj, patch, opts...)
}
