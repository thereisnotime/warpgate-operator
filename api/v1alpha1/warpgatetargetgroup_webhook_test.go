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

func TestWarpgateTargetGroupDefaulter(t *testing.T) {
	d := &WarpgateTargetGroupCustomDefaulter{}

	t.Run("no-op default does not error", func(t *testing.T) {
		group := &WarpgateTargetGroup{
			Spec: WarpgateTargetGroupSpec{
				ConnectionRef: "my-conn",
				Name:          "production",
			},
		}
		if err := d.Default(context.Background(), group); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestWarpgateTargetGroupValidator(t *testing.T) {
	v := &WarpgateTargetGroupCustomValidator{}
	ctx := context.Background()

	validGroup := func() *WarpgateTargetGroup {
		return &WarpgateTargetGroup{
			Spec: WarpgateTargetGroupSpec{
				ConnectionRef: "my-conn",
				Name:          "production",
				Color:         "Danger",
			},
		}
	}

	t.Run("valid group passes", func(t *testing.T) {
		_, err := v.ValidateCreate(ctx, validGroup())
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("empty connectionRef rejected", func(t *testing.T) {
		g := validGroup()
		g.Spec.ConnectionRef = ""
		_, err := v.ValidateCreate(ctx, g)
		if err == nil {
			t.Error("expected error for empty connectionRef")
		}
	})

	t.Run("empty name rejected", func(t *testing.T) {
		g := validGroup()
		g.Spec.Name = ""
		_, err := v.ValidateCreate(ctx, g)
		if err == nil {
			t.Error("expected error for empty name")
		}
	})

	t.Run("update validation works", func(t *testing.T) {
		old := validGroup()
		bad := validGroup()
		bad.Spec.Name = ""
		_, err := v.ValidateUpdate(ctx, old, bad)
		if err == nil {
			t.Error("expected error for empty name on update")
		}
	})

	t.Run("delete is a no-op", func(t *testing.T) {
		_, err := v.ValidateDelete(ctx, validGroup())
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})
}
