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

		It("should create ConfigMap, StatefulSet, HTTP Service, and set status", func() {
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

			// Verify StatefulSet is created with the right image and ports.
			var sts appsv1.StatefulSet
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

			// StatefulSet should have both ports.
			var sts appsv1.StatefulSet
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

		It("should update the StatefulSet image when version changes", func() {
			nn := types.NamespacedName{Name: instName, Namespace: testNamespace}

			// First reconcile creates all resources.
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			// Verify initial image.
			var sts appsv1.StatefulSet
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: instName, Namespace: testNamespace}, &sts)).To(Succeed())
			Expect(sts.Spec.Template.Spec.Containers[0].Image).To(Equal("ghcr.io/warp-tech/warpgate:v0.20.0"))

			// Update the version.
			var inst warpgatev1alpha1.WarpgateInstance
			Expect(k8sClient.Get(ctx, nn, &inst)).To(Succeed())
			inst.Spec.Version = "0.21.1"
			Expect(k8sClient.Update(ctx, &inst)).To(Succeed())

			// Second reconcile should update the StatefulSet.
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

			// StatefulSet should have all four protocol ports.
			var sts appsv1.StatefulSet
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

		It("should add MySQL and PostgreSQL ports to the StatefulSet", func() {
			nn := types.NamespacedName{Name: instName, Namespace: testNamespace}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			var sts appsv1.StatefulSet
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

			var sts appsv1.StatefulSet
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

			// StatefulSet should only have SSH port, no HTTP port.
			var sts appsv1.StatefulSet
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
	// 6. Update triggers StatefulSet update (replicas change)
	// -----------------------------------------------------------------------
	Context("Update triggers StatefulSet update", func() {
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

		It("should update StatefulSet replicas when spec.replicas changes", func() {
			nn := types.NamespacedName{Name: instName, Namespace: testNamespace}

			// First reconcile creates with 1 replica.
			_, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
			Expect(err).NotTo(HaveOccurred())

			var sts appsv1.StatefulSet
			Expect(k8sClient.Get(ctx, types.NamespacedName{Name: instName, Namespace: testNamespace}, &sts)).To(Succeed())
			Expect(*sts.Spec.Replicas).To(Equal(int32(1)))

			// Change replicas to 3.
			var inst warpgatev1alpha1.WarpgateInstance
			Expect(k8sClient.Get(ctx, nn, &inst)).To(Succeed())
			inst.Spec.Replicas = int32Ptr(3)
			Expect(k8sClient.Update(ctx, &inst)).To(Succeed())

			// Second reconcile should update the StatefulSet.
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

			// StatefulSet should still be created despite pre-existing PVC.
			var sts appsv1.StatefulSet
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
})
