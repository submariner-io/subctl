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
	"errors"
	"slices"
	"strconv"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/submariner-io/admiral/pkg/reporter"
	"github.com/submariner-io/subctl/cmd/subctl"
	"github.com/submariner-io/subctl/internal/component"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/pkg/broker"
	"github.com/submariner-io/subctl/pkg/client"
	"github.com/submariner-io/subctl/pkg/deploy"
	"github.com/submariner-io/submariner-operator/api/v1alpha1"
	"github.com/submariner-io/submariner-operator/pkg/discovery/clustersetip"
	"github.com/submariner-io/submariner-operator/pkg/discovery/globalnet"
	"golang.org/x/net/http/httpproxy"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/utils/set"
)

var _ = Describe("Deploy Broker", func() {
	t := newTestDriver()

	var (
		expectedBrokerOptions deploy.BrokerOptions
		expectedIPsecPSK      []byte
	)

	BeforeEach(func() {
		t.cmd = subctl.NewDeployBrokerCmd()

		expectedIPsecPSK = nil
		expectedBrokerOptions = deploy.BrokerOptions{
			BrokerNamespace: constants.DefaultBrokerNamespace,
			BrokerSpec: v1alpha1.BrokerSpec{
				Components:                  deploy.ValidComponents,
				GlobalnetCIDRRange:          globalnet.DefaultGlobalnetCIDR,
				ClustersetIPCIDRRange:       clustersetip.DefaultCIDR,
				DefaultGlobalnetClusterSize: globalnet.DefaultGlobalnetClusterSize,
			},
			HTTPProxyConfig: httpproxy.Config{},
		}

		subctl.DeployBroker = func(ctx context.Context, options *deploy.BrokerOptions, clientProducer client.Producer,
			status reporter.Interface,
		) error {
			Expect(options.Repository).To(Equal(expectedBrokerOptions.Repository))
			Expect(options.ImageVersion).To(Equal(expectedBrokerOptions.ImageVersion))
			Expect(options.BrokerNamespace).To(Equal(expectedBrokerOptions.BrokerNamespace))
			Expect(options.BrokerURL).To(Equal(expectedBrokerOptions.BrokerURL))
			Expect(options.BrokerSpec).To(Equal(expectedBrokerOptions.BrokerSpec))
			Expect(options.HTTPProxyConfig).To(Equal(expectedBrokerOptions.HTTPProxyConfig))

			return nil
		}

		subctl.WriteBrokerInfoToFile = func(ctx context.Context, restConfig *rest.Config, namespace, brokerURL string, ipsecPSK []byte,
			components set.Set[string], customDomains []string, status reporter.Interface,
		) error {
			Expect(namespace).To(Equal(expectedBrokerOptions.BrokerNamespace))
			Expect(brokerURL).To(Equal(expectedBrokerOptions.BrokerURL))

			Expect(customDomains).To(Equal(expectedBrokerOptions.BrokerSpec.DefaultCustomDomains))

			expComponents := slices.Clone(expectedBrokerOptions.BrokerSpec.Components)
			slices.Sort(expComponents)
			Expect(components.SortedList()).To(Equal(expComponents))

			if expectedIPsecPSK != nil {
				Expect(ipsecPSK).To(Equal(expectedIPsecPSK))
			} else {
				Expect(ipsecPSK).NotTo(BeEmpty())
			}

			return nil
		}
	})

	When("with no user input", func() {
		It("should invoke deploy with the defaults", func() {
			t.assertCmdSuccess()
		})
	})

	When("various flags are specified on the command line", func() {
		BeforeEach(func() {
			expectedBrokerOptions.Repository = "localhost"
			expectedBrokerOptions.ImageVersion = "0.21.0"
			expectedBrokerOptions.BrokerURL = "https://custom-broker:8443"
			expectedBrokerOptions.HTTPProxyConfig = httpproxy.Config{
				HTTPProxy:  "http://my-proxy",
				HTTPSProxy: "https://my-proxy",
				NoProxy:    "localhost",
			}
			expectedBrokerOptions.BrokerSpec = v1alpha1.BrokerSpec{
				Components:                  []string{component.Connectivity},
				GlobalnetEnabled:            true,
				GlobalnetCIDRRange:          "142.0.0.0/8",
				DefaultGlobalnetClusterSize: 8192,
				ClustersetIPEnabled:         true,
				ClustersetIPCIDRRange:       "143.0.0.0/8",
				DefaultCustomDomains:        []string{"custom-domain"},
			}

			t.args = []string{
				"--globalnet",
				"--globalnet-cidr-range=" + expectedBrokerOptions.BrokerSpec.GlobalnetCIDRRange,
				"--globalnet-cluster-size=" + strconv.FormatUint(uint64(expectedBrokerOptions.BrokerSpec.DefaultGlobalnetClusterSize), 10),
				"--custom-domains=" + strings.Join(expectedBrokerOptions.BrokerSpec.DefaultCustomDomains, ","),
				"--components=" + strings.Join(expectedBrokerOptions.BrokerSpec.Components, ","),
				"--repository=" + expectedBrokerOptions.Repository,
				"--version=" + expectedBrokerOptions.ImageVersion,
				"--broker-url=" + expectedBrokerOptions.BrokerURL,
				"--enable-clusterset-ip",
				"--clusterset-ip-cidr-range=" + expectedBrokerOptions.BrokerSpec.ClustersetIPCIDRRange,
				"--http-proxy=" + expectedBrokerOptions.HTTPProxyConfig.HTTPProxy,
				"--https-proxy=" + expectedBrokerOptions.HTTPProxyConfig.HTTPSProxy,
				"--no-proxy=" + expectedBrokerOptions.HTTPProxyConfig.NoProxy,
			}
		})

		It("should invoke deploy with the specified values", func() {
			t.assertCmdSuccess()
		})
	})

	When("an IPsec PSK file is specified", func() {
		const ipsecFile = "test-ipsec-psk"

		BeforeEach(func() {
			expectedIPsecPSK = []byte("test-psk-from-file")

			subctl.ReadBrokerInfoFromFile = func(filename string) (*broker.Info, error) {
				Expect(filename).To(Equal(ipsecFile))

				return &broker.Info{
					IPSecPSK: &corev1.Secret{
						Data: map[string][]byte{
							"psk": expectedIPsecPSK,
						},
					},
				}, nil
			}

			t.args = []string{"--ipsec-psk-from=" + ipsecFile}
		})

		It("should read the PSK from the file and use it", func() {
			t.assertCmdSuccess()
		})

		When("and ReadBrokerInfoFromFile fails", func() {
			BeforeEach(func() {
				subctl.ReadBrokerInfoFromFile = func(filename string) (*broker.Info, error) {
					return nil, errors.New("read error")
				}
			})

			It("should return an error", func() {
				Expect(t.cmd.Execute()).To(Succeed())
				Expect(t.exited).To(BeTrue())
			})
		})
	})

	When("DeployBroker fails", func() {
		BeforeEach(func() {
			subctl.DeployBroker = func(ctx context.Context, options *deploy.BrokerOptions, clientProducer client.Producer,
				status reporter.Interface,
			) error {
				return errors.New("broker deploy error")
			}
		})

		It("should exit", func() {
			Expect(t.cmd.Execute()).To(Succeed())
			Expect(t.exited).To(BeTrue())
		})
	})
})
