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

package serviceaccount_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/submariner-io/admiral/pkg/fake"
	"github.com/submariner-io/admiral/pkg/names"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/pkg/lighthouse/serviceaccount"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestServiceAccount(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Lighthouse ServiceAccount Suite")
}

var ctx = context.TODO()

var _ = Describe("Ensure", func() {
	var client *k8sfake.Clientset

	BeforeEach(func() {
		client = k8sfake.NewClientset()
	})

	When("no resources exist", func() {
		It("should create them and return true", func() {
			created, err := serviceaccount.Ensure(ctx, client, constants.OperatorNamespace)
			Expect(err).NotTo(HaveOccurred())
			Expect(created).To(BeTrue())

			assertServiceAccountsCreated(client)
			assertClusterRolesCreated(client)
			assertClusterRoleBindingsCreated(client)
			assertRolesCreated(client)
			assertRoleBindingsCreated(client)
		})
	})

	When("some resources already exist", func() {
		BeforeEach(func() {
			createdCount := 0
			client.PrependReactor("create", "*", func(action k8stesting.Action) (bool, runtime.Object, error) {
				createdCount++
				// Simulate that some resources are created (every other one)
				return createdCount%2 == 1, nil, nil
			})
		})

		It("should aggregate the returned flag correctly", func() {
			created, err := serviceaccount.Ensure(ctx, client, constants.OperatorNamespace)
			Expect(err).NotTo(HaveOccurred())
			Expect(created).To(BeTrue()) // Should be true if any resource was created
		})
	})

	When("the resources already exist", func() {
		BeforeEach(func() {
			_, err := serviceaccount.Ensure(ctx, client, constants.OperatorNamespace)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should succeed and return false", func() {
			created, err := serviceaccount.Ensure(ctx, client, constants.OperatorNamespace)
			Expect(err).NotTo(HaveOccurred())
			Expect(created).To(BeFalse())
		})
	})

	for _, resource := range []string{"ServiceAccount", "ClusterRole", "ClusterRoleBinding"} {
		When(fmt.Sprintf("a %s creation fails", resource), func() {
			JustBeforeEach(func() {
				fake.FailOnAction(&client.Fake, strings.ToLower(resource)+"s", "create", nil, false)
			})

			It("should return an error", func() {
				_, err := serviceaccount.Ensure(ctx, client, constants.OperatorNamespace)
				Expect(err).To(HaveOccurred())
			})
		})
	}
})

func assertServiceAccountsCreated(client *k8sfake.Clientset) {
	for _, name := range []string{
		names.ServiceDiscoveryComponent,
		names.LighthouseCoreDNSComponent,
	} {
		_, err := client.CoreV1().ServiceAccounts(constants.OperatorNamespace).Get(ctx, name, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
	}
}

func assertClusterRolesCreated(client *k8sfake.Clientset) {
	for _, name := range []string{
		names.ServiceDiscoveryComponent,
		names.LighthouseCoreDNSComponent,
	} {
		_, err := client.RbacV1().ClusterRoles().Get(ctx, name, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
	}
}

func assertClusterRoleBindingsCreated(client *k8sfake.Clientset) {
	for _, name := range []string{
		names.ServiceDiscoveryComponent,
		names.LighthouseCoreDNSComponent,
	} {
		_, err := client.RbacV1().ClusterRoleBindings().Get(ctx, name, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
	}
}

func assertRolesCreated(client *k8sfake.Clientset) {
	for _, name := range []string{
		names.ServiceDiscoveryComponent,
		names.LighthouseCoreDNSComponent,
	} {
		_, err := client.RbacV1().Roles(constants.OperatorNamespace).Get(ctx, name, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
	}
}

func assertRoleBindingsCreated(client *k8sfake.Clientset) {
	for _, name := range []string{
		names.ServiceDiscoveryComponent,
		names.LighthouseCoreDNSComponent,
	} {
		_, err := client.RbacV1().RoleBindings(constants.OperatorNamespace).Get(ctx, name, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
	}
}
