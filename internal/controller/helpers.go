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
	"crypto/sha256"
	"crypto/x509"
	"encoding/binary"
	"fmt"
	"strconv"
	"sync"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	warpgatev1alpha1 "github.com/thereisnotime/warpgate-operator/api/v1alpha1"
	"github.com/thereisnotime/warpgate-operator/internal/warpgate"
)

// clientCache keeps one long-lived warpgate.Client (and so one HTTP connection
// pool) per WarpgateConnection. Building a client per reconcile used to leak a
// keep-alive connection and its goroutines on every reconcile.
//
// Entries are keyed by the connection's namespace/name and carry a fingerprint
// of the resolved config (host, TLS settings, CA bundle and credentials), so a
// change to the connection spec or to the auth/CA Secrets swaps the client.
type clientCache struct {
	mu      sync.Mutex
	entries map[types.NamespacedName]clientCacheEntry
}

type clientCacheEntry struct {
	fingerprint [sha256.Size]byte
	client      *warpgate.Client
}

func newClientCache() *clientCache {
	return &clientCache{entries: map[types.NamespacedName]clientCacheEntry{}}
}

// warpgateClients is shared by all controllers in the manager.
var warpgateClients = newClientCache()

func configFingerprint(cfg warpgate.Config) [sha256.Size]byte {
	h := sha256.New()
	// Length-prefix every field so adjacent values can't run into each other.
	for _, f := range [][]byte{
		[]byte(cfg.Host),
		[]byte(cfg.Token),
		[]byte(cfg.Username),
		[]byte(cfg.Password),
		[]byte(strconv.FormatBool(cfg.InsecureSkipVerify)),
		cfg.CACert,
		[]byte(cfg.Timeout.String()),
	} {
		_ = binary.Write(h, binary.BigEndian, uint64(len(f)))
		_, _ = h.Write(f)
	}
	var sum [sha256.Size]byte
	copy(sum[:], h.Sum(nil))
	return sum
}

// get returns the cached client for key, building a new one when there is none
// or the config changed. A replaced client has its idle connections closed.
func (cc *clientCache) get(key types.NamespacedName, cfg warpgate.Config) *warpgate.Client {
	fp := configFingerprint(cfg)

	cc.mu.Lock()
	defer cc.mu.Unlock()

	if e, ok := cc.entries[key]; ok {
		if e.fingerprint == fp {
			return e.client
		}
		e.client.CloseIdleConnections()
	}
	c := warpgate.NewClient(cfg)
	cc.entries[key] = clientCacheEntry{fingerprint: fp, client: c}
	return c
}

// evict drops the client for key and closes its idle connections.
func (cc *clientCache) evict(key types.NamespacedName) {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	if e, ok := cc.entries[key]; ok {
		e.client.CloseIdleConnections()
		delete(cc.entries, key)
	}
}

func (cc *clientCache) has(key types.NamespacedName) bool {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	_, ok := cc.entries[key]
	return ok
}

// getWarpgateClient returns a Warpgate API client for the named WarpgateConnection,
// bound to ctx. The underlying client is cached per connection and reused across reconciles.
func getWarpgateClient(ctx context.Context, r client.Reader, namespace, connectionName string) (*warpgate.Client, error) {
	var conn warpgatev1alpha1.WarpgateConnection
	if err := r.Get(ctx, types.NamespacedName{Name: connectionName, Namespace: namespace}, &conn); err != nil {
		return nil, fmt.Errorf("getting WarpgateConnection %q: %w", connectionName, err)
	}
	return clientForConnection(ctx, r, &conn)
}

// clientForConnection resolves conn's credentials and CA and returns the cached
// client for it, bound to ctx.
func clientForConnection(ctx context.Context, r client.Reader, conn *warpgatev1alpha1.WarpgateConnection) (*warpgate.Client, error) {
	cfg, err := connectionConfig(ctx, r, conn)
	if err != nil {
		return nil, err
	}
	key := types.NamespacedName{Namespace: conn.Namespace, Name: conn.Name}
	return warpgateClients.get(key, cfg).WithContext(ctx), nil
}

// connectionConfig builds the warpgate.Config for conn from its auth Secret and
// optional CA Secret. It prefers bearer token auth when the token key exists in
// the Secret, falling back to username/password otherwise.
func connectionConfig(ctx context.Context, r client.Reader, conn *warpgatev1alpha1.WarpgateConnection) (warpgate.Config, error) {
	var secret corev1.Secret
	if err := r.Get(ctx, types.NamespacedName{
		Name:      conn.Spec.AuthSecretRef.Name,
		Namespace: conn.Namespace,
	}, &secret); err != nil {
		return warpgate.Config{}, fmt.Errorf("getting auth secret %q: %w", conn.Spec.AuthSecretRef.Name, err)
	}

	caCert, err := loadConnectionCA(ctx, r, conn)
	if err != nil {
		return warpgate.Config{}, err
	}

	cfg := warpgate.Config{
		Host:               conn.Spec.Host,
		InsecureSkipVerify: conn.Spec.InsecureSkipVerify,
		CACert:             caCert,
	}

	tokenKey := conn.Spec.AuthSecretRef.TokenKey
	if tokenKey == "" {
		tokenKey = "token"
	}

	// Prefer token-based auth if the token key exists in the Secret.
	if token, ok := secret.Data[tokenKey]; ok && len(token) > 0 {
		cfg.Token = string(token)
		return cfg, nil
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
		return warpgate.Config{}, fmt.Errorf("key %q not found in auth secret %q", usernameKey, conn.Spec.AuthSecretRef.Name)
	}

	password, ok := secret.Data[passwordKey]
	if !ok {
		return warpgate.Config{}, fmt.Errorf("key %q not found in auth secret %q", passwordKey, conn.Spec.AuthSecretRef.Name)
	}

	cfg.Username = string(username)
	cfg.Password = string(password)
	return cfg, nil
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
