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

func TestWarpgatePasswordCredentialValidateCreate(t *testing.T) {
	v := &WarpgatePasswordCredentialCustomValidator{}

	tests := []struct {
		name    string
		obj     *WarpgatePasswordCredential
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid",
			obj: &WarpgatePasswordCredential{
				Spec: WarpgatePasswordCredentialSpec{
					ConnectionRef:     "my-conn",
					Username:          "admin",
					PasswordSecretRef: SecretKeyRef{Name: "my-secret", Key: "password"},
				},
			},
		},
		{
			name: "empty connectionRef",
			obj: &WarpgatePasswordCredential{
				Spec: WarpgatePasswordCredentialSpec{
					ConnectionRef:     "",
					Username:          "admin",
					PasswordSecretRef: SecretKeyRef{Name: "my-secret", Key: "password"},
				},
			},
			wantErr: true,
			errMsg:  "spec.connectionRef must not be empty",
		},
		{
			name: "empty username",
			obj: &WarpgatePasswordCredential{
				Spec: WarpgatePasswordCredentialSpec{
					ConnectionRef:     "my-conn",
					Username:          "",
					PasswordSecretRef: SecretKeyRef{Name: "my-secret", Key: "password"},
				},
			},
			wantErr: true,
			errMsg:  "spec.username must not be empty",
		},
		{
			name: "empty passwordSecretRef.name",
			obj: &WarpgatePasswordCredential{
				Spec: WarpgatePasswordCredentialSpec{
					ConnectionRef:     "my-conn",
					Username:          "admin",
					PasswordSecretRef: SecretKeyRef{Name: "", Key: "password"},
				},
			},
			wantErr: true,
			errMsg:  "spec.passwordSecretRef.name must not be empty",
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

func TestWarpgatePasswordCredentialValidateUpdate(t *testing.T) {
	v := &WarpgatePasswordCredentialCustomValidator{}

	old := &WarpgatePasswordCredential{
		Spec: WarpgatePasswordCredentialSpec{
			ConnectionRef:     "my-conn",
			Username:          "admin",
			PasswordSecretRef: SecretKeyRef{Name: "my-secret", Key: "password"},
		},
	}
	valid := &WarpgatePasswordCredential{
		Spec: WarpgatePasswordCredentialSpec{
			ConnectionRef:     "my-conn",
			Username:          "admin",
			PasswordSecretRef: SecretKeyRef{Name: "new-secret", Key: "password"},
		},
	}
	invalid := &WarpgatePasswordCredential{
		Spec: WarpgatePasswordCredentialSpec{
			ConnectionRef:     "my-conn",
			Username:          "",
			PasswordSecretRef: SecretKeyRef{Name: "my-secret", Key: "password"},
		},
	}

	if _, err := v.ValidateUpdate(context.Background(), old, valid); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := v.ValidateUpdate(context.Background(), old, invalid); err == nil {
		t.Fatal("expected error for empty username, got nil")
	}
}

func TestWarpgatePasswordCredentialValidateDelete(t *testing.T) {
	v := &WarpgatePasswordCredentialCustomValidator{}
	_, err := v.ValidateDelete(context.Background(), &WarpgatePasswordCredential{})
	if err != nil {
		t.Fatalf("unexpected error on delete: %v", err)
	}
}

func TestWarpgatePasswordCredentialDefaultKey(t *testing.T) {
	d := &WarpgatePasswordCredentialCustomDefaulter{}

	// Key is empty, should default to "password".
	obj := &WarpgatePasswordCredential{
		Spec: WarpgatePasswordCredentialSpec{
			ConnectionRef:     "c",
			Username:          "u",
			PasswordSecretRef: SecretKeyRef{Name: "s", Key: ""},
		},
	}
	if err := d.Default(context.Background(), obj); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if obj.Spec.PasswordSecretRef.Key != "password" {
		t.Errorf("expected key to be defaulted to \"password\", got %q", obj.Spec.PasswordSecretRef.Key)
	}
}

func TestWarpgatePasswordCredentialDefaultKeyPreserved(t *testing.T) {
	d := &WarpgatePasswordCredentialCustomDefaulter{}

	// Key is already set, should not be overwritten.
	obj := &WarpgatePasswordCredential{
		Spec: WarpgatePasswordCredentialSpec{
			ConnectionRef:     "c",
			Username:          "u",
			PasswordSecretRef: SecretKeyRef{Name: "s", Key: "my-key"},
		},
	}
	if err := d.Default(context.Background(), obj); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if obj.Spec.PasswordSecretRef.Key != "my-key" {
		t.Errorf("expected key to remain \"my-key\", got %q", obj.Spec.PasswordSecretRef.Key)
	}
}
