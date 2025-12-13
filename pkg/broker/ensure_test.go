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

package broker_test

import (
	"context"
	"errors"
	"slices"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/submariner-io/admiral/pkg/fake"
	"github.com/submariner-io/subctl/internal/component"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/pkg/broker"
	"github.com/submariner-io/submariner-operator/pkg/crd"
	"github.com/submariner-io/submariner-operator/pkg/names"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	controllerclient "sigs.k8s.io/controller-runtime/pkg/client"
	controllerfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

const (
	testBrokerNamespace = "test-broker-ns"
	testClusterID       = "test-cluster"
)

var _ = Describe("", func() {
	Describe("Ensure", testEnsure)
	Describe("CreateSAForCluster", testCreateSAForCluster)
})

func testEnsure() {
	t := newTestDriver()

	It("should create the broker resources", func() {
		Expect(broker.Ensure(ctx, t.crdUpdater, t.kubeClient, []string{}, false, testBrokerNamespace)).To(Succeed())

		_, err := t.kubeClient.CoreV1().Namespaces().Get(ctx, testBrokerNamespace, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())

		_, err = t.kubeClient.CoreV1().ServiceAccounts(testBrokerNamespace).Get(
			ctx, constants.SubmarinerBrokerAdminSA, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())

		secrets, err := t.kubeClient.CoreV1().Secrets(testBrokerNamespace).List(ctx, metav1.ListOptions{})
		Expect(err).To(Succeed())
		Expect(slices.IndexFunc(secrets.Items, func(s corev1.Secret) bool {
			return s.Annotations[corev1.ServiceAccountNameKey] == constants.SubmarinerBrokerAdminSA
		})).To(BeNumerically(">=", 0))

		_, err = t.kubeClient.RbacV1().Roles(testBrokerNamespace).Get(ctx, constants.SubmarinerBrokerAdminSA, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())

		_, err = t.kubeClient.RbacV1().RoleBindings(testBrokerNamespace).Get(ctx, constants.SubmarinerBrokerAdminSA, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())

		_, err = t.kubeClient.RbacV1().Roles(testBrokerNamespace).Get(ctx, broker.SubmarinerBrokerClusterRole, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())

		_, err = t.kubeClient.RbacV1().Roles(testBrokerNamespace).Get(ctx, "submariner-certs-role", metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())

		_, err = t.kubeClient.RbacV1().RoleBindings(testBrokerNamespace).Get(ctx, "submariner-certs-role-binding", metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
	})

	Context("with CRD creation requested", func() {
		It("should create the CRDs", func() {
			Expect(broker.Ensure(ctx, t.crdUpdater, t.kubeClient,
				[]string{component.Connectivity, component.ServiceDiscovery, component.Globalnet}, true, testBrokerNamespace)).To(Succeed())

			_, err := t.crdUpdater.Get(ctx, "clusters.submariner.io", metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())

			_, err = t.crdUpdater.Get(ctx, "serviceimports.multicluster.x-k8s.io", metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
		})

		Context("and Globalnet component is specified without ServiceDiscovery", func() {
			It("should create the Globalnet and ServiceDiscovery CRDs", func() {
				Expect(broker.Ensure(ctx, t.crdUpdater, t.kubeClient,
					[]string{component.Connectivity, component.Globalnet}, true, testBrokerNamespace)).To(Succeed())

				_, err := t.crdUpdater.Get(ctx, "clusterglobalegressips.submariner.io", metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())

				_, err = t.crdUpdater.Get(ctx, "serviceimports.multicluster.x-k8s.io", metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

	When("namespace creation fails", func() {
		BeforeEach(func() {
			fake.FailOnAction(&t.kubeClient.(*k8sfake.Clientset).Fake, "namespaces", "create", nil, false)
		})

		It("should return an error", func() {
			Expect(broker.Ensure(ctx, t.crdUpdater, t.kubeClient, []string{component.Connectivity}, false, testBrokerNamespace)).
				NotTo(Succeed())
		})
	})

	DescribeTableSubtree("when CRD creation fails for",
		func(comp string) {
			BeforeEach(func() {
				t.crdUpdater = crd.UpdaterFromControllerClient(controllerfake.NewClientBuilder().WithScheme(scheme.Scheme).
					WithInterceptorFuncs(interceptor.Funcs{
						Create: func(_ context.Context, _ controllerclient.WithWatch, _ controllerclient.Object,
							_ ...controllerclient.CreateOption,
						) error {
							return errors.New("mock CRD creation error")
						},
					}).Build())
			})

			It("should return an error", func() {
				Expect(broker.Ensure(ctx, t.crdUpdater, t.kubeClient, []string{comp}, true, testBrokerNamespace)).NotTo(Succeed())
			})
		},
		Entry("Connectivity", component.Connectivity),
		Entry("ServiceDiscovery", component.ServiceDiscovery),
		Entry("Globalnet", component.Globalnet),
	)

	When("broker admin ServiceAccount creation fails", func() {
		BeforeEach(func() {
			fake.FailOnAction(&t.kubeClient.(*k8sfake.Clientset).Fake, "serviceaccounts", "create",
				errors.New("mock SA creation error"), false)
		})

		It("should return an error", func() {
			Expect(broker.Ensure(ctx, t.crdUpdater, t.kubeClient, []string{component.Connectivity}, false, testBrokerNamespace)).
				NotTo(Succeed())
		})
	})

	When("broker admin role creation fails", func() {
		BeforeEach(func() {
			fake.FailOnAction(&t.kubeClient.(*k8sfake.Clientset).Fake, "roles", "create",
				errors.New("mock role creation error"), false)
		})

		It("should return an error", func() {
			Expect(broker.Ensure(ctx, t.crdUpdater, t.kubeClient, []string{component.Connectivity}, false, testBrokerNamespace)).
				NotTo(Succeed())
		})
	})
}

func testCreateSAForCluster() {
	t := newTestDriver()

	It("should create the ServiceAccount and RoleBinding", func() {
		secret, err := broker.CreateSAForCluster(ctx, t.kubeClient, testClusterID, testBrokerNamespace)
		Expect(err).NotTo(HaveOccurred())
		Expect(secret).NotTo(BeNil())
		Expect(secret.Data["token"]).NotTo(BeEmpty())

		saName := names.ForClusterSA(testClusterID)

		_, err = t.kubeClient.CoreV1().ServiceAccounts(testBrokerNamespace).Get(ctx, saName, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())

		rbs, err := t.kubeClient.RbacV1().RoleBindings(testBrokerNamespace).List(ctx, metav1.ListOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(rbs.Items).To(HaveLen(1))
		Expect(rbs.Items[0].Subjects[0].Name).To(Equal(saName))
		Expect(rbs.Items[0].RoleRef.Name).To(Equal(broker.SubmarinerBrokerClusterRole))
	})

	When("the ServiceAccount already exists", func() {
		It("should succeed", func() {
			_, err := broker.CreateSAForCluster(ctx, t.kubeClient, testClusterID, testBrokerNamespace)
			Expect(err).NotTo(HaveOccurred())

			_, err = broker.CreateSAForCluster(ctx, t.kubeClient, testClusterID, testBrokerNamespace)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	When("ServiceAccount creation fails", func() {
		BeforeEach(func() {
			fake.FailOnAction(&t.kubeClient.(*k8sfake.Clientset).Fake, "serviceaccounts", "create",
				errors.New("mock SA creation error"), false)
		})

		It("should return an error", func() {
			_, err := broker.CreateSAForCluster(ctx, t.kubeClient, testClusterID, testBrokerNamespace)
			Expect(err).To(HaveOccurred())
		})
	})

	When("RoleBinding creation fails", func() {
		BeforeEach(func() {
			fake.FailOnAction(&t.kubeClient.(*k8sfake.Clientset).Fake, "rolebindings", "create",
				errors.New("mock role binding creation error"), false)
		})

		It("should return an error", func() {
			_, err := broker.CreateSAForCluster(ctx, t.kubeClient, testClusterID, testBrokerNamespace)
			Expect(err).To(HaveOccurred())
		})
	})

	When("token Secret creation fails", func() {
		BeforeEach(func() {
			fake.FailOnAction(&t.kubeClient.(*k8sfake.Clientset).Fake, "secrets", "create",
				errors.New("mock secret creation error"), false)
		})

		It("should return an error", func() {
			_, err := broker.CreateSAForCluster(ctx, t.kubeClient, testClusterID, testBrokerNamespace)
			Expect(err).To(HaveOccurred())
		})
	})
}
