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
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/pkg/diagnose"
	submarinerv1 "github.com/submariner-io/submariner/pkg/apis/submariner.io/v1"
	"github.com/submariner-io/submariner/pkg/cni"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const remoteEndpointIP = "192.168.1.0"

var _ = Describe("FirewallIntraVxLANConfig", func() {
	t := newVxLanFirewallTestDriver()

	When("configured properly", func() {
		t.testVxLANConnectivitySuccess(func(podCmd string) {
			Expect(podCmd).To(And(ContainSubstring("vx-submariner"), ContainSubstring("8080")))
		})
	})

	When("the network plugin is OVNKubernetes", func() {
		JustBeforeEach(func() {
			t.localClusterInfo.Submariner.Status.NetworkPlugin = cni.OVNKubernetes
		})

		It("should skip the check and succeed", func() {
			t.assertSuccess(t.run)
		})
	})

	When("there is no remote Endpoint", func() {
		BeforeEach(func() {
			t.remoteEndpoint = nil
		})

		t.testVxLANConfigFailure("endpoint", "not found")
	})

	When("the sniffer pod does not receive the remote endpoint IP in the tcpdump output", func() {
		t.testVxLANConnectivityFailure("pod output without remote IP", "remote endpoint IP", "firewall")
	})

	When("the sniffer pod does not receive the client pod IP in the tcpdump output", func() {
		// Output contains remote endpoint IP but NOT client pod IP
		t.testVxLANConnectivityFailure("tcpdump output with remote IP: "+remoteEndpointIP, "client pod's IP", "IPTable")
	})

	t.testFirewallCheckOnSingleNode(t.run)

	t.testActiveGatewayPodErrors(t.run)

	t.testPodCreationFailure(t.run)

	t.testIncompletePod(t.run)

	t.testImageRepositoryFailure(t.run)
})

type vxLanFirewallTestDriver struct {
	*firewallTestDriver
	remoteEndpoint *submarinerv1.Endpoint
}

func newVxLanFirewallTestDriver() *vxLanFirewallTestDriver {
	t := &vxLanFirewallTestDriver{firewallTestDriver: newFirewallTestDriver()}

	BeforeEach(func() {
		t.remoteEndpoint = &submarinerv1.Endpoint{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "remote-endpoint",
				Namespace: constants.OperatorNamespace,
			},
			Spec: submarinerv1.EndpointSpec{
				ClusterID: remoteCluster,
				Subnets:   []string{remoteEndpointIP + "/24"},
			},
		}
	})

	JustBeforeEach(func() {
		if t.remoteEndpoint != nil {
			t.createResource(t.remoteEndpoint)
		}
	})

	return t
}

func (t *vxLanFirewallTestDriver) run() error {
	return diagnose.FirewallIntraVxLANConfig(t.localClusterInfo, constants.OperatorNamespace, t.options, t.statusTracker)
}

func (t *vxLanFirewallTestDriver) testVxLANConnectivitySuccess(verifyCmd func(string)) {
	podOutput := "tcpdump: listening on vx-submariner\n" +
		"10:00:00.000000 IP " + clientPodIP + ".12345 > " + remoteEndpointIP + ".8080: Flags [S], seq 0, win 64240\n" +
		"10:00:00.100000 IP " + clientPodIP + ".12346 > " + remoteEndpointIP + ".8080: Flags [S], seq 0, win 64240\n"

	t.testPortConnectivitySuccess(t.run, podOutput, verifyCmd)
}

func (t *vxLanFirewallTestDriver) testVxLANConnectivityFailure(podOutput string, msgs ...string) {
	t.testPortConnectivityFailure(t.run, podOutput, msgs...)
}

func (t *vxLanFirewallTestDriver) testVxLANConfigFailure(msgs ...string) {
	t.testFailure(t.run, msgs...)
}
