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

package join_test

import (
	"encoding/base64"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	compnames "github.com/submariner-io/admiral/pkg/names"
	"github.com/submariner-io/admiral/pkg/reporter"
	"github.com/submariner-io/subctl/internal/component"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/pkg/broker"
	"github.com/submariner-io/subctl/pkg/join"
	"github.com/submariner-io/subctl/pkg/serviceaccount"
	submariner "github.com/submariner-io/submariner-operator/api/v1alpha1"
	"github.com/submariner-io/submariner-operator/pkg/discovery/clustersetip"
	"github.com/submariner-io/submariner-operator/pkg/discovery/globalnet"
	"github.com/submariner-io/submariner-operator/pkg/names"
	submarinerv1 "github.com/submariner-io/submariner/pkg/apis/submariner.io/v1"
	"golang.org/x/net/http/httpproxy"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("ClusterToBroker", func() {
	Describe("Deployment", testDeployment)
	Describe("Globalnet", testGlobalnet)
	Describe("Clusterset IP", testClustersetIP)
	Describe("Requirements", testRequirements)
	Describe("Image overrides", testImageOverrides)
	Describe("Invalid input", testInvalidInput)
})

func testDeployment() {
	t := newTestDriver()

	BeforeEach(func() {
		t.options = join.Options{
			PreferredServer:                 true,
			ForceUDPEncaps:                  true,
			NATTraversal:                    true,
			IPSecDebug:                      true,
			SubmarinerDebug:                 true,
			OperatorDebug:                   true,
			AirGappedDeployment:             true,
			LoadBalancerEnabled:             true,
			HealthCheckEnabled:              true,
			DisableIntraClusterConnectivity: true,
			BrokerK8sSecure:                 false,
			GlobalnetEnabled:                true,
			NATTPort:                        123,
			HealthCheckInterval:             1,
			HealthCheckMaxPacketLossCount:   5,
			ClusterID:                       "east",
			ServiceCIDR:                     "101.42.0.0/16",
			ClusterCIDR:                     "201.67.0.0/16",
			CableDriver:                     "vxlan",
			CoreDNSCustomConfigMap:          "my-map",
			CustomDomains:                   []string{"my-domain"},
			HTTPProxyConfig: httpproxy.Config{
				HTTPProxy: "http://my-proxy",
			},
			UseIPSecCertAuthMode: true,
		}
	})

	It("should deploy the operator and Submariner resource", func(ctx SpecContext) {
		Expect(t.runJoinClusterToBroker()).To(Succeed())

		saName := names.ForClusterSA(t.options.ClusterID)

		_, err := t.fakeProducer.KubeClient.CoreV1().ServiceAccounts(brokerNamespace).Get(ctx, saName, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())

		_, err = serviceaccount.GetTokenSecretFor(ctx, t.fakeProducer.KubeClient, brokerNamespace, saName)
		Expect(err).NotTo(HaveOccurred())

		secret, err := t.fakeProducer.KubeClient.CoreV1().Secrets(constants.OperatorNamespace).Get(ctx,
			broker.LocalClientBrokerSecretName, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())

		subm := t.assertSubmarinerResource()
		Expect(subm.Spec.CeIPSecUseOVNCertAuthMode).To(BeTrue())
		Expect(subm.Spec.Namespace).To(Equal(constants.OperatorNamespace))
		Expect(subm.Spec.ClusterID).To(Equal(t.options.ClusterID))
		Expect(subm.Spec.ServiceCIDR).To(Equal(t.options.ServiceCIDR))
		Expect(subm.Spec.ClusterCIDR).To(Equal(t.options.ClusterCIDR))
		Expect(subm.Spec.GlobalCIDR).To(BeEmpty())
		Expect(subm.Spec.CableDriver).To(Equal(t.options.CableDriver))

		Expect(subm.Spec.NatEnabled).To(Equal(t.options.NATTraversal))
		Expect(subm.Spec.AirGappedDeployment).To(Equal(t.options.AirGappedDeployment))
		Expect(subm.Spec.LoadBalancerEnabled).To(Equal(t.options.LoadBalancerEnabled))
		Expect(subm.Spec.Debug).To(Equal(t.options.SubmarinerDebug))
		Expect(subm.Spec.ServiceDiscoveryEnabled).To(BeTrue())
		Expect(subm.Spec.DisableIntraClusterConnectivity).To(BeTrue())

		Expect(subm.Spec.ClustersetIPEnabled).To(BeFalse())
		Expect(subm.Spec.ClustersetIPCIDR).To(HavePrefix("243."))

		Expect(subm.Spec.ConnectionHealthCheck).To(Equal(&submariner.HealthCheckSpec{
			Enabled:            t.options.HealthCheckEnabled,
			IntervalSeconds:    t.options.HealthCheckInterval,
			MaxPacketLossCount: t.options.HealthCheckMaxPacketLossCount,
		}))

		Expect(subm.Spec.Version).To(Equal(submariner.DefaultSubmarinerOperatorVersion))
		Expect(subm.Spec.Repository).To(Equal(submariner.DefaultRepo))
		Expect(subm.Spec.ImageOverrides).To(BeEmpty())

		Expect(subm.Spec.BrokerK8sRemoteNamespace).To(Equal(brokerNamespace))
		Expect(subm.Spec.BrokerK8sApiServerToken).To(Equal(string(secret.Data["token"])))
		Expect(subm.Spec.BrokerK8sApiServer).To(Equal(brokerApiServer))
		Expect(subm.Spec.BrokerK8sSecret).To(Equal(broker.LocalClientBrokerSecretName))
		Expect(subm.Spec.BrokerK8sInsecure).To(BeTrue())

		Expect(subm.Spec.CeIPSecNATTPort).To(Equal(t.options.NATTPort))
		Expect(subm.Spec.CeIPSecDebug).To(Equal(t.options.IPSecDebug))
		Expect(subm.Spec.CeIPSecForceUDPEncaps).To(Equal(t.options.ForceUDPEncaps))
		Expect(subm.Spec.CeIPSecPreferredServer).To(Equal(t.options.PreferredServer))
		Expect(subm.Spec.CeIPSecPSK).To(Equal(base64.StdEncoding.EncodeToString(t.brokerInfo.IPSecPSK.Data["psk"])))
		Expect(subm.Spec.CeIPSecPSKSecret).To(Equal(t.brokerInfo.IPSecPSK.Name))

		Expect(subm.Spec.CustomDomains).To(Equal(t.options.CustomDomains))
		Expect(subm.Spec.CoreDNSCustomConfig).To(Equal(&submariner.CoreDNSCustomConfig{
			ConfigMapName: t.options.CoreDNSCustomConfigMap,
		}))

		deployment := t.getOperatorDeployment()
		Expect(deployment.Spec.Template.Spec.Containers[0].Env).To(ContainElement(corev1.EnvVar{
			Name:  "HTTP_PROXY",
			Value: t.options.HTTPProxyConfig.HTTPProxy,
		}))

		t.assertNoServiceDiscoveryResource()
	})

	When("only service discovery is enabled", func() {
		BeforeEach(func() {
			t.brokerInfo.Components = []string{component.ServiceDiscovery}
		})

		It("should only deploy the ServiceDiscovery resource", func(ctx SpecContext) {
			Expect(t.runJoinClusterToBroker()).To(Succeed())

			secret, err := t.fakeProducer.KubeClient.CoreV1().Secrets(constants.OperatorNamespace).Get(ctx,
				broker.LocalClientBrokerSecretName, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())

			sd := t.assertServiceDiscoveryResource()
			Expect(sd.Spec.Namespace).To(Equal(constants.OperatorNamespace))
			Expect(sd.Spec.ClusterID).To(Equal(t.options.ClusterID))
			Expect(sd.Spec.ClustersetIPEnabled).To(BeFalse())
			Expect(sd.Spec.ClustersetIPCIDR).To(HavePrefix("243."))

			Expect(sd.Spec.BrokerK8sRemoteNamespace).To(Equal(brokerNamespace))
			Expect(sd.Spec.BrokerK8sApiServerToken).To(Equal(string(secret.Data["token"])))
			Expect(sd.Spec.BrokerK8sApiServer).To(Equal(brokerApiServer))
			Expect(sd.Spec.BrokerK8sSecret).To(Equal(broker.LocalClientBrokerSecretName))
			Expect(sd.Spec.BrokerK8sInsecure).To(BeTrue())

			Expect(sd.Spec.CustomDomains).To(Equal(t.options.CustomDomains))
			Expect(sd.Spec.CoreDNSCustomConfig).To(Equal(&submariner.CoreDNSCustomConfig{
				ConfigMapName: t.options.CoreDNSCustomConfigMap,
			}))

			t.assertNoSubmarinerResource()
		})
	})
}

func testGlobalnet() {
	t := newTestDriver()

	When("enabled", func() {
		BeforeEach(func() {
			t.globalnetEnabled = true
			t.options.GlobalnetEnabled = true
		})

		Context("and no global CIDR is specified", func() {
			It("should allocate a CIDR and set the GlobalCIDR field", func() {
				Expect(t.runJoinClusterToBroker()).To(Succeed())
				Expect(t.assertSubmarinerResource().Spec.GlobalCIDR).To(HavePrefix("242."))
			})
		})

		Context("and a global CIDR is specified", func() {
			BeforeEach(func() {
				t.options.GlobalnetCIDR = "242.0.0.0/32"
			})

			It("should use the specified CIDR to set the GlobalCIDR field", func() {
				Expect(t.runJoinClusterToBroker()).To(Succeed())
				Expect(t.assertSubmarinerResource().Spec.GlobalCIDR).To(Equal(t.options.GlobalnetCIDR))
			})

			Context("but it overlaps", func() {
				JustBeforeEach(func(ctx SpecContext) {
					err := globalnet.AllocateAndUpdateGlobalCIDRConfigMap(ctx, t.fakeProducer.ForGeneral(), brokerNamespace,
						&globalnet.Config{
							ClusterID:  "west",
							GlobalCIDR: t.options.GlobalnetCIDR,
						}, reporter.Silent())
					Expect(err).NotTo(HaveOccurred())
				})

				It("should fail", func() {
					Expect(t.runJoinClusterToBroker()).NotTo(Succeed())
				})
			})
		})

		Context("but disabled via the GlobalnetEnabled option", func() {
			BeforeEach(func() {
				t.options.GlobalnetEnabled = false
			})

			It("should not set the GlobalCIDR field", func() {
				Expect(t.runJoinClusterToBroker()).To(Succeed())
				Expect(t.assertSubmarinerResource().Spec.GlobalCIDR).To(BeEmpty())
			})
		})
	})
}

func testClustersetIP() {
	t := newTestDriver()

	When("enabled globally", func() {
		BeforeEach(func() {
			t.clustersetIPEnabled = true
		})

		It("should set the ClustersetIPEnabled field to true", func() {
			Expect(t.runJoinClusterToBroker()).To(Succeed())

			subm := t.assertSubmarinerResource()
			Expect(subm.Spec.ClustersetIPEnabled).To(BeTrue())
			Expect(subm.Spec.ClustersetIPCIDR).To(HavePrefix("243."))
		})

		Context("and a CIDR is specified", func() {
			BeforeEach(func() {
				t.options.ClustersetIPCIDR = "243.1.0.0/16"
			})

			It("should use the specified CIDR to set the ClustersetIPCIDR field", func() {
				Expect(t.runJoinClusterToBroker()).To(Succeed())

				subm := t.assertSubmarinerResource()
				Expect(subm.Spec.ClustersetIPEnabled).To(BeTrue())
				Expect(subm.Spec.ClustersetIPCIDR).To(Equal(t.options.ClustersetIPCIDR))
			})

			Context("but it overlaps", func() {
				JustBeforeEach(func(ctx SpecContext) {
					_, err := clustersetip.AllocateCIDRFromConfigMap(ctx, t.fakeProducer.ForGeneral(), brokerNamespace,
						&clustersetip.Config{
							ClusterID:        "west",
							ClustersetIPCIDR: t.options.ClustersetIPCIDR,
						}, reporter.Silent())
					Expect(err).NotTo(HaveOccurred())
				})

				It("should fail", func() {
					Expect(t.runJoinClusterToBroker()).NotTo(Succeed())
				})
			})
		})

		Context("but service discovery is disabled", func() {
			BeforeEach(func() {
				t.brokerInfo.Components = []string{component.Connectivity}
			})

			It("should set the ClustersetIPEnabled field to false and not allocate a CIDR", func() {
				Expect(t.runJoinClusterToBroker()).To(Succeed())

				subm := t.assertSubmarinerResource()
				Expect(subm.Spec.ServiceDiscoveryEnabled).To(BeFalse())
				Expect(subm.Spec.ClustersetIPEnabled).To(BeFalse())
				Expect(subm.Spec.ClustersetIPCIDR).To(BeEmpty())
			})
		})
	})

	When("disabled globally but enabled via the EnableClustersetIP option", func() {
		BeforeEach(func() {
			t.options.EnableClustersetIP = true
		})

		It("should set the ClustersetIPEnabled field to true", func() {
			Expect(t.runJoinClusterToBroker()).To(Succeed())
			Expect(t.assertSubmarinerResource().Spec.ClustersetIPEnabled).To(BeTrue())
		})
	})
}

func testRequirements() {
	t := newTestDriver()

	When("the K8s version isn't supported", func() {
		BeforeEach(func() {
			t.setFakedServerVersion("1", "15")
		})

		It("should fail", func() {
			Expect(t.runJoinClusterToBroker()).NotTo(Succeed())
		})

		Context("but the IgnoreRequirements option is set to true", func() {
			BeforeEach(func() {
				t.options.IgnoreRequirements = true
			})

			It("should succeed and emit a warning", func() {
				Expect(t.runJoinClusterToBroker()).To(Succeed())
				t.status.AssertWarningCount(1)
			})
		})
	})
}

func testImageOverrides() {
	t := newTestDriver()

	When("the operator image is specified", func() {
		operatorImage := "my-operator-image"

		BeforeEach(func() {
			t.options.ImageOverrideArr = []string{compnames.OperatorComponent + "=" + operatorImage}
		})

		It("should set the image in the deployment spec", func() {
			Expect(t.runJoinClusterToBroker()).To(Succeed())
			Expect(t.getOperatorDeployment().Spec.Template.Spec.Containers[0].Image).To(Equal(operatorImage))
		})
	})

	When("the Repository and ImageVersion are specified", func() {
		BeforeEach(func() {
			t.options.Repository = "my-repo.io"
			t.options.ImageVersion = "0.20.0"
		})

		It("should set the correct operator image in the deployment spec", func() {
			Expect(t.runJoinClusterToBroker()).To(Succeed())
			Expect(t.getOperatorDeployment().Spec.Template.Spec.Containers[0].Image).To(
				Equal(fmt.Sprintf("%s/%s:%s", t.options.Repository, names.OperatorImage, t.options.ImageVersion)))

			subm := t.assertSubmarinerResource()
			Expect(subm.Spec.Version).To(Equal(t.options.ImageVersion))
			Expect(subm.Spec.Repository).To(Equal(t.options.Repository))
		})
	})

	When("component image overrides are specified", func() {
		gwImage := "my-gw-image"
		raImage := "my-ra-image"

		BeforeEach(func() {
			t.options.ImageOverrideArr = []string{
				compnames.GatewayComponent + "=" + gwImage,
				compnames.RouteAgentComponent + "=" + raImage,
			}
		})

		It("should set the ImageOverrides field", func() {
			Expect(t.runJoinClusterToBroker()).To(Succeed())
			Expect(t.assertSubmarinerResource().Spec.ImageOverrides).To(Equal(map[string]string{
				compnames.GatewayComponent:    gwImage,
				compnames.RouteAgentComponent: raImage,
			}))
		})
	})
}

func testInvalidInput() {
	t := newTestDriver()

	When("the cluster ID isn't unique", func() {
		BeforeEach(func(ctx SpecContext) {
			Expect(t.fakeProducer.GeneralClient.Create(ctx, &submarinerv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: brokerNamespace,
					Name:      t.options.ClusterID,
				},
			})).To(Succeed())
		})

		It("should fail", func() {
			Expect(t.runJoinClusterToBroker()).NotTo(Succeed())
		})
	})

	When("an image override component isn't valid", func() {
		BeforeEach(func() {
			t.options.ImageOverrideArr = []string{"invalid=image"}
		})

		It("should fail", func() {
			Expect(t.runJoinClusterToBroker()).NotTo(Succeed())
		})
	})

	When("the specified CoreDNSCustomConfigMap is invalid", func() {
		BeforeEach(func() {
			t.options.CoreDNSCustomConfigMap = "this/is/invalid"
		})

		It("should fail", func() {
			Expect(t.runJoinClusterToBroker()).NotTo(Succeed())
		})
	})
}
