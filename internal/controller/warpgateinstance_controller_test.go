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
})
