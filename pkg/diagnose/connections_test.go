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

package diagnose_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/submariner-io/subctl/internal/cli"
	"github.com/submariner-io/subctl/pkg/diagnose"
	submarinerv1 "github.com/submariner-io/submariner/pkg/apis/submariner.io/v1"
)

var _ = Describe("CheckGatewayConnections", func() {
	t := newTestDriver()

	When("all connections are established", func() {
		BeforeEach(func() {
			t.createResource(newGateway(submarinerv1.HAStatusActive, submarinerv1.Connection{
				UsingIP: "1.2.3.4",
				Status:  submarinerv1.Connected,
			}))
		})

		It("should succeed", func() {
			Expect(diagnose.CheckGatewayConnections(newClusterInfo(), cli.NewReporter())).To(Succeed())
		})
	})

	When("a connection is in progress", func() {
		BeforeEach(func() {
			t.createResource(newGateway(submarinerv1.HAStatusActive, submarinerv1.Connection{
				UsingIP: "1.2.3.4",
				Status:  submarinerv1.Connected,
			}, submarinerv1.Connection{
				Endpoint: submarinerv1.EndpointSpec{ClusterID: "west"},
				UsingIP:  "5.6.7.8",
				Status:   submarinerv1.Connecting,
			}))
		})

		It("should fail", func() {
			Expect(diagnose.CheckGatewayConnections(newClusterInfo(), cli.NewReporter())).NotTo(Succeed())
		})
	})

	When("a connection is in error", func() {
		BeforeEach(func() {
			t.createResource(newGateway(submarinerv1.HAStatusActive, submarinerv1.Connection{
				Endpoint:      submarinerv1.EndpointSpec{ClusterID: "west"},
				UsingIP:       "2004:0:0:1234::",
				Status:        submarinerv1.ConnectionError,
				StatusMessage: "no initial handshake",
			}))
		})

		It("should fail", func() {
			Expect(diagnose.CheckGatewayConnections(newClusterInfo(), cli.NewReporter())).NotTo(Succeed())
		})
	})

	When("there's no connections", func() {
		BeforeEach(func() {
			t.createResource(newGateway(submarinerv1.HAStatusActive))
		})

		It("should fail", func() {
			Expect(diagnose.CheckGatewayConnections(newClusterInfo(), cli.NewReporter())).NotTo(Succeed())
		})
	})

	When("there's no active Gateway", func() {
		BeforeEach(func() {
			t.createResource(newGateway(submarinerv1.HAStatusPassive))
		})

		It("should fail", func() {
			Expect(diagnose.CheckGatewayConnections(newClusterInfo(), cli.NewReporter())).NotTo(Succeed())
		})
	})

	When("there's no Gateways", func() {
		It("should fail", func() {
			Expect(diagnose.CheckGatewayConnections(newClusterInfo(), cli.NewReporter())).NotTo(Succeed())
		})
	})
})

var _ = Describe("CheckRouteAgentConnections", func() {
	t := newTestDriver()

	When("all connections are established", func() {
		BeforeEach(func() {
			t.createResource(newRouteAgent(submarinerv1.RemoteEndpoint{
				Status: submarinerv1.Connected,
			}))
		})

		It("should succeed", func() {
			Expect(diagnose.CheckRouteAgentConnections(newClusterInfo(), cli.NewReporter())).To(Succeed())
		})
	})

	When("a connection is in progress", func() {
		BeforeEach(func() {
			t.createResource(newRouteAgent(
				submarinerv1.RemoteEndpoint{
					Status: submarinerv1.Connected,
				}, submarinerv1.RemoteEndpoint{
					Status: submarinerv1.Connecting,
					Spec: submarinerv1.EndpointSpec{
						ClusterID:      "west",
						HealthCheckIPs: []string{"1.2.3.4"},
					},
				}, submarinerv1.RemoteEndpoint{
					Status: submarinerv1.Connecting,
					Spec: submarinerv1.EndpointSpec{
						ClusterID:      "west",
						HealthCheckIPs: []string{"2004:0:0:1234::"},
					},
				}))
		})

		It("should fail", func() {
			Expect(diagnose.CheckRouteAgentConnections(newClusterInfo(), cli.NewReporter())).NotTo(Succeed())
		})
	})

	When("a connection is in error", func() {
		BeforeEach(func() {
			t.createResource(newRouteAgent(
				submarinerv1.RemoteEndpoint{
					Status:        submarinerv1.ConnectionError,
					StatusMessage: "no initial handshake",
					Spec: submarinerv1.EndpointSpec{
						ClusterID:      "west",
						HealthCheckIPs: []string{"1.2.3.4"},
					},
				}, submarinerv1.RemoteEndpoint{
					Status:        submarinerv1.ConnectionError,
					StatusMessage: "no initial handshake",
					Spec: submarinerv1.EndpointSpec{
						ClusterID:      "west",
						HealthCheckIPs: []string{"2004:0:0:1234::"},
					},
				}))
		})

		It("should fail", func() {
			Expect(diagnose.CheckRouteAgentConnections(newClusterInfo(), cli.NewReporter())).NotTo(Succeed())
		})
	})

	When("there's no connections", func() {
		BeforeEach(func() {
			t.createResource(newRouteAgent())
		})

		It("should succeed", func() {
			Expect(diagnose.CheckRouteAgentConnections(newClusterInfo(), cli.NewReporter())).To(Succeed())
		})
	})

	When("there's no RouteAgents", func() {
		It("should fail", func() {
			Expect(diagnose.CheckRouteAgentConnections(newClusterInfo(), cli.NewReporter())).NotTo(Succeed())
		})
	})
})

var _ = Describe("Connections", func() {
	t := newTestDriver()

	When("all connections are established", func() {
		BeforeEach(func() {
			t.createResource(newGateway(submarinerv1.HAStatusActive, submarinerv1.Connection{
				UsingIP: "1.2.3.4",
				Status:  submarinerv1.Connected,
			}))

			t.createResource(newRouteAgent(submarinerv1.RemoteEndpoint{
				Status: submarinerv1.Connected,
			}))
		})

		It("should succeed", func() {
			Expect(diagnose.Connections(newClusterInfo(), "", cli.NewReporter())).To(Succeed())
		})
	})

	When("there's a fully-connected RouteAgent but no Gateway", func() {
		BeforeEach(func() {
			t.createResource(newRouteAgent(submarinerv1.RemoteEndpoint{
				Status: submarinerv1.Connected,
			}))
		})

		It("should fail", func() {
			Expect(diagnose.Connections(newClusterInfo(), "", cli.NewReporter())).NotTo(Succeed())
		})
	})

	When("there's a fully-connected Gateway but no RouteAgent", func() {
		BeforeEach(func() {
			t.createResource(newGateway(submarinerv1.HAStatusActive, submarinerv1.Connection{
				UsingIP: "1.2.3.4",
				Status:  submarinerv1.Connected,
			}))
		})

		It("should fail", func() {
			Expect(diagnose.Connections(newClusterInfo(), "", cli.NewReporter())).NotTo(Succeed())
		})
	})
})
