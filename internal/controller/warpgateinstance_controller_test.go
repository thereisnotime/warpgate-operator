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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	warpgatev1alpha1 "github.com/thereisnotime/warpgate-operator/api/v1alpha1"
)

var _ = Describe("WarpgateInstance Controller", func() {

	var (
		reconciler *WarpgateInstanceReconciler
	)

	BeforeEach(func() {
		reconciler = &WarpgateInstanceReconciler{
			Client: k8sClient,
			Scheme: scheme.Scheme,
		}
	})

	// helpers for pointer values
	boolPtr := func(b bool) *bool { return &b }
	int32Ptr := func(i int32) *int32 { return &i }

	Context("Create instance with defaults", func() {
		const (
			instName   = "inst-defaults"
			secretName = "inst-defaults-admin-pw"
		)

		BeforeEach(func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: testNamespace,
				},
				Data: map[string][]byte{
					"password": []byte("super-secret"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			inst := &warpgatev1alpha1.WarpgateInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instName,
					Namespace: testNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					Version: "0.21.1",
					AdminPasswordSecretRef: warpgatev1alpha1.SecretKeyRef{
						Name: secretName,
					},
					Replicas: int32Ptr(1),
					HTTP: &warpgatev1alpha1.HTTPListenerSpec{
						Enabled:     boolPtr(true),
						Port:        int32Ptr(8888),
						ServiceType: "ClusterIP",
					},
					Storage: &warpgatev1alpha1.StorageSpec{
						Size: "1Gi",
					},
					TLS: &warpgatev1alpha1.InstanceTLSSpec{
						CertManager: boolPtr(false),
					},
					CreateConnection: boolPtr(false),
				},
			}
			Expect(k8sClient.Create(ctx, inst)).To(Succeed())
		})

		AfterEach(func() {
			inst := &warpgatev1alpha1.WarpgateInstance{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: instName, Namespace: testNamespace}, inst); err == nil {
				controllerutil.RemoveFinalizer(inst, instanceFinalizer)
				_ = k8sClient.Update(ctx, inst)
				_ = k8sClient.Delete(ctx, inst)
			}
			secret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: testNamespace}, secret); err == nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should create ConfigMap, Deployment, HTTP Service, and set status", func() {
			nn := types.NamespacedName{Name: instName, Namespace: testNamespace}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// Verify ConfigMap is created with warpgate.yaml.
			var cm corev1.ConfigMap
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: instName + "-config", Namespace: testNamespace}, &cm)).To(Succeed())
			Expect(cm.Data).To(HaveKey("warpgate.yaml"))
			Expect(cm.Data["warpgate.yaml"]).To(ContainSubstring("http:"))
			Expect(cm.Data["warpgate.yaml"]).To(ContainSubstring("enable: true"))
			Expect(cm.Data["warpgate.yaml"]).To(ContainSubstring("8888"))

			// Verify Deployment is created with the right image and ports.
			var sts appsv1.Deployment
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: instName, Namespace: testNamespace}, &sts)).To(Succeed())
			Expect(sts.Spec.Template.Spec.Containers).To(HaveLen(1))
			Expect(sts.Spec.Template.Spec.Containers[0].Image).To(Equal("ghcr.io/warp-tech/warpgate:v0.21.1"))
			Expect(sts.Spec.Template.Spec.Containers[0].Ports).To(ContainElement(
				corev1.ContainerPort{Name: "http", ContainerPort: 8888, Protocol: corev1.ProtocolTCP},
			))

			// Verify volume mounts include data (config is copied by init container).
			mounts := sts.Spec.Template.Spec.Containers[0].VolumeMounts
			Expect(mounts).To(ContainElement(corev1.VolumeMount{
				Name: "data", MountPath: "/data",
			}))

			// Verify HTTP Service is created.
			var svc corev1.Service
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: instName + "-http", Namespace: testNamespace}, &svc)).To(Succeed())
			Expect(svc.Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))
			Expect(svc.Spec.Ports).To(HaveLen(1))
			Expect(svc.Spec.Ports[0].Port).To(Equal(int32(8888)))

			// Verify status fields.
			var updated warpgatev1alpha1.WarpgateInstance
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())
			Expect(updated.Status.Version).To(Equal("0.21.1"))
			Expect(updated.Status.Endpoint).To(ContainSubstring(instName + "-http"))
			Expect(updated.Status.Endpoint).To(ContainSubstring("8888"))

			// Ready condition should be set (False because envtest doesn't run real pods).
			readyCond := findReadyCondition(updated.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("Unavailable"))
		})
	})

	Context("Create instance with SSH enabled", func() {
		const (
			instName   = "inst-ssh"
			secretName = "inst-ssh-admin-pw"
		)

		BeforeEach(func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: testNamespace,
				},
				Data: map[string][]byte{
					"password": []byte("ssh-secret"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			inst := &warpgatev1alpha1.WarpgateInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instName,
					Namespace: testNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					Version: "0.21.1",
					AdminPasswordSecretRef: warpgatev1alpha1.SecretKeyRef{
						Name: secretName,
					},
					Replicas: int32Ptr(1),
					HTTP: &warpgatev1alpha1.HTTPListenerSpec{
						Enabled:     boolPtr(true),
						Port:        int32Ptr(8888),
						ServiceType: "ClusterIP",
					},
					SSH: &warpgatev1alpha1.SSHListenerSpec{
						Enabled:     boolPtr(true),
						Port:        int32Ptr(2222),
						ServiceType: "ClusterIP",
					},
					Storage: &warpgatev1alpha1.StorageSpec{
						Size: "1Gi",
					},
					CreateConnection: boolPtr(false),
				},
			}
			Expect(k8sClient.Create(ctx, inst)).To(Succeed())
		})

		AfterEach(func() {
			inst := &warpgatev1alpha1.WarpgateInstance{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: instName, Namespace: testNamespace}, inst); err == nil {
				controllerutil.RemoveFinalizer(inst, instanceFinalizer)
				_ = k8sClient.Update(ctx, inst)
				_ = k8sClient.Delete(ctx, inst)
			}
			secret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: testNamespace}, secret); err == nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should create both HTTP and SSH services", func() {
			nn := types.NamespacedName{Name: instName, Namespace: testNamespace}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// HTTP Service should exist.
			var httpSvc corev1.Service
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: instName + "-http", Namespace: testNamespace}, &httpSvc)).To(Succeed())
			Expect(httpSvc.Spec.Ports[0].Port).To(Equal(int32(8888)))

			// SSH Service should exist.
			var sshSvc corev1.Service
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: instName + "-ssh", Namespace: testNamespace}, &sshSvc)).To(Succeed())
			Expect(sshSvc.Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))
			Expect(sshSvc.Spec.Ports).To(HaveLen(1))
			Expect(sshSvc.Spec.Ports[0].Port).To(Equal(int32(2222)))

			// Deployment should have both ports.
			var sts appsv1.Deployment
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: instName, Namespace: testNamespace}, &sts)).To(Succeed())
			ports := sts.Spec.Template.Spec.Containers[0].Ports
			Expect(ports).To(ContainElement(
				corev1.ContainerPort{Name: "http", ContainerPort: 8888, Protocol: corev1.ProtocolTCP},
			))
			Expect(ports).To(ContainElement(
				corev1.ContainerPort{Name: "ssh", ContainerPort: 2222, Protocol: corev1.ProtocolTCP},
			))
		})
	})

	Context("Create instance with createConnection=true", func() {
		const (
			instName   = "inst-conn"
			secretName = "inst-conn-admin-pw"
		)

		BeforeEach(func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: testNamespace,
				},
				Data: map[string][]byte{
					"password": []byte("conn-secret"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			inst := &warpgatev1alpha1.WarpgateInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instName,
					Namespace: testNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					Version: "0.21.1",
					AdminPasswordSecretRef: warpgatev1alpha1.SecretKeyRef{
						Name: secretName,
					},
					Replicas: int32Ptr(1),
					HTTP: &warpgatev1alpha1.HTTPListenerSpec{
						Enabled:     boolPtr(true),
						Port:        int32Ptr(8888),
						ServiceType: "ClusterIP",
					},
					Storage: &warpgatev1alpha1.StorageSpec{
						Size: "1Gi",
					},
					CreateConnection: boolPtr(true),
				},
			}
			Expect(k8sClient.Create(ctx, inst)).To(Succeed())
		})

		AfterEach(func() {
			// Clean up WarpgateConnection first (owned by the instance).
			conn := &warpgatev1alpha1.WarpgateConnection{}
			connNN := types.NamespacedName{Name: instName + "-connection", Namespace: testNamespace}
			if err := k8sClient.Get(ctx, connNN, conn); err == nil {
				controllerutil.RemoveFinalizer(conn, warpgateFinalizer)
				_ = k8sClient.Update(ctx, conn)
				_ = k8sClient.Delete(ctx, conn)
			}

			inst := &warpgatev1alpha1.WarpgateInstance{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: instName, Namespace: testNamespace}, inst); err == nil {
				controllerutil.RemoveFinalizer(inst, instanceFinalizer)
				_ = k8sClient.Update(ctx, inst)
				_ = k8sClient.Delete(ctx, inst)
			}

			// Clean up auth secret created by the controller.
			authSecret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: instName + "-admin-auth", Namespace: testNamespace}, authSecret); err == nil {
				_ = k8sClient.Delete(ctx, authSecret)
			}

			secret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: testNamespace}, secret); err == nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should create a WarpgateConnection CR pointing to the HTTP service", func() {
			nn := types.NamespacedName{Name: instName, Namespace: testNamespace}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// WarpgateConnection should exist.
			var conn warpgatev1alpha1.WarpgateConnection
			connNN := types.NamespacedName{Name: instName + "-connection", Namespace: testNamespace}
			Expect(k8sClient.Get(ctx, connNN, &conn)).To(Succeed())

			// Host should point to the HTTP service.
			Expect(conn.Spec.Host).To(ContainSubstring(instName + "-http"))
			Expect(conn.Spec.Host).To(ContainSubstring("8888"))

			// Auth secret should reference the auto-created auth secret.
			Expect(conn.Spec.AuthSecretRef.Name).To(Equal(instName + "-admin-auth"))

			// The auto-created auth secret should have the admin credentials.
			var authSecret corev1.Secret
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: instName + "-admin-auth", Namespace: testNamespace}, &authSecret)).To(Succeed())
			Expect(authSecret.Data["username"]).To(Equal([]byte("admin")))
			Expect(authSecret.Data["password"]).To(Equal([]byte("conn-secret")))

			// Status should have connectionRef set.
			var updated warpgatev1alpha1.WarpgateInstance
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())
			Expect(updated.Status.ConnectionRef).To(Equal(instName + "-connection"))
		})
	})

	Context("Update instance version", func() {
		const (
			instName   = "inst-update"
			secretName = "inst-update-admin-pw"
		)

		BeforeEach(func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: testNamespace,
				},
				Data: map[string][]byte{
					"password": []byte("update-secret"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			inst := &warpgatev1alpha1.WarpgateInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instName,
					Namespace: testNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					Version: "0.20.0",
					AdminPasswordSecretRef: warpgatev1alpha1.SecretKeyRef{
						Name: secretName,
					},
					Replicas: int32Ptr(1),
					HTTP: &warpgatev1alpha1.HTTPListenerSpec{
						Enabled:     boolPtr(true),
						Port:        int32Ptr(8888),
						ServiceType: "ClusterIP",
					},
					Storage: &warpgatev1alpha1.StorageSpec{
						Size: "1Gi",
					},
					CreateConnection: boolPtr(false),
				},
			}
			Expect(k8sClient.Create(ctx, inst)).To(Succeed())
		})

		AfterEach(func() {
			inst := &warpgatev1alpha1.WarpgateInstance{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: instName, Namespace: testNamespace}, inst); err == nil {
				controllerutil.RemoveFinalizer(inst, instanceFinalizer)
				_ = k8sClient.Update(ctx, inst)
				_ = k8sClient.Delete(ctx, inst)
			}
			secret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: testNamespace}, secret); err == nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should update the Deployment image when version changes", func() {
			nn := types.NamespacedName{Name: instName, Namespace: testNamespace}

			// First reconcile creates all resources.
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// Verify initial image.
			var sts appsv1.Deployment
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: instName, Namespace: testNamespace}, &sts)).To(Succeed())
			Expect(sts.Spec.Template.Spec.Containers[0].Image).To(Equal("ghcr.io/warp-tech/warpgate:v0.20.0"))

			// Update the version.
			var inst warpgatev1alpha1.WarpgateInstance
			Expect(k8sClient.Get(ctx, nn, &inst)).To(Succeed())
			inst.Spec.Version = "0.21.1"
			Expect(k8sClient.Update(ctx, &inst)).To(Succeed())

			// Second reconcile should update the Deployment.
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// Verify updated image.
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: instName, Namespace: testNamespace}, &sts)).To(Succeed())
			Expect(sts.Spec.Template.Spec.Containers[0].Image).To(Equal("ghcr.io/warp-tech/warpgate:v0.21.1"))

			// Status should reflect the new version.
			var updated warpgatev1alpha1.WarpgateInstance
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())
			Expect(updated.Status.Version).To(Equal("0.21.1"))
		})
	})

	Context("Delete instance", func() {
		const (
			instName   = "inst-delete"
			secretName = "inst-delete-admin-pw"
		)

		BeforeEach(func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: testNamespace,
				},
				Data: map[string][]byte{
					"password": []byte("delete-secret"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			inst := &warpgatev1alpha1.WarpgateInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instName,
					Namespace: testNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					Version: "0.21.1",
					AdminPasswordSecretRef: warpgatev1alpha1.SecretKeyRef{
						Name: secretName,
					},
					Replicas: int32Ptr(1),
					HTTP: &warpgatev1alpha1.HTTPListenerSpec{
						Enabled:     boolPtr(true),
						Port:        int32Ptr(8888),
						ServiceType: "ClusterIP",
					},
					Storage: &warpgatev1alpha1.StorageSpec{
						Size: "1Gi",
					},
					CreateConnection: boolPtr(false),
				},
			}
			Expect(k8sClient.Create(ctx, inst)).To(Succeed())
		})

		AfterEach(func() {
			secret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: testNamespace}, secret); err == nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should add a finalizer and remove it on deletion", func() {
			nn := types.NamespacedName{Name: instName, Namespace: testNamespace}

			// First reconcile should add the finalizer.
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			var inst warpgatev1alpha1.WarpgateInstance
			Expect(k8sClient.Get(ctx, nn, &inst)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(&inst, instanceFinalizer)).To(BeTrue())

			// Delete the resource (held by the finalizer).
			Expect(k8sClient.Delete(ctx, &inst)).To(Succeed())

			// Reconcile again to process the deletion.
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// Resource should be fully gone.
			var deleted warpgatev1alpha1.WarpgateInstance
			err = k8sClient.Get(ctx, nn, &deleted)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))
		})
	})

	Context("Resource not found", func() {
		It("should return no error when the resource doesn't exist", func() {
			nn := types.NamespacedName{
				Name:      "nonexistent-instance",
				Namespace: testNamespace,
			}

			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))
		})
	})

	Context("SSH disabled after being enabled", func() {
		const (
			instName   = "inst-ssh-disable"
			secretName = "inst-ssh-disable-pw"
		)

		BeforeEach(func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: testNamespace,
				},
				Data: map[string][]byte{
					"password": []byte("ssh-disable-secret"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			inst := &warpgatev1alpha1.WarpgateInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instName,
					Namespace: testNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					Version: "0.21.1",
					AdminPasswordSecretRef: warpgatev1alpha1.SecretKeyRef{
						Name: secretName,
					},
					Replicas: int32Ptr(1),
					HTTP: &warpgatev1alpha1.HTTPListenerSpec{
						Enabled:     boolPtr(true),
						Port:        int32Ptr(8888),
						ServiceType: "ClusterIP",
					},
					SSH: &warpgatev1alpha1.SSHListenerSpec{
						Enabled:     boolPtr(true),
						Port:        int32Ptr(2222),
						ServiceType: "ClusterIP",
					},
					Storage: &warpgatev1alpha1.StorageSpec{
						Size: "1Gi",
					},
					TLS: &warpgatev1alpha1.InstanceTLSSpec{
						CertManager: boolPtr(false),
					},
					CreateConnection: boolPtr(false),
				},
			}
			Expect(k8sClient.Create(ctx, inst)).To(Succeed())
		})

		AfterEach(func() {
			inst := &warpgatev1alpha1.WarpgateInstance{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: instName, Namespace: testNamespace}, inst); err == nil {
				controllerutil.RemoveFinalizer(inst, instanceFinalizer)
				_ = k8sClient.Update(ctx, inst)
				_ = k8sClient.Delete(ctx, inst)
			}
			secret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: testNamespace}, secret); err == nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should delete the SSH Service when SSH is disabled", func() {
			nn := types.NamespacedName{Name: instName, Namespace: testNamespace}

			// First reconcile: SSH is enabled, both services should exist.
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			var sshSvc corev1.Service
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: instName + "-ssh", Namespace: testNamespace}, &sshSvc)).To(Succeed())
			Expect(sshSvc.Spec.Ports[0].Port).To(Equal(int32(2222)))

			// Now disable SSH.
			var inst warpgatev1alpha1.WarpgateInstance
			Expect(k8sClient.Get(ctx, nn, &inst)).To(Succeed())
			inst.Spec.SSH.Enabled = boolPtr(false)
			Expect(k8sClient.Update(ctx, &inst)).To(Succeed())

			// Second reconcile: SSH service should be deleted.
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// SSH Service should be gone.
			err = k8sClient.Get(ctx, types.NamespacedName{Name: instName + "-ssh", Namespace: testNamespace}, &sshSvc)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))

			// HTTP Service should still exist.
			var httpSvc corev1.Service
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: instName + "-http", Namespace: testNamespace}, &httpSvc)).To(Succeed())
		})
	})

	Context("Connection already exists (update path)", func() {
		const (
			instName   = "inst-conn-exist"
			secretName = "inst-conn-exist-pw"
			connName   = "inst-conn-exist-connection"
		)

		BeforeEach(func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: testNamespace,
				},
				Data: map[string][]byte{
					"password": []byte("conn-exist-secret"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			// Pre-create the WarpgateConnection before the instance reconciles.
			conn := &warpgatev1alpha1.WarpgateConnection{
				ObjectMeta: metav1.ObjectMeta{
					Name:      connName,
					Namespace: testNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateConnectionSpec{
					Host:               "https://old-host:9999",
					AuthSecretRef:      warpgatev1alpha1.AuthSecretRef{Name: "old-auth-secret"},
					InsecureSkipVerify: false,
				},
			}
			Expect(k8sClient.Create(ctx, conn)).To(Succeed())

			inst := &warpgatev1alpha1.WarpgateInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instName,
					Namespace: testNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					Version: "0.21.1",
					AdminPasswordSecretRef: warpgatev1alpha1.SecretKeyRef{
						Name: secretName,
					},
					Replicas: int32Ptr(1),
					HTTP: &warpgatev1alpha1.HTTPListenerSpec{
						Enabled:     boolPtr(true),
						Port:        int32Ptr(8888),
						ServiceType: "ClusterIP",
					},
					Storage: &warpgatev1alpha1.StorageSpec{
						Size: "1Gi",
					},
					TLS: &warpgatev1alpha1.InstanceTLSSpec{
						CertManager: boolPtr(false),
					},
					CreateConnection: boolPtr(true),
				},
			}
			Expect(k8sClient.Create(ctx, inst)).To(Succeed())
		})

		AfterEach(func() {
			conn := &warpgatev1alpha1.WarpgateConnection{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: connName, Namespace: testNamespace}, conn); err == nil {
				controllerutil.RemoveFinalizer(conn, warpgateFinalizer)
				_ = k8sClient.Update(ctx, conn)
				_ = k8sClient.Delete(ctx, conn)
			}

			inst := &warpgatev1alpha1.WarpgateInstance{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: instName, Namespace: testNamespace}, inst); err == nil {
				controllerutil.RemoveFinalizer(inst, instanceFinalizer)
				_ = k8sClient.Update(ctx, inst)
				_ = k8sClient.Delete(ctx, inst)
			}

			authSecret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: instName + "-admin-auth", Namespace: testNamespace}, authSecret); err == nil {
				_ = k8sClient.Delete(ctx, authSecret)
			}

			secret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: testNamespace}, secret); err == nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should update the existing WarpgateConnection instead of erroring", func() {
			nn := types.NamespacedName{Name: instName, Namespace: testNamespace}

			// Reconcile should succeed even though the connection already exists.
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// The connection should be updated to point to the new host.
			var conn warpgatev1alpha1.WarpgateConnection
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: connName, Namespace: testNamespace}, &conn)).To(Succeed())
			Expect(conn.Spec.Host).To(ContainSubstring(instName + "-http"))
			Expect(conn.Spec.Host).To(ContainSubstring("8888"))
			Expect(conn.Spec.Host).NotTo(Equal("https://old-host:9999"))
			Expect(conn.Spec.AuthSecretRef.Name).To(Equal(instName + "-admin-auth"))
			Expect(conn.Spec.InsecureSkipVerify).To(BeTrue())

			// Status should reflect the connection.
			var updated warpgatev1alpha1.WarpgateInstance
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())
			Expect(updated.Status.ConnectionRef).To(Equal(connName))
		})
	})

	Context("Service update (ClusterIP to NodePort)", func() {
		const (
			instName   = "inst-svc-update"
			secretName = "inst-svc-update-pw"
		)

		BeforeEach(func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: testNamespace,
				},
				Data: map[string][]byte{
					"password": []byte("svc-update-secret"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			inst := &warpgatev1alpha1.WarpgateInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instName,
					Namespace: testNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					Version: "0.21.1",
					AdminPasswordSecretRef: warpgatev1alpha1.SecretKeyRef{
						Name: secretName,
					},
					Replicas: int32Ptr(1),
					HTTP: &warpgatev1alpha1.HTTPListenerSpec{
						Enabled:     boolPtr(true),
						Port:        int32Ptr(8888),
						ServiceType: "ClusterIP",
					},
					Storage: &warpgatev1alpha1.StorageSpec{
						Size: "1Gi",
					},
					TLS: &warpgatev1alpha1.InstanceTLSSpec{
						CertManager: boolPtr(false),
					},
					CreateConnection: boolPtr(false),
				},
			}
			Expect(k8sClient.Create(ctx, inst)).To(Succeed())
		})

		AfterEach(func() {
			inst := &warpgatev1alpha1.WarpgateInstance{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: instName, Namespace: testNamespace}, inst); err == nil {
				controllerutil.RemoveFinalizer(inst, instanceFinalizer)
				_ = k8sClient.Update(ctx, inst)
				_ = k8sClient.Delete(ctx, inst)
			}
			secret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: testNamespace}, secret); err == nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should update the Service type when spec changes", func() {
			nn := types.NamespacedName{Name: instName, Namespace: testNamespace}

			// First reconcile: creates HTTP service as ClusterIP.
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			var svc corev1.Service
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: instName + "-http", Namespace: testNamespace}, &svc)).To(Succeed())
			Expect(svc.Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))

			// Change the service type to NodePort.
			var inst warpgatev1alpha1.WarpgateInstance
			Expect(k8sClient.Get(ctx, nn, &inst)).To(Succeed())
			inst.Spec.HTTP.ServiceType = "NodePort"
			Expect(k8sClient.Update(ctx, &inst)).To(Succeed())

			// Second reconcile: service should be updated to NodePort.
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: instName + "-http", Namespace: testNamespace}, &svc)).To(Succeed())
			Expect(svc.Spec.Type).To(Equal(corev1.ServiceTypeNodePort))
			Expect(svc.Spec.Ports).To(HaveLen(1))
			Expect(svc.Spec.Ports[0].Port).To(Equal(int32(8888)))
		})
	})

	Context("Reconcile with all protocols in config", func() {
		const (
			instName   = "inst-all-proto"
			secretName = "inst-all-proto-pw"
		)

		BeforeEach(func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: testNamespace,
				},
				Data: map[string][]byte{
					"password": []byte("all-proto-secret"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			inst := &warpgatev1alpha1.WarpgateInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instName,
					Namespace: testNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					Version: "0.21.1",
					AdminPasswordSecretRef: warpgatev1alpha1.SecretKeyRef{
						Name: secretName,
					},
					Replicas: int32Ptr(1),
					HTTP: &warpgatev1alpha1.HTTPListenerSpec{
						Enabled:     boolPtr(true),
						Port:        int32Ptr(8888),
						ServiceType: "ClusterIP",
					},
					SSH: &warpgatev1alpha1.SSHListenerSpec{
						Enabled:     boolPtr(true),
						Port:        int32Ptr(2222),
						ServiceType: "ClusterIP",
					},
					MySQL: &warpgatev1alpha1.ProtocolListenerSpec{
						Enabled: boolPtr(true),
						Port:    int32Ptr(33306),
					},
					PostgreSQL: &warpgatev1alpha1.ProtocolListenerSpec{
						Enabled: boolPtr(true),
						Port:    int32Ptr(55432),
					},
					ExternalHost: "warpgate.example.com",
					Storage: &warpgatev1alpha1.StorageSpec{
						Size: "1Gi",
					},
					TLS: &warpgatev1alpha1.InstanceTLSSpec{
						CertManager: boolPtr(false),
					},
					CreateConnection: boolPtr(false),
				},
			}
			Expect(k8sClient.Create(ctx, inst)).To(Succeed())
		})

		AfterEach(func() {
			inst := &warpgatev1alpha1.WarpgateInstance{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: instName, Namespace: testNamespace}, inst); err == nil {
				controllerutil.RemoveFinalizer(inst, instanceFinalizer)
				_ = k8sClient.Update(ctx, inst)
				_ = k8sClient.Delete(ctx, inst)
			}
			secret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: testNamespace}, secret); err == nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should include all protocol sections and externalHost in the ConfigMap", func() {
			nn := types.NamespacedName{Name: instName, Namespace: testNamespace}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// Read the ConfigMap and check warpgate.yaml content.
			var cm corev1.ConfigMap
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: instName + "-config", Namespace: testNamespace}, &cm)).To(Succeed())
			Expect(cm.Data).To(HaveKey("warpgate.yaml"))

			yaml := cm.Data["warpgate.yaml"]

			// HTTP section
			Expect(yaml).To(ContainSubstring("http:"))
			Expect(yaml).To(ContainSubstring("enable: true"))
			Expect(yaml).To(ContainSubstring("8888"))

			// SSH section
			Expect(yaml).To(ContainSubstring("ssh:"))
			Expect(yaml).To(ContainSubstring("2222"))

			// MySQL section
			Expect(yaml).To(ContainSubstring("mysql:"))
			Expect(yaml).To(ContainSubstring("33306"))

			// PostgreSQL section
			Expect(yaml).To(ContainSubstring("postgres:"))
			Expect(yaml).To(ContainSubstring("55432"))

			// External host
			Expect(yaml).To(ContainSubstring("external_host: warpgate.example.com"))

			// Deployment should have all four protocol ports.
			var sts appsv1.Deployment
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: instName, Namespace: testNamespace}, &sts)).To(Succeed())
			ports := sts.Spec.Template.Spec.Containers[0].Ports
			Expect(ports).To(ContainElement(
				corev1.ContainerPort{Name: "http", ContainerPort: 8888, Protocol: corev1.ProtocolTCP},
			))
			Expect(ports).To(ContainElement(
				corev1.ContainerPort{Name: "ssh", ContainerPort: 2222, Protocol: corev1.ProtocolTCP},
			))
			Expect(ports).To(ContainElement(
				corev1.ContainerPort{Name: "mysql", ContainerPort: 33306, Protocol: corev1.ProtocolTCP},
			))
			Expect(ports).To(ContainElement(
				corev1.ContainerPort{Name: "postgresql", ContainerPort: 55432, Protocol: corev1.ProtocolTCP},
			))
		})
	})

	// -----------------------------------------------------------------------
	// 1. MySQL and PostgreSQL listeners
	// -----------------------------------------------------------------------
	Context("MySQL and PostgreSQL listeners", func() {
		const (
			instName   = "inst-mysql-pg"
			secretName = "inst-mysql-pg-pw"
		)

		BeforeEach(func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: testNamespace,
				},
				Data: map[string][]byte{
					"password": []byte("mysql-pg-secret"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			inst := &warpgatev1alpha1.WarpgateInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instName,
					Namespace: testNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					Version: "0.21.1",
					AdminPasswordSecretRef: warpgatev1alpha1.SecretKeyRef{
						Name: secretName,
					},
					Replicas: int32Ptr(1),
					HTTP: &warpgatev1alpha1.HTTPListenerSpec{
						Enabled:     boolPtr(true),
						Port:        int32Ptr(8888),
						ServiceType: "ClusterIP",
					},
					MySQL: &warpgatev1alpha1.ProtocolListenerSpec{
						Enabled: boolPtr(true),
						Port:    int32Ptr(33306),
					},
					PostgreSQL: &warpgatev1alpha1.ProtocolListenerSpec{
						Enabled: boolPtr(true),
						Port:    int32Ptr(55432),
					},
					Storage: &warpgatev1alpha1.StorageSpec{
						Size: "1Gi",
					},
					TLS: &warpgatev1alpha1.InstanceTLSSpec{
						CertManager: boolPtr(false),
					},
					CreateConnection: boolPtr(false),
				},
			}
			Expect(k8sClient.Create(ctx, inst)).To(Succeed())
		})

		AfterEach(func() {
			inst := &warpgatev1alpha1.WarpgateInstance{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: instName, Namespace: testNamespace}, inst); err == nil {
				controllerutil.RemoveFinalizer(inst, instanceFinalizer)
				_ = k8sClient.Update(ctx, inst)
				_ = k8sClient.Delete(ctx, inst)
			}
			secret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: testNamespace}, secret); err == nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should add MySQL and PostgreSQL ports to the Deployment", func() {
			nn := types.NamespacedName{Name: instName, Namespace: testNamespace}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			var sts appsv1.Deployment
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: instName, Namespace: testNamespace}, &sts)).To(Succeed())
			ports := sts.Spec.Template.Spec.Containers[0].Ports

			Expect(ports).To(ContainElement(
				corev1.ContainerPort{Name: "mysql", ContainerPort: 33306, Protocol: corev1.ProtocolTCP},
			))
			Expect(ports).To(ContainElement(
				corev1.ContainerPort{Name: "postgresql", ContainerPort: 55432, Protocol: corev1.ProtocolTCP},
			))

			// ConfigMap should also have mysql and postgres sections.
			var cm corev1.ConfigMap
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: instName + "-config", Namespace: testNamespace}, &cm)).To(Succeed())
			Expect(cm.Data["warpgate.yaml"]).To(ContainSubstring("mysql:"))
			Expect(cm.Data["warpgate.yaml"]).To(ContainSubstring("33306"))
			Expect(cm.Data["warpgate.yaml"]).To(ContainSubstring("postgres:"))
			Expect(cm.Data["warpgate.yaml"]).To(ContainSubstring("55432"))
		})
	})

	// -----------------------------------------------------------------------
	// 2. Custom image override
	// -----------------------------------------------------------------------
	Context("Custom image override", func() {
		const (
			instName   = "inst-custom-img"
			secretName = "inst-custom-img-pw"
		)

		BeforeEach(func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: testNamespace,
				},
				Data: map[string][]byte{
					"password": []byte("custom-img-secret"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			inst := &warpgatev1alpha1.WarpgateInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instName,
					Namespace: testNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					Version: "0.21.1",
					Image:   "my-registry.io/custom-warpgate:nightly",
					AdminPasswordSecretRef: warpgatev1alpha1.SecretKeyRef{
						Name: secretName,
					},
					Replicas: int32Ptr(1),
					HTTP: &warpgatev1alpha1.HTTPListenerSpec{
						Enabled:     boolPtr(true),
						Port:        int32Ptr(8888),
						ServiceType: "ClusterIP",
					},
					Storage: &warpgatev1alpha1.StorageSpec{
						Size: "1Gi",
					},
					TLS: &warpgatev1alpha1.InstanceTLSSpec{
						CertManager: boolPtr(false),
					},
					CreateConnection: boolPtr(false),
				},
			}
			Expect(k8sClient.Create(ctx, inst)).To(Succeed())
		})

		AfterEach(func() {
			inst := &warpgatev1alpha1.WarpgateInstance{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: instName, Namespace: testNamespace}, inst); err == nil {
				controllerutil.RemoveFinalizer(inst, instanceFinalizer)
				_ = k8sClient.Update(ctx, inst)
				_ = k8sClient.Delete(ctx, inst)
			}
			secret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: testNamespace}, secret); err == nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should use the custom image instead of the generated one", func() {
			nn := types.NamespacedName{Name: instName, Namespace: testNamespace}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			var sts appsv1.Deployment
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: instName, Namespace: testNamespace}, &sts)).To(Succeed())
			Expect(sts.Spec.Template.Spec.Containers[0].Image).To(Equal("my-registry.io/custom-warpgate:nightly"))
		})
	})

	// -----------------------------------------------------------------------
	// 3. Custom storage size
	// -----------------------------------------------------------------------
	Context("Custom storage size", func() {
		const (
			instName   = "inst-storage"
			secretName = "inst-storage-pw"
		)

		BeforeEach(func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: testNamespace,
				},
				Data: map[string][]byte{
					"password": []byte("storage-secret"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			inst := &warpgatev1alpha1.WarpgateInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instName,
					Namespace: testNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					Version: "0.21.1",
					AdminPasswordSecretRef: warpgatev1alpha1.SecretKeyRef{
						Name: secretName,
					},
					Replicas: int32Ptr(1),
					HTTP: &warpgatev1alpha1.HTTPListenerSpec{
						Enabled:     boolPtr(true),
						Port:        int32Ptr(8888),
						ServiceType: "ClusterIP",
					},
					Storage: &warpgatev1alpha1.StorageSpec{
						Size: "10Gi",
					},
					TLS: &warpgatev1alpha1.InstanceTLSSpec{
						CertManager: boolPtr(false),
					},
					CreateConnection: boolPtr(false),
				},
			}
			Expect(k8sClient.Create(ctx, inst)).To(Succeed())
		})

		AfterEach(func() {
			inst := &warpgatev1alpha1.WarpgateInstance{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: instName, Namespace: testNamespace}, inst); err == nil {
				controllerutil.RemoveFinalizer(inst, instanceFinalizer)
				_ = k8sClient.Update(ctx, inst)
				_ = k8sClient.Delete(ctx, inst)
			}
			secret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: testNamespace}, secret); err == nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should create a PVC with the specified storage size", func() {
			nn := types.NamespacedName{Name: instName, Namespace: testNamespace}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			var pvc corev1.PersistentVolumeClaim
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: instName + "-data", Namespace: testNamespace}, &pvc)).To(Succeed())
			expectedQty := resource.MustParse("10Gi")
			actualQty := pvc.Spec.Resources.Requests[corev1.ResourceStorage]
			Expect(actualQty.Cmp(expectedQty)).To(Equal(0))
		})
	})

	// -----------------------------------------------------------------------
	// 4. Disable HTTP, enable SSH only
	// -----------------------------------------------------------------------
	Context("HTTP disabled, SSH enabled only", func() {
		const (
			instName   = "inst-no-http"
			secretName = "inst-no-http-pw"
		)

		BeforeEach(func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: testNamespace,
				},
				Data: map[string][]byte{
					"password": []byte("no-http-secret"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			inst := &warpgatev1alpha1.WarpgateInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instName,
					Namespace: testNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					Version: "0.21.1",
					AdminPasswordSecretRef: warpgatev1alpha1.SecretKeyRef{
						Name: secretName,
					},
					Replicas: int32Ptr(1),
					HTTP: &warpgatev1alpha1.HTTPListenerSpec{
						Enabled:     boolPtr(false),
						Port:        int32Ptr(8888),
						ServiceType: "ClusterIP",
					},
					SSH: &warpgatev1alpha1.SSHListenerSpec{
						Enabled:     boolPtr(true),
						Port:        int32Ptr(2222),
						ServiceType: "ClusterIP",
					},
					Storage: &warpgatev1alpha1.StorageSpec{
						Size: "1Gi",
					},
					TLS: &warpgatev1alpha1.InstanceTLSSpec{
						CertManager: boolPtr(false),
					},
					CreateConnection: boolPtr(false),
				},
			}
			Expect(k8sClient.Create(ctx, inst)).To(Succeed())
		})

		AfterEach(func() {
			inst := &warpgatev1alpha1.WarpgateInstance{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: instName, Namespace: testNamespace}, inst); err == nil {
				controllerutil.RemoveFinalizer(inst, instanceFinalizer)
				_ = k8sClient.Update(ctx, inst)
				_ = k8sClient.Delete(ctx, inst)
			}
			secret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: testNamespace}, secret); err == nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should create SSH Service but no HTTP Service", func() {
			nn := types.NamespacedName{Name: instName, Namespace: testNamespace}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// HTTP Service should NOT exist.
			var httpSvc corev1.Service
			err = k8sClient.Get(ctx, types.NamespacedName{Name: instName + "-http", Namespace: testNamespace}, &httpSvc)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))

			// SSH Service should exist.
			var sshSvc corev1.Service
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: instName + "-ssh", Namespace: testNamespace}, &sshSvc)).To(Succeed())
			Expect(sshSvc.Spec.Ports[0].Port).To(Equal(int32(2222)))

			// Deployment should only have SSH port, no HTTP port.
			var sts appsv1.Deployment
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: instName, Namespace: testNamespace}, &sts)).To(Succeed())
			ports := sts.Spec.Template.Spec.Containers[0].Ports
			Expect(ports).To(ContainElement(
				corev1.ContainerPort{Name: "ssh", ContainerPort: 2222, Protocol: corev1.ProtocolTCP},
			))
			for _, p := range ports {
				Expect(p.Name).NotTo(Equal("http"))
			}
		})
	})

	// -----------------------------------------------------------------------
	// 5. createConnection=false explicitly
	// -----------------------------------------------------------------------
	Context("createConnection explicitly false", func() {
		const (
			instName   = "inst-no-conn"
			secretName = "inst-no-conn-pw"
		)

		BeforeEach(func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: testNamespace,
				},
				Data: map[string][]byte{
					"password": []byte("no-conn-secret"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			inst := &warpgatev1alpha1.WarpgateInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instName,
					Namespace: testNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					Version: "0.21.1",
					AdminPasswordSecretRef: warpgatev1alpha1.SecretKeyRef{
						Name: secretName,
					},
					Replicas: int32Ptr(1),
					HTTP: &warpgatev1alpha1.HTTPListenerSpec{
						Enabled:     boolPtr(true),
						Port:        int32Ptr(8888),
						ServiceType: "ClusterIP",
					},
					Storage: &warpgatev1alpha1.StorageSpec{
						Size: "1Gi",
					},
					TLS: &warpgatev1alpha1.InstanceTLSSpec{
						CertManager: boolPtr(false),
					},
					CreateConnection: boolPtr(false),
				},
			}
			Expect(k8sClient.Create(ctx, inst)).To(Succeed())
		})

		AfterEach(func() {
			inst := &warpgatev1alpha1.WarpgateInstance{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: instName, Namespace: testNamespace}, inst); err == nil {
				controllerutil.RemoveFinalizer(inst, instanceFinalizer)
				_ = k8sClient.Update(ctx, inst)
				_ = k8sClient.Delete(ctx, inst)
			}
			secret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: testNamespace}, secret); err == nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should not create a WarpgateConnection CR", func() {
			nn := types.NamespacedName{Name: instName, Namespace: testNamespace}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// No WarpgateConnection should exist.
			var conn warpgatev1alpha1.WarpgateConnection
			err = k8sClient.Get(ctx, types.NamespacedName{Name: instName + "-connection", Namespace: testNamespace}, &conn)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))

			// No auth secret should be created.
			var authSecret corev1.Secret
			err = k8sClient.Get(ctx, types.NamespacedName{Name: instName + "-admin-auth", Namespace: testNamespace}, &authSecret)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))

			// Status connectionRef should be empty.
			var updated warpgatev1alpha1.WarpgateInstance
			Expect(k8sClient.Get(ctx, nn, &updated)).To(Succeed())
			Expect(updated.Status.ConnectionRef).To(BeEmpty())
		})
	})

	// -----------------------------------------------------------------------
	// 6. Update triggers Deployment update (replicas change)
	// -----------------------------------------------------------------------
	Context("Update triggers Deployment update", func() {
		const (
			instName   = "inst-sts-update"
			secretName = "inst-sts-update-pw"
		)

		BeforeEach(func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: testNamespace,
				},
				Data: map[string][]byte{
					"password": []byte("sts-update-secret"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			inst := &warpgatev1alpha1.WarpgateInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instName,
					Namespace: testNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					Version: "0.21.1",
					AdminPasswordSecretRef: warpgatev1alpha1.SecretKeyRef{
						Name: secretName,
					},
					Replicas: int32Ptr(1),
					HTTP: &warpgatev1alpha1.HTTPListenerSpec{
						Enabled:     boolPtr(true),
						Port:        int32Ptr(8888),
						ServiceType: "ClusterIP",
					},
					Storage: &warpgatev1alpha1.StorageSpec{
						Size: "1Gi",
					},
					TLS: &warpgatev1alpha1.InstanceTLSSpec{
						CertManager: boolPtr(false),
					},
					CreateConnection: boolPtr(false),
				},
			}
			Expect(k8sClient.Create(ctx, inst)).To(Succeed())
		})

		AfterEach(func() {
			inst := &warpgatev1alpha1.WarpgateInstance{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: instName, Namespace: testNamespace}, inst); err == nil {
				controllerutil.RemoveFinalizer(inst, instanceFinalizer)
				_ = k8sClient.Update(ctx, inst)
				_ = k8sClient.Delete(ctx, inst)
			}
			secret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: testNamespace}, secret); err == nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should update Deployment replicas when spec.replicas changes", func() {
			nn := types.NamespacedName{Name: instName, Namespace: testNamespace}

			// First reconcile creates with 1 replica.
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			var sts appsv1.Deployment
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: instName, Namespace: testNamespace}, &sts)).To(Succeed())
			Expect(*sts.Spec.Replicas).To(Equal(int32(1)))

			// Change replicas to 3.
			var inst warpgatev1alpha1.WarpgateInstance
			Expect(k8sClient.Get(ctx, nn, &inst)).To(Succeed())
			inst.Spec.Replicas = int32Ptr(3)
			Expect(k8sClient.Update(ctx, &inst)).To(Succeed())

			// Second reconcile should update the Deployment.
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: instName, Namespace: testNamespace}, &sts)).To(Succeed())
			Expect(*sts.Spec.Replicas).To(Equal(int32(3)))
		})
	})

	// -----------------------------------------------------------------------
	// 7. PVC already exists
	// -----------------------------------------------------------------------
	Context("PVC already exists before reconcile", func() {
		const (
			instName   = "inst-pvc-exist"
			secretName = "inst-pvc-exist-pw"
		)

		BeforeEach(func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: testNamespace,
				},
				Data: map[string][]byte{
					"password": []byte("pvc-exist-secret"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			// Pre-create the PVC before the instance exists.
			pvc := &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instName + "-data",
					Namespace: testNamespace,
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse("5Gi"),
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, pvc)).To(Succeed())

			inst := &warpgatev1alpha1.WarpgateInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instName,
					Namespace: testNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					Version: "0.21.1",
					AdminPasswordSecretRef: warpgatev1alpha1.SecretKeyRef{
						Name: secretName,
					},
					Replicas: int32Ptr(1),
					HTTP: &warpgatev1alpha1.HTTPListenerSpec{
						Enabled:     boolPtr(true),
						Port:        int32Ptr(8888),
						ServiceType: "ClusterIP",
					},
					Storage: &warpgatev1alpha1.StorageSpec{
						Size: "1Gi",
					},
					TLS: &warpgatev1alpha1.InstanceTLSSpec{
						CertManager: boolPtr(false),
					},
					CreateConnection: boolPtr(false),
				},
			}
			Expect(k8sClient.Create(ctx, inst)).To(Succeed())
		})

		AfterEach(func() {
			inst := &warpgatev1alpha1.WarpgateInstance{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: instName, Namespace: testNamespace}, inst); err == nil {
				controllerutil.RemoveFinalizer(inst, instanceFinalizer)
				_ = k8sClient.Update(ctx, inst)
				_ = k8sClient.Delete(ctx, inst)
			}
			pvc := &corev1.PersistentVolumeClaim{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: instName + "-data", Namespace: testNamespace}, pvc); err == nil {
				_ = k8sClient.Delete(ctx, pvc)
			}
			secret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: testNamespace}, secret); err == nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should tolerate a pre-existing PVC and not fail", func() {
			nn := types.NamespacedName{Name: instName, Namespace: testNamespace}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// PVC should still exist with the original size (not overwritten).
			var pvc corev1.PersistentVolumeClaim
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: instName + "-data", Namespace: testNamespace}, &pvc)).To(Succeed())
			originalQty := resource.MustParse("5Gi")
			actualQty := pvc.Spec.Resources.Requests[corev1.ResourceStorage]
			Expect(actualQty.Cmp(originalQty)).To(Equal(0))

			// Deployment should still be created despite pre-existing PVC.
			var sts appsv1.Deployment
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: instName, Namespace: testNamespace}, &sts)).To(Succeed())
		})
	})

	// -----------------------------------------------------------------------
	// 8. ConfigMap update on spec change
	// -----------------------------------------------------------------------
	Context("ConfigMap update on spec change", func() {
		const (
			instName   = "inst-cm-update"
			secretName = "inst-cm-update-pw"
		)

		BeforeEach(func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: testNamespace,
				},
				Data: map[string][]byte{
					"password": []byte("cm-update-secret"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			inst := &warpgatev1alpha1.WarpgateInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instName,
					Namespace: testNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					Version: "0.21.1",
					AdminPasswordSecretRef: warpgatev1alpha1.SecretKeyRef{
						Name: secretName,
					},
					Replicas: int32Ptr(1),
					HTTP: &warpgatev1alpha1.HTTPListenerSpec{
						Enabled:     boolPtr(true),
						Port:        int32Ptr(8888),
						ServiceType: "ClusterIP",
					},
					Storage: &warpgatev1alpha1.StorageSpec{
						Size: "1Gi",
					},
					TLS: &warpgatev1alpha1.InstanceTLSSpec{
						CertManager: boolPtr(false),
					},
					CreateConnection: boolPtr(false),
				},
			}
			Expect(k8sClient.Create(ctx, inst)).To(Succeed())
		})

		AfterEach(func() {
			inst := &warpgatev1alpha1.WarpgateInstance{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: instName, Namespace: testNamespace}, inst); err == nil {
				controllerutil.RemoveFinalizer(inst, instanceFinalizer)
				_ = k8sClient.Update(ctx, inst)
				_ = k8sClient.Delete(ctx, inst)
			}
			secret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: testNamespace}, secret); err == nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should update ConfigMap content when HTTP port changes", func() {
			nn := types.NamespacedName{Name: instName, Namespace: testNamespace}

			// First reconcile.
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			var cm corev1.ConfigMap
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: instName + "-config", Namespace: testNamespace}, &cm)).To(Succeed())
			Expect(cm.Data["warpgate.yaml"]).To(ContainSubstring("8888"))

			// Change HTTP port.
			var inst warpgatev1alpha1.WarpgateInstance
			Expect(k8sClient.Get(ctx, nn, &inst)).To(Succeed())
			inst.Spec.HTTP.Port = int32Ptr(9999)
			Expect(k8sClient.Update(ctx, &inst)).To(Succeed())

			// Second reconcile should update the ConfigMap.
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: instName + "-config", Namespace: testNamespace}, &cm)).To(Succeed())
			Expect(cm.Data["warpgate.yaml"]).To(ContainSubstring("9999"))
			Expect(cm.Data["warpgate.yaml"]).NotTo(ContainSubstring("8888"))
		})
	})

	// -----------------------------------------------------------------------
	// 9. Service type NodePort
	// -----------------------------------------------------------------------
	Context("Service type NodePort", func() {
		const (
			instName   = "inst-nodeport"
			secretName = "inst-nodeport-pw"
		)

		BeforeEach(func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: testNamespace,
				},
				Data: map[string][]byte{
					"password": []byte("nodeport-secret"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			inst := &warpgatev1alpha1.WarpgateInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instName,
					Namespace: testNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					Version: "0.21.1",
					AdminPasswordSecretRef: warpgatev1alpha1.SecretKeyRef{
						Name: secretName,
					},
					Replicas: int32Ptr(1),
					HTTP: &warpgatev1alpha1.HTTPListenerSpec{
						Enabled:     boolPtr(true),
						Port:        int32Ptr(8888),
						ServiceType: "NodePort",
					},
					Storage: &warpgatev1alpha1.StorageSpec{
						Size: "1Gi",
					},
					TLS: &warpgatev1alpha1.InstanceTLSSpec{
						CertManager: boolPtr(false),
					},
					CreateConnection: boolPtr(false),
				},
			}
			Expect(k8sClient.Create(ctx, inst)).To(Succeed())
		})

		AfterEach(func() {
			inst := &warpgatev1alpha1.WarpgateInstance{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: instName, Namespace: testNamespace}, inst); err == nil {
				controllerutil.RemoveFinalizer(inst, instanceFinalizer)
				_ = k8sClient.Update(ctx, inst)
				_ = k8sClient.Delete(ctx, inst)
			}
			secret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: testNamespace}, secret); err == nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should create an HTTP Service with NodePort type", func() {
			nn := types.NamespacedName{Name: instName, Namespace: testNamespace}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			var svc corev1.Service
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: instName + "-http", Namespace: testNamespace}, &svc)).To(Succeed())
			Expect(svc.Spec.Type).To(Equal(corev1.ServiceTypeNodePort))
			Expect(svc.Spec.Ports).To(HaveLen(1))
			Expect(svc.Spec.Ports[0].Port).To(Equal(int32(8888)))
		})
	})

	// -----------------------------------------------------------------------
	// 10. buildWarpgateConfig with all protocols
	// -----------------------------------------------------------------------
	Context("buildWarpgateConfig with all protocols and externalHost", func() {
		It("should produce config YAML with all protocol sections", func() {
			inst := &warpgatev1alpha1.WarpgateInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "config-test",
					Namespace: testNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					Version: "0.21.1",
					AdminPasswordSecretRef: warpgatev1alpha1.SecretKeyRef{
						Name: "dummy",
					},
					HTTP: &warpgatev1alpha1.HTTPListenerSpec{
						Enabled: boolPtr(true),
						Port:    int32Ptr(8888),
					},
					SSH: &warpgatev1alpha1.SSHListenerSpec{
						Enabled: boolPtr(true),
						Port:    int32Ptr(2222),
					},
					MySQL: &warpgatev1alpha1.ProtocolListenerSpec{
						Enabled: boolPtr(true),
						Port:    int32Ptr(33306),
					},
					PostgreSQL: &warpgatev1alpha1.ProtocolListenerSpec{
						Enabled: boolPtr(true),
						Port:    int32Ptr(55432),
					},
					ExternalHost: "bastion.example.com",
					TLS: &warpgatev1alpha1.InstanceTLSSpec{
						CertManager: boolPtr(false),
					},
				},
			}

			yaml := reconciler.buildWarpgateConfig(inst)

			Expect(yaml).To(ContainSubstring("http:"))
			Expect(yaml).To(ContainSubstring("enable: true"))
			Expect(yaml).To(ContainSubstring("0.0.0.0:8888"))

			Expect(yaml).To(ContainSubstring("ssh:"))
			Expect(yaml).To(ContainSubstring("0.0.0.0:2222"))

			Expect(yaml).To(ContainSubstring("mysql:"))
			Expect(yaml).To(ContainSubstring("0.0.0.0:33306"))

			Expect(yaml).To(ContainSubstring("postgres:"))
			Expect(yaml).To(ContainSubstring("0.0.0.0:55432"))

			Expect(yaml).To(ContainSubstring("external_host: bastion.example.com"))
		})

		It("should omit MySQL and PostgreSQL sections when disabled", func() {
			inst := &warpgatev1alpha1.WarpgateInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "config-test-minimal",
					Namespace: testNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					Version: "0.21.1",
					AdminPasswordSecretRef: warpgatev1alpha1.SecretKeyRef{
						Name: "dummy",
					},
					HTTP: &warpgatev1alpha1.HTTPListenerSpec{
						Enabled: boolPtr(true),
						Port:    int32Ptr(8888),
					},
					TLS: &warpgatev1alpha1.InstanceTLSSpec{
						CertManager: boolPtr(false),
					},
				},
			}

			yaml := reconciler.buildWarpgateConfig(inst)

			Expect(yaml).To(ContainSubstring("http:"))
			Expect(yaml).NotTo(ContainSubstring("mysql:"))
			Expect(yaml).NotTo(ContainSubstring("postgres:"))
			Expect(yaml).NotTo(ContainSubstring("external_host"))
		})
	})

	// -----------------------------------------------------------------------
	// 11. Recreate strategy default
	// -----------------------------------------------------------------------
	Context("Recreate strategy default", func() {
		const (
			instName   = "inst-recreate"
			secretName = "inst-recreate-pw"
		)

		BeforeEach(func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: testNamespace,
				},
				Data: map[string][]byte{
					"password": []byte("recreate-secret"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			inst := &warpgatev1alpha1.WarpgateInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instName,
					Namespace: testNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					Version: "0.21.1",
					AdminPasswordSecretRef: warpgatev1alpha1.SecretKeyRef{
						Name: secretName,
					},
					Replicas: int32Ptr(1),
					Strategy: "Recreate",
					HTTP: &warpgatev1alpha1.HTTPListenerSpec{
						Enabled:     boolPtr(true),
						Port:        int32Ptr(8888),
						ServiceType: "ClusterIP",
					},
					Storage: &warpgatev1alpha1.StorageSpec{
						Size: "1Gi",
					},
					TLS: &warpgatev1alpha1.InstanceTLSSpec{
						CertManager: boolPtr(false),
					},
					CreateConnection: boolPtr(false),
				},
			}
			Expect(k8sClient.Create(ctx, inst)).To(Succeed())
		})

		AfterEach(func() {
			inst := &warpgatev1alpha1.WarpgateInstance{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: instName, Namespace: testNamespace}, inst); err == nil {
				controllerutil.RemoveFinalizer(inst, instanceFinalizer)
				_ = k8sClient.Update(ctx, inst)
				_ = k8sClient.Delete(ctx, inst)
			}
			secret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: testNamespace}, secret); err == nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should create a Deployment with Recreate strategy", func() {
			nn := types.NamespacedName{Name: instName, Namespace: testNamespace}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			var deploy appsv1.Deployment
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: instName, Namespace: testNamespace}, &deploy)).To(Succeed())
			Expect(deploy.Spec.Strategy.Type).To(Equal(appsv1.RecreateDeploymentStrategyType))
		})
	})

	// -----------------------------------------------------------------------
	// 12. EmptyDir storage (storage.enabled=false)
	// -----------------------------------------------------------------------
	Context("EmptyDir storage when storage disabled", func() {
		const (
			instName   = "inst-emptydir"
			secretName = "inst-emptydir-pw"
		)

		BeforeEach(func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: testNamespace,
				},
				Data: map[string][]byte{
					"password": []byte("emptydir-secret"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			inst := &warpgatev1alpha1.WarpgateInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instName,
					Namespace: testNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					Version: "0.21.1",
					AdminPasswordSecretRef: warpgatev1alpha1.SecretKeyRef{
						Name: secretName,
					},
					Replicas: int32Ptr(1),
					HTTP: &warpgatev1alpha1.HTTPListenerSpec{
						Enabled:     boolPtr(true),
						Port:        int32Ptr(8888),
						ServiceType: "ClusterIP",
					},
					Storage: &warpgatev1alpha1.StorageSpec{
						Enabled: boolPtr(false),
					},
					TLS: &warpgatev1alpha1.InstanceTLSSpec{
						CertManager: boolPtr(false),
					},
					CreateConnection: boolPtr(false),
				},
			}
			Expect(k8sClient.Create(ctx, inst)).To(Succeed())
		})

		AfterEach(func() {
			inst := &warpgatev1alpha1.WarpgateInstance{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: instName, Namespace: testNamespace}, inst); err == nil {
				controllerutil.RemoveFinalizer(inst, instanceFinalizer)
				_ = k8sClient.Update(ctx, inst)
				_ = k8sClient.Delete(ctx, inst)
			}
			secret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: testNamespace}, secret); err == nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should not create a PVC and use emptyDir volume", func() {
			nn := types.NamespacedName{Name: instName, Namespace: testNamespace}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// PVC should NOT exist.
			var pvc corev1.PersistentVolumeClaim
			err = k8sClient.Get(ctx, types.NamespacedName{Name: instName + "-data", Namespace: testNamespace}, &pvc)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))

			// Deployment should have an emptyDir volume for data.
			var deploy appsv1.Deployment
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: instName, Namespace: testNamespace}, &deploy)).To(Succeed())

			foundEmptyDir := false
			for _, vol := range deploy.Spec.Template.Spec.Volumes {
				if vol.Name == "data" && vol.EmptyDir != nil {
					foundEmptyDir = true
					break
				}
			}
			Expect(foundEmptyDir).To(BeTrue(), "expected data volume to use emptyDir")
		})
	})

	// -----------------------------------------------------------------------
	// 13. Existing PVC reference
	// -----------------------------------------------------------------------
	Context("Existing PVC via storage.existingClaimName", func() {
		const (
			instName      = "inst-existpvc"
			secretName    = "inst-existpvc-pw"
			existingClaim = "my-preexisting-pvc"
		)

		BeforeEach(func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: testNamespace,
				},
				Data: map[string][]byte{
					"password": []byte("existpvc-secret"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			// Create the existing PVC so the controller can reference it.
			pvc := &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      existingClaim,
					Namespace: testNamespace,
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse("20Gi"),
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, pvc)).To(Succeed())

			inst := &warpgatev1alpha1.WarpgateInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instName,
					Namespace: testNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					Version: "0.21.1",
					AdminPasswordSecretRef: warpgatev1alpha1.SecretKeyRef{
						Name: secretName,
					},
					Replicas: int32Ptr(1),
					HTTP: &warpgatev1alpha1.HTTPListenerSpec{
						Enabled:     boolPtr(true),
						Port:        int32Ptr(8888),
						ServiceType: "ClusterIP",
					},
					Storage: &warpgatev1alpha1.StorageSpec{
						ExistingClaimName: existingClaim,
					},
					TLS: &warpgatev1alpha1.InstanceTLSSpec{
						CertManager: boolPtr(false),
					},
					CreateConnection: boolPtr(false),
				},
			}
			Expect(k8sClient.Create(ctx, inst)).To(Succeed())
		})

		AfterEach(func() {
			inst := &warpgatev1alpha1.WarpgateInstance{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: instName, Namespace: testNamespace}, inst); err == nil {
				controllerutil.RemoveFinalizer(inst, instanceFinalizer)
				_ = k8sClient.Update(ctx, inst)
				_ = k8sClient.Delete(ctx, inst)
			}
			pvc := &corev1.PersistentVolumeClaim{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: existingClaim, Namespace: testNamespace}, pvc); err == nil {
				_ = k8sClient.Delete(ctx, pvc)
			}
			secret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: testNamespace}, secret); err == nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should reference the existing PVC instead of creating a new one", func() {
			nn := types.NamespacedName{Name: instName, Namespace: testNamespace}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// The auto-generated PVC should NOT be created.
			var autoPVC corev1.PersistentVolumeClaim
			err = k8sClient.Get(ctx, types.NamespacedName{Name: instName + "-data", Namespace: testNamespace}, &autoPVC)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))

			// Deployment should reference the existing claim.
			var deploy appsv1.Deployment
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: instName, Namespace: testNamespace}, &deploy)).To(Succeed())

			foundPVCRef := false
			for _, vol := range deploy.Spec.Template.Spec.Volumes {
				if vol.Name == "data" && vol.PersistentVolumeClaim != nil {
					if vol.PersistentVolumeClaim.ClaimName == existingClaim {
						foundPVCRef = true
						break
					}
				}
			}
			Expect(foundPVCRef).To(BeTrue(), "expected data volume to reference existing PVC %q", existingClaim)
		})
	})

	// -----------------------------------------------------------------------
	// 14. Config override ConfigMap
	// -----------------------------------------------------------------------
	Context("Config override creates a second ConfigMap", func() {
		const (
			instName   = "inst-cfg-override"
			secretName = "inst-cfg-override-pw"
		)

		BeforeEach(func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: testNamespace,
				},
				Data: map[string][]byte{
					"password": []byte("cfg-override-secret"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			inst := &warpgatev1alpha1.WarpgateInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instName,
					Namespace: testNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					Version: "0.21.1",
					AdminPasswordSecretRef: warpgatev1alpha1.SecretKeyRef{
						Name: secretName,
					},
					Replicas: int32Ptr(1),
					HTTP: &warpgatev1alpha1.HTTPListenerSpec{
						Enabled:     boolPtr(true),
						Port:        int32Ptr(8888),
						ServiceType: "ClusterIP",
					},
					Storage: &warpgatev1alpha1.StorageSpec{
						Size: "1Gi",
					},
					TLS: &warpgatev1alpha1.InstanceTLSSpec{
						CertManager: boolPtr(false),
					},
					CreateConnection: boolPtr(false),
					ConfigOverride:   "custom: config",
				},
			}
			Expect(k8sClient.Create(ctx, inst)).To(Succeed())
		})

		AfterEach(func() {
			inst := &warpgatev1alpha1.WarpgateInstance{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: instName, Namespace: testNamespace}, inst); err == nil {
				controllerutil.RemoveFinalizer(inst, instanceFinalizer)
				_ = k8sClient.Update(ctx, inst)
				_ = k8sClient.Delete(ctx, inst)
			}
			secret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: testNamespace}, secret); err == nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should create a config-override ConfigMap with the override content", func() {
			nn := types.NamespacedName{Name: instName, Namespace: testNamespace}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// The override ConfigMap should exist with the custom content.
			var overrideCM corev1.ConfigMap
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: instName + "-config-override", Namespace: testNamespace,
			}, &overrideCM)).To(Succeed())
			Expect(overrideCM.Data).To(HaveKey("warpgate.yaml"))
			Expect(overrideCM.Data["warpgate.yaml"]).To(Equal("custom: config"))

			// The base ConfigMap should still exist too.
			var baseCM corev1.ConfigMap
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: instName + "-config", Namespace: testNamespace,
			}, &baseCM)).To(Succeed())

			// Deployment should have a config-override volume.
			var deploy appsv1.Deployment
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: instName, Namespace: testNamespace}, &deploy)).To(Succeed())
			foundOverrideVol := false
			for _, vol := range deploy.Spec.Template.Spec.Volumes {
				if vol.Name == "config-override" && vol.ConfigMap != nil {
					Expect(vol.ConfigMap.Name).To(Equal(instName + "-config-override"))
					foundOverrideVol = true
					break
				}
			}
			Expect(foundOverrideVol).To(BeTrue(), "expected config-override volume in Deployment")
		})
	})

	// -----------------------------------------------------------------------
	// 15. Database URL in config (PostgreSQL branch)
	// -----------------------------------------------------------------------
	Context("Database URL produces postgres config instead of sqlite", func() {
		const (
			instName   = "inst-dburl"
			secretName = "inst-dburl-pw"
		)

		BeforeEach(func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: testNamespace,
				},
				Data: map[string][]byte{
					"password": []byte("dburl-secret"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			inst := &warpgatev1alpha1.WarpgateInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instName,
					Namespace: testNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					Version: "0.21.1",
					AdminPasswordSecretRef: warpgatev1alpha1.SecretKeyRef{
						Name: secretName,
					},
					Replicas: int32Ptr(1),
					HTTP: &warpgatev1alpha1.HTTPListenerSpec{
						Enabled:     boolPtr(true),
						Port:        int32Ptr(8888),
						ServiceType: "ClusterIP",
					},
					Storage: &warpgatev1alpha1.StorageSpec{
						Size: "1Gi",
					},
					TLS: &warpgatev1alpha1.InstanceTLSSpec{
						CertManager: boolPtr(false),
					},
					CreateConnection: boolPtr(false),
					DatabaseURL:      "postgres://host/db",
				},
			}
			Expect(k8sClient.Create(ctx, inst)).To(Succeed())
		})

		AfterEach(func() {
			inst := &warpgatev1alpha1.WarpgateInstance{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: instName, Namespace: testNamespace}, inst); err == nil {
				controllerutil.RemoveFinalizer(inst, instanceFinalizer)
				_ = k8sClient.Update(ctx, inst)
				_ = k8sClient.Delete(ctx, inst)
			}
			secret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: testNamespace}, secret); err == nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should generate config with postgres database_url instead of sqlite", func() {
			nn := types.NamespacedName{Name: instName, Namespace: testNamespace}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			var cm corev1.ConfigMap
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: instName + "-config", Namespace: testNamespace,
			}, &cm)).To(Succeed())

			yaml := cm.Data["warpgate.yaml"]
			Expect(yaml).To(ContainSubstring("postgres:"))
			Expect(yaml).To(ContainSubstring("postgres://host/db"))
			Expect(yaml).NotTo(ContainSubstring("sqlite:"))
		})
	})

	// -----------------------------------------------------------------------
	// 16. TLS from existing secret
	// -----------------------------------------------------------------------
	Context("TLS from existing secret", func() {
		const (
			instName      = "inst-tls-secret"
			secretName    = "inst-tls-secret-pw"
			tlsSecretName = "my-tls"
		)

		BeforeEach(func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: testNamespace,
				},
				Data: map[string][]byte{
					"password": []byte("tls-secret-pw"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			tlsSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tlsSecretName,
					Namespace: testNamespace,
				},
				Type: corev1.SecretTypeTLS,
				Data: map[string][]byte{
					"tls.crt": []byte("fake-cert"),
					"tls.key": []byte("fake-key"),
				},
			}
			Expect(k8sClient.Create(ctx, tlsSecret)).To(Succeed())

			inst := &warpgatev1alpha1.WarpgateInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instName,
					Namespace: testNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					Version: "0.21.1",
					AdminPasswordSecretRef: warpgatev1alpha1.SecretKeyRef{
						Name: secretName,
					},
					Replicas: int32Ptr(1),
					HTTP: &warpgatev1alpha1.HTTPListenerSpec{
						Enabled:     boolPtr(true),
						Port:        int32Ptr(8888),
						ServiceType: "ClusterIP",
					},
					Storage: &warpgatev1alpha1.StorageSpec{
						Size: "1Gi",
					},
					TLS: &warpgatev1alpha1.InstanceTLSSpec{
						CertManager: boolPtr(false),
						SecretName:  tlsSecretName,
					},
					CreateConnection: boolPtr(false),
				},
			}
			Expect(k8sClient.Create(ctx, inst)).To(Succeed())
		})

		AfterEach(func() {
			inst := &warpgatev1alpha1.WarpgateInstance{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: instName, Namespace: testNamespace}, inst); err == nil {
				controllerutil.RemoveFinalizer(inst, instanceFinalizer)
				_ = k8sClient.Update(ctx, inst)
				_ = k8sClient.Delete(ctx, inst)
			}
			for _, name := range []string{secretName, tlsSecretName} {
				s := &corev1.Secret{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: testNamespace}, s); err == nil {
					_ = k8sClient.Delete(ctx, s)
				}
			}
		})

		It("should mount the TLS secret as a volume in the Deployment", func() {
			nn := types.NamespacedName{Name: instName, Namespace: testNamespace}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			var deploy appsv1.Deployment
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: instName, Namespace: testNamespace}, &deploy)).To(Succeed())

			// Should have a tls-secret volume referencing the TLS secret.
			foundTLSVol := false
			for _, vol := range deploy.Spec.Template.Spec.Volumes {
				if vol.Name == "tls-secret" && vol.Secret != nil {
					Expect(vol.Secret.SecretName).To(Equal(tlsSecretName))
					foundTLSVol = true
					break
				}
			}
			Expect(foundTLSVol).To(BeTrue(), "expected tls-secret volume in Deployment")

			// Init container should have a tls-secret volume mount.
			initMounts := deploy.Spec.Template.Spec.InitContainers[0].VolumeMounts
			foundTLSMount := false
			for _, m := range initMounts {
				if m.Name == "tls-secret" && m.MountPath == "/tls-secret" {
					foundTLSMount = true
					break
				}
			}
			Expect(foundTLSMount).To(BeTrue(), "expected tls-secret volume mount in init container")
		})
	})

	// -----------------------------------------------------------------------
	// 17. SSH keys secret volume
	// -----------------------------------------------------------------------
	Context("SSH keys secret volume mount", func() {
		const (
			instName      = "inst-ssh-keys"
			secretName    = "inst-ssh-keys-pw"
			sshKeysSecret = "my-ssh-keys"
		)

		BeforeEach(func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: testNamespace,
				},
				Data: map[string][]byte{
					"password": []byte("ssh-keys-pw"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			sshSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      sshKeysSecret,
					Namespace: testNamespace,
				},
				Data: map[string][]byte{
					"host-ed25519": []byte("fake-host-key"),
				},
			}
			Expect(k8sClient.Create(ctx, sshSecret)).To(Succeed())

			inst := &warpgatev1alpha1.WarpgateInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instName,
					Namespace: testNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					Version: "0.21.1",
					AdminPasswordSecretRef: warpgatev1alpha1.SecretKeyRef{
						Name: secretName,
					},
					Replicas: int32Ptr(1),
					HTTP: &warpgatev1alpha1.HTTPListenerSpec{
						Enabled:     boolPtr(true),
						Port:        int32Ptr(8888),
						ServiceType: "ClusterIP",
					},
					Storage: &warpgatev1alpha1.StorageSpec{
						Size: "1Gi",
					},
					TLS: &warpgatev1alpha1.InstanceTLSSpec{
						CertManager: boolPtr(false),
					},
					CreateConnection:  boolPtr(false),
					SSHKeysSecretName: sshKeysSecret,
				},
			}
			Expect(k8sClient.Create(ctx, inst)).To(Succeed())
		})

		AfterEach(func() {
			inst := &warpgatev1alpha1.WarpgateInstance{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: instName, Namespace: testNamespace}, inst); err == nil {
				controllerutil.RemoveFinalizer(inst, instanceFinalizer)
				_ = k8sClient.Update(ctx, inst)
				_ = k8sClient.Delete(ctx, inst)
			}
			for _, name := range []string{secretName, sshKeysSecret} {
				s := &corev1.Secret{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: testNamespace}, s); err == nil {
					_ = k8sClient.Delete(ctx, s)
				}
			}
		})

		It("should mount the SSH keys secret as a volume in the Deployment", func() {
			nn := types.NamespacedName{Name: instName, Namespace: testNamespace}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			var deploy appsv1.Deployment
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: instName, Namespace: testNamespace}, &deploy)).To(Succeed())

			// Should have an ssh-keys volume referencing the secret.
			foundSSHVol := false
			for _, vol := range deploy.Spec.Template.Spec.Volumes {
				if vol.Name == "ssh-keys" && vol.Secret != nil {
					Expect(vol.Secret.SecretName).To(Equal(sshKeysSecret))
					foundSSHVol = true
					break
				}
			}
			Expect(foundSSHVol).To(BeTrue(), "expected ssh-keys volume in Deployment")

			// Init container should have an ssh-keys volume mount.
			initMounts := deploy.Spec.Template.Spec.InitContainers[0].VolumeMounts
			foundSSHMount := false
			for _, m := range initMounts {
				if m.Name == "ssh-keys" && m.MountPath == "/ssh-keys" {
					foundSSHMount = true
					break
				}
			}
			Expect(foundSSHMount).To(BeTrue(), "expected ssh-keys volume mount in init container")
		})
	})

	// -----------------------------------------------------------------------
	// 18. Kubernetes protocol enabled
	// -----------------------------------------------------------------------
	Context("Kubernetes protocol enabled", func() {
		const (
			instName   = "inst-k8s-proto"
			secretName = "inst-k8s-proto-pw"
		)

		BeforeEach(func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: testNamespace,
				},
				Data: map[string][]byte{
					"password": []byte("k8s-proto-secret"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			inst := &warpgatev1alpha1.WarpgateInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instName,
					Namespace: testNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					Version: "0.21.1",
					AdminPasswordSecretRef: warpgatev1alpha1.SecretKeyRef{
						Name: secretName,
					},
					Replicas: int32Ptr(1),
					HTTP: &warpgatev1alpha1.HTTPListenerSpec{
						Enabled:     boolPtr(true),
						Port:        int32Ptr(8888),
						ServiceType: "ClusterIP",
					},
					Kubernetes: &warpgatev1alpha1.ProtocolListenerSpec{
						Enabled: boolPtr(true),
						Port:    int32Ptr(8443),
					},
					Storage: &warpgatev1alpha1.StorageSpec{
						Size: "1Gi",
					},
					TLS: &warpgatev1alpha1.InstanceTLSSpec{
						CertManager: boolPtr(false),
					},
					CreateConnection: boolPtr(false),
				},
			}
			Expect(k8sClient.Create(ctx, inst)).To(Succeed())
		})

		AfterEach(func() {
			inst := &warpgatev1alpha1.WarpgateInstance{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: instName, Namespace: testNamespace}, inst); err == nil {
				controllerutil.RemoveFinalizer(inst, instanceFinalizer)
				_ = k8sClient.Update(ctx, inst)
				_ = k8sClient.Delete(ctx, inst)
			}
			secret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: testNamespace}, secret); err == nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should add the kubernetes port to Deployment and kubernetes section to ConfigMap", func() {
			nn := types.NamespacedName{Name: instName, Namespace: testNamespace}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// Deployment should have the kubernetes port.
			var deploy appsv1.Deployment
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: instName, Namespace: testNamespace}, &deploy)).To(Succeed())
			ports := deploy.Spec.Template.Spec.Containers[0].Ports
			Expect(ports).To(ContainElement(
				corev1.ContainerPort{Name: "kubernetes", ContainerPort: 8443, Protocol: corev1.ProtocolTCP},
			))

			// ConfigMap should have the kubernetes section.
			var cm corev1.ConfigMap
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: instName + "-config", Namespace: testNamespace,
			}, &cm)).To(Succeed())
			yaml := cm.Data["warpgate.yaml"]
			Expect(yaml).To(ContainSubstring("kubernetes:"))
			Expect(yaml).To(ContainSubstring("enable: true"))
			Expect(yaml).To(ContainSubstring("8443"))
		})
	})

	// -----------------------------------------------------------------------
	// 19. Delete instance with connection cleanup
	// -----------------------------------------------------------------------
	Context("Delete instance with createConnection cleans up connection", func() {
		const (
			instName   = "inst-del-conn"
			secretName = "inst-del-conn-pw"
		)

		BeforeEach(func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: testNamespace,
				},
				Data: map[string][]byte{
					"password": []byte("del-conn-secret"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			inst := &warpgatev1alpha1.WarpgateInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instName,
					Namespace: testNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					Version: "0.21.1",
					AdminPasswordSecretRef: warpgatev1alpha1.SecretKeyRef{
						Name: secretName,
					},
					Replicas: int32Ptr(1),
					HTTP: &warpgatev1alpha1.HTTPListenerSpec{
						Enabled:     boolPtr(true),
						Port:        int32Ptr(8888),
						ServiceType: "ClusterIP",
					},
					Storage: &warpgatev1alpha1.StorageSpec{
						Size: "1Gi",
					},
					TLS: &warpgatev1alpha1.InstanceTLSSpec{
						CertManager: boolPtr(false),
					},
					CreateConnection: boolPtr(true),
				},
			}
			Expect(k8sClient.Create(ctx, inst)).To(Succeed())
		})

		AfterEach(func() {
			// Clean up any leftover connection.
			conn := &warpgatev1alpha1.WarpgateConnection{}
			connNN := types.NamespacedName{Name: instName + "-connection", Namespace: testNamespace}
			if err := k8sClient.Get(ctx, connNN, conn); err == nil {
				controllerutil.RemoveFinalizer(conn, warpgateFinalizer)
				_ = k8sClient.Update(ctx, conn)
				_ = k8sClient.Delete(ctx, conn)
			}

			inst := &warpgatev1alpha1.WarpgateInstance{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: instName, Namespace: testNamespace}, inst); err == nil {
				controllerutil.RemoveFinalizer(inst, instanceFinalizer)
				_ = k8sClient.Update(ctx, inst)
				_ = k8sClient.Delete(ctx, inst)
			}

			authSecret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: instName + "-admin-auth", Namespace: testNamespace}, authSecret); err == nil {
				_ = k8sClient.Delete(ctx, authSecret)
			}

			secret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: testNamespace}, secret); err == nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should delete the WarpgateConnection when the instance is deleted", func() {
			nn := types.NamespacedName{Name: instName, Namespace: testNamespace}
			connNN := types.NamespacedName{Name: instName + "-connection", Namespace: testNamespace}

			// First reconcile: creates instance + connection.
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// Connection should exist.
			var conn warpgatev1alpha1.WarpgateConnection
			Expect(k8sClient.Get(ctx, connNN, &conn)).To(Succeed())

			// Status should have connectionRef.
			var inst warpgatev1alpha1.WarpgateInstance
			Expect(k8sClient.Get(ctx, nn, &inst)).To(Succeed())
			Expect(inst.Status.ConnectionRef).To(Equal(instName + "-connection"))

			// Remove finalizer from connection so deletion can proceed.
			if controllerutil.ContainsFinalizer(&conn, warpgateFinalizer) {
				controllerutil.RemoveFinalizer(&conn, warpgateFinalizer)
				Expect(k8sClient.Update(ctx, &conn)).To(Succeed())
			}

			// Delete the instance.
			Expect(k8sClient.Get(ctx, nn, &inst)).To(Succeed())
			Expect(k8sClient.Delete(ctx, &inst)).To(Succeed())

			// Reconcile deletion — finalizer should clean up the connection.
			_, err = reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// Connection should be gone (deleted by the finalizer).
			err = k8sClient.Get(ctx, connNN, &conn)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))

			// Instance should be fully gone too.
			var deleted warpgatev1alpha1.WarpgateInstance
			err = k8sClient.Get(ctx, nn, &deleted)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("not found"))
		})
	})

	// -----------------------------------------------------------------------
	// 20. Reconcile error on missing admin secret
	// -----------------------------------------------------------------------
	Context("Missing admin password secret with createConnection", func() {
		const (
			instName   = "inst-missing-sec"
			secretName = "inst-missing-sec-pw"
		)

		BeforeEach(func() {
			// Create the admin password secret so the instance passes validation,
			// but we will delete it before reconcile triggers connection creation.
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: testNamespace,
				},
				Data: map[string][]byte{
					"password": []byte("will-be-deleted"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			inst := &warpgatev1alpha1.WarpgateInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instName,
					Namespace: testNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					Version: "0.21.1",
					AdminPasswordSecretRef: warpgatev1alpha1.SecretKeyRef{
						Name: "nonexistent-admin-pw-secret",
					},
					Replicas: int32Ptr(1),
					HTTP: &warpgatev1alpha1.HTTPListenerSpec{
						Enabled:     boolPtr(true),
						Port:        int32Ptr(8888),
						ServiceType: "ClusterIP",
					},
					Storage: &warpgatev1alpha1.StorageSpec{
						Size: "1Gi",
					},
					TLS: &warpgatev1alpha1.InstanceTLSSpec{
						CertManager: boolPtr(false),
					},
					CreateConnection: boolPtr(true),
				},
			}
			Expect(k8sClient.Create(ctx, inst)).To(Succeed())
		})

		AfterEach(func() {
			inst := &warpgatev1alpha1.WarpgateInstance{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: instName, Namespace: testNamespace}, inst); err == nil {
				controllerutil.RemoveFinalizer(inst, instanceFinalizer)
				_ = k8sClient.Update(ctx, inst)
				_ = k8sClient.Delete(ctx, inst)
			}
			secret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: testNamespace}, secret); err == nil {
				_ = k8sClient.Delete(ctx, secret)
			}
			// Clean up auth secret if it was partially created.
			authSecret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: instName + "-admin-auth", Namespace: testNamespace}, authSecret); err == nil {
				_ = k8sClient.Delete(ctx, authSecret)
			}
			conn := &warpgatev1alpha1.WarpgateConnection{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: instName + "-connection", Namespace: testNamespace}, conn); err == nil {
				controllerutil.RemoveFinalizer(conn, warpgateFinalizer)
				_ = k8sClient.Update(ctx, conn)
				_ = k8sClient.Delete(ctx, conn)
			}
		})

		It("should return an error and set a Failed condition when the admin secret is missing", func() {
			nn := types.NamespacedName{Name: instName, Namespace: testNamespace}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("nonexistent-admin-pw-secret"))

			// The instance should have a ConnectionFailed condition.
			var inst warpgatev1alpha1.WarpgateInstance
			Expect(k8sClient.Get(ctx, nn, &inst)).To(Succeed())
			readyCond := findReadyCondition(inst.Status.Conditions)
			Expect(readyCond).NotTo(BeNil())
			Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCond.Reason).To(Equal("ConnectionFailed"))
		})
	})

	// -----------------------------------------------------------------------
	// 21. Helper function unit tests
	// -----------------------------------------------------------------------
	Context("Helper functions", func() {
		It("instanceReplicas returns 1 when Replicas is nil", func() {
			inst := &warpgatev1alpha1.WarpgateInstance{}
			Expect(instanceReplicas(inst)).To(Equal(int32(1)))
		})

		It("instanceReplicas returns the value when set", func() {
			r := int32(5)
			inst := &warpgatev1alpha1.WarpgateInstance{
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{Replicas: &r},
			}
			Expect(instanceReplicas(inst)).To(Equal(int32(5)))
		})

		It("httpEnabled returns true when HTTP is nil", func() {
			inst := &warpgatev1alpha1.WarpgateInstance{}
			Expect(httpEnabled(inst)).To(BeTrue())
		})

		It("httpEnabled returns true when HTTP.Enabled is nil", func() {
			inst := &warpgatev1alpha1.WarpgateInstance{
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					HTTP: &warpgatev1alpha1.HTTPListenerSpec{},
				},
			}
			Expect(httpEnabled(inst)).To(BeTrue())
		})

		It("httpEnabled returns false when explicitly disabled", func() {
			f := false
			inst := &warpgatev1alpha1.WarpgateInstance{
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					HTTP: &warpgatev1alpha1.HTTPListenerSpec{Enabled: &f},
				},
			}
			Expect(httpEnabled(inst)).To(BeFalse())
		})

		It("kubernetesEnabled returns false when Kubernetes is nil", func() {
			inst := &warpgatev1alpha1.WarpgateInstance{}
			Expect(kubernetesEnabled(inst)).To(BeFalse())
		})

		It("kubernetesEnabled returns false when Kubernetes.Enabled is nil", func() {
			inst := &warpgatev1alpha1.WarpgateInstance{
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					Kubernetes: &warpgatev1alpha1.ProtocolListenerSpec{},
				},
			}
			Expect(kubernetesEnabled(inst)).To(BeFalse())
		})

		It("kubernetesEnabled returns true when explicitly enabled", func() {
			t := true
			inst := &warpgatev1alpha1.WarpgateInstance{
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					Kubernetes: &warpgatev1alpha1.ProtocolListenerSpec{Enabled: &t},
				},
			}
			Expect(kubernetesEnabled(inst)).To(BeTrue())
		})

		It("instanceHTTPPort returns 8888 when HTTP is nil", func() {
			inst := &warpgatev1alpha1.WarpgateInstance{}
			Expect(instanceHTTPPort(inst)).To(Equal(int32(8888)))
		})

		It("instanceHTTPPort returns 8888 when HTTP.Port is nil", func() {
			inst := &warpgatev1alpha1.WarpgateInstance{
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					HTTP: &warpgatev1alpha1.HTTPListenerSpec{},
				},
			}
			Expect(instanceHTTPPort(inst)).To(Equal(int32(8888)))
		})

		It("instanceHTTPPort returns the custom value when set", func() {
			p := int32(9999)
			inst := &warpgatev1alpha1.WarpgateInstance{
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					HTTP: &warpgatev1alpha1.HTTPListenerSpec{Port: &p},
				},
			}
			Expect(instanceHTTPPort(inst)).To(Equal(int32(9999)))
		})

		It("instanceKubernetesPort returns 8443 when Kubernetes is nil", func() {
			inst := &warpgatev1alpha1.WarpgateInstance{}
			Expect(instanceKubernetesPort(inst)).To(Equal(int32(8443)))
		})

		It("instanceKubernetesPort returns 8443 when Kubernetes.Port is nil", func() {
			inst := &warpgatev1alpha1.WarpgateInstance{
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					Kubernetes: &warpgatev1alpha1.ProtocolListenerSpec{},
				},
			}
			Expect(instanceKubernetesPort(inst)).To(Equal(int32(8443)))
		})

		It("instanceKubernetesPort returns the custom value when set", func() {
			p := int32(6443)
			inst := &warpgatev1alpha1.WarpgateInstance{
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					Kubernetes: &warpgatev1alpha1.ProtocolListenerSpec{Port: &p},
				},
			}
			Expect(instanceKubernetesPort(inst)).To(Equal(int32(6443)))
		})

		It("instanceStorageSize returns 1Gi when Storage is nil", func() {
			inst := &warpgatev1alpha1.WarpgateInstance{}
			Expect(instanceStorageSize(inst)).To(Equal("1Gi"))
		})

		It("instanceStorageSize returns 1Gi when Storage.Size is empty", func() {
			inst := &warpgatev1alpha1.WarpgateInstance{
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					Storage: &warpgatev1alpha1.StorageSpec{},
				},
			}
			Expect(instanceStorageSize(inst)).To(Equal("1Gi"))
		})

		It("instanceStorageSize returns the custom value when set", func() {
			inst := &warpgatev1alpha1.WarpgateInstance{
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					Storage: &warpgatev1alpha1.StorageSpec{Size: "50Gi"},
				},
			}
			Expect(instanceStorageSize(inst)).To(Equal("50Gi"))
		})

		It("storageEnabled returns true when Storage is nil", func() {
			inst := &warpgatev1alpha1.WarpgateInstance{}
			Expect(storageEnabled(inst)).To(BeTrue())
		})

		It("storageEnabled returns true when Storage.Enabled is nil", func() {
			inst := &warpgatev1alpha1.WarpgateInstance{
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					Storage: &warpgatev1alpha1.StorageSpec{},
				},
			}
			Expect(storageEnabled(inst)).To(BeTrue())
		})

		It("storageEnabled returns false when explicitly disabled", func() {
			f := false
			inst := &warpgatev1alpha1.WarpgateInstance{
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					Storage: &warpgatev1alpha1.StorageSpec{Enabled: &f},
				},
			}
			Expect(storageEnabled(inst)).To(BeFalse())
		})

		It("shouldCreateConnection returns true when CreateConnection is nil", func() {
			inst := &warpgatev1alpha1.WarpgateInstance{}
			Expect(shouldCreateConnection(inst)).To(BeTrue())
		})

		It("shouldCreateConnection returns false when explicitly set to false", func() {
			f := false
			inst := &warpgatev1alpha1.WarpgateInstance{
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{CreateConnection: &f},
			}
			Expect(shouldCreateConnection(inst)).To(BeFalse())
		})

		It("shouldCreateConnection returns true when explicitly set to true", func() {
			t := true
			inst := &warpgatev1alpha1.WarpgateInstance{
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{CreateConnection: &t},
			}
			Expect(shouldCreateConnection(inst)).To(BeTrue())
		})

		It("adminPasswordKey returns 'password' when Key is empty", func() {
			inst := &warpgatev1alpha1.WarpgateInstance{
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					AdminPasswordSecretRef: warpgatev1alpha1.SecretKeyRef{Name: "s"},
				},
			}
			Expect(adminPasswordKey(inst)).To(Equal("password"))
		})

		It("adminPasswordKey returns the custom key when set", func() {
			inst := &warpgatev1alpha1.WarpgateInstance{
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					AdminPasswordSecretRef: warpgatev1alpha1.SecretKeyRef{
						Name: "s",
						Key:  "admin-pw",
					},
				},
			}
			Expect(adminPasswordKey(inst)).To(Equal("admin-pw"))
		})

		It("sshEnabled returns false when SSH is nil", func() {
			inst := &warpgatev1alpha1.WarpgateInstance{}
			Expect(sshEnabled(inst)).To(BeFalse())
		})

		It("mysqlEnabled returns false when MySQL is nil", func() {
			inst := &warpgatev1alpha1.WarpgateInstance{}
			Expect(mysqlEnabled(inst)).To(BeFalse())
		})

		It("pgEnabled returns false when PostgreSQL is nil", func() {
			inst := &warpgatev1alpha1.WarpgateInstance{}
			Expect(pgEnabled(inst)).To(BeFalse())
		})

		It("certManagerEnabled returns false when TLS is nil", func() {
			inst := &warpgatev1alpha1.WarpgateInstance{}
			Expect(certManagerEnabled(inst)).To(BeFalse())
		})

		It("certManagerEnabled returns false when CertManager is nil", func() {
			inst := &warpgatev1alpha1.WarpgateInstance{
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					TLS: &warpgatev1alpha1.InstanceTLSSpec{},
				},
			}
			Expect(certManagerEnabled(inst)).To(BeFalse())
		})

		It("certManagerEnabled returns true when CertManager is true", func() {
			t := true
			inst := &warpgatev1alpha1.WarpgateInstance{
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					TLS: &warpgatev1alpha1.InstanceTLSSpec{CertManager: &t},
				},
			}
			Expect(certManagerEnabled(inst)).To(BeTrue())
		})

		It("tlsSecretProvided returns false when TLS is nil", func() {
			inst := &warpgatev1alpha1.WarpgateInstance{}
			Expect(tlsSecretProvided(inst)).To(BeFalse())
		})

		It("tlsSecretProvided returns false when SecretName is empty", func() {
			inst := &warpgatev1alpha1.WarpgateInstance{
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					TLS: &warpgatev1alpha1.InstanceTLSSpec{},
				},
			}
			Expect(tlsSecretProvided(inst)).To(BeFalse())
		})

		It("tlsSecretProvided returns true when SecretName is set", func() {
			inst := &warpgatev1alpha1.WarpgateInstance{
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					TLS: &warpgatev1alpha1.InstanceTLSSpec{SecretName: "my-tls-secret"},
				},
			}
			Expect(tlsSecretProvided(inst)).To(BeTrue())
		})

		It("hasExistingClaim returns false when Storage is nil", func() {
			inst := &warpgatev1alpha1.WarpgateInstance{}
			Expect(hasExistingClaim(inst)).To(BeFalse())
		})

		It("hasExistingClaim returns true when ExistingClaimName is set", func() {
			inst := &warpgatev1alpha1.WarpgateInstance{
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					Storage: &warpgatev1alpha1.StorageSpec{ExistingClaimName: "my-pvc"},
				},
			}
			Expect(hasExistingClaim(inst)).To(BeTrue())
		})

		It("instanceSSHPort returns 2222 when SSH is nil", func() {
			inst := &warpgatev1alpha1.WarpgateInstance{}
			Expect(instanceSSHPort(inst)).To(Equal(int32(2222)))
		})

		It("instanceMySQLPort returns 33306 when MySQL is nil", func() {
			inst := &warpgatev1alpha1.WarpgateInstance{}
			Expect(instanceMySQLPort(inst)).To(Equal(int32(33306)))
		})

		It("instancePGPort returns 55432 when PostgreSQL is nil", func() {
			inst := &warpgatev1alpha1.WarpgateInstance{}
			Expect(instancePGPort(inst)).To(Equal(int32(55432)))
		})

		It("resolveImage uses custom image when set", func() {
			inst := &warpgatev1alpha1.WarpgateInstance{
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					Version: "0.21.1",
					Image:   "custom-registry.io/warpgate:custom",
				},
			}
			Expect(resolveImage(inst)).To(Equal("custom-registry.io/warpgate:custom"))
		})

		It("resolveImage builds default image from version", func() {
			inst := &warpgatev1alpha1.WarpgateInstance{
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					Version: "0.21.1",
				},
			}
			Expect(resolveImage(inst)).To(Equal("ghcr.io/warp-tech/warpgate:v0.21.1"))
		})

		It("resolveImage does not double-prefix 'v'", func() {
			inst := &warpgatev1alpha1.WarpgateInstance{
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					Version: "v0.21.1",
				},
			}
			Expect(resolveImage(inst)).To(Equal("ghcr.io/warp-tech/warpgate:v0.21.1"))
		})

		It("configHash changes when DatabaseURL changes", func() {
			base := &warpgatev1alpha1.WarpgateInstance{
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{Version: "0.21.1"},
			}
			withDB := &warpgatev1alpha1.WarpgateInstance{
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					Version:     "0.21.1",
					DatabaseURL: "postgres://host/db",
				},
			}
			Expect(configHash(base)).NotTo(Equal(configHash(withDB)))
		})

		It("configHash changes when SSHKeysSecretName changes", func() {
			base := &warpgatev1alpha1.WarpgateInstance{
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{Version: "0.21.1"},
			}
			withKeys := &warpgatev1alpha1.WarpgateInstance{
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					Version:           "0.21.1",
					SSHKeysSecretName: "my-keys",
				},
			}
			Expect(configHash(base)).NotTo(Equal(configHash(withKeys)))
		})
	})

	// -----------------------------------------------------------------------
	// 22. buildWarpgateConfig edge cases
	// -----------------------------------------------------------------------
	Context("buildWarpgateConfig edge cases", func() {
		It("should include database_url postgres section when DatabaseURL is set", func() {
			inst := &warpgatev1alpha1.WarpgateInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cfg-dburl",
					Namespace: testNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					Version:     "0.21.1",
					DatabaseURL: "postgres://user:pass@host:5432/warpgate",
					HTTP: &warpgatev1alpha1.HTTPListenerSpec{
						Enabled: boolPtr(true),
						Port:    int32Ptr(8888),
					},
				},
			}
			yaml := reconciler.buildWarpgateConfig(inst)
			Expect(yaml).To(ContainSubstring("postgres: \"postgres://user:pass@host:5432/warpgate\""))
			Expect(yaml).NotTo(ContainSubstring("sqlite"))
		})

		It("should use sqlite when DatabaseURL is empty", func() {
			inst := &warpgatev1alpha1.WarpgateInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cfg-sqlite",
					Namespace: testNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					Version: "0.21.1",
					HTTP: &warpgatev1alpha1.HTTPListenerSpec{
						Enabled: boolPtr(true),
						Port:    int32Ptr(8888),
					},
				},
			}
			yaml := reconciler.buildWarpgateConfig(inst)
			Expect(yaml).To(ContainSubstring("sqlite:"))
			Expect(yaml).To(ContainSubstring("path: /data/db"))
		})

		It("should include kubernetes section when enabled", func() {
			t := true
			p := int32(6443)
			inst := &warpgatev1alpha1.WarpgateInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cfg-k8s",
					Namespace: testNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					Version: "0.21.1",
					HTTP: &warpgatev1alpha1.HTTPListenerSpec{
						Enabled: boolPtr(true),
						Port:    int32Ptr(8888),
					},
					Kubernetes: &warpgatev1alpha1.ProtocolListenerSpec{
						Enabled: &t,
						Port:    &p,
					},
				},
			}
			yaml := reconciler.buildWarpgateConfig(inst)
			Expect(yaml).To(ContainSubstring("kubernetes:"))
			Expect(yaml).To(ContainSubstring("enable: true"))
			Expect(yaml).To(ContainSubstring("0.0.0.0:6443"))
		})

		It("should include recordings section when RecordSessions is true", func() {
			t := true
			inst := &warpgatev1alpha1.WarpgateInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cfg-rec",
					Namespace: testNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					Version:        "0.21.1",
					RecordSessions: &t,
					HTTP: &warpgatev1alpha1.HTTPListenerSpec{
						Enabled: boolPtr(true),
						Port:    int32Ptr(8888),
					},
				},
			}
			yaml := reconciler.buildWarpgateConfig(inst)
			Expect(yaml).To(ContainSubstring("recordings:"))
			Expect(yaml).To(ContainSubstring("enable: true"))
			Expect(yaml).To(ContainSubstring("path: /data/recordings"))
		})

		It("should set HTTP enable: false when HTTP is disabled", func() {
			f := false
			inst := &warpgatev1alpha1.WarpgateInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cfg-no-http",
					Namespace: testNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					Version: "0.21.1",
					HTTP: &warpgatev1alpha1.HTTPListenerSpec{
						Enabled: &f,
						Port:    int32Ptr(8888),
					},
				},
			}
			yaml := reconciler.buildWarpgateConfig(inst)
			Expect(yaml).To(ContainSubstring("http:"))
			Expect(yaml).To(ContainSubstring("enable: false"))
		})
	})

	// -----------------------------------------------------------------------
	// 23. buildDeployment edge cases
	// -----------------------------------------------------------------------
	Context("buildDeployment variations", func() {
		It("should include config-override volume mount when ConfigOverride is set", func() {
			inst := &warpgatev1alpha1.WarpgateInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deploy-override",
					Namespace: testNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					Version: "0.21.1",
					AdminPasswordSecretRef: warpgatev1alpha1.SecretKeyRef{
						Name: "dummy",
					},
					Replicas: int32Ptr(1),
					HTTP: &warpgatev1alpha1.HTTPListenerSpec{
						Enabled: boolPtr(true),
						Port:    int32Ptr(8888),
					},
					Storage: &warpgatev1alpha1.StorageSpec{
						Size: "1Gi",
					},
					TLS: &warpgatev1alpha1.InstanceTLSSpec{
						CertManager: boolPtr(false),
					},
					ConfigOverride: "custom: override",
				},
			}
			deploy := reconciler.buildDeployment(inst)
			initMounts := deploy.Spec.Template.Spec.InitContainers[0].VolumeMounts
			foundOverride := false
			for _, m := range initMounts {
				if m.Name == "config-override" && m.MountPath == "/override" {
					foundOverride = true
					break
				}
			}
			Expect(foundOverride).To(BeTrue(), "expected config-override mount in init container")

			// Init script should mention applying config override.
			initCmd := deploy.Spec.Template.Spec.InitContainers[0].Command[2]
			Expect(initCmd).To(ContainSubstring("Applying config override"))
			Expect(initCmd).To(ContainSubstring("cp /override/warpgate.yaml /data/warpgate.yaml"))
		})

		It("should include SSH keys volume mount when SSHKeysSecretName is set", func() {
			inst := &warpgatev1alpha1.WarpgateInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deploy-ssh-keys",
					Namespace: testNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					Version: "0.21.1",
					AdminPasswordSecretRef: warpgatev1alpha1.SecretKeyRef{
						Name: "dummy",
					},
					Replicas: int32Ptr(1),
					HTTP: &warpgatev1alpha1.HTTPListenerSpec{
						Enabled: boolPtr(true),
						Port:    int32Ptr(8888),
					},
					Storage: &warpgatev1alpha1.StorageSpec{
						Size: "1Gi",
					},
					TLS: &warpgatev1alpha1.InstanceTLSSpec{
						CertManager: boolPtr(false),
					},
					SSHKeysSecretName: "my-ssh-keys",
				},
			}
			deploy := reconciler.buildDeployment(inst)

			// Volume should exist.
			foundVol := false
			for _, vol := range deploy.Spec.Template.Spec.Volumes {
				if vol.Name == "ssh-keys" && vol.Secret != nil && vol.Secret.SecretName == "my-ssh-keys" {
					foundVol = true
					break
				}
			}
			Expect(foundVol).To(BeTrue(), "expected ssh-keys volume")

			// Init container mount should exist.
			initMounts := deploy.Spec.Template.Spec.InitContainers[0].VolumeMounts
			foundMount := false
			for _, m := range initMounts {
				if m.Name == "ssh-keys" && m.MountPath == "/ssh-keys" {
					foundMount = true
					break
				}
			}
			Expect(foundMount).To(BeTrue(), "expected ssh-keys mount in init container")

			// Init script should mention SSH keys.
			initCmd := deploy.Spec.Template.Spec.InitContainers[0].Command[2]
			Expect(initCmd).To(ContainSubstring("Copying SSH keys"))
		})

		It("should include TLS secret volume mount when tls.secretName is set", func() {
			inst := &warpgatev1alpha1.WarpgateInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deploy-tls-sec",
					Namespace: testNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					Version: "0.21.1",
					AdminPasswordSecretRef: warpgatev1alpha1.SecretKeyRef{
						Name: "dummy",
					},
					Replicas: int32Ptr(1),
					HTTP: &warpgatev1alpha1.HTTPListenerSpec{
						Enabled: boolPtr(true),
						Port:    int32Ptr(8888),
					},
					Storage: &warpgatev1alpha1.StorageSpec{
						Size: "1Gi",
					},
					TLS: &warpgatev1alpha1.InstanceTLSSpec{
						CertManager: boolPtr(false),
						SecretName:  "my-tls-cert",
					},
				},
			}
			deploy := reconciler.buildDeployment(inst)

			// Volume should reference the TLS secret.
			foundVol := false
			for _, vol := range deploy.Spec.Template.Spec.Volumes {
				if vol.Name == "tls-secret" && vol.Secret != nil && vol.Secret.SecretName == "my-tls-cert" {
					foundVol = true
					break
				}
			}
			Expect(foundVol).To(BeTrue(), "expected tls-secret volume")

			// Init container mount should exist.
			initMounts := deploy.Spec.Template.Spec.InitContainers[0].VolumeMounts
			foundMount := false
			for _, m := range initMounts {
				if m.Name == "tls-secret" && m.MountPath == "/tls-secret" {
					foundMount = true
					break
				}
			}
			Expect(foundMount).To(BeTrue(), "expected tls-secret mount in init container")

			// Init script should NOT include self-signed cert generation.
			initCmd := deploy.Spec.Template.Spec.InitContainers[0].Command[2]
			Expect(initCmd).To(ContainSubstring("Copying TLS certificates"))
			Expect(initCmd).NotTo(ContainSubstring("Generating self-signed TLS"))
		})

		It("should include kubernetes port and --kubernetes-port in setup when enabled", func() {
			t := true
			p := int32(8443)
			inst := &warpgatev1alpha1.WarpgateInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deploy-k8s",
					Namespace: testNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					Version: "0.21.1",
					AdminPasswordSecretRef: warpgatev1alpha1.SecretKeyRef{
						Name: "dummy",
					},
					Replicas: int32Ptr(1),
					HTTP: &warpgatev1alpha1.HTTPListenerSpec{
						Enabled: boolPtr(true),
						Port:    int32Ptr(8888),
					},
					Kubernetes: &warpgatev1alpha1.ProtocolListenerSpec{
						Enabled: &t,
						Port:    &p,
					},
					Storage: &warpgatev1alpha1.StorageSpec{
						Size: "1Gi",
					},
					TLS: &warpgatev1alpha1.InstanceTLSSpec{
						CertManager: boolPtr(false),
					},
				},
			}
			deploy := reconciler.buildDeployment(inst)

			// Container ports should include kubernetes.
			ports := deploy.Spec.Template.Spec.Containers[0].Ports
			Expect(ports).To(ContainElement(
				corev1.ContainerPort{Name: "kubernetes", ContainerPort: 8443, Protocol: corev1.ProtocolTCP},
			))

			// Init script should contain --kubernetes-port.
			initCmd := deploy.Spec.Template.Spec.InitContainers[0].Command[2]
			Expect(initCmd).To(ContainSubstring("--kubernetes-port 8443"))
		})

		It("should include --database-url in setup when DatabaseURL is set", func() {
			inst := &warpgatev1alpha1.WarpgateInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deploy-dburl",
					Namespace: testNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					Version: "0.21.1",
					AdminPasswordSecretRef: warpgatev1alpha1.SecretKeyRef{
						Name: "dummy",
					},
					Replicas: int32Ptr(1),
					HTTP: &warpgatev1alpha1.HTTPListenerSpec{
						Enabled: boolPtr(true),
						Port:    int32Ptr(8888),
					},
					Storage: &warpgatev1alpha1.StorageSpec{
						Size: "1Gi",
					},
					TLS: &warpgatev1alpha1.InstanceTLSSpec{
						CertManager: boolPtr(false),
					},
					DatabaseURL: "postgres://host/db",
				},
			}
			deploy := reconciler.buildDeployment(inst)
			initCmd := deploy.Spec.Template.Spec.InitContainers[0].Command[2]
			Expect(initCmd).To(ContainSubstring("--database-url"))
			Expect(initCmd).To(ContainSubstring("postgres://host/db"))
		})

		It("should use emptyDir volume when storage is disabled", func() {
			f := false
			inst := &warpgatev1alpha1.WarpgateInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deploy-emptydir",
					Namespace: testNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					Version: "0.21.1",
					AdminPasswordSecretRef: warpgatev1alpha1.SecretKeyRef{
						Name: "dummy",
					},
					Replicas: int32Ptr(1),
					HTTP: &warpgatev1alpha1.HTTPListenerSpec{
						Enabled: boolPtr(true),
						Port:    int32Ptr(8888),
					},
					Storage: &warpgatev1alpha1.StorageSpec{
						Enabled: &f,
					},
					TLS: &warpgatev1alpha1.InstanceTLSSpec{
						CertManager: boolPtr(false),
					},
				},
			}
			deploy := reconciler.buildDeployment(inst)
			foundEmptyDir := false
			for _, vol := range deploy.Spec.Template.Spec.Volumes {
				if vol.Name == "data" && vol.EmptyDir != nil {
					foundEmptyDir = true
					break
				}
			}
			Expect(foundEmptyDir).To(BeTrue(), "expected emptyDir for data volume")
		})

		It("should use RollingUpdate strategy when configured", func() {
			inst := &warpgatev1alpha1.WarpgateInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deploy-rolling",
					Namespace: testNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					Version: "0.21.1",
					AdminPasswordSecretRef: warpgatev1alpha1.SecretKeyRef{
						Name: "dummy",
					},
					Replicas: int32Ptr(1),
					Strategy: "RollingUpdate",
					HTTP: &warpgatev1alpha1.HTTPListenerSpec{
						Enabled: boolPtr(true),
						Port:    int32Ptr(8888),
					},
					Storage: &warpgatev1alpha1.StorageSpec{
						Size: "1Gi",
					},
					TLS: &warpgatev1alpha1.InstanceTLSSpec{
						CertManager: boolPtr(false),
					},
				},
			}
			deploy := reconciler.buildDeployment(inst)
			Expect(deploy.Spec.Strategy.Type).To(Equal(appsv1.RollingUpdateDeploymentStrategyType))
		})

		It("should use custom adminPasswordKey in init container env", func() {
			inst := &warpgatev1alpha1.WarpgateInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deploy-custom-key",
					Namespace: testNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					Version: "0.21.1",
					AdminPasswordSecretRef: warpgatev1alpha1.SecretKeyRef{
						Name: "my-secret",
						Key:  "admin-pw",
					},
					Replicas: int32Ptr(1),
					HTTP: &warpgatev1alpha1.HTTPListenerSpec{
						Enabled: boolPtr(true),
						Port:    int32Ptr(8888),
					},
					Storage: &warpgatev1alpha1.StorageSpec{
						Size: "1Gi",
					},
					TLS: &warpgatev1alpha1.InstanceTLSSpec{
						CertManager: boolPtr(false),
					},
				},
			}
			deploy := reconciler.buildDeployment(inst)
			initEnv := deploy.Spec.Template.Spec.InitContainers[0].Env
			Expect(initEnv).To(HaveLen(1))
			Expect(initEnv[0].ValueFrom.SecretKeyRef.Key).To(Equal("admin-pw"))
		})

		It("should not have probes when HTTP is disabled", func() {
			f := false
			inst := &warpgatev1alpha1.WarpgateInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deploy-no-probes",
					Namespace: testNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					Version: "0.21.1",
					AdminPasswordSecretRef: warpgatev1alpha1.SecretKeyRef{
						Name: "dummy",
					},
					Replicas: int32Ptr(1),
					HTTP: &warpgatev1alpha1.HTTPListenerSpec{
						Enabled: &f,
						Port:    int32Ptr(8888),
					},
					SSH: &warpgatev1alpha1.SSHListenerSpec{
						Enabled: boolPtr(true),
						Port:    int32Ptr(2222),
					},
					Storage: &warpgatev1alpha1.StorageSpec{
						Size: "1Gi",
					},
					TLS: &warpgatev1alpha1.InstanceTLSSpec{
						CertManager: boolPtr(false),
					},
				},
			}
			deploy := reconciler.buildDeployment(inst)
			container := deploy.Spec.Template.Spec.Containers[0]
			Expect(container.LivenessProbe).To(BeNil())
			Expect(container.ReadinessProbe).To(BeNil())
		})

		It("should use existing claim name when ExistingClaimName is set", func() {
			inst := &warpgatev1alpha1.WarpgateInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deploy-existing-pvc",
					Namespace: testNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					Version: "0.21.1",
					AdminPasswordSecretRef: warpgatev1alpha1.SecretKeyRef{
						Name: "dummy",
					},
					Replicas: int32Ptr(1),
					HTTP: &warpgatev1alpha1.HTTPListenerSpec{
						Enabled: boolPtr(true),
						Port:    int32Ptr(8888),
					},
					Storage: &warpgatev1alpha1.StorageSpec{
						ExistingClaimName: "pre-existing-pvc",
					},
					TLS: &warpgatev1alpha1.InstanceTLSSpec{
						CertManager: boolPtr(false),
					},
				},
			}
			deploy := reconciler.buildDeployment(inst)
			foundPVC := false
			for _, vol := range deploy.Spec.Template.Spec.Volumes {
				if vol.Name == "data" && vol.PersistentVolumeClaim != nil {
					Expect(vol.PersistentVolumeClaim.ClaimName).To(Equal("pre-existing-pvc"))
					foundPVC = true
					break
				}
			}
			Expect(foundPVC).To(BeTrue())
		})

		It("should generate self-signed TLS when no TLS secret and no cert-manager", func() {
			inst := &warpgatev1alpha1.WarpgateInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deploy-selfcert",
					Namespace: testNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					Version: "0.21.1",
					AdminPasswordSecretRef: warpgatev1alpha1.SecretKeyRef{
						Name: "dummy",
					},
					Replicas: int32Ptr(1),
					HTTP: &warpgatev1alpha1.HTTPListenerSpec{
						Enabled: boolPtr(true),
						Port:    int32Ptr(8888),
					},
					Storage: &warpgatev1alpha1.StorageSpec{
						Size: "1Gi",
					},
					TLS: &warpgatev1alpha1.InstanceTLSSpec{
						CertManager: boolPtr(false),
					},
				},
			}
			deploy := reconciler.buildDeployment(inst)
			initCmd := deploy.Spec.Template.Spec.InitContainers[0].Command[2]
			Expect(initCmd).To(ContainSubstring("Generating self-signed TLS certificate"))
		})
	})

	// -----------------------------------------------------------------------
	// 24. createConnection=nil defaults to true
	// -----------------------------------------------------------------------
	Context("createConnection nil defaults to true", func() {
		const (
			instName   = "inst-conn-nil"
			secretName = "inst-conn-nil-pw"
		)

		BeforeEach(func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: testNamespace,
				},
				Data: map[string][]byte{
					"password": []byte("conn-nil-secret"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			inst := &warpgatev1alpha1.WarpgateInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instName,
					Namespace: testNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					Version: "0.21.1",
					AdminPasswordSecretRef: warpgatev1alpha1.SecretKeyRef{
						Name: secretName,
					},
					Replicas: int32Ptr(1),
					HTTP: &warpgatev1alpha1.HTTPListenerSpec{
						Enabled:     boolPtr(true),
						Port:        int32Ptr(8888),
						ServiceType: "ClusterIP",
					},
					Storage: &warpgatev1alpha1.StorageSpec{
						Size: "1Gi",
					},
					TLS: &warpgatev1alpha1.InstanceTLSSpec{
						CertManager: boolPtr(false),
					},
					// CreateConnection deliberately not set (nil) -- defaults to true.
				},
			}
			Expect(k8sClient.Create(ctx, inst)).To(Succeed())
		})

		AfterEach(func() {
			conn := &warpgatev1alpha1.WarpgateConnection{}
			connNN := types.NamespacedName{Name: instName + "-connection", Namespace: testNamespace}
			if err := k8sClient.Get(ctx, connNN, conn); err == nil {
				controllerutil.RemoveFinalizer(conn, warpgateFinalizer)
				_ = k8sClient.Update(ctx, conn)
				_ = k8sClient.Delete(ctx, conn)
			}

			inst := &warpgatev1alpha1.WarpgateInstance{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: instName, Namespace: testNamespace}, inst); err == nil {
				controllerutil.RemoveFinalizer(inst, instanceFinalizer)
				_ = k8sClient.Update(ctx, inst)
				_ = k8sClient.Delete(ctx, inst)
			}

			authSecret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: instName + "-admin-auth", Namespace: testNamespace}, authSecret); err == nil {
				_ = k8sClient.Delete(ctx, authSecret)
			}

			secret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: testNamespace}, secret); err == nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should create a WarpgateConnection when CreateConnection is nil", func() {
			nn := types.NamespacedName{Name: instName, Namespace: testNamespace}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			var conn warpgatev1alpha1.WarpgateConnection
			connNN := types.NamespacedName{Name: instName + "-connection", Namespace: testNamespace}
			Expect(k8sClient.Get(ctx, connNN, &conn)).To(Succeed())
			Expect(conn.Spec.Host).To(ContainSubstring(instName + "-http"))
		})
	})

	// -----------------------------------------------------------------------
	// 25. Session recordings in config
	// -----------------------------------------------------------------------
	Context("Session recordings enabled", func() {
		const (
			instName   = "inst-rec"
			secretName = "inst-rec-pw"
		)

		BeforeEach(func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: testNamespace,
				},
				Data: map[string][]byte{
					"password": []byte("rec-secret"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			inst := &warpgatev1alpha1.WarpgateInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instName,
					Namespace: testNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					Version: "0.21.1",
					AdminPasswordSecretRef: warpgatev1alpha1.SecretKeyRef{
						Name: secretName,
					},
					Replicas: int32Ptr(1),
					HTTP: &warpgatev1alpha1.HTTPListenerSpec{
						Enabled:     boolPtr(true),
						Port:        int32Ptr(8888),
						ServiceType: "ClusterIP",
					},
					Storage: &warpgatev1alpha1.StorageSpec{
						Size: "1Gi",
					},
					TLS: &warpgatev1alpha1.InstanceTLSSpec{
						CertManager: boolPtr(false),
					},
					CreateConnection: boolPtr(false),
					RecordSessions:   boolPtr(true),
				},
			}
			Expect(k8sClient.Create(ctx, inst)).To(Succeed())
		})

		AfterEach(func() {
			inst := &warpgatev1alpha1.WarpgateInstance{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: instName, Namespace: testNamespace}, inst); err == nil {
				controllerutil.RemoveFinalizer(inst, instanceFinalizer)
				_ = k8sClient.Update(ctx, inst)
				_ = k8sClient.Delete(ctx, inst)
			}
			secret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: testNamespace}, secret); err == nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should include recordings section in ConfigMap", func() {
			nn := types.NamespacedName{Name: instName, Namespace: testNamespace}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			var cm corev1.ConfigMap
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: instName + "-config", Namespace: testNamespace,
			}, &cm)).To(Succeed())

			yaml := cm.Data["warpgate.yaml"]
			Expect(yaml).To(ContainSubstring("recordings:"))
			Expect(yaml).To(ContainSubstring("enable: true"))
			Expect(yaml).To(ContainSubstring("path: /data/recordings"))
		})
	})

	// -----------------------------------------------------------------------
	// 26. Custom admin password key
	// -----------------------------------------------------------------------
	Context("Custom admin password key with createConnection", func() {
		const (
			instName   = "inst-custom-key"
			secretName = "inst-custom-key-pw"
		)

		BeforeEach(func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: testNamespace,
				},
				Data: map[string][]byte{
					"admin-pw": []byte("custom-key-secret"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			inst := &warpgatev1alpha1.WarpgateInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instName,
					Namespace: testNamespace,
				},
				Spec: warpgatev1alpha1.WarpgateInstanceSpec{
					Version: "0.21.1",
					AdminPasswordSecretRef: warpgatev1alpha1.SecretKeyRef{
						Name: secretName,
						Key:  "admin-pw",
					},
					Replicas: int32Ptr(1),
					HTTP: &warpgatev1alpha1.HTTPListenerSpec{
						Enabled:     boolPtr(true),
						Port:        int32Ptr(8888),
						ServiceType: "ClusterIP",
					},
					Storage: &warpgatev1alpha1.StorageSpec{
						Size: "1Gi",
					},
					TLS: &warpgatev1alpha1.InstanceTLSSpec{
						CertManager: boolPtr(false),
					},
					CreateConnection: boolPtr(true),
				},
			}
			Expect(k8sClient.Create(ctx, inst)).To(Succeed())
		})

		AfterEach(func() {
			conn := &warpgatev1alpha1.WarpgateConnection{}
			connNN := types.NamespacedName{Name: instName + "-connection", Namespace: testNamespace}
			if err := k8sClient.Get(ctx, connNN, conn); err == nil {
				controllerutil.RemoveFinalizer(conn, warpgateFinalizer)
				_ = k8sClient.Update(ctx, conn)
				_ = k8sClient.Delete(ctx, conn)
			}

			inst := &warpgatev1alpha1.WarpgateInstance{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: instName, Namespace: testNamespace}, inst); err == nil {
				controllerutil.RemoveFinalizer(inst, instanceFinalizer)
				_ = k8sClient.Update(ctx, inst)
				_ = k8sClient.Delete(ctx, inst)
			}

			authSecret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: instName + "-admin-auth", Namespace: testNamespace}, authSecret); err == nil {
				_ = k8sClient.Delete(ctx, authSecret)
			}

			secret := &corev1.Secret{}
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: testNamespace}, secret); err == nil {
				_ = k8sClient.Delete(ctx, secret)
			}
		})

		It("should read the password from the custom key and create the auth secret", func() {
			nn := types.NamespacedName{Name: instName, Namespace: testNamespace}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// Auth secret should use the password from the custom key.
			var authSecret corev1.Secret
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: instName + "-admin-auth", Namespace: testNamespace,
			}, &authSecret)).To(Succeed())
			Expect(authSecret.Data["password"]).To(Equal([]byte("custom-key-secret")))
			Expect(authSecret.Data["username"]).To(Equal([]byte("admin")))
		})
	})
})
