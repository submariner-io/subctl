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
	"errors"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/submariner-io/admiral/pkg/fake"
	compnames "github.com/submariner-io/admiral/pkg/names"
	"github.com/submariner-io/subctl/internal/component"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/pkg/brokercr"
	"github.com/submariner-io/subctl/pkg/deploy"
	"github.com/submariner-io/submariner-operator/api/v1alpha1"
	"github.com/submariner-io/submariner-operator/pkg/discovery/clustersetip"
	"github.com/submariner-io/submariner-operator/pkg/discovery/globalnet"
	"golang.org/x/net/http/httpproxy"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	ctrlClient "sigs.k8s.io/controller-runtime/pkg/client"
	controllerfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

const (
	testGlobalnetCIDR          = "242.0.0.0/8"
	testClustersetIPCIDR       = "243.0.0.0/8"
	validGlobalnetCluster uint = 8192
)

var _ = Describe("Broker", func() {
	t := newBrokerTestDriver()

	It("should successfully deploy the operator and the Broker resource", func() {
		t.assertSuccess()

		deployment, err := t.fakeProducer.KubeClient.AppsV1().Deployments(constants.OperatorNamespace).Get(
			ctx, compnames.OperatorComponent, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())

		Expect(deployment.Spec.Template.Spec.Containers[0].Env).To(ContainElement(corev1.EnvVar{
			Name:  "HTTP_PROXY",
			Value: t.options.HTTPProxyConfig.HTTPProxy,
		}))

		image := deployment.Spec.Template.Spec.Containers[0].Image
		Expect(image).To(And(HavePrefix(testRepository), HaveSuffix(testImageVersion)))

		_, err = t.fakeProducer.KubeClient.CoreV1().Namespaces().Get(ctx, testBrokerNamespace, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())

		t.assertBrokerResource()

		ginfo, _, err := globalnet.GetGlobalNetworks(ctx, t.fakeProducer.ForGeneral(), testBrokerNamespace)
		Expect(err).NotTo(HaveOccurred())
		Expect(ginfo.Enabled).To(BeFalse())

		cinfo, _, err := clustersetip.GetClustersetIPNetworks(ctx, t.fakeProducer.ForGeneral(), testBrokerNamespace)
		Expect(err).NotTo(HaveOccurred())
		Expect(cinfo.Enabled).To(BeFalse())
	})

	When("only the Connectivity component is specified", func() {
		BeforeEach(func() {
			t.options.BrokerSpec.Components = []string{component.Connectivity}
		})

		It("should deploy the Broker resource correctly", func() {
			t.assertSuccess()
			t.assertBrokerResource()
		})
	})

	When("no components are specified", func() {
		BeforeEach(func() {
			t.options.BrokerSpec.Components = []string{}
		})

		t.testFailure("one component")
	})

	When("an invalid component is specified", func() {
		BeforeEach(func() {
			t.options.BrokerSpec.Components = []string{component.Connectivity, "invalid-component"}
		})

		t.testFailure("unknown component")
	})

	When("globalnet is enabled", func() {
		BeforeEach(func() {
			t.options.BrokerSpec.GlobalnetEnabled = true
			t.options.BrokerSpec.GlobalnetCIDRRange = testGlobalnetCIDR
			t.options.BrokerSpec.DefaultGlobalnetClusterSize = validGlobalnetCluster
		})

		Context("with a valid globalnet configuration", func() {
			It("should create the globalnet ConfigMap correctly", func() {
				t.assertSuccess()

				info, _, err := globalnet.GetGlobalNetworks(ctx, t.fakeProducer.ForGeneral(), testBrokerNamespace)
				Expect(err).NotTo(HaveOccurred())
				Expect(info.Enabled).To(BeTrue())
				Expect(info.CIDR).To(Equal(testGlobalnetCIDR))
				Expect(info.AllocationSize).To(Equal(validGlobalnetCluster))
			})
		})

		Context("with an invalid globalnet CIDR", func() {
			BeforeEach(func() {
				t.options.BrokerSpec.GlobalnetCIDRRange = "invalid-cidr"
			})

			t.testFailure(t.options.BrokerSpec.GlobalnetCIDRRange)
		})

		Context("with an invalid cluster size", func() {
			BeforeEach(func() {
				t.options.BrokerSpec.DefaultGlobalnetClusterSize = 0
			})

			t.testFailure("cluster size")
		})

		Context("but globalnet ConfigMap creation fails", func() {
			BeforeEach(func() {
				t.fakeProducer.GeneralClient = controllerfake.NewClientBuilder().WithScheme(scheme.Scheme).WithInterceptorFuncs(
					interceptor.Funcs{
						Create: func(ctx context.Context, client ctrlClient.WithWatch, obj ctrlClient.Object, opts ...ctrlClient.CreateOption) error {
							if strings.Contains(obj.GetName(), "globalnet") {
								return errors.New("mock error")
							}

							return client.Create(ctx, obj, opts...)
						},
					}).Build()
			})

			t.testFailure("global", "ConfigMap", "error")
		})
	})

	When("clusterset IP is enabled", func() {
		BeforeEach(func() {
			t.options.BrokerSpec.ClustersetIPEnabled = true
			t.options.BrokerSpec.ClustersetIPCIDRRange = testClustersetIPCIDR
		})

		Context("with a valid clusterset IP configuration", func() {
			It("should create the clusterset IP ConfigMap correctly", func() {
				t.assertSuccess()

				info, _, err := clustersetip.GetClustersetIPNetworks(ctx, t.fakeProducer.ForGeneral(), testBrokerNamespace)
				Expect(err).NotTo(HaveOccurred())
				Expect(info.Enabled).To(BeTrue())
				Expect(info.CIDR).To(Equal(testClustersetIPCIDR))
			})
		})

		Context("with an invalid clustersetIP CIDR", func() {
			BeforeEach(func() {
				t.options.BrokerSpec.ClustersetIPCIDRRange = "invalid-cidr"
			})

			t.testFailure(t.options.BrokerSpec.ClustersetIPCIDRRange)
		})

		Context("but clusterset IP ConfigMap creation fails", func() {
			BeforeEach(func() {
				t.fakeProducer.GeneralClient = controllerfake.NewClientBuilder().WithScheme(scheme.Scheme).WithInterceptorFuncs(
					interceptor.Funcs{
						Create: func(ctx context.Context, client ctrlClient.WithWatch, obj ctrlClient.Object,
							opts ...ctrlClient.CreateOption,
						) error {
							if strings.Contains(obj.GetName(), "clustersetip") {
								return errors.New("mock error")
							}

							return client.Create(ctx, obj, opts...)
						},
					}).Build()
			})

			t.testFailure("clusterset", "ConfigMap", "error")
		})
	})

	Context("when operator deployment fails", func() {
		BeforeEach(func() {
			fake.FailOnAction(&t.fakeProducer.KubeClient.(*k8sfake.Clientset).Fake, "deployments", "create", nil, false)
		})

		t.testFailure("submariner-operator", "error")
	})
})

