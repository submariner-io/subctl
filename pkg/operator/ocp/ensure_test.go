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

package ocp_test

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	securityv1 "github.com/openshift/api/security/v1"
	"github.com/submariner-io/admiral/pkg/fake"
	"github.com/submariner-io/admiral/pkg/names"
	"github.com/submariner-io/admiral/pkg/resource"
	"github.com/submariner-io/subctl/pkg/operator/ocp"
	operatorrbac "github.com/submariner-io/submariner-operator/config/rbac/submariner-operator"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
)

const (
	testNamespace = "test-namespace"
)

func TestOcp(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "OCP Suite")
}

var _ = BeforeSuite(func() {
	Expect(securityv1.Install(scheme.Scheme)).To(Succeed())
})

var _ = Describe("EnsureRBAC", func() {
	ctx := context.TODO()
	rbacResourceName := "ocp-submariner-operator"
	rbacInfos := []ocp.RbacInfo{
		{
			ComponentName:      names.OperatorComponent,
			ClusterRole:        operatorrbac.OCPClusterRole,
			ClusterRoleBinding: operatorrbac.OCPClusterRoleBinding,
		},
	}

	var (
		kubeClient    *k8sfake.Clientset
		dynamicClient dynamic.Interface
	)

	BeforeEach(func() {
		kubeClient = k8sfake.NewClientset()
		dynamicClient = dynamicfake.NewSimpleDynamicClient(scheme.Scheme)
	})

	When("not running on OCP platform", func() {
		It("should return false without creating RBAC resources", func() {
			updated, err := ocp.EnsureRBAC(ctx, dynamicClient, kubeClient, testNamespace, rbacInfos)
			Expect(err).NotTo(HaveOccurred())
			Expect(updated).To(BeFalse())

			crList, err := kubeClient.RbacV1().ClusterRoles().List(ctx, metav1.ListOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(crList.Items).To(BeEmpty())

			crbList, err := kubeClient.RbacV1().ClusterRoleBindings().List(ctx, metav1.ListOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(crbList.Items).To(BeEmpty())
		})
	})

	When("running on OCP platform", func() {
		BeforeEach(func() {
			_, err := dynamicClient.Resource(securityv1.GroupVersion.WithResource("securitycontextconstraints")).Create(
				ctx, resource.MustToUnstructuredUsingScheme(&securityv1.SecurityContextConstraints{
					ObjectMeta: metav1.ObjectMeta{
						Name: "privileged",
					},
				}, scheme.Scheme), metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
		})

		Context("and the RBAC resources don't exist", func() {
			It("should create them and return true", func() {
				updated, err := ocp.EnsureRBAC(ctx, dynamicClient, kubeClient, testNamespace, rbacInfos)
				Expect(err).NotTo(HaveOccurred())
				Expect(updated).To(BeTrue())

				_, err = kubeClient.RbacV1().ClusterRoles().Get(ctx, rbacResourceName, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())

				crb, err := kubeClient.RbacV1().ClusterRoleBindings().Get(ctx, rbacResourceName, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
				Expect(crb.Subjects).To(Equal([]rbacv1.Subject{
					{
						Kind:      "ServiceAccount",
						Namespace: testNamespace,
						Name:      names.OperatorComponent,
					},
				}))
			})
		})

		Context("and the RBAC resources already exist", func() {
			BeforeEach(func() {
				kubeClient = k8sfake.NewClientset(&rbacv1.ClusterRole{
					ObjectMeta: metav1.ObjectMeta{
						Name: rbacResourceName,
					},
				}, &rbacv1.ClusterRoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name: rbacResourceName,
					},
				})
			})

			It("should return false", func() {
				updated, err := ocp.EnsureRBAC(ctx, dynamicClient, kubeClient, testNamespace, rbacInfos)
				Expect(err).NotTo(HaveOccurred())
				Expect(updated).To(BeFalse())
			})
		})

		Context("when ClusterRole creation fails", func() {
			BeforeEach(func() {
				fake.FailOnAction(&kubeClient.Fake, "clusterroles", "create", nil, false)
			})

			It("should return an error", func() {
				_, err := ocp.EnsureRBAC(ctx, dynamicClient, kubeClient, testNamespace, rbacInfos)
				Expect(err).To(HaveOccurred())
			})
		})

		Context("when ClusterRoleBinding creation fails", func() {
			BeforeEach(func() {
				fake.FailOnAction(&kubeClient.Fake, "clusterrolebindings", "create", nil, false)
			})

			It("should return an error", func() {
				_, err := ocp.EnsureRBAC(ctx, dynamicClient, kubeClient, testNamespace, rbacInfos)
				Expect(err).To(HaveOccurred())
			})
		})
	})
})
