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
	"testing"

	"k8s.io/apimachinery/pkg/util/validation/field"
)

// FuzzValidateConnection feeds arbitrary strings into the WarpgateConnection
// validator to ensure it never panics regardless of input.
// Run with: go test -fuzz=FuzzValidateConnection ./api/v1alpha1/
func FuzzValidateConnection(f *testing.F) {
	// Seed corpus: representative valid and invalid inputs.
	seeds := []struct {
		host       string
		secretName string
		tokenKey   string
	}{
		{"https://warpgate.example.com", "my-secret", "token"},
		{"http://localhost:8080", "auth", ""},
		{"", "secret", ""},
		{"ftp://invalid.scheme", "secret", ""},
		{"https://", "secret", "token"},
		{"https://host", "", ""},
		{"\x00\xff", "secret", ""},
		{"https://host with spaces", "secret", ""},
	}
	for _, s := range seeds {
		f.Add(s.host, s.secretName, s.tokenKey)
	}

	f.Fuzz(func(t *testing.T, host, secretName, tokenKey string) {
		conn := &WarpgateConnection{}
		conn.Spec.Host = host
		conn.Spec.AuthSecretRef.Name = secretName
		conn.Spec.AuthSecretRef.TokenKey = tokenKey

		// Must never panic — errors are expected and acceptable.
		_, _ = validateConnection(conn)
	})
}

// FuzzValidateWarpgateTarget feeds arbitrary SSH target fields into the
// WarpgateTarget validator to ensure complex multi-field validation never panics.
// Run with: go test -fuzz=FuzzValidateWarpgateTarget ./api/v1alpha1/
func FuzzValidateWarpgateTarget(f *testing.F) {
	// Seed corpus: representative SSH specs including boundary and invalid values.
	seeds := []struct {
		connRef  string
		name     string
		sshHost  string
		sshUser  string
		sshPort  int
		authKind string
	}{
		{"my-conn", "target", "10.0.0.1", "root", 22, "PublicKey"},
		{"my-conn", "target", "10.0.0.1", "root", 22, "Password"},
		{"", "target", "10.0.0.1", "root", 22, "PublicKey"},
		{"conn", "", "host", "user", 0, ""},
		{"conn", "target", "", "", -1, "Unknown"},
		{"conn", "target", "host", "user", 65536, "PublicKey"},
		{"\x00", "target\xff", "host", "user", 22, "PublicKey"},
		{"conn", "target", "host", "user", 65535, "PublicKey"},
		{"conn", "target", "host", "user", 1, "Password"},
	}
	for _, s := range seeds {
		f.Add(s.connRef, s.name, s.sshHost, s.sshUser, s.sshPort, s.authKind)
	}

	f.Fuzz(func(t *testing.T, connRef, name, sshHost, sshUser string, sshPort int, authKind string) {
		target := &WarpgateTarget{}
		target.Spec.ConnectionRef = connRef
		target.Spec.Name = name
		target.Spec.SSH = &SSHTargetSpec{
			Host:     sshHost,
			Username: sshUser,
			Port:     sshPort,
			AuthKind: authKind,
		}

		// Must never panic — errors are expected and acceptable.
		_ = validateWarpgateTarget(target)
	})
}

// FuzzValidateTLS feeds arbitrary TLS mode strings into the TLS config
// validator to ensure it handles all inputs safely.
// Run with: go test -fuzz=FuzzValidateTLS ./api/v1alpha1/
func FuzzValidateTLS(f *testing.F) {
	seeds := []string{"Disabled", "Preferred", "Required", "", "invalid", "\x00", "preferred", "REQUIRED"}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, mode string) {
		tlsCfg := &TLSConfigSpec{Mode: mode}
		fldPath := field.NewPath("spec").Child("tls")

		// Must never panic — errors are expected and acceptable.
		_ = validateTLS(tlsCfg, fldPath)
	})
}
