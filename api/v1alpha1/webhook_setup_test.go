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

	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

func TestSetupWebhookWithManager(t *testing.T) {
	if err := AddToScheme(scheme.Scheme); err != nil {
		t.Fatalf("failed to add scheme: %v", err)
	}

	testEnv := &envtest.Environment{}
	cfg, err := testEnv.Start()
	if err != nil {
		t.Fatalf("failed to start envtest: %v", err)
	}
	defer func() { _ = testEnv.Stop() }()

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
	})
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	tests := []struct {
		name string
		fn   func(ctrl.Manager) error
	}{
		{"WarpgateConnection", (&WarpgateConnection{}).SetupWebhookWithManager},
		{"WarpgateRole", (&WarpgateRole{}).SetupWebhookWithManager},
		{"WarpgateUser", (&WarpgateUser{}).SetupWebhookWithManager},
		{"WarpgateTarget", (&WarpgateTarget{}).SetupWebhookWithManager},
		{"WarpgateUserRole", (&WarpgateUserRole{}).SetupWebhookWithManager},
		{"WarpgateTargetRole", (&WarpgateTargetRole{}).SetupWebhookWithManager},
		{"WarpgatePasswordCredential", (&WarpgatePasswordCredential{}).SetupWebhookWithManager},
		{"WarpgatePublicKeyCredential", (&WarpgatePublicKeyCredential{}).SetupWebhookWithManager},
		{"WarpgateTicket", (&WarpgateTicket{}).SetupWebhookWithManager},
		{"WarpgateInstance", (&WarpgateInstance{}).SetupWebhookWithManager},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.fn(mgr); err != nil {
				t.Errorf("SetupWebhookWithManager failed for %s: %v", tt.name, err)
			}
		})
	}
}
