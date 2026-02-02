/*
SPDX-License-Identifier: Apache-2.0

Copyright Contributors to the Submariner project.

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

package uninstall_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/submariner-io/admiral/pkg/fake"
	"github.com/submariner-io/admiral/pkg/names"
	"github.com/submariner-io/subctl/pkg/uninstall"
	operatorv1alpha1 "github.com/submariner-io/submariner-operator/api/v1alpha1"
	opnames "github.com/submariner-io/submariner-operator/pkg/names"
	submarinerv1 "github.com/submariner-io/submariner/pkg/apis/submariner.io/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	controllerclient "sigs.k8s.io/controller-runtime/pkg/client"
	controllerfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("All", func() {
	t := newTestDriver()

	assertUninstallSuccess := func() {
		Expect(uninstall.All(t.clients, testClusterName, testSubmarinerNS, t.statusReporter)).To(Succeed())
	}

	It("should uninstall all Submariner components", func(ctx SpecContext) {
		assertUninstallSuccess()
		t.assertComponentsDeleted(ctx)
	})

	When("only ServiceDiscovery is installed", func() {
		BeforeEach(func(ctx SpecContext) {
			t.submariner = nil

			Expect(t.clients.ForGeneral().Create(ctx, &operatorv1alpha1.ServiceDiscovery{
				ObjectMeta: metav1.ObjectMeta{
					Name:      opnames.ServiceDiscoveryCrName,
					Namespace: testSubmarinerNS,
				},
			})).To(Succeed())
		})

		It("should delete the ServiceDiscovery resource", func(ctx SpecContext) {
			assertUninstallSuccess()

			// Verify ServiceDiscovery resource is deleted
			err := t.clients.ForGeneral().Get(ctx, controllerclient.ObjectKey{
				Namespace: testSubmarinerNS,
				Name:      opnames.ServiceDiscoveryCrName,
			}, &operatorv1alpha1.ServiceDiscovery{})
			Expect(apierrors.IsNotFound(err)).To(BeTrue())

			t.assertComponentsDeleted(ctx)
		})
	})

	When("neither Connectivity or ServiceDiscovery is installed", func() {
		BeforeEach(func() {
			t.submariner = nil
		})

		It("should uninstall all remaining Submariner components", func(ctx SpecContext) {
			assertUninstallSuccess()
			t.assertComponentsDeleted(ctx)
		})
	})

	When("the broker is in use by other clusters", func() {
		BeforeEach(func(ctx SpecContext) {
			Expect(t.clients.ForGeneral().Create(ctx, &submarinerv1.Endpoint{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "remote-endpoint",
					Namespace: testBrokerNS,
				},
				Spec: submarinerv1.EndpointSpec{
					ClusterID: "other-cluster",
				},
			})).To(Succeed())

			_, err := t.clients.ForKubernetes().RbacV1().ClusterRoles().Create(ctx, &rbacv1.ClusterRole{
				ObjectMeta: metav1.ObjectMeta{
					Name: names.GatewayComponent,
				},
			}, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			_, err = t.clients.ForKubernetes().RbacV1().ClusterRoleBindings().Create(ctx, &rbacv1.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name: names.GatewayComponent,
				},
				RoleRef: rbacv1.RoleRef{
					Name: names.GatewayComponent,
				},
			}, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should not delete broker and operator resources", func(ctx SpecContext) {
			assertUninstallSuccess()

			// Verify broker namespace still exists
			_, err := t.clients.ForKubernetes().CoreV1().Namespaces().Get(ctx, testBrokerNS, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())

			// Verify CRDs still exist
			err = t.clients.ForGeneral().Get(ctx, controllerclient.ObjectKey{Name: endpointsCRDName}, &apiextensionsv1.CustomResourceDefinition{})
			Expect(err).NotTo(HaveOccurred())

			// Verify Submariner namespace still exists
			_, err = t.clients.ForKubernetes().CoreV1().Namespaces().Get(ctx, testSubmarinerNS, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())

			// Verify the operator ClusterRoles still exists
			_, err = t.clients.ForKubernetes().RbacV1().ClusterRoleBindings().Get(ctx, names.OperatorComponent, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())

			_, err = t.clients.ForKubernetes().RbacV1().ClusterRoles().Get(ctx, names.OperatorComponent, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())

			// Verify other submariner ClusterRoles are deleted
			_, err = t.clients.ForKubernetes().RbacV1().ClusterRoles().Get(ctx, names.GatewayComponent, metav1.GetOptions{})
			Expect(apierrors.IsNotFound(err)).To(BeTrue())

			_, err = t.clients.ForKubernetes().RbacV1().ClusterRoleBindings().Get(ctx, names.GatewayComponent, metav1.GetOptions{})
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
		})
	})

	When("no broker namespace is found", func() {
		BeforeEach(func() {
			t.broker = nil
		})

		It("should uninstall all Submariner components", func(ctx SpecContext) {
			assertUninstallSuccess()
			t.assertComponentsDeleted(ctx)
		})
	})

	When("the Submariner resource isn't initially deleted due to a finalizer present", func() {
		BeforeEach(func() {
			t.submariner.Finalizers = []string{opnames.CleanupFinalizer}
		})

		Context("and the operator deployment does not exist", func() {
			It("should log a warning and eventually delete it", func(ctx SpecContext) {
				assertUninstallSuccess()
				t.assertComponentsDeleted(ctx)
				t.statusReporter.AssertWarningContainsStrings("deployment does not exist")
			})
		})

		Context("and the operator deployment exists", func() {
			BeforeEach(func(ctx SpecContext) {
				_, err := t.clients.ForKubernetes().AppsV1().Deployments(testSubmarinerNS).Create(ctx, &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name: names.OperatorComponent,
					},
					Spec: appsv1.DeploymentSpec{
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{"app": names.OperatorComponent},
							},
						},
					},
				}, metav1.CreateOptions{})
				Expect(err).NotTo(HaveOccurred())
			})

			Context("but no pod exists", func() {
				It("should log a warning and eventually delete it", func(ctx SpecContext) {
					assertUninstallSuccess()
					t.assertComponentsDeleted(ctx)
					t.statusReporter.AssertWarningContainsStrings("pod does not exist")
				})
			})

			Context("and the pod is not running", func() {
				BeforeEach(func(ctx SpecContext) {
					t.createOperatorPod(ctx, corev1.PodSucceeded)
				})

				It("should log a warning and eventually delete it", func(ctx SpecContext) {
					assertUninstallSuccess()
					t.assertComponentsDeleted(ctx)
					t.statusReporter.AssertWarningContainsStrings("pod is not running")
				})
			})

			Context("and the pod is running", func() {
				BeforeEach(func(ctx SpecContext) {
					t.createOperatorPod(ctx, corev1.PodRunning)
				})

				It("should fail", func() {
					Expect(uninstall.All(t.clients, testClusterName, testSubmarinerNS, t.statusReporter)).NotTo(Succeed())
					t.statusReporter.AssertFailureContainsStrings("did not complete deletion")
				})
			})
		})
	})

	testFailures(t)
})

func testFailures(t *testDriver) {
	testUninstallError := func() {
		It("should return an error", func() {
			Expect(uninstall.All(t.clients, testClusterName, testSubmarinerNS, t.statusReporter)).NotTo(Succeed())
		})
	}

	When("node update fails", func() {
		BeforeEach(func() {
			fake.FailOnAction(&t.clients.ForKubernetes().(*k8sfake.Clientset).Fake, "nodes", "update", nil, false)
		})

		testUninstallError()
	})

	When("ClusterRole deletion fails", func() {
		BeforeEach(func() {
			fake.FailOnAction(&t.clients.ForKubernetes().(*k8sfake.Clientset).Fake, "clusterroles", "delete", nil, false)
		})

		testUninstallError()
	})

	When("ClusterRoleBinding deletion fails", func() {
		BeforeEach(func() {
			fake.FailOnAction(&t.clients.ForKubernetes().(*k8sfake.Clientset).Fake, "clusterrolebindings", "delete", nil, false)
		})

		testUninstallError()
	})

	When("Namespace deletion fails", func() {
		BeforeEach(func() {
			fake.FailOnAction(&t.clients.ForKubernetes().(*k8sfake.Clientset).Fake, "namespaces", "delete", nil, false)
		})

		testUninstallError()
	})

	When("Namespace retrieval fails", func() {
		BeforeEach(func() {
			fake.FailOnAction(&t.clients.ForKubernetes().(*k8sfake.Clientset).Fake, "namespaces", "get", nil, false)
		})

		testUninstallError()
	})

	When("Broker retrieval fails", func() {
		BeforeEach(func() {
			t.clients.GeneralClient = controllerfake.NewClientBuilder().WithScheme(scheme.Scheme).WithInterceptorFuncs(
				fake.FailingListInterceptor[*operatorv1alpha1.BrokerList]()).Build()
		})

		testUninstallError()
	})
}
