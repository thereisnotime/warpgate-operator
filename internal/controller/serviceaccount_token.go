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

// Short-lived bearer tokens for Kubernetes targets (spec.kubernetes.serviceAccountToken).
//
// The operator asks the TokenRequest API for a token for a ServiceAccount in the
// target's namespace, sends it to Warpgate and asks for a new one after half the
// lifetime the API server granted. The token exists only in this process's memory
// and in Warpgate: it is never written to a Secret, the CR status, a log line or a
// condition. Two things must allow a request before one is made:
//
//   - the ServiceAccount opts in with two annotations: warpgate.warp.tech/token-audiences
//     (every audience a token may be issued for) and warpgate.warp.tech/token-cluster-urls
//     (every clusterURL a token may be sent to);
//   - the operator has create on serviceaccounts/token for that ServiceAccount
//     (not in the generated ClusterRole; Helm grants it per ServiceAccount).

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	warpgatev1alpha1 "github.com/thereisnotime/warpgate-operator/api/v1alpha1"
	"github.com/thereisnotime/warpgate-operator/internal/warpgate"
)

// Condition reasons for serviceAccountToken failures.
const (
	ReasonServiceAccountNotFound  = "ServiceAccountNotFound"
	ReasonServiceAccountNotOptIn  = "ServiceAccountNotOptedIn"
	ReasonTokenAudienceNotAllowed = "TokenAudienceNotAllowed"
	ReasonClusterURLNotAllowed    = "TokenClusterURLNotAllowed"
	ReasonTokenRequestForbidden   = "TokenRequestForbidden"
	ReasonTokenRequestError       = "TokenRequestError"
)

// tokenRetryInterval caps the requeue after a token failure, or after a Warpgate API
// failure on a serviceAccountToken target, so a renewal that failed once is retried well
// before the current token expires even with a long reconcile interval.
const tokenRetryInterval = time.Minute

// redactedToken replaces the token wherever it would otherwise appear in a message.
const redactedToken = "[REDACTED]"

// tokenError carries the Ready condition reason for a serviceAccountToken failure.
type tokenError struct {
	reason string
	err    error
}

func (e *tokenError) Error() string { return e.err.Error() }
func (e *tokenError) Unwrap() error { return e.err }

func newTokenError(reason string, format string, args ...any) error {
	return &tokenError{reason: reason, err: fmt.Errorf(format, args...)}
}

// tokenErrorReason returns the condition reason carried by err, if it is a tokenError.
func tokenErrorReason(err error) (string, bool) {
	var te *tokenError
	if errors.As(err, &te) {
		return te.reason, true
	}
	return "", false
}

// issuedToken is a token the operator holds for one target.
type issuedToken struct {
	token       string
	expiresAt   time.Time
	renewAt     time.Time
	fingerprint string
}

// tokenCache holds the current token per WarpgateTarget UID, in memory only.
type tokenCache struct {
	mu      sync.Mutex
	entries map[types.UID]issuedToken
}

func newTokenCache() *tokenCache {
	return &tokenCache{entries: map[types.UID]issuedToken{}}
}

// get returns the cached token for uid if it was issued for the same source and is not
// yet due for renewal.
func (c *tokenCache) get(uid types.UID, fingerprint string, now time.Time) (issuedToken, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[uid]
	if !ok || e.fingerprint != fingerprint || !now.Before(e.renewAt) {
		return issuedToken{}, false
	}
	return e, true
}

// put stores t and drops entries whose token has expired, so entries of targets that
// vanished without a reconcile (finalizer removed by hand) don't stay in memory.
func (c *tokenCache) put(uid types.UID, t issuedToken, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, e := range c.entries {
		if !now.Before(e.expiresAt) {
			delete(c.entries, k)
		}
	}
	c.entries[uid] = t
}

func (c *tokenCache) drop(uid types.UID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, uid)
}

func (c *tokenCache) len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}

