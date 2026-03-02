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

package show_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/submariner-io/admiral/pkg/fake"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/internal/show"
	"github.com/submariner-io/subctl/pkg/client"
	"github.com/submariner-io/submariner-operator/api/v1alpha1"
	"github.com/submariner-io/submariner/pkg/cni"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	controllerfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("Network", func() {
	t := newTestDriver()

	doNetwork := func(ctx context.Context) error {
		return show.Network(ctx, t.clusterInfo, "", t.status)
	}

	When("the Submariner resource exists", func() {
		It("should display network details from the Submariner status", func(ctx SpecContext) {
			submariner := &v1alpha1.Submariner{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "submariner",
					Namespace: constants.OperatorNamespace,
				},
				Status: v1alpha1.SubmarinerStatus{
					ClusterCIDR:      podCIDR,
					ServiceCIDR:      serviceCIDR,
					NetworkPlugin:    "calico",
					GlobalCIDR:       "242.0.0.0/8",
					ClustersetIPCIDR: "242.254.0.0/16",
				},
			}

			Expect(t.clusterInfo.ClientProducer.ForGeneral().Create(ctx, submariner)).To(Succeed())

			t.clusterInfo.Submariner = submariner

			Expect(doNetwork(ctx)).To(Succeed())

			output := t.getOutput()
			Expect(output).To(ContainSubstring(podCIDR))
			Expect(output).To(ContainSubstring(serviceCIDR))
			Expect(output).To(ContainSubstring(submariner.Status.NetworkPlugin))
			Expect(output).To(ContainSubstring(submariner.Status.GlobalCIDR))
			Expect(output).To(ContainSubstring(submariner.Status.ClustersetIPCIDR))
		})
	})

	When("the Submariner resource does not exist", func() {
		It("should discover network details from the cluster", func(ctx SpecContext) {
			Expect(t.clusterInfo.ClientProducer.ForGeneral().Create(ctx, &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "node1"},
				Spec:       corev1.NodeSpec{PodCIDR: podCIDR},
			})).To(Succeed())

			Expect(t.clusterInfo.ClientProducer.ForGeneral().Create(ctx, &networkingv1.ServiceCIDR{
				ObjectMeta: metav1.ObjectMeta{Name: "serviceCIDR"},
				Spec:       networkingv1.ServiceCIDRSpec{CIDRs: []string{serviceCIDR}},
			})).To(Succeed())

			Expect(doNetwork(ctx)).To(Succeed())

			output := t.getOutput()
			Expect(output).To(ContainSubstring(podCIDR))
			Expect(output).To(ContainSubstring(serviceCIDR))
			Expect(output).To(ContainSubstring(cni.Generic))
		})
	})

	When("network discovery fails", func() {
		BeforeEach(func() {
			cp := t.clusterInfo.ClientProducer.(*client.DefaultProducer)
			cp.GeneralClient = controllerfake.NewClientBuilder().WithScheme(scheme.Scheme).WithInterceptorFuncs(
				fake.FailingListInterceptor[*corev1.NodeList]()).Build()
		})

		It("should return an error", func(ctx SpecContext) {
			Expect(doNetwork(ctx)).NotTo(Succeed())
		})
	})

	When("the network isn't discovered", func() {
		It("should succeed but report a warning", func(ctx SpecContext) {
			Expect(doNetwork(ctx)).To(Succeed())
			t.status.AssertHasWarning()
		})
	})
})
