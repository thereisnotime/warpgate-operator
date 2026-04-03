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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	warpgatev1alpha1 "github.com/thereisnotime/warpgate-operator/api/v1alpha1"
)

var _ = Describe("getWarpgateClient helper", func() {

	const helperNS = "default"

	Context("Happy path", func() {
		var (
			connName   = "helper-conn-ok"
			secretName = "helper-secret-ok"
		)

		BeforeEach(func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: helperNS},
				Data:       map[string][]byte{"username": []byte("admin"), "password": []byte("pass")},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			conn := &warpgatev1alpha1.WarpgateConnection{
				ObjectMeta: metav1.ObjectMeta{Name: connName, Namespace: helperNS},
				Spec: warpgatev1alpha1.WarpgateConnectionSpec{
					Host:                 "https://warpgate.example.com",
					CredentialsSecretRef: warpgatev1alpha1.CredentialsSecretRef{Name: secretName},
					InsecureSkipVerify:   false,
				},
			}
			Expect(k8sClient.Create(ctx, conn)).To(Succeed())
		})

		AfterEach(func() {
			conn := &warpgatev1alpha1.WarpgateConnection{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: connName, Namespace: helperNS}, conn); err == nil {
				_ = k8sClient.Delete(ctx, conn)
			}
			secret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: helperNS}, secret); err == nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should return a non-nil client when the connection and secret exist", func() {
			client, err := getWarpgateClient(ctx, k8sClient, helperNS, connName)
			Expect(err).NotTo(HaveOccurred())
			Expect(client).NotTo(BeNil())
		})
	})

	Context("Missing connection", func() {
		It("should return an error when the WarpgateConnection does not exist", func() {
			client, err := getWarpgateClient(ctx, k8sClient, helperNS, "nonexistent-connection")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("nonexistent-connection"))
			Expect(client).To(BeNil())
		})
	})

	Context("Missing secret", func() {
		var connName = "helper-conn-nosecret"

		BeforeEach(func() {
			conn := &warpgatev1alpha1.WarpgateConnection{
				ObjectMeta: metav1.ObjectMeta{Name: connName, Namespace: helperNS},
				Spec: warpgatev1alpha1.WarpgateConnectionSpec{
					Host:                 "https://warpgate.example.com",
					CredentialsSecretRef: warpgatev1alpha1.CredentialsSecretRef{Name: "ghost-secret"},
				},
			}
			Expect(k8sClient.Create(ctx, conn)).To(Succeed())
		})

		AfterEach(func() {
			conn := &warpgatev1alpha1.WarpgateConnection{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: connName, Namespace: helperNS}, conn); err == nil {
				_ = k8sClient.Delete(ctx, conn)
			}
		})

		It("should return an error when the referenced secret does not exist", func() {
			client, err := getWarpgateClient(ctx, k8sClient, helperNS, connName)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("ghost-secret"))
			Expect(client).To(BeNil())
		})
	})

	Context("Missing username key in secret", func() {
		var (
			connName   = "helper-conn-badkey"
			secretName = "helper-secret-badkey"
		)

		BeforeEach(func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: helperNS},
				Data:       map[string][]byte{"password": []byte("pass")},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			conn := &warpgatev1alpha1.WarpgateConnection{
				ObjectMeta: metav1.ObjectMeta{Name: connName, Namespace: helperNS},
				Spec: warpgatev1alpha1.WarpgateConnectionSpec{
					Host:                 "https://warpgate.example.com",
					CredentialsSecretRef: warpgatev1alpha1.CredentialsSecretRef{Name: secretName},
				},
			}
			Expect(k8sClient.Create(ctx, conn)).To(Succeed())
		})

		AfterEach(func() {
			conn := &warpgatev1alpha1.WarpgateConnection{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: connName, Namespace: helperNS}, conn); err == nil {
				_ = k8sClient.Delete(ctx, conn)
			}
			secret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: helperNS}, secret); err == nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should return an error when the username key is missing from the secret", func() {
			client, err := getWarpgateClient(ctx, k8sClient, helperNS, connName)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(`key "username" not found`))
			Expect(client).To(BeNil())
		})
	})

	Context("Missing password key in secret", func() {
		var (
			connName   = "helper-conn-nopass"
			secretName = "helper-secret-nopass"
		)

		BeforeEach(func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: helperNS},
				Data:       map[string][]byte{"username": []byte("admin")},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			conn := &warpgatev1alpha1.WarpgateConnection{
				ObjectMeta: metav1.ObjectMeta{Name: connName, Namespace: helperNS},
				Spec: warpgatev1alpha1.WarpgateConnectionSpec{
					Host:                 "https://warpgate.example.com",
					CredentialsSecretRef: warpgatev1alpha1.CredentialsSecretRef{Name: secretName},
				},
			}
			Expect(k8sClient.Create(ctx, conn)).To(Succeed())
		})

		AfterEach(func() {
			conn := &warpgatev1alpha1.WarpgateConnection{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: connName, Namespace: helperNS}, conn); err == nil {
				_ = k8sClient.Delete(ctx, conn)
			}
			secret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: helperNS}, secret); err == nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should return an error when the password key is missing from the secret", func() {
			client, err := getWarpgateClient(ctx, k8sClient, helperNS, connName)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(`key "password" not found`))
			Expect(client).To(BeNil())
		})
	})

	Context("Custom UsernameKey", func() {
		var (
			connName   = "helper-conn-custom-ukey"
			secretName = "helper-secret-custom-ukey"
		)

		BeforeEach(func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: helperNS},
				Data:       map[string][]byte{"my-user": []byte("admin"), "password": []byte("pass")},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			conn := &warpgatev1alpha1.WarpgateConnection{
				ObjectMeta: metav1.ObjectMeta{Name: connName, Namespace: helperNS},
				Spec: warpgatev1alpha1.WarpgateConnectionSpec{
					Host: "https://warpgate.example.com",
					CredentialsSecretRef: warpgatev1alpha1.CredentialsSecretRef{
						Name:        secretName,
						UsernameKey: "my-user",
					},
				},
			}
			Expect(k8sClient.Create(ctx, conn)).To(Succeed())
		})

		AfterEach(func() {
			conn := &warpgatev1alpha1.WarpgateConnection{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: connName, Namespace: helperNS}, conn); err == nil {
				_ = k8sClient.Delete(ctx, conn)
			}
			secret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: helperNS}, secret); err == nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should use the custom UsernameKey to look up the username", func() {
			client, err := getWarpgateClient(ctx, k8sClient, helperNS, connName)
			Expect(err).NotTo(HaveOccurred())
			Expect(client).NotTo(BeNil())
		})
	})

	Context("Custom PasswordKey", func() {
		var (
			connName   = "helper-conn-custom-pkey"
			secretName = "helper-secret-custom-pkey"
		)

		BeforeEach(func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: helperNS},
				Data:       map[string][]byte{"username": []byte("admin"), "my-pass": []byte("secret")},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			conn := &warpgatev1alpha1.WarpgateConnection{
				ObjectMeta: metav1.ObjectMeta{Name: connName, Namespace: helperNS},
				Spec: warpgatev1alpha1.WarpgateConnectionSpec{
					Host: "https://warpgate.example.com",
					CredentialsSecretRef: warpgatev1alpha1.CredentialsSecretRef{
						Name:        secretName,
						PasswordKey: "my-pass",
					},
				},
			}
			Expect(k8sClient.Create(ctx, conn)).To(Succeed())
		})

		AfterEach(func() {
			conn := &warpgatev1alpha1.WarpgateConnection{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: connName, Namespace: helperNS}, conn); err == nil {
				_ = k8sClient.Delete(ctx, conn)
			}
			secret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: helperNS}, secret); err == nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should use the custom PasswordKey to look up the password", func() {
			client, err := getWarpgateClient(ctx, k8sClient, helperNS, connName)
			Expect(err).NotTo(HaveOccurred())
			Expect(client).NotTo(BeNil())
		})
	})

	Context("Custom UsernameKey and PasswordKey together", func() {
		var (
			connName   = "helper-conn-custom-both"
			secretName = "helper-secret-custom-both"
		)

		BeforeEach(func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: helperNS},
				Data:       map[string][]byte{"wg-user": []byte("admin"), "wg-pass": []byte("secret")},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			conn := &warpgatev1alpha1.WarpgateConnection{
				ObjectMeta: metav1.ObjectMeta{Name: connName, Namespace: helperNS},
				Spec: warpgatev1alpha1.WarpgateConnectionSpec{
					Host: "https://warpgate.example.com",
					CredentialsSecretRef: warpgatev1alpha1.CredentialsSecretRef{
						Name:        secretName,
						UsernameKey: "wg-user",
						PasswordKey: "wg-pass",
					},
				},
			}
			Expect(k8sClient.Create(ctx, conn)).To(Succeed())
		})

		AfterEach(func() {
			conn := &warpgatev1alpha1.WarpgateConnection{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: connName, Namespace: helperNS}, conn); err == nil {
				_ = k8sClient.Delete(ctx, conn)
			}
			secret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: helperNS}, secret); err == nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should use both custom keys successfully", func() {
			client, err := getWarpgateClient(ctx, k8sClient, helperNS, connName)
			Expect(err).NotTo(HaveOccurred())
			Expect(client).NotTo(BeNil())
		})
	})

	Context("InsecureSkipVerify flag", func() {
		var (
			connName   = "helper-conn-insecure"
			secretName = "helper-secret-insecure"
		)

		BeforeEach(func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: helperNS},
				Data:       map[string][]byte{"username": []byte("admin"), "password": []byte("pass")},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			conn := &warpgatev1alpha1.WarpgateConnection{
				ObjectMeta: metav1.ObjectMeta{Name: connName, Namespace: helperNS},
				Spec: warpgatev1alpha1.WarpgateConnectionSpec{
					Host:                 "https://warpgate.example.com",
					CredentialsSecretRef: warpgatev1alpha1.CredentialsSecretRef{Name: secretName},
					InsecureSkipVerify:   true,
				},
			}
			Expect(k8sClient.Create(ctx, conn)).To(Succeed())
		})

		AfterEach(func() {
			conn := &warpgatev1alpha1.WarpgateConnection{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: connName, Namespace: helperNS}, conn); err == nil {
				_ = k8sClient.Delete(ctx, conn)
			}
			secret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: helperNS}, secret); err == nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should create a client successfully when insecureSkipVerify is true", func() {
			client, err := getWarpgateClient(ctx, k8sClient, helperNS, connName)
			Expect(err).NotTo(HaveOccurred())
			Expect(client).NotTo(BeNil())
		})
	})
})
