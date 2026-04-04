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

func boolPtr(b bool) *bool    { return &b }
func int32Ptr(i int32) *int32 { return &i }

func validInstance() *WarpgateInstance {
	return &WarpgateInstance{
		Spec: WarpgateInstanceSpec{
			Version: "0.21.1",
			AdminPasswordSecretRef: SecretKeyRef{
				Name: "warpgate-admin",
			},
			HTTP: &HTTPListenerSpec{
				Enabled:     boolPtr(true),
				Port:        int32Ptr(8888),
				ServiceType: "ClusterIP",
			},
		},
	}
}

func validInstanceSSHOnly() *WarpgateInstance {
	return &WarpgateInstance{
		Spec: WarpgateInstanceSpec{
			Version: "0.21.1",
			AdminPasswordSecretRef: SecretKeyRef{
				Name: "warpgate-admin",
			},
			HTTP: &HTTPListenerSpec{
				Enabled: boolPtr(false),
			},
			SSH: &SSHListenerSpec{
				Enabled: boolPtr(true),
			},
		},
	}
}

// --- Defaulting tests ---

func TestInstanceDefault_Replicas(t *testing.T) {
	inst := validInstance()
	inst.Spec.Replicas = nil

	d := &WarpgateInstanceCustomDefaulter{}
	if err := d.Default(context.Background(), inst); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inst.Spec.Replicas == nil || *inst.Spec.Replicas != 1 {
		t.Errorf("expected replicas 1, got %v", inst.Spec.Replicas)
	}
}

func TestInstanceDefault_ReplicasPreserved(t *testing.T) {
	inst := validInstance()
	inst.Spec.Replicas = int32Ptr(3)

	d := &WarpgateInstanceCustomDefaulter{}
	if err := d.Default(context.Background(), inst); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if *inst.Spec.Replicas != 3 {
		t.Errorf("expected replicas 3, got %d", *inst.Spec.Replicas)
	}
}

func TestInstanceDefault_Image(t *testing.T) {
	inst := validInstance()
	inst.Spec.Image = ""

	d := &WarpgateInstanceCustomDefaulter{}
	if err := d.Default(context.Background(), inst); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "ghcr.io/warp-tech/warpgate:0.21.1"
	if inst.Spec.Image != expected {
		t.Errorf("expected image %q, got %q", expected, inst.Spec.Image)
	}
}

func TestInstanceDefault_ImagePreserved(t *testing.T) {
	inst := validInstance()
	inst.Spec.Image = "custom/warpgate:latest"

	d := &WarpgateInstanceCustomDefaulter{}
	if err := d.Default(context.Background(), inst); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inst.Spec.Image != "custom/warpgate:latest" {
		t.Errorf("expected custom image preserved, got %q", inst.Spec.Image)
	}
}

func TestInstanceDefault_HTTPDefaults(t *testing.T) {
	inst := validInstance()
	inst.Spec.HTTP = nil

	d := &WarpgateInstanceCustomDefaulter{}
	if err := d.Default(context.Background(), inst); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inst.Spec.HTTP == nil {
		t.Fatal("expected HTTP to be created")
	}
	if inst.Spec.HTTP.Enabled == nil || !*inst.Spec.HTTP.Enabled {
		t.Error("expected HTTP enabled to default to true")
	}
	if inst.Spec.HTTP.Port == nil || *inst.Spec.HTTP.Port != 8888 {
		t.Errorf("expected HTTP port 8888, got %v", inst.Spec.HTTP.Port)
	}
	if inst.Spec.HTTP.ServiceType != "ClusterIP" {
		t.Errorf("expected HTTP serviceType ClusterIP, got %s", inst.Spec.HTTP.ServiceType)
	}
}

func TestInstanceDefault_SSHDefaults(t *testing.T) {
	inst := validInstance()
	inst.Spec.SSH = &SSHListenerSpec{Enabled: boolPtr(true)}

	d := &WarpgateInstanceCustomDefaulter{}
	if err := d.Default(context.Background(), inst); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inst.Spec.SSH.Port == nil || *inst.Spec.SSH.Port != 2222 {
		t.Errorf("expected SSH port 2222, got %v", inst.Spec.SSH.Port)
	}
	if inst.Spec.SSH.ServiceType != "ClusterIP" {
		t.Errorf("expected SSH serviceType ClusterIP, got %s", inst.Spec.SSH.ServiceType)
	}
}

