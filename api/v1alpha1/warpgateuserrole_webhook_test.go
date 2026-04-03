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

func TestWarpgateUserRoleValidateCreate(t *testing.T) {
	v := &WarpgateUserRoleCustomValidator{}

	tests := []struct {
		name    string
		obj     *WarpgateUserRole
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid",
			obj: &WarpgateUserRole{
				Spec: WarpgateUserRoleSpec{
					ConnectionRef: "my-conn",
					Username:      "admin",
					RoleName:      "editors",
				},
			},
		},
		{
			name: "empty connectionRef",
			obj: &WarpgateUserRole{
				Spec: WarpgateUserRoleSpec{
					ConnectionRef: "",
					Username:      "admin",
					RoleName:      "editors",
				},
			},
			wantErr: true,
			errMsg:  "spec.connectionRef must not be empty",
		},
		{
			name: "empty username",
			obj: &WarpgateUserRole{
				Spec: WarpgateUserRoleSpec{
					ConnectionRef: "my-conn",
					Username:      "",
					RoleName:      "editors",
				},
			},
			wantErr: true,
			errMsg:  "spec.username must not be empty",
		},
		{
			name: "empty roleName",
			obj: &WarpgateUserRole{
				Spec: WarpgateUserRoleSpec{
					ConnectionRef: "my-conn",
					Username:      "admin",
					RoleName:      "",
				},
			},
			wantErr: true,
			errMsg:  "spec.roleName must not be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := v.ValidateCreate(context.Background(), tt.obj)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if err.Error() != tt.errMsg {
					t.Errorf("expected error %q, got %q", tt.errMsg, err.Error())
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestWarpgateUserRoleValidateUpdate(t *testing.T) {
	v := &WarpgateUserRoleCustomValidator{}

	old := &WarpgateUserRole{
		Spec: WarpgateUserRoleSpec{
			ConnectionRef: "my-conn",
			Username:      "admin",
			RoleName:      "editors",
		},
	}
	valid := &WarpgateUserRole{
		Spec: WarpgateUserRoleSpec{
			ConnectionRef: "my-conn",
			Username:      "admin",
			RoleName:      "viewers",
		},
	}
	invalid := &WarpgateUserRole{
		Spec: WarpgateUserRoleSpec{
			ConnectionRef: "",
			Username:      "admin",
			RoleName:      "viewers",
		},
	}

	if _, err := v.ValidateUpdate(context.Background(), old, valid); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := v.ValidateUpdate(context.Background(), old, invalid); err == nil {
		t.Fatal("expected error for empty connectionRef, got nil")
	}
}

func TestWarpgateUserRoleValidateDelete(t *testing.T) {
	v := &WarpgateUserRoleCustomValidator{}
	_, err := v.ValidateDelete(context.Background(), &WarpgateUserRole{})
	if err != nil {
		t.Fatalf("unexpected error on delete: %v", err)
	}
}

func TestWarpgateUserRoleDefault(t *testing.T) {
	d := &WarpgateUserRoleCustomDefaulter{}
	obj := &WarpgateUserRole{
		Spec: WarpgateUserRoleSpec{
			ConnectionRef: "c",
			Username:      "u",
			RoleName:      "r",
		},
	}
	if err := d.Default(context.Background(), obj); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
