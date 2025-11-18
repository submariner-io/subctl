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
	"strconv"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/pkg/diagnose"
	submarinerv1 "github.com/submariner-io/submariner/pkg/apis/submariner.io/v1"
)

var _ = Describe("TunnelConfigAcrossClusters", func() {
	t := &tunnelFirewallTestDriver{firewallTestDriver: newFirewallTestDriver()}

	When("configured properly", func() {
		t.testTunnelConnectivitySuccess(func(podCmd string) {
			Expect(podCmd).To(ContainSubstring(strconv.Itoa(t.submariner.Spec.CeIPSecNATTPort)))
		})
	})

	When("the tunnel port is configured", func() {
		BeforeEach(func() {
			t.localEndpoint.Spec.BackendConfig[submarinerv1.UDPPortConfig] = strconv.Itoa(777)
		})

		t.testTunnelConnectivitySuccess(func(podCmd string) {
			Expect(podCmd).To(ContainSubstring(t.localEndpoint.Spec.BackendConfig[submarinerv1.UDPPortConfig]))
		})

		Context("and it's invalid", func() {
			BeforeEach(func() {
				t.localEndpoint.Spec.BackendConfig[submarinerv1.UDPPortConfig] = "invalid-port"
			})

			t.testTunnelConfigFailure("tunnel", "invalid-port")
		})
	})

	When("load balancer is enabled", func() {
		BeforeEach(func() {
			t.localEndpoint.Spec.BackendConfig[submarinerv1.UsingLoadBalancer] = strconv.FormatBool(true)
		})

		t.testTunnelConnectivitySuccess(func(podCmd string) {
			Expect(podCmd).To(ContainSubstring(strconv.Itoa(lbEncapsPort)))
		})
	})

	When("the sniffer pod does not receive the expected message from the client pod", func() {
		t.testTunnelConnectivityFailure("client message not received", "sniffer pod")
	})
})

type tunnelFirewallTestDriver struct {
	*firewallTestDriver
}

func (t *tunnelFirewallTestDriver) run() error {
	return diagnose.TunnelConfigAcrossClusters(t.localClusterInfo, t.remoteClusterInfo,
		constants.OperatorNamespace, t.options, t.statusTracker)
}

func (t *tunnelFirewallTestDriver) testTunnelConnectivitySuccess(verifyCmd func(string)) {
	t.testPortConnectivitySuccess(t.run, "", verifyCmd)
}

func (t *tunnelFirewallTestDriver) testTunnelConnectivityFailure(podOutput string, msgs ...string) {
	t.testPortConnectivityFailure(t.run, podOutput, msgs...)
}

func (t *tunnelFirewallTestDriver) testTunnelConfigFailure(msgs ...string) {
	t.testFailure(t.run, msgs...)
}
