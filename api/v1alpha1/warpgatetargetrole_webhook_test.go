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

func TestWarpgateTargetRoleValidateCreate(t *testing.T) {
	v := &WarpgateTargetRoleCustomValidator{}

	tests := []struct {
		name    string
		obj     *WarpgateTargetRole
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid",
			obj: &WarpgateTargetRole{
				Spec: WarpgateTargetRoleSpec{
					ConnectionRef: "my-conn",
					TargetName:    "web-server",
					RoleName:      "editors",
				},
			},
		},
		{
			name: "empty connectionRef",
			obj: &WarpgateTargetRole{
				Spec: WarpgateTargetRoleSpec{
					ConnectionRef: "",
					TargetName:    "web-server",
					RoleName:      "editors",
				},
			},
			wantErr: true,
			errMsg:  "spec.connectionRef must not be empty",
		},
		{
			name: "empty targetName",
			obj: &WarpgateTargetRole{
				Spec: WarpgateTargetRoleSpec{
					ConnectionRef: "my-conn",
					TargetName:    "",
					RoleName:      "editors",
				},
			},
			wantErr: true,
			errMsg:  "spec.targetName must not be empty",
		},
		{
			name: "empty roleName",
			obj: &WarpgateTargetRole{
				Spec: WarpgateTargetRoleSpec{
					ConnectionRef: "my-conn",
					TargetName:    "web-server",
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

func TestWarpgateTargetRoleValidateUpdate(t *testing.T) {
	v := &WarpgateTargetRoleCustomValidator{}

	old := &WarpgateTargetRole{
		Spec: WarpgateTargetRoleSpec{
			ConnectionRef: "my-conn",
			TargetName:    "web-server",
			RoleName:      "editors",
		},
	}
	valid := &WarpgateTargetRole{
		Spec: WarpgateTargetRoleSpec{
			ConnectionRef: "my-conn",
			TargetName:    "web-server",
			RoleName:      "viewers",
		},
	}
	invalid := &WarpgateTargetRole{
		Spec: WarpgateTargetRoleSpec{
			ConnectionRef: "",
			TargetName:    "web-server",
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

func TestWarpgateTargetRoleValidateDelete(t *testing.T) {
	v := &WarpgateTargetRoleCustomValidator{}
	_, err := v.ValidateDelete(context.Background(), &WarpgateTargetRole{})
	if err != nil {
		t.Fatalf("unexpected error on delete: %v", err)
	}
}

func TestWarpgateTargetRoleDefault(t *testing.T) {
	d := &WarpgateTargetRoleCustomDefaulter{}
	obj := &WarpgateTargetRole{
		Spec: WarpgateTargetRoleSpec{
			ConnectionRef: "c",
			TargetName:    "t",
			RoleName:      "r",
		},
	}
	if err := d.Default(context.Background(), obj); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
