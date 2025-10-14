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

package subctl_test

import (
	"context"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"
	"github.com/submariner-io/admiral/pkg/reporter"
	"github.com/submariner-io/subctl/cmd/subctl"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/pkg/broker"
	"github.com/submariner-io/subctl/pkg/client"
	"github.com/submariner-io/subctl/pkg/join"
	"golang.org/x/net/http/httpproxy"
)

const (
	clusterCIDR = "1.2.3.4/16"
	serviceCIDR = "4.5.6.7/16"
)

var _ = Describe("Join", func() {
	Describe("Check Arguments", testCheckArguments)
	Describe("Run Command", testRunJoinCommand)
})

//nolint:maintidx // Ignore Maintainability Index
func testRunJoinCommand() {
	t := newTestDriver()

	BeforeEach(func() {
		t.cmd = subctl.NewJoinCmd()

		subctl.JoinClusterToBroker = func(_ context.Context, _ *broker.Info, _ *join.Options, _ client.Producer, _ reporter.Interface) error {
			return nil
		}

		t.args = []string{brokerInfoFileName}
	})

	When("with no user input", func() {
		BeforeEach(func() {
			t.setupNetworkDiscovery(clusterCIDR, serviceCIDR)
			t.createGatewayNode()
		})

		It("should invoke join with the defaults", func() {
			subctl.JoinClusterToBroker = func(_ context.Context, info *broker.Info, options *join.Options,
				clientProducer client.Producer, _ reporter.Interface,
			) error {
				Expect(info).To(Equal(&brokerInfo))

				Expect(clientProducer.ForKubernetes()).To(Equal(t.fakeProducer.KubeClient))
				Expect(clientProducer.ForDynamic()).To(Equal(t.fakeProducer.DynamicClient))
				Expect(clientProducer.ForGeneral()).To(Equal(t.fakeProducer.GeneralClient))

				Expect(options.PreferredServer).To(BeFalse())
				Expect(options.ForceUDPEncaps).To(BeFalse())
				Expect(options.NATTraversal).To(BeTrue())
				Expect(options.IgnoreRequirements).To(BeFalse())
				Expect(options.GlobalnetEnabled).To(BeTrue())
				Expect(options.IPSecDebug).To(BeFalse())
				Expect(options.SubmarinerDebug).To(BeFalse())
				Expect(options.OperatorDebug).To(BeFalse())
				Expect(options.AirGappedDeployment).To(BeFalse())
				Expect(options.LoadBalancerEnabled).To(BeFalse())
				Expect(options.HealthCheckEnabled).To(BeTrue())
				Expect(options.BrokerK8sSecure).To(BeTrue())
				Expect(options.EnableClustersetIP).To(BeFalse())
				Expect(options.DisableIntraClusterConnectivity).To(BeFalse())
				Expect(options.NATTPort).To(Equal(4500))
				Expect(options.GlobalnetClusterSize).To(Equal(uint(0)))
				Expect(options.HealthCheckInterval).To(Equal(uint(1)))
				Expect(options.HealthCheckMaxPacketLossCount).To(Equal(uint(5)))
				Expect(options.ClusterID).To(Equal(clusterName))
				Expect(options.ClusterCIDR).To(BeEmpty())
				Expect(options.ServiceCIDR).To(BeEmpty())
				Expect(options.GlobalnetCIDR).To(BeEmpty())
				Expect(options.Repository).To(BeEmpty())
				Expect(options.ImageVersion).To(BeEmpty())
				Expect(options.CableDriver).To(Equal("libreswan"))
				Expect(options.CoreDNSCustomConfigMap).To(BeEmpty())
				Expect(options.BrokerURL).To(BeEmpty())
				Expect(options.ClustersetIPCIDR).To(BeEmpty())
				Expect(options.CustomDomains).To(BeEmpty())
				Expect(options.ImageOverrideArr).To(BeEmpty())
				Expect(options.HTTPProxyConfig).To(Equal(httpproxy.Config{}))
				Expect(options.UseIPSecCertAuthMode).To(BeFalse())

				return nil
			}

			Expect(t.cmd.Execute()).To(Succeed())
			t.status.AssertFailureCount(0)
			t.status.AssertWarningCount(0)
		})
	})

	When("various flags are specified on the command line", func() {
		otherBrokerURL := "http://other-broker"

		BeforeEach(func() {
			t.setupNetworkDiscovery(clusterCIDR, serviceCIDR)
			t.createGatewayNode()

			t.args = append(t.args, "--preferred-server", "--force-udp-encaps", "--natt=false",
				"--ignore-requirements", "--globalnet=false", "--ipsec-debug", "--pod-debug", "--operator-debug",
				"--air-gapped", "--load-balancer", "--health-check=false", "--check-broker-certificate=false",
				"--enable-clusterset-ip", "--nattport=1234", "--globalnet-cluster-size=16",
				"--health-check-max-packet-loss-count=9", "--globalnet-cidr=100.0.0.0/32", "--repository=localhost",
				"--version=0.21.0", "--cable-driver=vxlan", "--coredns-custom-configmap=my-map",
				"--clusterset-ip-cidr=10.0.0.0/32", "--custom-domains=domain1,domain2",
				"--image-override=submariner-gateway=http://localhost", "--broker-url="+otherBrokerURL,
				"--http-proxy=http://my-proxy", "--https-proxy=https://my-proxy", "--no-proxy=foo",
				"--disable-intra-cluster-connectivity", "--use-ipsec-cert-auth-mode")
		})

		It("should invoke join with the specified values", func() {
			subctl.JoinClusterToBroker = func(_ context.Context, info *broker.Info, options *join.Options,
				clientProducer client.Producer, _ reporter.Interface,
			) error {
				Expect(options.PreferredServer).To(BeTrue())
				Expect(options.ForceUDPEncaps).To(BeTrue())
				Expect(options.NATTraversal).To(BeFalse())
				Expect(options.IgnoreRequirements).To(BeTrue())
				Expect(options.GlobalnetEnabled).To(BeFalse())
				Expect(options.IPSecDebug).To(BeTrue())
				Expect(options.SubmarinerDebug).To(BeTrue())
				Expect(options.OperatorDebug).To(BeTrue())
				Expect(options.AirGappedDeployment).To(BeTrue())
				Expect(options.LoadBalancerEnabled).To(BeTrue())
				Expect(options.HealthCheckEnabled).To(BeFalse())
				Expect(options.BrokerK8sSecure).To(BeFalse())
				Expect(options.EnableClustersetIP).To(BeTrue())
				Expect(options.DisableIntraClusterConnectivity).To(BeTrue())
				Expect(options.NATTPort).To(Equal(1234))
				Expect(options.GlobalnetClusterSize).To(Equal(uint(16)))
				Expect(options.HealthCheckInterval).To(Equal(uint(1)))
				Expect(options.HealthCheckMaxPacketLossCount).To(Equal(uint(9)))
				Expect(options.ClusterID).To(Equal(clusterName))
				Expect(options.ClusterCIDR).To(BeEmpty())
				Expect(options.ServiceCIDR).To(BeEmpty())
				Expect(options.GlobalnetCIDR).To(Equal("100.0.0.0/32"))
				Expect(options.Repository).To(Equal("localhost"))
				Expect(options.ImageVersion).To(Equal("0.21.0"))
				Expect(options.CableDriver).To(Equal("vxlan"))
				Expect(options.CoreDNSCustomConfigMap).To(Equal("my-map"))
				Expect(options.BrokerURL).To(Equal(otherBrokerURL))
				Expect(options.ClustersetIPCIDR).To(Equal("10.0.0.0/32"))
				Expect(options.CustomDomains).To(Equal([]string{"domain1", "domain2"}))
				Expect(options.ImageOverrideArr).To(Equal([]string{"submariner-gateway=http://localhost"}))
				Expect(options.HTTPProxyConfig).To(Equal(httpproxy.Config{
					HTTPProxy:  "http://my-proxy",
					HTTPSProxy: "https://my-proxy",
					NoProxy:    "foo",
				}))

				Expect(info.BrokerURL).To(Equal(options.BrokerURL))
				Expect(options.UseIPSecCertAuthMode).To(BeTrue())

				return nil
			}

			Expect(t.cmd.Execute()).To(Succeed())
			t.status.AssertFailureCount(0)
			t.status.AssertWarningCount(0)
		})
	})

	When("the network details aren't discovered", func() {
		BeforeEach(func() {
			t.createGatewayNode()
			setupPrompts(map[string]any{"Pod CIDR": clusterCIDR, "Service CIDR": serviceCIDR})
		})

		It("should emit a warning and prompt for the cluster and service CIDRs", func() {
			subctl.JoinClusterToBroker = func(_ context.Context, _ *broker.Info, options *join.Options,
				_ client.Producer, _ reporter.Interface,
			) error {
				Expect(options.ClusterCIDR).To(Equal(clusterCIDR))
				Expect(options.ServiceCIDR).To(Equal(serviceCIDR))

				return nil
			}

			Expect(t.cmd.Execute()).To(Succeed())
			t.status.AssertWarningCount(1)
		})
	})

	When("the cluster and service CIDRs are specified on the command line", func() {
		BeforeEach(func() {
			t.createGatewayNode()

			t.args = append(t.args, "--clustercidr="+clusterCIDR, "--servicecidr="+serviceCIDR)

			subctl.JoinClusterToBroker = func(_ context.Context, _ *broker.Info, options *join.Options,
				_ client.Producer, _ reporter.Interface,
			) error {
				Expect(options.ClusterCIDR).To(Equal(clusterCIDR))
				Expect(options.ServiceCIDR).To(Equal(serviceCIDR))

				return nil
			}
		})

		Context("and the network details aren't discovered", func() {
			It("should invoke join with the specified values", func() {
				Expect(t.cmd.Execute()).To(Succeed())
			})
		})

		Context("and they match the discovered network details", func() {
			BeforeEach(func() {
				t.setupNetworkDiscovery(clusterCIDR, serviceCIDR)
			})

			It("should invoke join with the specified values", func() {
				Expect(t.cmd.Execute()).To(Succeed())
				t.status.AssertWarningCount(0)
			})
		})

		Context("and they don't match the discovered network details", func() {
			BeforeEach(func() {
				t.setupNetworkDiscovery("10.20.30.40/32", "11.20.30.40/32")
			})

			It("should emit a warning and invoke join with the specified values", func() {
				Expect(t.cmd.Execute()).To(Succeed())
				t.status.AssertWarningCount(2)
				t.status.AssertContainsWarning(ContainSubstring("10.20.30.40/32"))
				t.status.AssertContainsWarning(ContainSubstring("11.20.30.40/32"))
			})
		})
	})

	When("the cluster ID is specified on the command line", func() {
		var clusterID string

		BeforeEach(func() {
			t.createGatewayNode()
			t.setupNetworkDiscovery(clusterCIDR, serviceCIDR)

			clusterID = "north-america"
			t.args = append(t.args, "--clusterid="+clusterID)
		})

		JustBeforeEach(func() {
			subctl.JoinClusterToBroker = func(_ context.Context, _ *broker.Info, options *join.Options,
				_ client.Producer, _ reporter.Interface,
			) error {
				Expect(options.ClusterID).To(Equal(clusterID))

				return nil
			}
		})

		It("should invoke join with the specified value", func() {
			Expect(t.cmd.Execute()).To(Succeed())
		})

		Context("but it's not valid", func() {
			BeforeEach(func() {
				setupPrompts(map[string]any{"cluster ID": clusterID})

				last := len(t.args) - 1
				t.args[last] = strings.Replace(t.args[last], clusterID, "!invalid", 1)
			})

			It("should emit a message and prompt for the cluster ID", func() {
				Expect(t.cmd.Execute()).To(Succeed())
				t.status.AssertContainsFailure(ContainSubstring("Invalid cluster ID"))
			})
		})
	})

	When("there's one worker node", func() {
		BeforeEach(func() {
			t.setupNetworkDiscovery(clusterCIDR, serviceCIDR)
			t.createNodes("worker1")
		})

		It("should label it as a gateway", func() {
			Expect(t.cmd.Execute()).To(Succeed())
			Expect(t.getNode("worker1").Labels).To(HaveKeyWithValue(constants.SubmarinerGatewayLabel, constants.TrueLabel))
			t.status.AssertWarningCount(0)
		})

		Context("and the label-gateway flag is specified as false on the command line", func() {
			BeforeEach(func() {
				t.args = append(t.args, "--label-gateway=false")
			})

			It("should not label it as a gateway", func() {
				Expect(t.cmd.Execute()).To(Succeed())
				Expect(t.getNode("worker1").Labels).NotTo(HaveKeyWithValue(constants.SubmarinerGatewayLabel, constants.TrueLabel))
				t.status.AssertWarningCount(0)
			})
		})
	})

	When("there's multiple worker nodes", func() {
		selectedNode := "worker2"

		BeforeEach(func() {
			t.setupNetworkDiscovery(clusterCIDR, serviceCIDR)
			t.createNodes("worker1", selectedNode)
			setupPrompts(map[string]any{"gateway": selectedNode})
		})

		It("should prompt for which node to label as a gateway", func() {
			Expect(t.cmd.Execute()).To(Succeed())
			Expect(t.getNode(selectedNode).Labels).To(HaveKeyWithValue(constants.SubmarinerGatewayLabel, constants.TrueLabel))
			t.status.AssertWarningCount(0)
		})
	})

	When("there's no worker nodes", func() {
		BeforeEach(func() {
			t.setupNetworkDiscovery(clusterCIDR, serviceCIDR)
		})

		It("should emit a warning", func() {
			Expect(t.cmd.Execute()).To(Succeed())
			t.status.AssertWarningCount(1)
		})
	})
}

func testCheckArguments() {
	var cmd *cobra.Command

	BeforeEach(func() {
		cmd = subctl.NewJoinCmd()
	})

	When("the broker-info file arg is missing", func() {
		It("should fail", func() {
			Expect(cmd.Args(cmd, []string{})).NotTo(Succeed())
		})
	})

	When("the more than one argument is specified", func() {
		It("should fail", func() {
			Expect(cmd.Args(cmd, []string{"broker-info.subm", "other"})).NotTo(Succeed())
		})
	})

	When("a specified image override is invalid", func() {
		It("should fail", func() {
			Expect(cmd.Flags().Set("image-override", "invalid=http://localhost")).To(Succeed())
			Expect(cmd.Args(cmd, []string{"broker-info.subm"})).NotTo(Succeed())
		})
	})
}
