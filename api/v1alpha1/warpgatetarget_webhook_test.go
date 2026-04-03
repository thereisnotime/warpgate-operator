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

// --- Helpers ---

func validSSHTarget() *WarpgateTarget {
	return &WarpgateTarget{
		Spec: WarpgateTargetSpec{
			ConnectionRef: "my-conn",
			Name:          "ssh-target",
			SSH: &SSHTargetSpec{
				Host:     "10.0.0.1",
				Port:     22,
				Username: "admin",
				AuthKind: "PublicKey",
			},
		},
	}
}

func validHTTPTarget() *WarpgateTarget {
	return &WarpgateTarget{
		Spec: WarpgateTargetSpec{
			ConnectionRef: "my-conn",
			Name:          "http-target",
			HTTP: &HTTPTargetSpec{
				URL: "https://internal.example.com",
			},
		},
	}
}

func validMySQLTarget() *WarpgateTarget {
	return &WarpgateTarget{
		Spec: WarpgateTargetSpec{
			ConnectionRef: "my-conn",
			Name:          "mysql-target",
			MySQL: &MySQLTargetSpec{
				Host:     "db.local",
				Port:     3306,
				Username: "root",
			},
		},
	}
}

func validPostgreSQLTarget() *WarpgateTarget {
	return &WarpgateTarget{
		Spec: WarpgateTargetSpec{
			ConnectionRef: "my-conn",
			Name:          "pg-target",
			PostgreSQL: &PostgreSQLTargetSpec{
				Host:     "pg.local",
				Port:     5432,
				Username: "postgres",
			},
		},
	}
}

// --- Defaulting tests ---

func TestDefault_SSHPortAndAuthKind(t *testing.T) {
	target := validSSHTarget()
	target.Spec.SSH.Port = 0
	target.Spec.SSH.AuthKind = ""

	d := &WarpgateTargetCustomDefaulter{}
	if err := d.Default(context.Background(), target); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if target.Spec.SSH.Port != 22 {
		t.Errorf("expected SSH port 22, got %d", target.Spec.SSH.Port)
	}
	if target.Spec.SSH.AuthKind != "PublicKey" {
		t.Errorf("expected SSH authKind PublicKey, got %s", target.Spec.SSH.AuthKind)
	}
}

func TestDefault_SSHPortPreserved(t *testing.T) {
	target := validSSHTarget()
	target.Spec.SSH.Port = 2222

	d := &WarpgateTargetCustomDefaulter{}
	if err := d.Default(context.Background(), target); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if target.Spec.SSH.Port != 2222 {
		t.Errorf("expected SSH port 2222, got %d", target.Spec.SSH.Port)
	}
}

func TestDefault_MySQLPort(t *testing.T) {
	target := validMySQLTarget()
	target.Spec.MySQL.Port = 0

	d := &WarpgateTargetCustomDefaulter{}
	if err := d.Default(context.Background(), target); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if target.Spec.MySQL.Port != 3306 {
		t.Errorf("expected MySQL port 3306, got %d", target.Spec.MySQL.Port)
	}
}

func TestDefault_PostgreSQLPort(t *testing.T) {
	target := validPostgreSQLTarget()
	target.Spec.PostgreSQL.Port = 0

	d := &WarpgateTargetCustomDefaulter{}
	if err := d.Default(context.Background(), target); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if target.Spec.PostgreSQL.Port != 5432 {
		t.Errorf("expected PostgreSQL port 5432, got %d", target.Spec.PostgreSQL.Port)
	}
}

func TestDefault_TLSModeHTTP(t *testing.T) {
	target := validHTTPTarget()
	target.Spec.HTTP.TLS = &TLSConfigSpec{}

	d := &WarpgateTargetCustomDefaulter{}
	if err := d.Default(context.Background(), target); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if target.Spec.HTTP.TLS.Mode != "Preferred" {
		t.Errorf("expected TLS mode Preferred, got %s", target.Spec.HTTP.TLS.Mode)
	}
}

func TestDefault_TLSModeMySQL(t *testing.T) {
	target := validMySQLTarget()
	target.Spec.MySQL.TLS = &TLSConfigSpec{}

	d := &WarpgateTargetCustomDefaulter{}
	if err := d.Default(context.Background(), target); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if target.Spec.MySQL.TLS.Mode != "Preferred" {
		t.Errorf("expected TLS mode Preferred, got %s", target.Spec.MySQL.TLS.Mode)
	}
}

func TestDefault_TLSModePostgreSQL(t *testing.T) {
	target := validPostgreSQLTarget()
	target.Spec.PostgreSQL.TLS = &TLSConfigSpec{}

	d := &WarpgateTargetCustomDefaulter{}
	if err := d.Default(context.Background(), target); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if target.Spec.PostgreSQL.TLS.Mode != "Preferred" {
		t.Errorf("expected TLS mode Preferred, got %s", target.Spec.PostgreSQL.TLS.Mode)
	}
}

