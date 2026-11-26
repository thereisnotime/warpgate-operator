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

package v1alpha1

import (
	"context"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func kubernetesTarget(k8s *KubernetesTargetSpec) *WarpgateTarget {
	return &WarpgateTarget{
		Spec: WarpgateTargetSpec{
			ConnectionRef: "my-conn",
			Name:          "k8s-target",
			Kubernetes:    k8s,
		},
	}
}

func saTokenSpec(mutate func(*ServiceAccountTokenSpec)) *KubernetesTargetSpec {
	sat := &ServiceAccountTokenSpec{
		Name:              "warpgate-k8s",
		Audiences:         []string{"zone10-drinking"},
		ExpirationSeconds: new(int64(7200)),
	}
	if mutate != nil {
		mutate(sat)
	}
	return &KubernetesTargetSpec{
		ClusterURL:          "https://192.168.42.49:6443",
		AuthKind:            "Token",
		ServiceAccountToken: sat,
	}
}

func TestKubernetesTargetValidation(t *testing.T) {
	secretRef := &SecretKeyRef{Name: "k8s-token"}
	tests := []struct {
		name    string
		spec    *KubernetesTargetSpec
		wantErr string // empty: must be accepted
	}{
		{name: "token from secret", spec: &KubernetesTargetSpec{ClusterURL: "https://k", AuthKind: "Token", TokenSecretRef: secretRef}},
		{name: "token from service account", spec: saTokenSpec(nil)},
		{name: "service account, no expiration (defaulted later)", spec: saTokenSpec(func(s *ServiceAccountTokenSpec) { s.ExpirationSeconds = nil })},
		{name: "service account, eight audiences", spec: saTokenSpec(func(s *ServiceAccountTokenSpec) {
			s.Audiences = []string{"a", "b", "c", "d", "e", "f", "g", "h"}
		})},
		{name: "service account, lifetime at the minimum", spec: saTokenSpec(func(s *ServiceAccountTokenSpec) { s.ExpirationSeconds = new(int64(7200)) })},
		{name: "service account, lifetime at the maximum", spec: saTokenSpec(func(s *ServiceAccountTokenSpec) { s.ExpirationSeconds = new(int64(172800)) })},
		{name: "service account, URL audience", spec: saTokenSpec(func(s *ServiceAccountTokenSpec) {
			s.Audiences = []string{"https://k8s-oidc.example.com/zone09"}
		})},
		{name: "certificate with both refs", spec: &KubernetesTargetSpec{
			ClusterURL: "https://k", AuthKind: "Certificate",
			CertificateSecretRef: &SecretKeyRef{Name: "c"}, PrivateKeySecretRef: &SecretKeyRef{Name: "k"},
		}},
		{name: "iam role", spec: &KubernetesTargetSpec{ClusterURL: "https://k", AuthKind: "IamRole"}},

		{name: "token without a source", spec: &KubernetesTargetSpec{ClusterURL: "https://k", AuthKind: "Token"},
			wantErr: "needs tokenSecretRef or serviceAccountToken"},
		{name: "token with both sources", spec: func() *KubernetesTargetSpec {
			s := saTokenSpec(nil)
			s.TokenSecretRef = secretRef
			return s
		}(), wantErr: "mutually exclusive"},
		{name: "token secret ref without a name", spec: &KubernetesTargetSpec{ClusterURL: "https://k", AuthKind: "Token", TokenSecretRef: &SecretKeyRef{}},
			wantErr: "tokenSecretRef.name"},
		{name: "service account with certificate auth", spec: func() *KubernetesTargetSpec {
			s := saTokenSpec(nil)
			s.AuthKind = "Certificate"
			s.CertificateSecretRef = &SecretKeyRef{Name: "c"}
			s.PrivateKeySecretRef = &SecretKeyRef{Name: "k"}
			return s
		}(), wantErr: "only valid with authKind Token"},
		{name: "service account with iam role", spec: func() *KubernetesTargetSpec {
			s := saTokenSpec(nil)
			s.AuthKind = "IamRole"
			return s
		}(), wantErr: "only valid with authKind Token"},
		{name: "certificate without certificate ref", spec: &KubernetesTargetSpec{
			ClusterURL: "https://k", AuthKind: "Certificate", PrivateKeySecretRef: &SecretKeyRef{Name: "k"},
		}, wantErr: "needs certificateSecretRef"},
		{name: "certificate without private key ref", spec: &KubernetesTargetSpec{
			ClusterURL: "https://k", AuthKind: "Certificate", CertificateSecretRef: &SecretKeyRef{Name: "c"},
		}, wantErr: "needs privateKeySecretRef"},
		{name: "service account without a name", spec: saTokenSpec(func(s *ServiceAccountTokenSpec) { s.Name = "" }),
			wantErr: "serviceAccountToken.name"},
		{name: "service account name not a DNS subdomain", spec: saTokenSpec(func(s *ServiceAccountTokenSpec) { s.Name = "Not_Valid" }),
			wantErr: "serviceAccountToken.name"},
		{name: "no audiences", spec: saTokenSpec(func(s *ServiceAccountTokenSpec) { s.Audiences = nil }),
			wantErr: "at least one audience is required"},
		{name: "nine audiences", spec: saTokenSpec(func(s *ServiceAccountTokenSpec) {
			s.Audiences = []string{"a", "b", "c", "d", "e", "f", "g", "h", "i"}
		}), wantErr: "audiences"},
		{name: "empty audience", spec: saTokenSpec(func(s *ServiceAccountTokenSpec) { s.Audiences = []string{"ok", ""} }),
			wantErr: "audience must not be empty"},
		{name: "audience with a comma", spec: saTokenSpec(func(s *ServiceAccountTokenSpec) { s.Audiences = []string{"a,b"} }),
			wantErr: "must not contain commas"},
		{name: "audience with a space", spec: saTokenSpec(func(s *ServiceAccountTokenSpec) { s.Audiences = []string{"a b"} }),
			wantErr: "must not contain commas"},
		{name: "audience with a newline", spec: saTokenSpec(func(s *ServiceAccountTokenSpec) { s.Audiences = []string{"a\nb"} }),
			wantErr: "must not contain commas"},
		{name: "audience too long", spec: saTokenSpec(func(s *ServiceAccountTokenSpec) { s.Audiences = []string{strings.Repeat("a", 257)} }),
			wantErr: "audiences[0]"},
		{name: "duplicate audience", spec: saTokenSpec(func(s *ServiceAccountTokenSpec) { s.Audiences = []string{"a", "a"} }),
			wantErr: "Duplicate value"},
		{name: "lifetime below the minimum", spec: saTokenSpec(func(s *ServiceAccountTokenSpec) { s.ExpirationSeconds = new(int64(7199)) }),
			wantErr: "between 7200 (2h) and 172800 (48h)"},
		{name: "lifetime above the maximum", spec: saTokenSpec(func(s *ServiceAccountTokenSpec) { s.ExpirationSeconds = new(int64(172801)) }),
			wantErr: "between 7200 (2h) and 172800 (48h)"},
		{name: "negative lifetime", spec: saTokenSpec(func(s *ServiceAccountTokenSpec) { s.ExpirationSeconds = new(int64(-1)) }),
			wantErr: "between 7200 (2h) and 172800 (48h)"},
	}

	v := &WarpgateTargetCustomValidator{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := kubernetesTarget(tt.spec)
			_, errCreate := v.ValidateCreate(context.Background(), target)
			// A real spec change, so the update isn't skipped by the ratchet.
			old := target.DeepCopy()
			old.Spec.Description = "before"
			_, errUpdate := v.ValidateUpdate(context.Background(), old, target)
			for op, err := range map[string]error{"create": errCreate, "update": errUpdate} {
				if tt.wantErr == "" {
					if err != nil {
						t.Fatalf("%s: unexpected error: %v", op, err)
					}
					continue
				}
				if err == nil {
					t.Fatalf("%s: expected an error containing %q, got none", op, tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("%s: expected an error containing %q, got: %v", op, tt.wantErr, err)
				}
			}
		})
	}
}

