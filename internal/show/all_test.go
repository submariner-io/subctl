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
	"github.com/submariner-io/admiral/pkg/names"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/internal/show"
	"github.com/submariner-io/submariner-operator/api/v1alpha1"
	submarinerv1 "github.com/submariner-io/submariner/pkg/apis/submariner.io/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("All", func() {
	t := newTestDriver()

	JustBeforeEach(func(ctx SpecContext) {
		Expect(t.clusterInfo.ClientProducer.ForGeneral().Create(ctx, &v1alpha1.Broker{
			ObjectMeta: metav1.ObjectMeta{
				Name:      brokerName,
				Namespace: "broker-ns",
			},
		})).To(Succeed())
	})

	doAll := func(ctx context.Context) error {
		return show.All(ctx, t.clusterInfo, "", t.status)
	}

	When("Submariner is installed", func() {
		var submariner *v1alpha1.Submariner

		BeforeEach(func(ctx SpecContext) {
			submariner = &v1alpha1.Submariner{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "submariner",
					Namespace: constants.OperatorNamespace,
				},
				Status: v1alpha1.SubmarinerStatus{
					ClusterCIDR: podCIDR,
					ServiceCIDR: serviceCIDR,
				},
			}

			Expect(t.clusterInfo.ClientProducer.ForGeneral().Create(ctx, submariner)).To(Succeed())
			t.clusterInfo.Submariner = submariner

			gateway := &submarinerv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gateway",
					Namespace: constants.OperatorNamespace,
				},
				Status: submarinerv1.GatewayStatus{
					HAStatus: submarinerv1.HAStatusActive,
					LocalEndpoint: submarinerv1.EndpointSpec{
						ClusterID: "local-cluster",
						Hostname:  "local-node-host",
					},
					Connections: []submarinerv1.Connection{
						{
							Status: submarinerv1.Connected,
							Endpoint: submarinerv1.EndpointSpec{
								ClusterID: "remote-cluster",
								Hostname:  "remote-node",
							},
						},
					},
				},
			}
			Expect(t.clusterInfo.ClientProducer.ForGeneral().Create(ctx, gateway)).To(Succeed())

			t.createDaemonSet(ctx, names.GatewayComponent)
		})

		It("should display all Submariner information", func(ctx SpecContext) {
			Expect(doAll(ctx)).To(Succeed())

			output := t.getOutput()
			Expect(output).To(ContainSubstring(brokerName))
			Expect(output).To(ContainSubstring("remote-node"))
			Expect(output).To(ContainSubstring("remote-cluster"))
			Expect(output).To(ContainSubstring("local-node-host"))
			Expect(output).To(ContainSubstring(podCIDR))
			Expect(output).To(ContainSubstring(names.GatewayComponent))
		})
	})

	When("Submariner is not installed", func() {
		BeforeEach(func(ctx SpecContext) {
			t.createDeployment(ctx, names.OperatorComponent)
		})

		It("should display broker and version information with a warning", func(ctx SpecContext) {
			Expect(doAll(ctx)).To(Succeed())

			output := t.getOutput()
			Expect(output).To(ContainSubstring(brokerName))
			Expect(output).To(ContainSubstring(names.OperatorComponent))

			t.status.AssertHasWarning()
		})
	})
})