func TestDefault_TLSModePreserved(t *testing.T) {
	target := validHTTPTarget()
	target.Spec.HTTP.TLS = &TLSConfigSpec{Mode: "Required"}

	d := &WarpgateTargetCustomDefaulter{}
	if err := d.Default(context.Background(), target); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if target.Spec.HTTP.TLS.Mode != "Required" {
		t.Errorf("expected TLS mode Required, got %s", target.Spec.HTTP.TLS.Mode)
	}
}

func TestDefault_NoTLSObjectNoChange(t *testing.T) {
	target := validHTTPTarget()
	// No TLS object set — shouldn't create one.
	d := &WarpgateTargetCustomDefaulter{}
	if err := d.Default(context.Background(), target); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if target.Spec.HTTP.TLS != nil {
		t.Error("expected TLS to remain nil when not set")
	}
}

// --- Validation tests ---

func TestValidate_ValidSSH(t *testing.T) {
	v := &WarpgateTargetCustomValidator{}
	if _, err := v.ValidateCreate(context.Background(), validSSHTarget()); err != nil {
		t.Errorf("expected valid SSH target to pass, got: %v", err)
	}
}

func TestValidate_ValidHTTP(t *testing.T) {
	v := &WarpgateTargetCustomValidator{}
	if _, err := v.ValidateCreate(context.Background(), validHTTPTarget()); err != nil {
		t.Errorf("expected valid HTTP target to pass, got: %v", err)
	}
}

func TestValidate_ValidMySQL(t *testing.T) {
	v := &WarpgateTargetCustomValidator{}
	if _, err := v.ValidateCreate(context.Background(), validMySQLTarget()); err != nil {
		t.Errorf("expected valid MySQL target to pass, got: %v", err)
	}
}

func TestValidate_ValidPostgreSQL(t *testing.T) {
	v := &WarpgateTargetCustomValidator{}
	if _, err := v.ValidateCreate(context.Background(), validPostgreSQLTarget()); err != nil {
		t.Errorf("expected valid PostgreSQL target to pass, got: %v", err)
	}
}

func TestValidate_EmptyConnectionRef(t *testing.T) {
	target := validSSHTarget()
	target.Spec.ConnectionRef = ""

	v := &WarpgateTargetCustomValidator{}
	_, err := v.ValidateCreate(context.Background(), target)
	if err == nil {
		t.Fatal("expected error for empty connectionRef")
	}
	if !strings.Contains(err.Error(), "connectionRef") {
		t.Errorf("error should mention connectionRef: %v", err)
	}
}

func TestValidate_EmptyName(t *testing.T) {
	target := validSSHTarget()
	target.Spec.Name = ""

	v := &WarpgateTargetCustomValidator{}
	_, err := v.ValidateCreate(context.Background(), target)
	if err == nil {
		t.Fatal("expected error for empty name")
	}
	if !strings.Contains(err.Error(), "name") {
		t.Errorf("error should mention name: %v", err)
	}
}

func TestValidate_NoTargetType(t *testing.T) {
	target := &WarpgateTarget{
		Spec: WarpgateTargetSpec{
			ConnectionRef: "conn",
			Name:          "empty",
		},
	}

	v := &WarpgateTargetCustomValidator{}
	_, err := v.ValidateCreate(context.Background(), target)
	if err == nil {
		t.Fatal("expected error when no target type is set")
	}
	if !strings.Contains(err.Error(), "exactly one") {
		t.Errorf("error should mention 'exactly one': %v", err)
	}
}

func TestValidate_MultipleTargetTypes(t *testing.T) {
	target := validSSHTarget()
	target.Spec.HTTP = &HTTPTargetSpec{URL: "http://example.com"}

	v := &WarpgateTargetCustomValidator{}
	_, err := v.ValidateCreate(context.Background(), target)
	if err == nil {
		t.Fatal("expected error when multiple target types are set")
	}
	if !strings.Contains(err.Error(), "only one") {
		t.Errorf("error should mention 'only one': %v", err)
	}
}

func TestValidate_SSHEmptyHost(t *testing.T) {
	target := validSSHTarget()
	target.Spec.SSH.Host = ""

	v := &WarpgateTargetCustomValidator{}
	_, err := v.ValidateCreate(context.Background(), target)
	if err == nil {
		t.Fatal("expected error for empty SSH host")
	}
	if !strings.Contains(err.Error(), "host") {
		t.Errorf("error should mention host: %v", err)
	}
}

func TestValidate_SSHInvalidPort(t *testing.T) {
	for _, port := range []int{0, -1, 65536} {
		target := validSSHTarget()
		target.Spec.SSH.Port = port

		v := &WarpgateTargetCustomValidator{}
		_, err := v.ValidateCreate(context.Background(), target)
		if err == nil {
			t.Errorf("expected error for SSH port %d", port)
		}
	}
}

func TestValidate_SSHEmptyUsername(t *testing.T) {
	target := validSSHTarget()
	target.Spec.SSH.Username = ""

	v := &WarpgateTargetCustomValidator{}
	_, err := v.ValidateCreate(context.Background(), target)
	if err == nil {
		t.Fatal("expected error for empty SSH username")
	}
}

