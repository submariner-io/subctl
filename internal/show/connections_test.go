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
	"github.com/onsi/gomega/types"
	"github.com/submariner-io/admiral/pkg/fake"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/internal/show"
	"github.com/submariner-io/subctl/pkg/client"
	submarinerv1 "github.com/submariner-io/submariner/pkg/apis/submariner.io/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	controllerfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("Connections", func() {
	connection1 := submarinerv1.Connection{
		Status: submarinerv1.Connected,
		Endpoint: submarinerv1.EndpointSpec{
			ClusterID:  "cluster1",
			Hostname:   "host1",
			PrivateIPs: []string{"10.0.1.100"},
			PublicIPs:  []string{"203.0.113.10"},
			Subnets:    []string{"10.1.0.0/16"},
			Backend:    "libreswan",
			NATEnabled: false,
		},
		UsingIP:  "10.0.1.100",
		UsingNAT: false,
		LatencyRTT: &submarinerv1.LatencyRTTSpec{
			Average: "5ms",
		},
	}

	connection2 := submarinerv1.Connection{
		Status: submarinerv1.Connected,
		Endpoint: submarinerv1.EndpointSpec{
			ClusterID:  "cluster2",
			Hostname:   "host2",
			PrivateIPs: []string{"10.0.2.100"},
			PublicIPs:  []string{"203.0.113.20"},
			Subnets:    []string{"10.2.0.0/16"},
			Backend:    "vxlan",
			NATEnabled: true,
		},
		UsingIP:  "203.0.113.20",
		UsingNAT: true,
		LatencyRTT: &submarinerv1.LatencyRTTSpec{
			Average: "6ms",
		},
	}

	connection3 := submarinerv1.Connection{
		Status: submarinerv1.Connecting,
		Endpoint: submarinerv1.EndpointSpec{
			ClusterID:  "cluster3",
			Hostname:   "host3",
			PrivateIPs: []string{"10.0.3.100"},
			PublicIPs:  []string{"203.0.113.30"},
			Subnets:    []string{"10.3.0.0/16"},
			Backend:    "wireguard",
			NATEnabled: true,
		},
	}

	connection4 := submarinerv1.Connection{
		Status: submarinerv1.ConnectionError,
		Endpoint: submarinerv1.EndpointSpec{
			ClusterID:  "cluster4",
			Hostname:   "host4",
			PrivateIPs: []string{"10.0.4.100"},
			PublicIPs:  []string{"203.0.113.40"},
			Subnets:    []string{"10.4.0.0/16"},
			Backend:    "libreswan",
			NATEnabled: false,
		},
	}

	t := newTestDriver()

	doConnections := func(ctx context.Context) error {
		return show.Connections(ctx, t.clusterInfo, "", t.status)
	}

	connectionMatcher := func(c submarinerv1.Connection, ip string, nat bool) types.GomegaMatcher {
		natStr := "no"
		if nat {
			natStr = "yes"
		}

		return tableRowMatcher(c.Endpoint.Hostname, c.Endpoint.ClusterID, ip, natStr, c.Endpoint.Backend, c.Endpoint.Subnets[0],
			string(c.Status), ptr.Deref(c.LatencyRTT, submarinerv1.LatencyRTTSpec{}).Average)
	}

	It("should display connection information", func(ctx SpecContext) {
		gateway := &submarinerv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "gateway-1",
				Namespace: constants.OperatorNamespace,
			},
			Status: submarinerv1.GatewayStatus{
				HAStatus: submarinerv1.HAStatusActive,
				Connections: []submarinerv1.Connection{
					connection1,
					connection2,
					connection3,
					connection4,
				},
			},
		}

		Expect(t.clusterInfo.ClientProducer.ForGeneral().Create(ctx, gateway)).To(Succeed())

		Expect(doConnections(ctx)).To(Succeed())

		t.assertTableOutput(
			connectionMatcher(connection1, connection1.UsingIP, connection1.UsingNAT),
			connectionMatcher(connection2, connection2.UsingIP, connection2.UsingNAT),
			connectionMatcher(connection3, connection3.Endpoint.PublicIPs[0], connection3.Endpoint.NATEnabled),
			connectionMatcher(connection4, connection4.Endpoint.PrivateIPs[0], connection4.Endpoint.NATEnabled),
		)
	})

	When("no gateways exist", func() {
		It("should return an error", func(ctx SpecContext) {
			Expect(doConnections(ctx)).NotTo(Succeed())
		})
	})

	When("a gateway exists but has no connections", func() {
		JustBeforeEach(func(ctx SpecContext) {
			Expect(t.clusterInfo.ClientProducer.ForGeneral().Create(ctx, &submarinerv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gateway-1",
					Namespace: constants.OperatorNamespace,
				},
				Status: submarinerv1.GatewayStatus{
					HAStatus:    submarinerv1.HAStatusActive,
					Connections: []submarinerv1.Connection{},
				},
			})).To(Succeed())
		})

		It("should return an error", func(ctx SpecContext) {
			Expect(doConnections(ctx)).NotTo(Succeed())
		})
	})

	When("gateway retrieval fails", func() {
		BeforeEach(func() {
			cp := t.clusterInfo.ClientProducer.(*client.DefaultProducer)
			cp.GeneralClient = controllerfake.NewClientBuilder().WithScheme(scheme.Scheme).WithInterceptorFuncs(
				fake.FailingListInterceptor[*submarinerv1.GatewayList]()).Build()
		})

		It("should return an error", func(ctx SpecContext) {
			Expect(doConnections(ctx)).NotTo(Succeed())
		})
	})
})
