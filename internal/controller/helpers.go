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
	"crypto/x509"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	warpgatev1alpha1 "github.com/thereisnotime/warpgate-operator/api/v1alpha1"
	"github.com/thereisnotime/warpgate-operator/internal/warpgate"
)

// getWarpgateClient builds a Warpgate API client by looking up the named WarpgateConnection
// CR and its referenced credentials Secret.
func getWarpgateClient(ctx context.Context, r client.Reader, namespace, connectionName string) (*warpgate.Client, error) {
	var conn warpgatev1alpha1.WarpgateConnection
	if err := r.Get(ctx, types.NamespacedName{Name: connectionName, Namespace: namespace}, &conn); err != nil {
		return nil, fmt.Errorf("getting WarpgateConnection %q: %w", connectionName, err)
	}

	var secret corev1.Secret
	if err := r.Get(ctx, types.NamespacedName{
		Name:      conn.Spec.AuthSecretRef.Name,
		Namespace: namespace,
	}, &secret); err != nil {
		return nil, fmt.Errorf("getting auth secret %q: %w", conn.Spec.AuthSecretRef.Name, err)
	}

	caCert, err := loadConnectionCA(ctx, r, &conn)
	if err != nil {
		return nil, err
	}

	tokenKey := conn.Spec.AuthSecretRef.TokenKey
	if tokenKey == "" {
		tokenKey = "token"
	}

	// Prefer token-based auth if the token key exists in the Secret.
	if token, ok := secret.Data[tokenKey]; ok && len(token) > 0 {
		return warpgate.NewClient(warpgate.Config{
			Host:               conn.Spec.Host,
			Token:              string(token),
			InsecureSkipVerify: conn.Spec.InsecureSkipVerify,
			CACert:             caCert,
		}), nil
	}

	// Fall back to username/password auth.
	usernameKey := conn.Spec.AuthSecretRef.UsernameKey
	if usernameKey == "" {
		usernameKey = "username"
	}
	passwordKey := conn.Spec.AuthSecretRef.PasswordKey
	if passwordKey == "" {
		passwordKey = "password"
	}

	username, ok := secret.Data[usernameKey]
	if !ok {
		return nil, fmt.Errorf("key %q not found in auth secret %q", usernameKey, conn.Spec.AuthSecretRef.Name)
	}

	password, ok := secret.Data[passwordKey]
	if !ok {
		return nil, fmt.Errorf("key %q not found in auth secret %q", passwordKey, conn.Spec.AuthSecretRef.Name)
	}

	return warpgate.NewClient(warpgate.Config{
		Host:               conn.Spec.Host,
		Username:           string(username),
		Password:           string(password),
		InsecureSkipVerify: conn.Spec.InsecureSkipVerify,
		CACert:             caCert,
	}), nil
}

// loadConnectionCA returns the PEM CA bundle referenced by conn.spec.caSecretRef,
// or nil when there is none or verification is disabled anyway.
func loadConnectionCA(ctx context.Context, r client.Reader, conn *warpgatev1alpha1.WarpgateConnection) ([]byte, error) {
	ref := conn.Spec.CASecretRef
	if ref == nil || conn.Spec.InsecureSkipVerify {
		return nil, nil
	}
	key := ref.Key
	if key == "" {
		key = "ca.crt"
	}
	var secret corev1.Secret
	if err := r.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: conn.Namespace}, &secret); err != nil {
		return nil, fmt.Errorf("getting CA secret %q: %w", ref.Name, err)
	}
	pem := secret.Data[key]
	if !x509.NewCertPool().AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("key %q in CA secret %q does not contain a PEM certificate", key, ref.Name)
	}
	return pem, nil
}
