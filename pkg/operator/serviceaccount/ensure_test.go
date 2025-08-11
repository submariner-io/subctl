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
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/submariner-io/admiral/pkg/fake"
	"github.com/submariner-io/admiral/pkg/names"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/pkg/operator/serviceaccount"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestServiceAccount(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Operator ServiceAccount Suite")
}

var _ = Describe("Ensure", func() {
	ctx := context.TODO()

	var (
		client     *k8sfake.Clientset
		initalObjs []runtime.Object
	)

	BeforeEach(func() {
		initalObjs = []runtime.Object{}
	})

	JustBeforeEach(func() {
		client = k8sfake.NewClientset(initalObjs...)
	})

	When("no resources exist", func() {
		It("should create them and return true", func() {
			created, err := serviceaccount.Ensure(ctx, client, constants.OperatorNamespace)
			Expect(err).NotTo(HaveOccurred())
			Expect(created).To(BeTrue())

			_, err = client.CoreV1().ServiceAccounts(constants.OperatorNamespace).Get(ctx, names.OperatorComponent, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())

			_, err = client.RbacV1().Roles(constants.OperatorNamespace).Get(ctx, names.OperatorComponent, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())

			_, err = client.RbacV1().RoleBindings(constants.OperatorNamespace).Get(ctx, names.OperatorComponent, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())

			_, err = client.RbacV1().ClusterRoles().Get(ctx, names.OperatorComponent, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())

			_, err = client.RbacV1().ClusterRoleBindings().Get(ctx, names.OperatorComponent, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	When("the resources already exist", func() {
		BeforeEach(func() {
			initalObjs = append(initalObjs, &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      names.OperatorComponent,
					Namespace: constants.OperatorNamespace,
				},
			}, &rbacv1.Role{
				ObjectMeta: metav1.ObjectMeta{
					Name:      names.OperatorComponent,
					Namespace: constants.OperatorNamespace,
				},
			}, &rbacv1.RoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      names.OperatorComponent,
					Namespace: constants.OperatorNamespace,
				},
			}, &rbacv1.ClusterRole{
				ObjectMeta: metav1.ObjectMeta{
					Name: names.OperatorComponent,
				},
			}, &rbacv1.ClusterRoleBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name: names.OperatorComponent,
				},
			})
		})

		It("should succeed and return false", func() {
			created, err := serviceaccount.Ensure(ctx, client, constants.OperatorNamespace)
			Expect(err).NotTo(HaveOccurred())
			Expect(created).To(BeFalse())
		})
	})

	for _, resource := range []string{"ServiceAccount", "Role", "RoleBinding", "ClusterRole", "ClusterRoleBinding"} {
		When(resource+" creation fails", func() {
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
