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

package apply_test

import (
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/submariner-io/admiral/pkg/fake"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/pkg/apply"
	"github.com/submariner-io/subctl/pkg/role"
	"github.com/submariner-io/subctl/pkg/rolebinding"
	"github.com/submariner-io/subctl/pkg/serviceaccount"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

const (
	saTemplate = `apiVersion: v1
kind: ServiceAccount
metadata:
  name: %NAME`

	roleTemplate = `apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: %NAME`

	rbTemplate = `apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: %NAME`
)

var embeddedYAMLs = []apply.EmbeddedYAMLRefsApplier{
	{
		Applier: serviceaccount.EnsureFromYAML,
		Refs: []apply.EmbeddedYAMLRef{
			{
				Name:    "first ServiceAccount",
				Content: []byte(strings.Replace(saTemplate, "%NAME", "first", 1)),
			},
			{
				Name:    "second ServiceAccount",
				Content: []byte(strings.Replace(saTemplate, "%NAME", "second", 1)),
			},
		},
	},
	{
		Applier: role.EnsureFromYAML,
		Refs: []apply.EmbeddedYAMLRef{
			{
				Name:    "first Role",
				Content: []byte(strings.Replace(roleTemplate, "%NAME", "first", 1)),
			},
			{
				Name:    "second Role",
				Content: []byte(strings.Replace(roleTemplate, "%NAME", "second", 1)),
			},
		},
	},
	{
		Applier: rolebinding.EnsureFromYAML,
		Refs: []apply.EmbeddedYAMLRef{
			{
				Name:    "first RoleBinding",
				Content: []byte(strings.Replace(rbTemplate, "%NAME", "first", 1)),
			},
			{
				Name:    "second RoleBinding",
				Content: []byte(strings.Replace(rbTemplate, "%NAME", "second", 1)),
			},
		},
	},
}

var _ = Describe("EmbeddedYAMLs", func() {
	var client *k8sfake.Clientset

	BeforeEach(func() {
		client = k8sfake.NewClientset()
	})

	When("no resources exist", func() {
		It("should create them and return true", func(ctx SpecContext) {
			created, err := apply.EmbeddedYAMLs(ctx, client, constants.OperatorNamespace, embeddedYAMLs)
			Expect(err).NotTo(HaveOccurred())
			Expect(created).To(BeTrue())

			for _, name := range []string{"first", "second"} {
				_, err := client.CoreV1().ServiceAccounts(constants.OperatorNamespace).Get(ctx, name, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())

				_, err = client.RbacV1().Roles(constants.OperatorNamespace).Get(ctx, name, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())

				_, err = client.RbacV1().RoleBindings(constants.OperatorNamespace).Get(ctx, name, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
			}
		})
	})

	When("some resources already exist", func() {
		It("should aggregate the returned flag correctly", func(ctx SpecContext) {
			createdCount := 0

			client.PrependReactor("create", "*", func(action k8stesting.Action) (bool, runtime.Object, error) {
				createdCount++
				// Simulate that some resources are created (every other one)
				return createdCount%2 == 1, nil, nil
			})

			created, err := apply.EmbeddedYAMLs(ctx, client, constants.OperatorNamespace, embeddedYAMLs)
			Expect(err).NotTo(HaveOccurred())
			Expect(created).To(BeTrue()) // Should be true if any resource was created
			Expect(createdCount).To(Equal(len(embeddedYAMLs) * 2))
		})
	})

	When("the resources already exist", func() {
		BeforeEach(func(ctx SpecContext) {
			_, err := apply.EmbeddedYAMLs(ctx, client, constants.OperatorNamespace, embeddedYAMLs)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should succeed and return false", func(ctx SpecContext) {
			created, err := apply.EmbeddedYAMLs(ctx, client, constants.OperatorNamespace, embeddedYAMLs)
			Expect(err).NotTo(HaveOccurred())
			Expect(created).To(BeFalse())
		})
	})

	for _, resource := range []string{"ServiceAccount", "Role", "RoleBinding"} {
		When(fmt.Sprintf("a %s creation fails", resource), func() {
			JustBeforeEach(func() {
				fake.FailOnAction(&client.Fake, strings.ToLower(resource)+"s", "create", nil, false)
			})

			It("should return an error", func(ctx SpecContext) {
				_, err := apply.EmbeddedYAMLs(ctx, client, constants.OperatorNamespace, embeddedYAMLs)
				Expect(err).To(HaveOccurred())
			})
		})
	}
})
