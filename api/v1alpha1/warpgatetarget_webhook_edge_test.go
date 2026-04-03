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

func TestEdge_TargetTwoTypesSSHAndHTTP(t *testing.T) {
	target := &WarpgateTarget{
		Spec: WarpgateTargetSpec{
			ConnectionRef: "conn",
			Name:          "dual-target",
			SSH: &SSHTargetSpec{
				Host:     "10.0.0.1",
				Port:     22,
				Username: "root",
				AuthKind: "PublicKey",
			},
			HTTP: &HTTPTargetSpec{
				URL: "https://example.com",
			},
		},
	}

	v := &WarpgateTargetCustomValidator{}
	_, err := v.ValidateCreate(context.Background(), target)
	if err == nil {
		t.Fatal("expected rejection when both ssh and http are set")
	}
	if !strings.Contains(err.Error(), "only one") {
		t.Errorf("error should mention 'only one', got: %v", err)
	}
}

func TestEdge_TargetAllTypesNil(t *testing.T) {
	target := &WarpgateTarget{
		Spec: WarpgateTargetSpec{
			ConnectionRef: "conn",
			Name:          "no-type",
		},
	}

	v := &WarpgateTargetCustomValidator{}
	_, err := v.ValidateCreate(context.Background(), target)
	if err == nil {
		t.Fatal("expected rejection when all target types are nil")
	}
	if !strings.Contains(err.Error(), "exactly one") {
		t.Errorf("error should mention 'exactly one', got: %v", err)
	}
}

func TestEdge_SSHPortZeroDefaultsTo22(t *testing.T) {
	target := &WarpgateTarget{
		Spec: WarpgateTargetSpec{
			ConnectionRef: "conn",
			Name:          "ssh-default-port",
			SSH: &SSHTargetSpec{
				Host:     "10.0.0.1",
				Port:     0,
				Username: "root",
				AuthKind: "PublicKey",
			},
		},
	}

	d := &WarpgateTargetCustomDefaulter{}
	if err := d.Default(context.Background(), target); err != nil {
		t.Fatalf("unexpected defaulting error: %v", err)
	}
	if target.Spec.SSH.Port != 22 {
		t.Errorf("expected SSH port to default to 22, got %d", target.Spec.SSH.Port)
	}

	// After defaulting, validation should pass.
	v := &WarpgateTargetCustomValidator{}
	if _, err := v.ValidateCreate(context.Background(), target); err != nil {
		t.Errorf("expected valid target after defaulting, got: %v", err)
	}
}

func TestEdge_SSHPort65536Rejected(t *testing.T) {
	target := &WarpgateTarget{
		Spec: WarpgateTargetSpec{
			ConnectionRef: "conn",
			Name:          "ssh-bad-port",
			SSH: &SSHTargetSpec{
				Host:     "10.0.0.1",
				Port:     65536,
				Username: "root",
				AuthKind: "PublicKey",
			},
		},
	}

	v := &WarpgateTargetCustomValidator{}
	_, err := v.ValidateCreate(context.Background(), target)
	if err == nil {
		t.Fatal("expected rejection for SSH port 65536")
	}
	if !strings.Contains(err.Error(), "port") {
		t.Errorf("error should mention port, got: %v", err)
	}
}

func TestEdge_MySQLEmptyHostRejected(t *testing.T) {
	target := &WarpgateTarget{
		Spec: WarpgateTargetSpec{
			ConnectionRef: "conn",
			Name:          "mysql-no-host",
			MySQL: &MySQLTargetSpec{
				Host:     "",
				Port:     3306,
				Username: "root",
			},
		},
	}

	v := &WarpgateTargetCustomValidator{}
	_, err := v.ValidateCreate(context.Background(), target)
	if err == nil {
		t.Fatal("expected rejection for empty MySQL host")
	}
	if !strings.Contains(err.Error(), "host") {
		t.Errorf("error should mention host, got: %v", err)
	}
}

