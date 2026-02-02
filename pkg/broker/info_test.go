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

package broker_test

import (
	"context"
	"crypto/x509"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/submariner-io/admiral/pkg/fake"
	"github.com/submariner-io/subctl/internal/component"
	"github.com/submariner-io/subctl/pkg/broker"
	submarinerv1 "github.com/submariner-io/submariner/pkg/apis/submariner.io/v1"
	submarinerClientset "github.com/submariner-io/submariner/pkg/client/clientset/versioned"
	submarinerfake "github.com/submariner-io/submariner/pkg/client/clientset/versioned/fake"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
)

var _ = Describe("Info", func() {
	Describe("GetBrokerAdministratorConfig", testGetBrokerAdministratorConfig)
	Describe("IsConnectivityEnabled", testIsConnectivityEnabled)
	Describe("GetComponents", testGetComponents)
	Describe("IsServiceDiscoveryEnabled", testIsServiceDiscoveryEnabled)
})

func testGetBrokerAdministratorConfig() {
	bearerToken := "test-token"
	caCert := []byte("test-ca-cert")

	var (
		info             *broker.Info
		submarinerClient *submarinerfake.Clientset
	)

	BeforeEach(func() {
		info = &broker.Info{
			BrokerURL: "https://test-broker:6443",
			ClientToken: &corev1.Secret{
				Data: map[string][]byte{
					"token":     []byte(bearerToken),
					"ca.crt":    caCert,
					"namespace": []byte("test-ns"),
				},
			},
		}

		submarinerClient = submarinerfake.NewClientset()

		broker.NewSubmarinerClientset = func(_ *rest.Config) (submarinerClientset.Interface, error) {
			return submarinerClient, nil
		}
	})

	assertGetBrokerAdministratorConfigSuccess := func(ctx context.Context, insecure bool) *rest.Config {
		config, err := info.GetBrokerAdministratorConfig(ctx, insecure)
		Expect(err).NotTo(HaveOccurred())
		Expect(config).NotTo(BeNil())
		Expect(config.TLSClientConfig.Insecure).To(Equal(insecure))
		Expect(config.Host).To(Equal(info.BrokerURL))
		Expect(config.BearerToken).To(Equal(bearerToken))

		return config
	}

	When("insecure is true", func() {
		It("should return a config with insecure TLS", func(ctx SpecContext) {
			config := assertGetBrokerAdministratorConfigSuccess(ctx, true)
			Expect(config.TLSClientConfig.CAData).To(BeNil())
		})
	})

	When("insecure is false", func() {
		Context("and the connection succeeds without CA data", func() {
			It("should return a config without CA data", func(ctx SpecContext) {
				config := assertGetBrokerAdministratorConfigSuccess(ctx, false)
				Expect(config.TLSClientConfig.CAData).To(BeNil())
			})
		})

		Context("and connection initially fails with unknown authority error", func() {
			BeforeEach(func() {
				broker.NewSubmarinerClientset = func(config *rest.Config) (submarinerClientset.Interface, error) {
					// Return error only when CAData is not set (first attempt)
					if config.TLSClientConfig.CAData == nil {
						return nil, x509.UnknownAuthorityError{}
					}

					// Second attempt with CAData should succeed
					return submarinerClient, nil
				}
			})

			It("should retry with CA data", func(ctx SpecContext) {
				config := assertGetBrokerAdministratorConfigSuccess(ctx, false)
				Expect(config.TLSClientConfig.CAData).To(Equal(caCert))
			})
		})
	})

	When("listing Cluster resources returns NotFound error", func() {
		BeforeEach(func() {
			fake.FailOnAction(&submarinerClient.Fake, "clusters", "list", apierrors.NewNotFound(schema.GroupResource{
				Group:    submarinerv1.SchemeGroupVersion.Group,
				Resource: "clusters",
			}, ""), false)
		})

		It("should treat it as success", func(ctx SpecContext) {
			assertGetBrokerAdministratorConfigSuccess(ctx, false)
		})
	})

	When("listing Cluster resources fails", func() {
		BeforeEach(func() {
			fake.FailOnAction(&submarinerClient.Fake, "clusters", "list", nil, false)
		})

		It("should return an error", func(ctx SpecContext) {
			_, err := info.GetBrokerAdministratorConfig(ctx, false)
			Expect(err).To(HaveOccurred())
		})
	})
}

func testIsConnectivityEnabled() {
	It("should return true when the Connectivity component is present", func() {
		info := &broker.Info{
			Components: []string{component.Connectivity, component.ServiceDiscovery},
		}
		Expect(info.IsConnectivityEnabled()).To(BeTrue())
	})

	It("should return false when the Connectivity component is not present", func() {
		info := &broker.Info{
			Components: []string{component.ServiceDiscovery},
		}
		Expect(info.IsConnectivityEnabled()).To(BeFalse())
	})
}

func testGetComponents() {
	It("should return a set with all components", func() {
		info := &broker.Info{
			Components: []string{component.Connectivity, component.ServiceDiscovery, component.Globalnet},
		}

		components := info.GetComponents()
		Expect(components.Has(component.Connectivity)).To(BeTrue())
		Expect(components.Has(component.ServiceDiscovery)).To(BeTrue())
		Expect(components.Has(component.Globalnet)).To(BeTrue())
		Expect(components.Len()).To(Equal(3))
	})

	It("should return an empty set when Components is nil", func() {
		info := &broker.Info{}
		Expect(info.GetComponents().Len()).To(Equal(0))
	})
}

func testIsServiceDiscoveryEnabled() {
	It("should return true when the ServiceDiscovery component is present", func() {
		info := &broker.Info{
			Components: []string{component.Connectivity, component.ServiceDiscovery},
		}
		Expect(info.IsServiceDiscoveryEnabled()).To(BeTrue())
	})

	It("should return true when the ServiceDiscovery flag is true", func() {
		info := &broker.Info{
			Components:       []string{component.Connectivity},
			ServiceDiscovery: true,
		}
		Expect(info.IsServiceDiscoveryEnabled()).To(BeTrue())
	})

	It("should return false when ServiceDiscovery is not present", func() {
		info := &broker.Info{
			Components: []string{component.Connectivity},
		}
		Expect(info.IsServiceDiscoveryEnabled()).To(BeFalse())
	})
}
