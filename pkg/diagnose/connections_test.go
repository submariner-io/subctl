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
	"context"

	. "github.com/onsi/ginkgo/v2"
	"github.com/submariner-io/subctl/pkg/diagnose"
	submarinerv1 "github.com/submariner-io/submariner/pkg/apis/submariner.io/v1"
)

var _ = Describe("CheckGatewayConnections", func() {
	t := newTestDriver()

	run := func() error {
		return diagnose.CheckGatewayConnections(newClusterInfo(context.TODO()), t.statusTracker)
	}

	When("all connections are established", func() {
		BeforeEach(func() {
			t.createResource(newGateway(submarinerv1.HAStatusActive, submarinerv1.Connection{
				UsingIP: "1.2.3.4",
				Status:  submarinerv1.Connected,
			}))
		})

		t.testSuccess(run)
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

		t.testFailure(run, "in progress")
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

		t.testFailure(run, "not established")
	})

	When("there's no connections", func() {
		BeforeEach(func() {
			t.createResource(newGateway(submarinerv1.HAStatusActive))
		})

		t.testFailure(run, "no", "connections")
	})

	When("there's no active Gateway", func() {
		BeforeEach(func() {
			t.createResource(newGateway(submarinerv1.HAStatusPassive))
		})

		t.testFailure(run, "active gateway")
	})

	When("there's no Gateways", func() {
		t.testFailure(run, "gateways")
	})
})

var _ = Describe("CheckRouteAgentConnections", func() {
	t := newTestDriver()

	run := func() error {
		return diagnose.CheckRouteAgentConnections(newClusterInfo(context.TODO()), t.statusTracker)
	}

	When("all connections are established", func() {
		BeforeEach(func() {
			t.createResource(newRouteAgent(submarinerv1.RemoteEndpoint{
				Status: submarinerv1.Connected,
			}))
		})

		t.testSuccess(run)
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

		t.testFailure(run, "in progress")
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

		t.testFailure(run, "not established")
	})

	When("there's no connections", func() {
		BeforeEach(func() {
			t.createResource(newRouteAgent())
		})

		t.testSuccess(run)
	})

	When("there's no RouteAgents", func() {
		t.testFailure(run, "route agents")
	})
})

var _ = Describe("Connections", func() {
	t := newTestDriver()

	run := func() error {
		return diagnose.Connections(newClusterInfo(context.TODO()), "", t.statusTracker)
	}

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

		t.testSuccess(run)
	})

	When("there's a fully-connected RouteAgent but no Gateway", func() {
		BeforeEach(func() {
			t.createResource(newRouteAgent(submarinerv1.RemoteEndpoint{
				Status: submarinerv1.Connected,
			}))
		})

		t.testFailure(run, "gateways", "detected")
	})

	When("there's a fully-connected Gateway but no RouteAgent", func() {
		BeforeEach(func() {
			t.createResource(newGateway(submarinerv1.HAStatusActive, submarinerv1.Connection{
				UsingIP: "1.2.3.4",
				Status:  submarinerv1.Connected,
			}))
		})

		t.testFailure(run, "route agents", "detected")
	})
})
