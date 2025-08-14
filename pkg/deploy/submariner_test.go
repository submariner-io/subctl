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

package deploy_test

import (
	"context"
	"encoding/base64"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/submariner-io/admiral/pkg/fake"
	"github.com/submariner-io/subctl/internal/component"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/pkg/deploy"
	operatorv1alpha1 "github.com/submariner-io/submariner-operator/api/v1alpha1"
	"github.com/submariner-io/submariner-operator/pkg/discovery/globalnet"
	"github.com/submariner-io/submariner-operator/pkg/names"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	ctrlClient "sigs.k8s.io/controller-runtime/pkg/client"
	controllerfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

var (
	_ = Describe("Submariner", testSubmariner)
	_ = Describe("RemoveSchemaPrefix", testRemoveSchemaPrefix)
	_ = Describe("ParseCustomCoreDNSParam", testParseCustomCoreDNSParam)
)

func testSubmariner() {
	t := newTestDriver()

	var (
		options   *deploy.SubmarinerOptions
		netconfig globalnet.Config
	)

	BeforeEach(func() {
		options = &deploy.SubmarinerOptions{
			PreferredServer:               true,
			ForceUDPEncaps:                true,
			NATTraversal:                  true,
			IPSecDebug:                    true,
			SubmarinerDebug:               true,
			AirGappedDeployment:           true,
			LoadBalancerEnabled:           true,
			HealthCheckEnabled:            true,
			BrokerK8sInsecure:             true,
			ClustersetIPEnabled:           true,
			NATTPort:                      4500,
			HealthCheckInterval:           30,
			HealthCheckMaxPacketLossCount: 5,
			ClusterID:                     "test-cluster",
			CableDriver:                   "vxlan",
			Repository:                    "quay.io/submariner",
			ImageVersion:                  "devel",
			ServiceCIDR:                   "10.96.0.0/12",
			ClusterCIDR:                   "10.244.0.0/16",
		}
	})

	runDepoy := func() error {
		return deploy.Submariner(ctx, t.fakeProducer, options, t.brokerInfo, t.brokerSecret, netconfig,
			t.clustersetConfig, t.repositoryInfo, t.statusReporter)
	}

	expectDeploySuccess := func() *operatorv1alpha1.SubmarinerSpec {
		Expect(runDepoy()).To(Succeed())

		submarinerCR := &operatorv1alpha1.Submariner{}
		Expect(t.fakeProducer.ForGeneral().Get(ctx, ctrlClient.ObjectKey{
			Name:      names.SubmarinerCrName,
			Namespace: constants.OperatorNamespace,
		}, submarinerCR)).To(Succeed())

		t.statusReporter.AssertWarningCount(0)
		t.statusReporter.AssertFailureCount(0)

		return &submarinerCR.Spec
	}

	expectDeployFailure := func() {
		Expect(runDepoy()).NotTo(Succeed())
		t.statusReporter.AssertHasFailure()
	}

	It("should successfully deploy the Submariner resource", func() {
		spec := expectDeploySuccess()
		Expect(spec.Repository).To(Equal(t.repositoryInfo.Name))
		Expect(spec.Version).To(Equal(t.repositoryInfo.Version))
		Expect(spec.CeIPSecNATTPort).To(Equal(options.NATTPort))
		Expect(spec.CeIPSecDebug).To(Equal(options.IPSecDebug))
		Expect(spec.CeIPSecForceUDPEncaps).To(Equal(options.ForceUDPEncaps))
		Expect(spec.CeIPSecPreferredServer).To(Equal(options.PreferredServer))
		Expect(spec.CeIPSecPSK).To(Equal(base64.StdEncoding.EncodeToString(t.brokerInfo.IPSecPSK.Data["psk"])))
		Expect(spec.CeIPSecPSKSecret).To(Equal(t.brokerInfo.IPSecPSK.Name))
		Expect(spec.BrokerK8sCA).To(Equal(base64.StdEncoding.EncodeToString(t.brokerSecret.Data["ca.crt"])))
		Expect(spec.BrokerK8sRemoteNamespace).To(Equal(string(t.brokerSecret.Data["namespace"])))
		Expect(spec.BrokerK8sApiServerToken).To(Equal(string(t.brokerSecret.Data["token"])))
		Expect(spec.BrokerK8sApiServer).To(Equal("broker.example.com:8443"))
		Expect(spec.BrokerK8sSecret).To(Equal(t.brokerSecret.ObjectMeta.Name))
		Expect(spec.BrokerK8sInsecure).To(Equal(options.BrokerK8sInsecure))
		Expect(spec.ClustersetIPEnabled).To(Equal(options.ClustersetIPEnabled))
		Expect(spec.ClustersetIPCIDR).To(Equal(t.clustersetConfig.ClustersetIPCIDR))
		Expect(spec.NatEnabled).To(Equal(options.NATTraversal))
		Expect(spec.Debug).To(Equal(options.SubmarinerDebug))
		Expect(spec.ClusterID).To(Equal(options.ClusterID))
		Expect(spec.ServiceCIDR).To(Equal(options.ServiceCIDR))
		Expect(spec.ClusterCIDR).To(Equal(options.ClusterCIDR))
		Expect(spec.Namespace).To(Equal(constants.OperatorNamespace))
		Expect(spec.CableDriver).To(Equal(options.CableDriver))
		Expect(spec.ServiceDiscoveryEnabled).To(BeTrue())
		Expect(spec.ImageOverrides).To(Equal(t.repositoryInfo.Overrides))
		Expect(spec.AirGappedDeployment).To(Equal(options.AirGappedDeployment))
		Expect(spec.LoadBalancerEnabled).To(Equal(options.LoadBalancerEnabled))
		Expect(spec.GlobalCIDR).To(Equal(netconfig.GlobalCIDR))
		Expect(spec.CustomDomains).To(Equal(options.CustomDomains))
		Expect(spec.CoreDNSCustomConfig).To(BeNil())
	})

	When("globalnet is enabled", func() {
		BeforeEach(func() {
			netconfig.GlobalCIDR = "242.0.0.0/8"
		})

		It("should set the GlobalCIDR field", func() {
			Expect(expectDeploySuccess().GlobalCIDR).To(Equal(netconfig.GlobalCIDR))
		})
	})

	When("the CoreDNS custom config map is provided", func() {
		BeforeEach(func() {
			options.CoreDNSCustomConfigMap = "test-namespace/test-configmap"
		})

		It("should set the CoreDNSCustomConfig field", func() {
			Expect(expectDeploySuccess().CoreDNSCustomConfig).To(Equal(&operatorv1alpha1.CoreDNSCustomConfig{
				ConfigMapName: "test-configmap",
				Namespace:     "test-namespace",
			}))
		})
	})

	When("custom domains are provided", func() {
		BeforeEach(func() {
			options.CustomDomains = []string{"custom.domain"}
		})

		It("should not set the CustomDomains field", func() {
			Expect(expectDeploySuccess().CustomDomains).To(Equal(options.CustomDomains))
		})
	})

	When("service discovery is disabled", func() {
		BeforeEach(func() {
			t.brokerInfo.Components = []string{component.Connectivity}
		})

		It("should set ServiceDiscoveryEnabled to false", func() {
			Expect(expectDeploySuccess().ServiceDiscoveryEnabled).To(BeFalse())
		})
	})

	Context("when PSK secret creation fails", func() {
		BeforeEach(func() {
			fake.FailOnAction(&t.fakeProducer.KubeClient.(*k8sfake.Clientset).Fake, "secrets", "create", nil, false)
		})

		It("should return an error", func() {
			expectDeployFailure()
		})
	})

	Context("when Submariner resource creation fails", func() {
		BeforeEach(func() {
			t.fakeProducer.GeneralClient = controllerfake.NewClientBuilder().WithScheme(scheme.Scheme).WithInterceptorFuncs(interceptor.Funcs{
				Create: func(_ context.Context, _ ctrlClient.WithWatch, _ ctrlClient.Object, _ ...ctrlClient.CreateOption) error {
					return errors.New("mock")
				},
			}).Build()
		})

		It("should return an error", func() {
			expectDeployFailure()
		})
	})
}

func testRemoveSchemaPrefix() {
	DescribeTable("should correctly remove the schema prefix",
		func(input, expected string) {
			result := deploy.RemoveSchemaPrefix(input)
			Expect(result).To(Equal(expected))
		},
		Entry("with https schema", "https://broker.example.com:8443", "broker.example.com:8443"),
		Entry("with http schema", "http://broker.example.com:8080", "broker.example.com:8080"),
		Entry("with custom schema", "custom://broker.example.com:9000", "broker.example.com:9000"),
		Entry("without schema", "broker.example.com:8443", "broker.example.com:8443"),
		Entry("with empty string", "", ""),
		Entry("with only schema", "https://", ""),
	)
}

func testParseCustomCoreDNSParam() {
	DescribeTable("should correctly parse the CoreDNS parameter",
		func(input, expectedNamespace, expectedName string) {
			namespace, name := deploy.ParseCustomCoreDNSParam(input)
			Expect(namespace).To(Equal(expectedNamespace))
			Expect(name).To(Equal(expectedName))
		},
		Entry("with namespace", "test-namespace/test-configmap", "test-namespace", "test-configmap"),
		Entry("without namespace", "test-configmap", "", "test-configmap"),
		Entry("with empty string", "", "", ""),
		Entry("with multiple slashes", "ns1/ns2/configmap", "ns1", "ns2"),
	)
}
