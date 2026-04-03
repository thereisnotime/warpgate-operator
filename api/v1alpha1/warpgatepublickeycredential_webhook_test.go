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

func TestWarpgatePublicKeyCredentialValidateCreate(t *testing.T) {
	v := &WarpgatePublicKeyCredentialCustomValidator{}

	tests := []struct {
		name    string
		obj     *WarpgatePublicKeyCredential
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid rsa key",
			obj: &WarpgatePublicKeyCredential{
				Spec: WarpgatePublicKeyCredentialSpec{
					ConnectionRef:    "my-conn",
					Username:         "admin",
					Label:            "my laptop",
					OpenSSHPublicKey: "ssh-rsa AAAAB3NzaC1yc2EAAA...",
				},
			},
		},
		{
			name: "valid ed25519 key",
			obj: &WarpgatePublicKeyCredential{
				Spec: WarpgatePublicKeyCredentialSpec{
					ConnectionRef:    "my-conn",
					Username:         "admin",
					Label:            "my laptop",
					OpenSSHPublicKey: "ssh-ed25519 AAAAC3Nza...",
				},
			},
		},
		{
			name: "empty connectionRef",
			obj: &WarpgatePublicKeyCredential{
				Spec: WarpgatePublicKeyCredentialSpec{
					ConnectionRef:    "",
					Username:         "admin",
					Label:            "my laptop",
					OpenSSHPublicKey: "ssh-rsa AAAAB3...",
				},
			},
			wantErr: true,
			errMsg:  "spec.connectionRef must not be empty",
		},
		{
			name: "empty username",
			obj: &WarpgatePublicKeyCredential{
				Spec: WarpgatePublicKeyCredentialSpec{
					ConnectionRef:    "my-conn",
					Username:         "",
					Label:            "my laptop",
					OpenSSHPublicKey: "ssh-rsa AAAAB3...",
				},
			},
			wantErr: true,
			errMsg:  "spec.username must not be empty",
		},
		{
			name: "empty label",
			obj: &WarpgatePublicKeyCredential{
				Spec: WarpgatePublicKeyCredentialSpec{
					ConnectionRef:    "my-conn",
					Username:         "admin",
					Label:            "",
					OpenSSHPublicKey: "ssh-rsa AAAAB3...",
				},
			},
			wantErr: true,
			errMsg:  "spec.label must not be empty",
		},
		{
			name: "empty opensshPublicKey",
			obj: &WarpgatePublicKeyCredential{
				Spec: WarpgatePublicKeyCredentialSpec{
					ConnectionRef:    "my-conn",
					Username:         "admin",
					Label:            "my laptop",
					OpenSSHPublicKey: "",
				},
			},
			wantErr: true,
			errMsg:  "spec.opensshPublicKey must not be empty",
		},
		{
			name: "invalid key format",
			obj: &WarpgatePublicKeyCredential{
				Spec: WarpgatePublicKeyCredentialSpec{
					ConnectionRef:    "my-conn",
					Username:         "admin",
					Label:            "my laptop",
					OpenSSHPublicKey: "AAAAB3NzaC1yc2EAAA...",
				},
			},
			wantErr: true,
			errMsg:  "spec.opensshPublicKey must start with \"ssh-\" (e.g. ssh-rsa, ssh-ed25519)",
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

func TestWarpgatePublicKeyCredentialValidateUpdate(t *testing.T) {
	v := &WarpgatePublicKeyCredentialCustomValidator{}

	old := &WarpgatePublicKeyCredential{
		Spec: WarpgatePublicKeyCredentialSpec{
			ConnectionRef:    "my-conn",
			Username:         "admin",
			Label:            "my laptop",
			OpenSSHPublicKey: "ssh-rsa AAAAB3...",
		},
	}
	valid := &WarpgatePublicKeyCredential{
		Spec: WarpgatePublicKeyCredentialSpec{
			ConnectionRef:    "my-conn",
			Username:         "admin",
			Label:            "new label",
			OpenSSHPublicKey: "ssh-ed25519 AAAAC3...",
		},
	}
	invalid := &WarpgatePublicKeyCredential{
		Spec: WarpgatePublicKeyCredentialSpec{
			ConnectionRef:    "my-conn",
			Username:         "admin",
			Label:            "my laptop",
			OpenSSHPublicKey: "not-a-key",
		},
	}

	if _, err := v.ValidateUpdate(context.Background(), old, valid); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := v.ValidateUpdate(context.Background(), old, invalid); err == nil {
		t.Fatal("expected error for invalid key format, got nil")
	}
}

func TestWarpgatePublicKeyCredentialValidateDelete(t *testing.T) {
	v := &WarpgatePublicKeyCredentialCustomValidator{}
	_, err := v.ValidateDelete(context.Background(), &WarpgatePublicKeyCredential{})
	if err != nil {
		t.Fatalf("unexpected error on delete: %v", err)
	}
}

func TestWarpgatePublicKeyCredentialDefault(t *testing.T) {
	d := &WarpgatePublicKeyCredentialCustomDefaulter{}
	obj := &WarpgatePublicKeyCredential{
		Spec: WarpgatePublicKeyCredentialSpec{
			ConnectionRef:    "c",
			Username:         "u",
			Label:            "l",
			OpenSSHPublicKey: "ssh-rsa AAA...",
		},
	}
	if err := d.Default(context.Background(), obj); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
