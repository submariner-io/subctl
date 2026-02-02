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
	"slices"
	"strconv"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/pkg/diagnose"
	submarinerv1 "github.com/submariner-io/submariner/pkg/apis/submariner.io/v1"
	"github.com/submariner-io/submariner/pkg/port"
	corev1 "k8s.io/api/core/v1"
)

var _ = Describe("NatDiscoveryConfigAcrossClusters", func() {
	t := &natFirewallTestDriver{firewallTestDriver: newFirewallTestDriver()}

	When("configured properly", func() {
		t.testNatPortConnectivitySuccess(func(podCmd string) {
			Expect(podCmd).To(ContainSubstring(strconv.Itoa(port.NATTDiscovery)))
		})
	})

	When("there is no local Endpoint", func() {
		BeforeEach(func() {
			t.localEndpoint = nil
		})

		t.testNatDiscoveryConfigFailure("local endpoint", "not found")
	})

	When("the NAT discovery port is configured", func() {
		BeforeEach(func() {
			t.localEndpoint.Spec.BackendConfig[submarinerv1.NATTDiscoveryPortConfig] = strconv.Itoa(555)
		})

		t.testNatPortConnectivitySuccess(func(podCmd string) {
			Expect(podCmd).To(ContainSubstring(t.localEndpoint.Spec.BackendConfig[submarinerv1.NATTDiscoveryPortConfig]))
		})

		Context("and it's invalid", func() {
			BeforeEach(func() {
				t.localEndpoint.Spec.BackendConfig[submarinerv1.NATTDiscoveryPortConfig] = "invalid-port"
			})

			t.testNatDiscoveryConfigFailure("nat", "invalid-port")
		})
	})

	When("load balancer is enabled", func() {
		BeforeEach(func() {
			t.localEndpoint.Spec.BackendConfig[submarinerv1.UsingLoadBalancer] = strconv.FormatBool(true)
		})

		t.testNatPortConnectivitySuccess(func(podCmd string) {
			Expect(podCmd).To(ContainSubstring(strconv.Itoa(lbNatPort)))
		})

		Context("and the load balancer service does not exist", func() {
			BeforeEach(func() {
				t.loadBalancerSvc = nil
			})

			t.testNatDiscoveryConfigFailure(diagnose.LoadBalancerName, "not found")
		})

		Context("and the load balancer service NAT port isn't found", func() {
			BeforeEach(func() {
				t.loadBalancerSvc.Spec.Ports = slices.DeleteFunc(t.loadBalancerSvc.Spec.Ports,
					func(port corev1.ServicePort) bool {
						return port.Name == diagnose.LoadBalancerNattPortName
					})
			})

			t.testNatDiscoveryConfigFailure(diagnose.LoadBalancerNattPortName, "port")
		})
	})

	When("the sniffer pod does not receive the expected message from the client pod", func() {
		t.testNatPortConnectivityFailure("client message not received", "sniffer pod", "ESP traffic")
	})

	When("the remote cluster does not have a connection to the local cluster", func() {
		BeforeEach(func() {
			t.remoteGateway.Status.Connections = nil
		})

		t.testNatPortConnectivityFailure("", remoteCluster, "connection")
	})

	When("the remote cluster's connection to the local cluster does not have an IP", func() {
		BeforeEach(func() {
			t.remoteGateway.Status.Connections[0].UsingIP = ""
		})

		t.testNatPortConnectivityFailure("", remoteCluster, "connection")
	})

	When("the remote cluster does not have a Gateway", func() {
		BeforeEach(func() {
			t.remoteGateway = nil
		})

		t.testNatPortConnectivityFailure("", remoteCluster, "gateways")
	})

	t.testFirewallCheckOnSingleNode(t.run)

	t.testActiveGatewayPodErrors(t.run)

	t.testPodCreationFailure(t.run)

	t.testIncompletePod(t.run)

	t.testImageRepositoryFailure(t.run)
})

type natFirewallTestDriver struct {
	*firewallTestDriver
}

func (t *natFirewallTestDriver) run(ctx context.Context) error {
	return diagnose.NatDiscoveryConfigAcrossClusters(ctx, t.localClusterInfo, t.remoteClusterInfo,
		constants.OperatorNamespace, t.options, t.statusTracker)
}

func (t *natFirewallTestDriver) testNatPortConnectivitySuccess(verifyCmd func(string)) {
	t.testPortConnectivitySuccess(t.run, "", verifyCmd)
}

func (t *natFirewallTestDriver) testNatPortConnectivityFailure(podOutput string, msgs ...string) {
	t.testPortConnectivityFailure(t.run, podOutput, msgs...)
}

func (t *natFirewallTestDriver) testNatDiscoveryConfigFailure(msgs ...string) {
	t.testFailure(t.run, msgs...)
}
