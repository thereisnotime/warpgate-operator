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
	"testing"
)

func TestWarpgateConnectionDefaulter(t *testing.T) {
	d := &WarpgateConnectionCustomDefaulter{}

	t.Run("defaults tokenSecretRef.key to token", func(t *testing.T) {
		conn := &WarpgateConnection{
			Spec: WarpgateConnectionSpec{
				Host:           "https://warpgate.example.com",
				TokenSecretRef: SecretKeyRef{Name: "my-secret"},
			},
		}
		if err := d.Default(context.Background(), conn); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if conn.Spec.TokenSecretRef.Key != "token" {
			t.Errorf("expected key 'token', got %q", conn.Spec.TokenSecretRef.Key)
		}
	})

	t.Run("preserves explicit tokenSecretRef.key", func(t *testing.T) {
		conn := &WarpgateConnection{
			Spec: WarpgateConnectionSpec{
				Host:           "https://warpgate.example.com",
				TokenSecretRef: SecretKeyRef{Name: "my-secret", Key: "api-token"},
			},
		}
		if err := d.Default(context.Background(), conn); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if conn.Spec.TokenSecretRef.Key != "api-token" {
			t.Errorf("expected key 'api-token', got %q", conn.Spec.TokenSecretRef.Key)
		}
	})
}

func TestWarpgateConnectionValidator(t *testing.T) {
	v := &WarpgateConnectionCustomValidator{}
	ctx := context.Background()

	validConn := func() *WarpgateConnection {
		return &WarpgateConnection{
			Spec: WarpgateConnectionSpec{
				Host:           "https://warpgate.example.com",
				TokenSecretRef: SecretKeyRef{Name: "my-secret", Key: "token"},
			},
		}
	}

	t.Run("valid connection passes", func(t *testing.T) {
		_, err := v.ValidateCreate(ctx, validConn())
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("empty host rejected", func(t *testing.T) {
		c := validConn()
		c.Spec.Host = ""
		_, err := v.ValidateCreate(ctx, c)
		if err == nil {
			t.Error("expected error for empty host")
		}
	})

	t.Run("host without scheme rejected", func(t *testing.T) {
		c := validConn()
		c.Spec.Host = "warpgate.example.com"
		_, err := v.ValidateCreate(ctx, c)
		if err == nil {
			t.Error("expected error for host without http(s) scheme")
		}
	})

	t.Run("http scheme accepted", func(t *testing.T) {
		c := validConn()
		c.Spec.Host = "http://warpgate.example.com"
		_, err := v.ValidateCreate(ctx, c)
		if err != nil {
			t.Errorf("expected no error for http scheme, got %v", err)
		}
	})

	t.Run("empty tokenSecretRef.name rejected", func(t *testing.T) {
		c := validConn()
		c.Spec.TokenSecretRef.Name = ""
		_, err := v.ValidateCreate(ctx, c)
		if err == nil {
			t.Error("expected error for empty tokenSecretRef.name")
		}
	})

	t.Run("update validation works", func(t *testing.T) {
		old := validConn()
		bad := validConn()
		bad.Spec.Host = "ftp://wrong"
		_, err := v.ValidateUpdate(ctx, old, bad)
		if err == nil {
			t.Error("expected error for invalid host on update")
		}
	})

	t.Run("delete always passes", func(t *testing.T) {
		_, err := v.ValidateDelete(ctx, validConn())
		if err != nil {
			t.Errorf("expected no error on delete, got %v", err)
		}
	})
}
