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

package lighthouse_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/submariner-io/admiral/pkg/fake"
	"github.com/submariner-io/admiral/pkg/reporter/test"
	"github.com/submariner-io/admiral/pkg/resource"
	"github.com/submariner-io/subctl/internal/cli"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/pkg/lighthouse"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	dynfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestSubmariner(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Lighthouse Suite")
}

var _ = Describe("Ensure", func() {
	ctx := context.TODO()

	var (
		kubeClient     *k8sfake.Clientset
		dynClient      *dynfake.FakeDynamicClient
		statusReporter *test.Tracker
	)

	BeforeEach(func() {
		kubeClient = k8sfake.NewClientset()
		dynClient = dynfake.NewSimpleDynamicClient(runtime.NewScheme())
		statusReporter = &test.Tracker{Interface: cli.NewReporter()}

		dynClient.PrependReactor("get", "securitycontextconstraints", func(action k8stesting.Action) (bool, runtime.Object, error) {
			return true, &unstructured.Unstructured{Object: map[string]any{}}, nil // Simulate OCP platform detection
		})
	})

	When("no resources exist", func() {
		It("should create them and return true", func() {
			Expect(lighthouse.Ensure(ctx, statusReporter, kubeClient, dynClient, constants.OperatorNamespace)).To(Succeed())

			saList, err := kubeClient.CoreV1().ServiceAccounts(constants.OperatorNamespace).List(ctx, metav1.ListOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(saList.Items).NotTo(BeEmpty())

			for _, name := range []string{"ocp-submariner-lighthouse-agent", "ocp-submariner-lighthouse-coredns"} {
				_, err := kubeClient.RbacV1().ClusterRoles().Get(ctx, name, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())

				_, err = kubeClient.RbacV1().ClusterRoleBindings().Get(ctx, name, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
			}
		})
	})

	When("a ServiceAccount creation fails", func() {
		BeforeEach(func() {
			fake.FailOnAction(&kubeClient.Fake, "serviceaccounts", "create", nil, false)
		})

		It("should return an error", func() {
			err := lighthouse.Ensure(ctx, statusReporter, kubeClient, dynClient, constants.OperatorNamespace)
			Expect(err).To(HaveOccurred())
		})
	})

	When("an OCP ClusterRole creation fails", func() {
		BeforeEach(func() {
			kubeClient.Fake.PrependReactor("create", "clusterroles",
				func(action k8stesting.Action) (bool, runtime.Object, error) {
					if strings.Contains(resource.ToJSON(action.(k8stesting.CreateAction).GetObject()), "securitycontextconstraints") {
						return true, nil, errors.New("fake error")
					}

					return false, nil, nil
				})
		})

		It("should return an error", func() {
			err := lighthouse.Ensure(ctx, statusReporter, kubeClient, dynClient, constants.OperatorNamespace)
			Expect(err).To(HaveOccurred())
		})
	})
})