func TestValidate_SSHInvalidAuthKind(t *testing.T) {
	target := validSSHTarget()
	target.Spec.SSH.AuthKind = "Certificate"

	v := &WarpgateTargetCustomValidator{}
	_, err := v.ValidateCreate(context.Background(), target)
	if err == nil {
		t.Fatal("expected error for invalid authKind")
	}
	if !strings.Contains(err.Error(), "authKind") {
		t.Errorf("error should mention authKind: %v", err)
	}
}

func TestValidate_SSHPasswordAuthKind(t *testing.T) {
	target := validSSHTarget()
	target.Spec.SSH.AuthKind = "Password"

	v := &WarpgateTargetCustomValidator{}
	if _, err := v.ValidateCreate(context.Background(), target); err != nil {
		t.Errorf("Password authKind should be valid: %v", err)
	}
}

func TestValidate_HTTPEmptyURL(t *testing.T) {
	target := validHTTPTarget()
	target.Spec.HTTP.URL = ""

	v := &WarpgateTargetCustomValidator{}
	_, err := v.ValidateCreate(context.Background(), target)
	if err == nil {
		t.Fatal("expected error for empty HTTP url")
	}
	if !strings.Contains(err.Error(), "url") {
		t.Errorf("error should mention url: %v", err)
	}
}

func TestValidate_HTTPInvalidTLSMode(t *testing.T) {
	target := validHTTPTarget()
	target.Spec.HTTP.TLS = &TLSConfigSpec{Mode: "Bogus"}

	v := &WarpgateTargetCustomValidator{}
	_, err := v.ValidateCreate(context.Background(), target)
	if err == nil {
		t.Fatal("expected error for invalid TLS mode")
	}
	if !strings.Contains(err.Error(), "mode") {
		t.Errorf("error should mention mode: %v", err)
	}
}

func TestValidate_HTTPValidTLSModes(t *testing.T) {
	for _, mode := range []string{"Disabled", "Preferred", "Required"} {
		target := validHTTPTarget()
		target.Spec.HTTP.TLS = &TLSConfigSpec{Mode: mode}

		v := &WarpgateTargetCustomValidator{}
		if _, err := v.ValidateCreate(context.Background(), target); err != nil {
			t.Errorf("TLS mode %q should be valid: %v", mode, err)
		}
	}
}

func TestValidate_MySQLEmptyHost(t *testing.T) {
	target := validMySQLTarget()
	target.Spec.MySQL.Host = ""

	v := &WarpgateTargetCustomValidator{}
	_, err := v.ValidateCreate(context.Background(), target)
	if err == nil {
		t.Fatal("expected error for empty MySQL host")
	}
}

func TestValidate_MySQLInvalidPort(t *testing.T) {
	target := validMySQLTarget()
	target.Spec.MySQL.Port = 70000

	v := &WarpgateTargetCustomValidator{}
	_, err := v.ValidateCreate(context.Background(), target)
	if err == nil {
		t.Fatal("expected error for invalid MySQL port")
	}
}

func TestValidate_MySQLEmptyUsername(t *testing.T) {
	target := validMySQLTarget()
	target.Spec.MySQL.Username = ""

	v := &WarpgateTargetCustomValidator{}
	_, err := v.ValidateCreate(context.Background(), target)
	if err == nil {
		t.Fatal("expected error for empty MySQL username")
	}
}

func TestValidate_PostgreSQLEmptyHost(t *testing.T) {
	target := validPostgreSQLTarget()
	target.Spec.PostgreSQL.Host = ""

	v := &WarpgateTargetCustomValidator{}
	_, err := v.ValidateCreate(context.Background(), target)
	if err == nil {
		t.Fatal("expected error for empty PostgreSQL host")
	}
}

func TestValidate_PostgreSQLInvalidPort(t *testing.T) {
	target := validPostgreSQLTarget()
	target.Spec.PostgreSQL.Port = 0

	v := &WarpgateTargetCustomValidator{}
	_, err := v.ValidateCreate(context.Background(), target)
	if err == nil {
		t.Fatal("expected error for invalid PostgreSQL port")
	}
}

func TestValidate_PostgreSQLEmptyUsername(t *testing.T) {
	target := validPostgreSQLTarget()
	target.Spec.PostgreSQL.Username = ""

	v := &WarpgateTargetCustomValidator{}
	_, err := v.ValidateCreate(context.Background(), target)
	if err == nil {
		t.Fatal("expected error for empty PostgreSQL username")
	}
}

func TestValidate_UpdateAlsoValidates(t *testing.T) {
	target := validSSHTarget()
	target.Spec.SSH.Host = ""

	v := &WarpgateTargetCustomValidator{}
	_, err := v.ValidateUpdate(context.Background(), validSSHTarget(), target)
	if err == nil {
		t.Fatal("expected update validation to catch empty host")
	}
}

func TestValidate_DeleteAlwaysSucceeds(t *testing.T) {
	v := &WarpgateTargetCustomValidator{}
	_, err := v.ValidateDelete(context.Background(), validSSHTarget())
	if err != nil {
		t.Errorf("delete validation should always succeed: %v", err)
	}
}
