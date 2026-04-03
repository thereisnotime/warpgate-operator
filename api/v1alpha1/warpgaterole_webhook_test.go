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

func TestWarpgateRoleDefaulter(t *testing.T) {
	d := &WarpgateRoleCustomDefaulter{}

	t.Run("no-op default does not error", func(t *testing.T) {
		role := &WarpgateRole{
			Spec: WarpgateRoleSpec{
				ConnectionRef: "my-conn",
				Name:          "admin",
			},
		}
		if err := d.Default(context.Background(), role); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestWarpgateRoleValidator(t *testing.T) {
	v := &WarpgateRoleCustomValidator{}
	ctx := context.Background()

	validRole := func() *WarpgateRole {
		return &WarpgateRole{
			Spec: WarpgateRoleSpec{
				ConnectionRef: "my-conn",
				Name:          "admin",
			},
		}
	}

	t.Run("valid role passes", func(t *testing.T) {
		_, err := v.ValidateCreate(ctx, validRole())
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("empty connectionRef rejected", func(t *testing.T) {
		r := validRole()
		r.Spec.ConnectionRef = ""
		_, err := v.ValidateCreate(ctx, r)
		if err == nil {
			t.Error("expected error for empty connectionRef")
		}
	})

	t.Run("empty name rejected", func(t *testing.T) {
		r := validRole()
		r.Spec.Name = ""
		_, err := v.ValidateCreate(ctx, r)
		if err == nil {
			t.Error("expected error for empty name")
		}
	})

	t.Run("update validation works", func(t *testing.T) {
		old := validRole()
		bad := validRole()
		bad.Spec.Name = ""
		_, err := v.ValidateUpdate(ctx, old, bad)
		if err == nil {
			t.Error("expected error for empty name on update")
		}
	})

	t.Run("delete always passes", func(t *testing.T) {
		_, err := v.ValidateDelete(ctx, validRole())
		if err != nil {
			t.Errorf("expected no error on delete, got %v", err)
		}
	})
}
