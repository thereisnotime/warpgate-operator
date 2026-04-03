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

func TestWarpgateUserDefaulter(t *testing.T) {
	d := &WarpgateUserCustomDefaulter{}

	t.Run("defaults generatePassword to true", func(t *testing.T) {
		user := &WarpgateUser{
			Spec: WarpgateUserSpec{
				ConnectionRef: "my-conn",
				Username:      "alice",
			},
		}
		if err := d.Default(context.Background(), user); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if user.Spec.GeneratePassword == nil || !*user.Spec.GeneratePassword {
			t.Error("expected generatePassword to default to true")
		}
	})

	t.Run("defaults passwordLength to 32", func(t *testing.T) {
		user := &WarpgateUser{
			Spec: WarpgateUserSpec{
				ConnectionRef: "my-conn",
				Username:      "alice",
			},
		}
		if err := d.Default(context.Background(), user); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if user.Spec.PasswordLength == nil || *user.Spec.PasswordLength != 32 {
			t.Errorf("expected passwordLength 32, got %v", user.Spec.PasswordLength)
		}
	})

	t.Run("preserves explicit generatePassword false", func(t *testing.T) {
		f := false
		user := &WarpgateUser{
			Spec: WarpgateUserSpec{
				ConnectionRef:    "my-conn",
				Username:         "alice",
				GeneratePassword: &f,
			},
		}
		if err := d.Default(context.Background(), user); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if *user.Spec.GeneratePassword != false {
			t.Error("expected generatePassword to remain false")
		}
	})

	t.Run("preserves explicit passwordLength", func(t *testing.T) {
		l := 64
		user := &WarpgateUser{
			Spec: WarpgateUserSpec{
				ConnectionRef:  "my-conn",
				Username:       "alice",
				PasswordLength: &l,
			},
		}
		if err := d.Default(context.Background(), user); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if *user.Spec.PasswordLength != 64 {
			t.Errorf("expected passwordLength 64, got %d", *user.Spec.PasswordLength)
		}
	})
}

func TestWarpgateUserValidator(t *testing.T) {
	v := &WarpgateUserCustomValidator{}
	ctx := context.Background()

	validUser := func() *WarpgateUser {
		genPw := true
		pwLen := 32
		return &WarpgateUser{
			Spec: WarpgateUserSpec{
				ConnectionRef:    "my-conn",
				Username:         "alice",
				GeneratePassword: &genPw,
				PasswordLength:   &pwLen,
			},
		}
	}

	t.Run("valid user passes", func(t *testing.T) {
		_, err := v.ValidateCreate(ctx, validUser())
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("empty connectionRef rejected", func(t *testing.T) {
		u := validUser()
		u.Spec.ConnectionRef = ""
		_, err := v.ValidateCreate(ctx, u)
		if err == nil {
			t.Error("expected error for empty connectionRef")
		}
	})

	t.Run("empty username rejected", func(t *testing.T) {
		u := validUser()
		u.Spec.Username = ""
		_, err := v.ValidateCreate(ctx, u)
		if err == nil {
			t.Error("expected error for empty username")
		}
	})

	t.Run("passwordLength too short rejected", func(t *testing.T) {
		u := validUser()
		short := 8
		u.Spec.PasswordLength = &short
		_, err := v.ValidateCreate(ctx, u)
		if err == nil {
			t.Error("expected error for passwordLength < 16")
		}
	})

	t.Run("passwordLength too long rejected", func(t *testing.T) {
		u := validUser()
		long := 256
		u.Spec.PasswordLength = &long
		_, err := v.ValidateCreate(ctx, u)
		if err == nil {
			t.Error("expected error for passwordLength > 128")
		}
	})

	t.Run("passwordLength at boundaries accepted", func(t *testing.T) {
		for _, val := range []int{16, 128} {
			u := validUser()
			u.Spec.PasswordLength = &val
			_, err := v.ValidateCreate(ctx, u)
			if err != nil {
				t.Errorf("expected no error for passwordLength %d, got %v", val, err)
			}
		}
	})

	t.Run("nil passwordLength accepted", func(t *testing.T) {
		u := validUser()
		u.Spec.PasswordLength = nil
		_, err := v.ValidateCreate(ctx, u)
		if err != nil {
			t.Errorf("expected no error for nil passwordLength, got %v", err)
		}
	})

	t.Run("update validation works", func(t *testing.T) {
		old := validUser()
		bad := validUser()
		bad.Spec.Username = ""
		_, err := v.ValidateUpdate(ctx, old, bad)
		if err == nil {
			t.Error("expected error for empty username on update")
		}
	})

	t.Run("delete always passes", func(t *testing.T) {
		_, err := v.ValidateDelete(ctx, validUser())
		if err != nil {
			t.Errorf("expected no error on delete, got %v", err)
		}
	})
}
