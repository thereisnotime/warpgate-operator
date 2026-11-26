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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	warpgatev1alpha1 "github.com/thereisnotime/warpgate-operator/api/v1alpha1"
)

var _ = Describe("cert-manager TLS", func() {
	boolPtr := func(b bool) *bool { return &b }

	const (
		instName   = "cmtls-inst"
		pwSecret   = "cmtls-inst-pw"
		tlsSecret  = instName + "-tls" // the Secret the Certificate writes to
		connName   = instName + "-connection"
		authSecret = instName + "-admin-auth"
	)

	var (
		reconciler *WarpgateInstanceReconciler
		nn         = types.NamespacedName{Name: instName, Namespace: testNamespace}
	)

	BeforeEach(func() {
		reconciler = &WarpgateInstanceReconciler{Client: k8sClient, Scheme: scheme.Scheme}
		Expect(k8sClient.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: pwSecret, Namespace: testNamespace},
			Data:       map[string][]byte{"password": []byte("pw")},
		})).To(Succeed())
	})

	AfterEach(func() {
		cert := &unstructured.Unstructured{}
		cert.SetGroupVersionKind(certificateGVK)
		cert.SetName(tlsSecret)
		cert.SetNamespace(testNamespace)
		issuer := &unstructured.Unstructured{}
		issuer.SetGroupVersionKind(issuerGVK)
		issuer.SetName(instName + "-selfsigned")
		issuer.SetNamespace(testNamespace)

		deleteIgnoringFinalizers(
			&warpgatev1alpha1.WarpgateConnection{ObjectMeta: metav1.ObjectMeta{Name: connName, Namespace: testNamespace}},
			&warpgatev1alpha1.WarpgateInstance{ObjectMeta: metav1.ObjectMeta{Name: instName, Namespace: testNamespace}},
			&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: instName, Namespace: testNamespace}},
			&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: pwSecret, Namespace: testNamespace}},
			&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: tlsSecret, Namespace: testNamespace}},
			&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: authSecret, Namespace: testNamespace}},
			cert, issuer,
		)
	})

	newInstance := func(tls *warpgatev1alpha1.InstanceTLSSpec) *warpgatev1alpha1.WarpgateInstance {
		return &warpgatev1alpha1.WarpgateInstance{
			ObjectMeta: metav1.ObjectMeta{Name: instName, Namespace: testNamespace},
			Spec: warpgatev1alpha1.WarpgateInstanceSpec{
				Version:                "0.21.1",
				AdminPasswordSecretRef: warpgatev1alpha1.SecretKeyRef{Name: pwSecret},
				TLS:                    tls,
			},
		}
	}

	issueSecret := func(data map[string][]byte) {
		Expect(k8sClient.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: tlsSecret, Namespace: testNamespace},
			Data:       data,
		})).To(Succeed())
	}

	issued := map[string][]byte{
		"ca.crt":  []byte("ca"),
		"tls.crt": []byte("crt"),
		"tls.key": []byte("key"),
	}

	readyReason := func() string {
		var updated warpgatev1alpha1.WarpgateInstance
		Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())
		cond := findReadyCondition(updated.Status.Conditions)
		Expect(cond).NotTo(BeNil())
		return cond.Reason
	}

	getDeployment := func() *appsv1.Deployment {
		var deploy appsv1.Deployment
		Expect(k8sClient.Get(ctx, nn, &deploy)).To(Succeed())
		return &deploy
	}

	It("waits for cert-manager to issue the Secret before creating the Deployment", func() {
		Expect(k8sClient.Create(ctx, newInstance(&warpgatev1alpha1.InstanceTLSSpec{CertManager: boolPtr(true)}))).To(Succeed())

		res, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())
		Expect(res.RequeueAfter).To(Equal(certificatePendingRequeue))
		Expect(readyReason()).To(Equal("CertificatePending"))

		// The Certificate is requested, but nothing that mounts its Secret exists yet.
		cert := &unstructured.Unstructured{}
		cert.SetGroupVersionKind(certificateGVK)
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: tlsSecret, Namespace: testNamespace}, cert)).To(Succeed())
		secretName, _, _ := unstructured.NestedString(cert.Object, "spec", "secretName")
		Expect(secretName).To(Equal(tlsSecret))

		var deploy appsv1.Deployment
		Expect(k8sClient.Get(ctx, nn, &deploy)).NotTo(Succeed())
		var conn warpgatev1alpha1.WarpgateConnection
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: connName, Namespace: testNamespace}, &conn)).NotTo(Succeed())
	})

	It("keeps waiting while the Secret has no key pair yet", func() {
		issueSecret(map[string][]byte{"ca.crt": []byte("ca")})
		Expect(k8sClient.Create(ctx, newInstance(&warpgatev1alpha1.InstanceTLSSpec{CertManager: boolPtr(true)}))).To(Succeed())

		res, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())
		Expect(res.RequeueAfter).To(Equal(certificatePendingRequeue))
		Expect(readyReason()).To(Equal("CertificatePending"))
	})

	It("mounts the cert-manager Secret and verifies the connection against its CA", func() {
		issueSecret(issued)
		Expect(k8sClient.Create(ctx, newInstance(&warpgatev1alpha1.InstanceTLSSpec{CertManager: boolPtr(true)}))).To(Succeed())

		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())
		Expect(readyReason()).To(Equal("Unavailable")) // no real pods in envtest

		deploy := getDeployment()
		var tlsVol *corev1.Volume
		for i := range deploy.Spec.Template.Spec.Volumes {
			if deploy.Spec.Template.Spec.Volumes[i].Name == "tls-secret" {
				tlsVol = &deploy.Spec.Template.Spec.Volumes[i]
			}
		}
		Expect(tlsVol).NotTo(BeNil())
		Expect(tlsVol.Secret).NotTo(BeNil())
		Expect(tlsVol.Secret.SecretName).To(Equal(tlsSecret))
		initC := deploy.Spec.Template.Spec.InitContainers[0]
		Expect(initC.VolumeMounts).To(ContainElement(corev1.VolumeMount{
			Name: "tls-secret", MountPath: "/tls-secret", ReadOnly: true,
		}))
		script := initC.Command[2]
		Expect(script).To(ContainSubstring("cp /tls-secret/tls.crt /data/tls.certificate.pem"))
		Expect(script).To(ContainSubstring("cp /tls-secret/tls.key /data/tls.key.pem"))
		Expect(script).NotTo(ContainSubstring("openssl req"))
		Expect(deploy.Spec.Template.Annotations).To(HaveKey(tlsHashAnnotation))

		var conn warpgatev1alpha1.WarpgateConnection
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: connName, Namespace: testNamespace}, &conn)).To(Succeed())
		Expect(conn.Spec.InsecureSkipVerify).To(BeFalse())
		Expect(conn.Spec.CASecretRef).To(Equal(&warpgatev1alpha1.SecretKeyRef{Name: tlsSecret, Key: "ca.crt"}))
	})

	It("rolls the pods when cert-manager renews the certificate", func() {
		issueSecret(issued)
		Expect(k8sClient.Create(ctx, newInstance(&warpgatev1alpha1.InstanceTLSSpec{CertManager: boolPtr(true)}))).To(Succeed())
		_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())
		before := getDeployment().Spec.Template.Annotations[tlsHashAnnotation]

		var secret corev1.Secret
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: tlsSecret, Namespace: testNamespace}, &secret)).To(Succeed())
		secret.Data["tls.crt"] = []byte("renewed")
		Expect(k8sClient.Update(ctx, &secret)).To(Succeed())

		_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())
		Expect(getDeployment().Spec.Template.Annotations[tlsHashAnnotation]).NotTo(Equal(before))
	})

	It("falls back to tls.crt when the cert-manager Secret has no ca.crt", func() {
		issueSecret(map[string][]byte{"tls.crt": []byte("crt"), "tls.key": []byte("key")})
		insecure, caRef, err := reconciler.connectionTLS(ctx, newInstance(&warpgatev1alpha1.InstanceTLSSpec{CertManager: boolPtr(true)}))
		Expect(err).NotTo(HaveOccurred())
		Expect(insecure).To(BeFalse())
		Expect(caRef).To(Equal(&warpgatev1alpha1.SecretKeyRef{Name: tlsSecret, Key: "tls.crt"}))
	})

	It("prefers tls.secretName over the cert-manager Secret", func() {
		inst := newInstance(&warpgatev1alpha1.InstanceTLSSpec{CertManager: boolPtr(true), SecretName: "user-tls"})
		Expect(servedTLSSecret(inst)).To(Equal("user-tls"))
		Expect(usesCertManagerSecret(inst)).To(BeFalse())

		deploy := reconciler.buildDeployment(inst)
		var names []string
		for _, v := range deploy.Spec.Template.Spec.Volumes {
			if v.Secret != nil {
				names = append(names, v.Secret.SecretName)
			}
		}
		Expect(names).To(ContainElement("user-tls"))
		Expect(names).NotTo(ContainElement(tlsSecret))
	})

	It("generates a self-signed certificate only without any TLS Secret", func() {
		inst := newInstance(&warpgatev1alpha1.InstanceTLSSpec{CertManager: boolPtr(false)})
		Expect(servedTLSSecret(inst)).To(BeEmpty())
		script := reconciler.buildDeployment(inst).Spec.Template.Spec.InitContainers[0].Command[2]
		Expect(script).To(ContainSubstring("openssl req"))
		Expect(script).NotTo(ContainSubstring("/tls-secret"))
	})

	It("maps the served TLS Secret back to its instance", func() {
		Expect(k8sClient.Create(ctx, newInstance(&warpgatev1alpha1.InstanceTLSSpec{CertManager: boolPtr(true)}))).To(Succeed())

		reqs := reconciler.instancesForTLSSecret(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: tlsSecret, Namespace: testNamespace},
		})
		Expect(reqs).To(ConsistOf(reconcile.Request{NamespacedName: nn}))

		Expect(reconciler.instancesForTLSSecret(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "unrelated", Namespace: testNamespace},
		})).To(BeEmpty())
	})

	It("does not report a missing CA when verifyConnection is true and cert-manager is on", func() {
		issueSecret(issued)
		insecure, caRef, err := reconciler.connectionTLS(ctx, newInstance(&warpgatev1alpha1.InstanceTLSSpec{
			CertManager: boolPtr(true), VerifyConnection: boolPtr(true),
		}))
		Expect(err).NotTo(HaveOccurred())
		Expect(insecure).To(BeFalse())
		Expect(caRef).NotTo(BeNil())
	})
})