func TestInstanceDefault_SSHNilNotCreated(t *testing.T) {
	inst := validInstance()
	inst.Spec.SSH = nil

	d := &WarpgateInstanceCustomDefaulter{}
	if err := d.Default(context.Background(), inst); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inst.Spec.SSH != nil {
		t.Error("expected SSH to remain nil when not set")
	}
}

func TestInstanceDefault_MySQLPort(t *testing.T) {
	inst := validInstance()
	inst.Spec.MySQL = &ProtocolListenerSpec{Enabled: boolPtr(true)}

	d := &WarpgateInstanceCustomDefaulter{}
	if err := d.Default(context.Background(), inst); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inst.Spec.MySQL.Port == nil || *inst.Spec.MySQL.Port != 33306 {
		t.Errorf("expected MySQL port 33306, got %v", inst.Spec.MySQL.Port)
	}
}

func TestInstanceDefault_PostgreSQLPort(t *testing.T) {
	inst := validInstance()
	inst.Spec.PostgreSQL = &ProtocolListenerSpec{Enabled: boolPtr(true)}

	d := &WarpgateInstanceCustomDefaulter{}
	if err := d.Default(context.Background(), inst); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inst.Spec.PostgreSQL.Port == nil || *inst.Spec.PostgreSQL.Port != 55432 {
		t.Errorf("expected PostgreSQL port 55432, got %v", inst.Spec.PostgreSQL.Port)
	}
}

func TestInstanceDefault_StorageSize(t *testing.T) {
	inst := validInstance()
	inst.Spec.Storage = nil

	d := &WarpgateInstanceCustomDefaulter{}
	if err := d.Default(context.Background(), inst); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inst.Spec.Storage == nil || inst.Spec.Storage.Size != "1Gi" {
		t.Errorf("expected storage size 1Gi, got %v", inst.Spec.Storage)
	}
}

func TestInstanceDefault_StorageSizePreserved(t *testing.T) {
	inst := validInstance()
	inst.Spec.Storage = &StorageSpec{Size: "10Gi"}

	d := &WarpgateInstanceCustomDefaulter{}
	if err := d.Default(context.Background(), inst); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inst.Spec.Storage.Size != "10Gi" {
		t.Errorf("expected storage size 10Gi, got %s", inst.Spec.Storage.Size)
	}
}

func TestInstanceDefault_TLSCertManager(t *testing.T) {
	inst := validInstance()
	inst.Spec.TLS = nil

	d := &WarpgateInstanceCustomDefaulter{}
	if err := d.Default(context.Background(), inst); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inst.Spec.TLS == nil || inst.Spec.TLS.CertManager == nil || !*inst.Spec.TLS.CertManager {
		t.Error("expected TLS certManager to default to true")
	}
}

func TestInstanceDefault_CreateConnection(t *testing.T) {
	inst := validInstance()
	inst.Spec.CreateConnection = nil

	d := &WarpgateInstanceCustomDefaulter{}
	if err := d.Default(context.Background(), inst); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inst.Spec.CreateConnection == nil || !*inst.Spec.CreateConnection {
		t.Error("expected createConnection to default to true")
	}
}

func TestInstanceDefault_CreateConnectionPreserved(t *testing.T) {
	inst := validInstance()
	inst.Spec.CreateConnection = boolPtr(false)

	d := &WarpgateInstanceCustomDefaulter{}
	if err := d.Default(context.Background(), inst); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if *inst.Spec.CreateConnection {
		t.Error("expected createConnection=false to be preserved")
	}
}

// --- Validation tests ---

func TestInstanceValidate_Valid(t *testing.T) {
	v := &WarpgateInstanceCustomValidator{}
	if _, err := v.ValidateCreate(context.Background(), validInstance()); err != nil {
		t.Errorf("expected valid instance to pass, got: %v", err)
	}
}

func TestInstanceValidate_ValidSSHOnly(t *testing.T) {
	v := &WarpgateInstanceCustomValidator{}
	if _, err := v.ValidateCreate(context.Background(), validInstanceSSHOnly()); err != nil {
		t.Errorf("expected SSH-only instance to pass, got: %v", err)
	}
}

func TestInstanceValidate_EmptyVersion(t *testing.T) {
	inst := validInstance()
	inst.Spec.Version = ""

	v := &WarpgateInstanceCustomValidator{}
	_, err := v.ValidateCreate(context.Background(), inst)
	if err == nil {
		t.Fatal("expected error for empty version")
	}
	if !strings.Contains(err.Error(), "version") {
		t.Errorf("error should mention version: %v", err)
	}
}

