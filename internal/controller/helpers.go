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

package controller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	warpgatev1alpha1 "github.com/thereisnotime/warpgate-operator/api/v1alpha1"
	"github.com/thereisnotime/warpgate-operator/internal/warpgate"
)

// getWarpgateClient builds a Warpgate API client by looking up the named WarpgateConnection
// CR and its referenced token Secret.
func getWarpgateClient(ctx context.Context, r client.Reader, namespace, connectionName string) (*warpgate.Client, error) {
	var conn warpgatev1alpha1.WarpgateConnection
	if err := r.Get(ctx, types.NamespacedName{Name: connectionName, Namespace: namespace}, &conn); err != nil {
		return nil, fmt.Errorf("getting WarpgateConnection %q: %w", connectionName, err)
	}

	var secret corev1.Secret
	if err := r.Get(ctx, types.NamespacedName{
		Name:      conn.Spec.TokenSecretRef.Name,
		Namespace: namespace,
	}, &secret); err != nil {
		return nil, fmt.Errorf("getting token secret %q: %w", conn.Spec.TokenSecretRef.Name, err)
	}

	key := conn.Spec.TokenSecretRef.Key
	if key == "" {
		key = "token"
	}

	token, ok := secret.Data[key]
	if !ok {
		return nil, fmt.Errorf("key %q not found in secret %q", key, conn.Spec.TokenSecretRef.Name)
	}

	return warpgate.NewClient(warpgate.Config{
		Host:               conn.Spec.Host,
		Token:              string(token),
		InsecureSkipVerify: conn.Spec.InsecureSkipVerify,
	}), nil
}