func TestEdge_PostgreSQLInvalidTLSModeRejected(t *testing.T) {
	target := &WarpgateTarget{
		Spec: WarpgateTargetSpec{
			ConnectionRef: "conn",
			Name:          "pg-bad-tls",
			PostgreSQL: &PostgreSQLTargetSpec{
				Host:     "pg.local",
				Port:     5432,
				Username: "postgres",
				TLS:      &TLSConfigSpec{Mode: "Invalid"},
			},
		},
	}

	v := &WarpgateTargetCustomValidator{}
	_, err := v.ValidateCreate(context.Background(), target)
	if err == nil {
		t.Fatal("expected rejection for PostgreSQL TLS mode 'Invalid'")
	}
	if !strings.Contains(err.Error(), "mode") {
		t.Errorf("error should mention mode, got: %v", err)
	}
}

func TestEdge_HTTPEmptyURLRejected(t *testing.T) {
	target := &WarpgateTarget{
		Spec: WarpgateTargetSpec{
			ConnectionRef: "conn",
			Name:          "http-no-url",
			HTTP: &HTTPTargetSpec{
				URL: "",
			},
		},
	}

	v := &WarpgateTargetCustomValidator{}
	_, err := v.ValidateCreate(context.Background(), target)
	if err == nil {
		t.Fatal("expected rejection for empty HTTP URL")
	}
	if !strings.Contains(err.Error(), "url") {
		t.Errorf("error should mention url, got: %v", err)
	}
}

func TestEdge_TargetThreeTypesSetRejected(t *testing.T) {
	target := &WarpgateTarget{
		Spec: WarpgateTargetSpec{
			ConnectionRef: "conn",
			Name:          "triple-target",
			SSH: &SSHTargetSpec{
				Host:     "10.0.0.1",
				Port:     22,
				Username: "root",
				AuthKind: "PublicKey",
			},
			HTTP: &HTTPTargetSpec{
				URL: "https://example.com",
			},
			MySQL: &MySQLTargetSpec{
				Host:     "db.local",
				Port:     3306,
				Username: "root",
			},
		},
	}

	v := &WarpgateTargetCustomValidator{}
	_, err := v.ValidateCreate(context.Background(), target)
	if err == nil {
		t.Fatal("expected rejection when three target types are set")
	}
	if !strings.Contains(err.Error(), "only one") {
		t.Errorf("error should mention 'only one', got: %v", err)
	}
}

func TestEdge_SSHPortBoundaryValues(t *testing.T) {
	v := &WarpgateTargetCustomValidator{}

	for _, tc := range []struct {
		port    int
		wantErr bool
	}{
		{port: 1, wantErr: false},
		{port: 65535, wantErr: false},
		{port: 0, wantErr: true},
		{port: -1, wantErr: true},
		{port: 65536, wantErr: true},
	} {
		target := &WarpgateTarget{
			Spec: WarpgateTargetSpec{
				ConnectionRef: "conn",
				Name:          "ssh-port-boundary",
				SSH: &SSHTargetSpec{
					Host:     "10.0.0.1",
					Port:     tc.port,
					Username: "root",
					AuthKind: "PublicKey",
				},
			},
		}

		_, err := v.ValidateCreate(context.Background(), target)
		if tc.wantErr && err == nil {
			t.Errorf("expected rejection for SSH port %d", tc.port)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("expected acceptance for SSH port %d, got: %v", tc.port, err)
		}
	}
}

func TestEdge_MySQLTLSInvalidModeRejected(t *testing.T) {
	target := &WarpgateTarget{
		Spec: WarpgateTargetSpec{
			ConnectionRef: "conn",
			Name:          "mysql-bad-tls",
			MySQL: &MySQLTargetSpec{
				Host:     "db.local",
				Port:     3306,
				Username: "root",
				TLS:      &TLSConfigSpec{Mode: "Invalid"},
			},
		},
	}

	v := &WarpgateTargetCustomValidator{}
	_, err := v.ValidateCreate(context.Background(), target)
	if err == nil {
		t.Fatal("expected rejection for MySQL TLS mode 'Invalid'")
	}
	if !strings.Contains(err.Error(), "mode") {
		t.Errorf("error should mention mode, got: %v", err)
	}
}