func TestKubernetesTargetDefaultsTokenLifetime(t *testing.T) {
	d := &WarpgateTargetCustomDefaulter{}

	target := kubernetesTarget(saTokenSpec(func(s *ServiceAccountTokenSpec) { s.ExpirationSeconds = nil }))
	if err := d.Default(context.Background(), target); err != nil {
		t.Fatalf("Default: %v", err)
	}
	got := target.Spec.Kubernetes.ServiceAccountToken.ExpirationSeconds
	if got == nil || *got != ServiceAccountTokenDefaultExpirationSeconds {
		t.Fatalf("expirationSeconds = %v, want %d", got, ServiceAccountTokenDefaultExpirationSeconds)
	}

	target = kubernetesTarget(saTokenSpec(func(s *ServiceAccountTokenSpec) { s.ExpirationSeconds = new(int64(10800)) }))
	if err := d.Default(context.Background(), target); err != nil {
		t.Fatalf("Default: %v", err)
	}
	if got := *target.Spec.Kubernetes.ServiceAccountToken.ExpirationSeconds; got != 10800 {
		t.Fatalf("an explicit expirationSeconds was overwritten: %d", got)
	}

	// A Secret-backed token target gets no serviceAccountToken block.
	target = kubernetesTarget(&KubernetesTargetSpec{ClusterURL: "https://k", AuthKind: "Token", TokenSecretRef: &SecretKeyRef{Name: "t"}})
	if err := d.Default(context.Background(), target); err != nil {
		t.Fatalf("Default: %v", err)
	}
	if target.Spec.Kubernetes.ServiceAccountToken != nil {
		t.Fatal("Default added a serviceAccountToken block to a tokenSecretRef target")
	}
}