// tokenFingerprint identifies what a token was issued for and where it is sent. The
// ServiceAccount UID is part of it, so a deleted and recreated ServiceAccount gets a new
// token (the old ones stop validating on the issuing cluster; a remote cluster that only
// checks signatures keeps accepting them until they expire). clusterURL is part of it so
// a token is never reused for a different cluster.
func tokenFingerprint(saName string, saUID types.UID, clusterURL string, audiences []string, expirationSeconds int64) string {
	sorted := slices.Clone(audiences)
	slices.Sort(sorted)
	h := sha256.New()
	for _, part := range []string{saName, string(saUID), clusterURL, strconv.FormatInt(expirationSeconds, 10)} {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	for _, aud := range sorted {
		h.Write([]byte(aud))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// annotationList parses a comma-separated annotation value. Empty entries and
// surrounding whitespace are ignored; matching against it is exact.
func annotationList(value string) sets.Set[string] {
	allowed := sets.New[string]()
	for aud := range strings.SplitSeq(value, ",") {
		if aud = strings.TrimSpace(aud); aud != "" {
			allowed.Insert(aud)
		}
	}
	return allowed
}

// renewTime is halfway between issue and expiry.
func renewTime(issued, expires time.Time) time.Time {
	return issued.Add(expires.Sub(issued) / 2)
}

// minRedactedPart is the shortest piece of a token redacted on its own. JWT segments
// are far longer; this only keeps tiny fragments from blanking unrelated text.
const minRedactedPart = 8

// redact removes secret from s: the whole value and, for a JWT, each of its
// dot-separated segments, so an echo of part of the token is caught too. An empty
// secret leaves s unchanged.
func redact(s, secret string) string {
	if secret == "" {
		return s
	}
	s = strings.ReplaceAll(s, secret, redactedToken)
	for part := range strings.SplitSeq(secret, ".") {
		if len(part) >= minRedactedPart {
			s = strings.ReplaceAll(s, part, redactedToken)
		}
	}
	return s
}

// redactError returns err with secret removed from its message, or err itself when
// there is nothing to remove. A Warpgate API error stays a *warpgate.APIError with its
// status code, so checks such as warpgate.IsNotFound keep working.
func redactError(err error, secret string) error {
	if err == nil || secret == "" || redact(err.Error(), secret) == err.Error() {
		return err
	}
	if apiErr, ok := err.(*warpgate.APIError); ok {
		return &warpgate.APIError{StatusCode: apiErr.StatusCode, Body: redact(apiErr.Body, secret)}
	}
	return errors.New(redact(err.Error(), secret))
}

func expirationSeconds(spec *warpgatev1alpha1.ServiceAccountTokenSpec) int64 {
	if spec.ExpirationSeconds != nil {
		return *spec.ExpirationSeconds
	}
	return warpgatev1alpha1.ServiceAccountTokenDefaultExpirationSeconds
}

// serviceAccountToken returns a valid token for the target's serviceAccountToken spec,
// reusing the cached one until it is due for renewal.
func (r *WarpgateTargetReconciler) serviceAccountToken(ctx context.Context, target *warpgatev1alpha1.WarpgateTarget) (issuedToken, error) {
	spec := target.Spec.Kubernetes.ServiceAccountToken
	if len(spec.Audiences) == 0 {
		// Never send an empty audience list: the API server would issue a token for
		// its own audiences, valid against this cluster.
		return issuedToken{}, newTokenError(ReasonTokenRequestError, "serviceAccountToken.audiences must not be empty")
	}
	lifetime := expirationSeconds(spec)

	var sa corev1.ServiceAccount
	if err := r.apiReader().Get(ctx, types.NamespacedName{Namespace: target.Namespace, Name: spec.Name}, &sa); err != nil {
		switch {
		case apierrors.IsNotFound(err):
			return issuedToken{}, newTokenError(ReasonServiceAccountNotFound,
				"ServiceAccount %q not found in namespace %q", spec.Name, target.Namespace)
		case apierrors.IsForbidden(err):
			// The chart grants get and create together, so this is the usual
			// symptom of a ServiceAccount missing from serviceAccountTokens.allowed.
			return issuedToken{}, newTokenError(ReasonTokenRequestForbidden,
				"the operator may not read ServiceAccount %s/%s (needs get on serviceaccounts and create on "+
					"serviceaccounts/token; Helm value serviceAccountTokens.allowed): %w", target.Namespace, spec.Name, err)
		}
		return issuedToken{}, newTokenError(ReasonTokenRequestError, "getting ServiceAccount %q: %w", spec.Name, err)
	}

	annotation, ok := sa.Annotations[warpgatev1alpha1.ServiceAccountTokenAudiencesAnnotation]
	if !ok {
		return issuedToken{}, newTokenError(ReasonServiceAccountNotOptIn,
			"ServiceAccount %q has no %s annotation; add it with the audiences tokens may be issued for",
			spec.Name, warpgatev1alpha1.ServiceAccountTokenAudiencesAnnotation)
	}
	allowed := annotationList(annotation)
	var denied []string
	for _, aud := range spec.Audiences {
		if !allowed.Has(aud) {
			denied = append(denied, aud)
		}
	}
	if len(denied) > 0 {
		return issuedToken{}, newTokenError(ReasonTokenAudienceNotAllowed,
			"ServiceAccount %q does not allow audiences %v in %s",
			spec.Name, denied, warpgatev1alpha1.ServiceAccountTokenAudiencesAnnotation)
	}

	clusterURL := target.Spec.Kubernetes.ClusterURL
	urls, ok := sa.Annotations[warpgatev1alpha1.ServiceAccountTokenClusterURLsAnnotation]
	if !ok {
		return issuedToken{}, newTokenError(ReasonServiceAccountNotOptIn,
			"ServiceAccount %q has no %s annotation; add it with the clusterURLs its tokens may be sent to",
			spec.Name, warpgatev1alpha1.ServiceAccountTokenClusterURLsAnnotation)
	}
	if !annotationList(urls).Has(clusterURL) {
		return issuedToken{}, newTokenError(ReasonClusterURLNotAllowed,
			"ServiceAccount %q does not allow clusterURL %q in %s",
			spec.Name, clusterURL, warpgatev1alpha1.ServiceAccountTokenClusterURLsAnnotation)
	}

	fingerprint := tokenFingerprint(sa.Name, sa.UID, clusterURL, spec.Audiences, lifetime)
	now := r.now()
	if cached, ok := r.tokenCache().get(target.UID, fingerprint, now); ok {
		return cached, nil
	}

	tr := &authenticationv1.TokenRequest{
		Spec: authenticationv1.TokenRequestSpec{
			Audiences:         slices.Clone(spec.Audiences),
			ExpirationSeconds: &lifetime,
		},
	}
	if err := r.Client.SubResource("token").Create(ctx, &sa, tr); err != nil {
		r.tokenCache().drop(target.UID)
		if apierrors.IsForbidden(err) {
			return issuedToken{}, newTokenError(ReasonTokenRequestForbidden,
				"the operator may not request tokens for ServiceAccount %s/%s (needs create on serviceaccounts/token; "+
					"Helm value serviceAccountTokens.allowed): %w", target.Namespace, spec.Name, err)
		}
		return issuedToken{}, newTokenError(ReasonTokenRequestError,
			"requesting a token for ServiceAccount %s/%s: %w", target.Namespace, spec.Name, err)
	}

	issued := now
	expires := tr.Status.ExpirationTimestamp.Time
	if tr.Status.Token == "" || !expires.After(issued) {
		r.tokenCache().drop(target.UID)
		return issuedToken{}, newTokenError(ReasonTokenRequestError,
			"the TokenRequest for ServiceAccount %s/%s returned no usable token", target.Namespace, spec.Name)
	}
	if granted := expires.Sub(issued); granted < time.Duration(lifetime)*time.Second-time.Minute {
		logf.FromContext(ctx).Info("the API server granted a shorter token lifetime than requested",
			"serviceAccount", spec.Name, "requested", time.Duration(lifetime)*time.Second, "granted", granted.Round(time.Second))
	}
	t := issuedToken{
		token:       tr.Status.Token,
		expiresAt:   expires,
		renewAt:     renewTime(issued, expires),
		fingerprint: fingerprint,
	}
	r.tokenCache().put(target.UID, t, now)
	return t, nil
}