type brokerTestDriver struct {
	*testDriver
	options deploy.BrokerOptions
}

func newBrokerTestDriver() *brokerTestDriver {
	t := &brokerTestDriver{testDriver: newTestDriver()}

	BeforeEach(func() {
		t.options = deploy.BrokerOptions{
			OperatorDebug:   true,
			Repository:      testRepository,
			ImageVersion:    testImageVersion,
			BrokerNamespace: testBrokerNamespace,
			BrokerURL:       "https://broker.example.com:8443",
			BrokerSpec: v1alpha1.BrokerSpec{
				Components:            []string{component.ServiceDiscovery, component.Connectivity},
				ClustersetIPCIDRRange: testClustersetIPCIDR,
			},
			HTTPProxyConfig: httpproxy.Config{
				HTTPProxy: "http://my-proxy",
			},
		}
	})

	return t
}

func (t *brokerTestDriver) run() error {
	return deploy.Broker(&t.options, t.fakeProducer, t.statusReporter)
}

func (t *brokerTestDriver) assertSuccess() {
	Expect(t.run()).To(Succeed())
	t.statusReporter.AssertWarningCount(0)
	t.statusReporter.AssertFailureCount(0)
}

func (t *brokerTestDriver) testFailure(msgs ...string) {
	It("should fail", func() {
		Expect(t.run()).NotTo(Succeed())
		t.statusReporter.AssertFailureContainsStrings(msgs...)
	})
}

func (t *brokerTestDriver) assertBrokerResource() {
	broker := &v1alpha1.Broker{}
	Expect(t.fakeProducer.ForGeneral().Get(ctx, ctrlClient.ObjectKey{
		Name:      brokercr.Name,
		Namespace: testBrokerNamespace,
	}, broker)).To(Succeed())
	Expect(broker.Spec).To(Equal(t.options.BrokerSpec))
}