func TestInstanceValidate_EmptyAdminPasswordSecretRef(t *testing.T) {
	inst := validInstance()
	inst.Spec.AdminPasswordSecretRef.Name = ""

	v := &WarpgateInstanceCustomValidator{}
	_, err := v.ValidateCreate(context.Background(), inst)
	if err == nil {
		t.Fatal("expected error for empty adminPasswordSecretRef.name")
	}
	if !strings.Contains(err.Error(), "adminPasswordSecretRef") {
		t.Errorf("error should mention adminPasswordSecretRef: %v", err)
	}
}

func TestInstanceValidate_NeitherHTTPNorSSHEnabled(t *testing.T) {
	inst := validInstance()
	inst.Spec.HTTP.Enabled = boolPtr(false)
	inst.Spec.SSH = nil

	v := &WarpgateInstanceCustomValidator{}
	_, err := v.ValidateCreate(context.Background(), inst)
	if err == nil {
		t.Fatal("expected error when neither HTTP nor SSH enabled")
	}
	if !strings.Contains(err.Error(), "at least one") {
		t.Errorf("error should mention 'at least one': %v", err)
	}
}

func TestInstanceValidate_InvalidHTTPPort(t *testing.T) {
	inst := validInstance()
	inst.Spec.HTTP.Port = int32Ptr(0)

	v := &WarpgateInstanceCustomValidator{}
	_, err := v.ValidateCreate(context.Background(), inst)
	if err == nil {
		t.Fatal("expected error for invalid HTTP port")
	}
	if !strings.Contains(err.Error(), "port") {
		t.Errorf("error should mention port: %v", err)
	}
}

func TestInstanceValidate_InvalidSSHPort(t *testing.T) {
	inst := validInstance()
	inst.Spec.SSH = &SSHListenerSpec{
		Enabled: boolPtr(true),
		Port:    int32Ptr(70000),
	}

	v := &WarpgateInstanceCustomValidator{}
	_, err := v.ValidateCreate(context.Background(), inst)
	if err == nil {
		t.Fatal("expected error for invalid SSH port")
	}
	if !strings.Contains(err.Error(), "port") {
		t.Errorf("error should mention port: %v", err)
	}
}

func TestInstanceValidate_InvalidMySQLPort(t *testing.T) {
	inst := validInstance()
	inst.Spec.MySQL = &ProtocolListenerSpec{Port: int32Ptr(-1)}

	v := &WarpgateInstanceCustomValidator{}
	_, err := v.ValidateCreate(context.Background(), inst)
	if err == nil {
		t.Fatal("expected error for invalid MySQL port")
	}
}

func TestInstanceValidate_InvalidPostgreSQLPort(t *testing.T) {
	inst := validInstance()
	inst.Spec.PostgreSQL = &ProtocolListenerSpec{Port: int32Ptr(99999)}

	v := &WarpgateInstanceCustomValidator{}
	_, err := v.ValidateCreate(context.Background(), inst)
	if err == nil {
		t.Fatal("expected error for invalid PostgreSQL port")
	}
}

func TestInstanceValidate_InvalidStorageSize(t *testing.T) {
	inst := validInstance()
	inst.Spec.Storage = &StorageSpec{Size: "not-a-quantity"}

	v := &WarpgateInstanceCustomValidator{}
	_, err := v.ValidateCreate(context.Background(), inst)
	if err == nil {
		t.Fatal("expected error for invalid storage size")
	}
	if !strings.Contains(err.Error(), "storage") {
		t.Errorf("error should mention storage: %v", err)
	}
}

func TestInstanceValidate_ValidStorageSize(t *testing.T) {
	inst := validInstance()
	inst.Spec.Storage = &StorageSpec{Size: "5Gi"}

	v := &WarpgateInstanceCustomValidator{}
	if _, err := v.ValidateCreate(context.Background(), inst); err != nil {
		t.Errorf("expected valid storage size to pass, got: %v", err)
	}
}

func TestInstanceValidate_UpdateAlsoValidates(t *testing.T) {
	inst := validInstance()
	inst.Spec.Version = ""

	v := &WarpgateInstanceCustomValidator{}
	_, err := v.ValidateUpdate(context.Background(), validInstance(), inst)
	if err == nil {
		t.Fatal("expected update validation to catch empty version")
	}
}

func TestInstanceValidate_DeleteAlwaysSucceeds(t *testing.T) {
	v := &WarpgateInstanceCustomValidator{}
	_, err := v.ValidateDelete(context.Background(), validInstance())
	if err != nil {
		t.Errorf("delete validation should always succeed: %v", err)
	}
}