// The minimum lifetime must leave at least twice Warpgate's default
// kubernetes.session_max_age (30m) after a renewal at half the lifetime, so sessions
// still holding the old token are covered with time to spare for retries.
func TestServiceAccountTokenLifetimeBounds(t *testing.T) {
	const warpgateSessionMaxAge = int64(30 * 60)
	if ServiceAccountTokenMinExpirationSeconds/2 < 2*warpgateSessionMaxAge {
		t.Fatalf("minimum lifetime %ds leaves less than %ds after renewing at half", ServiceAccountTokenMinExpirationSeconds, warpgateSessionMaxAge)
	}
	if ServiceAccountTokenDefaultExpirationSeconds < ServiceAccountTokenMinExpirationSeconds ||
		ServiceAccountTokenDefaultExpirationSeconds > ServiceAccountTokenMaxExpirationSeconds {
		t.Fatal("default lifetime is outside the allowed range")
	}
}

// Updates that don't touch the spec, and updates of objects being deleted, must pass even
// when the stored spec predates the current rules; otherwise the operator's finalizer
// removal would be rejected and the object would never finish deleting.
func TestKubernetesTargetUpdateRatchet(t *testing.T) {
	v := &WarpgateTargetCustomValidator{}
	// authKind Token with no source: rejected on create since this change.
	legacy := kubernetesTarget(&KubernetesTargetSpec{ClusterURL: "https://k", AuthKind: "Token"})
	if _, err := v.ValidateCreate(context.Background(), legacy); err == nil {
		t.Fatal("create of an invalid spec must still be rejected")
	}

	metaOnly := legacy.DeepCopy()
	metaOnly.Finalizers = nil
	metaOnly.Labels = map[string]string{"x": "y"}
	if _, err := v.ValidateUpdate(context.Background(), legacy, metaOnly); err != nil {
		t.Fatalf("metadata-only update rejected: %v", err)
	}

	deleting := legacy.DeepCopy()
	now := metav1.Now()
	deleting.DeletionTimestamp = &now
	deleting.Spec.Description = "changed while deleting"
	if _, err := v.ValidateUpdate(context.Background(), legacy, deleting); err != nil {
		t.Fatalf("update of an object being deleted rejected: %v", err)
	}

	specChange := legacy.DeepCopy()
	specChange.Spec.Description = "changed"
	if _, err := v.ValidateUpdate(context.Background(), legacy, specChange); err == nil {
		t.Fatal("a spec change must be validated")
	}
}
