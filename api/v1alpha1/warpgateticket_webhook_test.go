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

func TestWarpgateTicketValidateCreate(t *testing.T) {
	v := &WarpgateTicketCustomValidator{}

	tests := []struct {
		name        string
		obj         *WarpgateTicket
		wantErr     bool
		errMsg      string
		wantWarning bool
	}{
		{
			name: "valid with username and target",
			obj: &WarpgateTicket{
				Spec: WarpgateTicketSpec{
					ConnectionRef: "my-conn",
					Username:      "admin",
					TargetName:    "web-server",
				},
			},
		},
		{
			name: "valid with username only",
			obj: &WarpgateTicket{
				Spec: WarpgateTicketSpec{
					ConnectionRef: "my-conn",
					Username:      "admin",
				},
			},
		},
		{
			name: "valid with target only",
			obj: &WarpgateTicket{
				Spec: WarpgateTicketSpec{
					ConnectionRef: "my-conn",
					TargetName:    "web-server",
				},
			},
		},
		{
			name: "valid with numberOfUses",
			obj: &WarpgateTicket{
				Spec: WarpgateTicketSpec{
					ConnectionRef: "my-conn",
					Username:      "admin",
					NumberOfUses:  &[]int{5}[0],
				},
			},
		},
		{
			name: "empty connectionRef",
			obj: &WarpgateTicket{
				Spec: WarpgateTicketSpec{
					ConnectionRef: "",
					Username:      "admin",
				},
			},
			wantErr: true,
			errMsg:  "spec.connectionRef must not be empty",
		},
		{
			name: "numberOfUses zero",
			obj: &WarpgateTicket{
				Spec: WarpgateTicketSpec{
					ConnectionRef: "my-conn",
					Username:      "admin",
					NumberOfUses:  &[]int{0}[0],
				},
			},
			wantErr: true,
			errMsg:  "spec.numberOfUses must be > 0 when set",
		},
		{
			name: "numberOfUses negative",
			obj: &WarpgateTicket{
				Spec: WarpgateTicketSpec{
					ConnectionRef: "my-conn",
					Username:      "admin",
					NumberOfUses:  &[]int{-1}[0],
				},
			},
			wantErr: true,
			errMsg:  "spec.numberOfUses must be > 0 when set",
		},
		{
			name: "warning when neither username nor targetName set",
			obj: &WarpgateTicket{
				Spec: WarpgateTicketSpec{
					ConnectionRef: "my-conn",
				},
			},
			wantWarning: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			warnings, err := v.ValidateCreate(context.Background(), tt.obj)
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
			if tt.wantWarning && len(warnings) == 0 {
				t.Error("expected warning, got none")
			}
			if !tt.wantWarning && len(warnings) > 0 {
				t.Errorf("unexpected warnings: %v", warnings)
			}
		})
	}
}

func TestWarpgateTicketValidateUpdateImmutable(t *testing.T) {
	v := &WarpgateTicketCustomValidator{}

	old := &WarpgateTicket{
		Spec: WarpgateTicketSpec{
			ConnectionRef: "my-conn",
			Username:      "admin",
			TargetName:    "web-server",
			NumberOfUses:  &[]int{3}[0],
		},
	}

	// Same spec should pass.
	same := &WarpgateTicket{
		Spec: WarpgateTicketSpec{
			ConnectionRef: "my-conn",
			Username:      "admin",
			TargetName:    "web-server",
			NumberOfUses:  &[]int{3}[0],
		},
	}
	if _, err := v.ValidateUpdate(context.Background(), old, same); err != nil {
		t.Fatalf("expected no error for identical spec, got: %v", err)
	}

	// Changed username should fail.
	changed := &WarpgateTicket{
		Spec: WarpgateTicketSpec{
			ConnectionRef: "my-conn",
			Username:      "other-user",
			TargetName:    "web-server",
			NumberOfUses:  &[]int{3}[0],
		},
	}
	if _, err := v.ValidateUpdate(context.Background(), old, changed); err == nil {
		t.Fatal("expected error for spec change, got nil")
	}

	// Changed numberOfUses should fail.
	changedUses := &WarpgateTicket{
		Spec: WarpgateTicketSpec{
			ConnectionRef: "my-conn",
			Username:      "admin",
			TargetName:    "web-server",
			NumberOfUses:  &[]int{10}[0],
		},
	}
	if _, err := v.ValidateUpdate(context.Background(), old, changedUses); err == nil {
		t.Fatal("expected error for numberOfUses change, got nil")
	}

	// Removed numberOfUses should fail.
	removedUses := &WarpgateTicket{
		Spec: WarpgateTicketSpec{
			ConnectionRef: "my-conn",
			Username:      "admin",
			TargetName:    "web-server",
		},
	}
	if _, err := v.ValidateUpdate(context.Background(), old, removedUses); err == nil {
		t.Fatal("expected error for removing numberOfUses, got nil")
	}
}

func TestWarpgateTicketValidateDelete(t *testing.T) {
	v := &WarpgateTicketCustomValidator{}
	_, err := v.ValidateDelete(context.Background(), &WarpgateTicket{})
	if err != nil {
		t.Fatalf("unexpected error on delete: %v", err)
	}
}

func TestWarpgateTicketDefault(t *testing.T) {
	d := &WarpgateTicketCustomDefaulter{}
	obj := &WarpgateTicket{
		Spec: WarpgateTicketSpec{
			ConnectionRef: "c",
			Username:      "u",
		},
	}
	if err := d.Default(context.Background(), obj); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
