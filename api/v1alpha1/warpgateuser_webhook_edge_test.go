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
)

func validUserForEdgeTests() *WarpgateUser {
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

func TestEdge_UserPasswordLength15Rejected(t *testing.T) {
	v := &WarpgateUserCustomValidator{}
	u := validUserForEdgeTests()
	length := 15
	u.Spec.PasswordLength = &length

	_, err := v.ValidateCreate(context.Background(), u)
	if err == nil {
		t.Fatal("expected rejection for passwordLength 15 (below minimum 16)")
	}
	if !strings.Contains(err.Error(), "passwordLength") {
		t.Errorf("error should mention passwordLength, got: %v", err)
	}
}

func TestEdge_UserPasswordLength129Rejected(t *testing.T) {
	v := &WarpgateUserCustomValidator{}
	u := validUserForEdgeTests()
	length := 129
	u.Spec.PasswordLength = &length

	_, err := v.ValidateCreate(context.Background(), u)
	if err == nil {
		t.Fatal("expected rejection for passwordLength 129 (above maximum 128)")
	}
	if !strings.Contains(err.Error(), "passwordLength") {
		t.Errorf("error should mention passwordLength, got: %v", err)
	}
}

func TestEdge_UserPasswordLength16Accepted(t *testing.T) {
	v := &WarpgateUserCustomValidator{}
	u := validUserForEdgeTests()
	length := 16
	u.Spec.PasswordLength = &length

	_, err := v.ValidateCreate(context.Background(), u)
	if err != nil {
		t.Errorf("expected passwordLength 16 (lower boundary) to be accepted, got: %v", err)
	}
}

func TestEdge_UserPasswordLength128Accepted(t *testing.T) {
	v := &WarpgateUserCustomValidator{}
	u := validUserForEdgeTests()
	length := 128
	u.Spec.PasswordLength = &length

	_, err := v.ValidateCreate(context.Background(), u)
	if err != nil {
		t.Errorf("expected passwordLength 128 (upper boundary) to be accepted, got: %v", err)
	}
}

func TestEdge_UserPasswordLengthBoundaryOnUpdate(t *testing.T) {
	v := &WarpgateUserCustomValidator{}
	old := validUserForEdgeTests()

	for _, tc := range []struct {
		length  int
		wantErr bool
	}{
		{length: 15, wantErr: true},
		{length: 16, wantErr: false},
		{length: 64, wantErr: false},
		{length: 128, wantErr: false},
		{length: 129, wantErr: true},
	} {
		u := validUserForEdgeTests()
		u.Spec.PasswordLength = &tc.length

		_, err := v.ValidateUpdate(context.Background(), old, u)
		if tc.wantErr && err == nil {
			t.Errorf("expected rejection for passwordLength %d on update", tc.length)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("expected acceptance for passwordLength %d on update, got: %v", tc.length, err)
		}
	}
}

func TestEdge_UserPasswordLengthExtremes(t *testing.T) {
	v := &WarpgateUserCustomValidator{}

	for _, tc := range []struct {
		name    string
		length  int
		wantErr bool
	}{
		{name: "zero", length: 0, wantErr: true},
		{name: "one", length: 1, wantErr: true},
		{name: "negative", length: -1, wantErr: true},
		{name: "very large", length: 10000, wantErr: true},
		{name: "min boundary", length: 16, wantErr: false},
		{name: "max boundary", length: 128, wantErr: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			u := validUserForEdgeTests()
			u.Spec.PasswordLength = &tc.length

			_, err := v.ValidateCreate(context.Background(), u)
			if tc.wantErr && err == nil {
				t.Errorf("expected rejection for passwordLength %d", tc.length)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("expected acceptance for passwordLength %d, got: %v", tc.length, err)
			}
		})
	}
}

func TestEdge_UserNilPasswordLengthAcceptedOnCreate(t *testing.T) {
	v := &WarpgateUserCustomValidator{}
	u := validUserForEdgeTests()
	u.Spec.PasswordLength = nil

	_, err := v.ValidateCreate(context.Background(), u)
	if err != nil {
		t.Errorf("expected nil passwordLength to be accepted, got: %v", err)
	}
}

func TestEdge_UserDeleteAlwaysAllowed(t *testing.T) {
	v := &WarpgateUserCustomValidator{}
	_, err := v.ValidateDelete(context.Background(), validUserForEdgeTests())
	if err != nil {
		t.Fatalf("expected delete to always succeed, got: %v", err)
	}
}
