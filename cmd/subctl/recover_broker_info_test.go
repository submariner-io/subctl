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
	"errors"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/submariner-io/admiral/pkg/fake"
	"github.com/submariner-io/admiral/pkg/reporter"
	"github.com/submariner-io/admiral/pkg/resource"
	"github.com/submariner-io/subctl/cmd/subctl"
	"github.com/submariner-io/subctl/internal/component"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/pkg/brokercr"
	"github.com/submariner-io/subctl/pkg/client"
	"github.com/submariner-io/subctl/pkg/cluster"
	"github.com/submariner-io/submariner-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	controllerfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("Recover Broker Info", func() {
	t := newTestDriver()

	const (
		testBrokerNamespace = "broker-ns"
		testBrokerHost      = "test-broker:6443"
	)

	var (
		recoverDataCalled bool
		submarinerObj     *v1alpha1.Submariner
		brokerObj         *v1alpha1.Broker
		brokerProducer    *client.DefaultProducer
	)

	BeforeEach(func() {
		recoverDataCalled = false
		t.cmd = subctl.NewRecoverBrokerInfoCmd()

		brokerProducer = &client.DefaultProducer{
			KubeClient:    k8sfake.NewClientset(),
			DynamicClient: dynamicfake.NewSimpleDynamicClient(scheme.Scheme),
			GeneralClient: controllerfake.NewClientBuilder().WithScheme(scheme.Scheme).Build(),
		}

		client.NewProducerFromRestConfig = func(config *rest.Config) (client.Producer, error) {
			if strings.HasSuffix(config.Host, testBrokerHost) {
				return brokerProducer, nil
			}

			return t.fakeProducer, nil
		}

		brokerObj = &v1alpha1.Broker{
			ObjectMeta: metav1.ObjectMeta{
				Name:      brokercr.Name,
				Namespace: testBrokerNamespace,
			},
			Spec: v1alpha1.BrokerSpec{
				Components:           []string{component.Connectivity, component.ServiceDiscovery},
				DefaultCustomDomains: []string{"custom.domain"},
			},
		}

		submarinerObj = &v1alpha1.Submariner{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "submariner",
				Namespace: constants.OperatorNamespace,
			},
			Spec: v1alpha1.SubmarinerSpec{
				BrokerK8sRemoteNamespace: testBrokerNamespace,
				BrokerK8sApiServer:       testBrokerHost,
				CeIPSecPSK:               "test-psk-encoded",
				Version:                  "0.21.0",
				Repository:               v1alpha1.DefaultRepo,
			},
		}

		subctl.RecoverData = func(submCluster *cluster.Info, broker *v1alpha1.Broker, brokerNamespace, brokerURL string,
			brokerRestConfig *rest.Config, status reporter.Interface,
		) error {
			recoverDataCalled = true
			Expect(submCluster.Submariner.Spec).To(Equal(submarinerObj.Spec))
			Expect(broker.Spec).To(Equal(brokerObj.Spec))
			Expect(brokerNamespace).To(Equal(testBrokerNamespace))
			Expect(brokerRestConfig).NotTo(BeNil())

			return nil
		}

		resource.NewDynamicClient = func(_ *rest.Config) (dynamic.Interface, error) {
			return dynamicfake.NewSimpleDynamicClient(scheme.Scheme), nil
		}
	})

	JustBeforeEach(func() {
		Expect(t.fakeProducer.GeneralClient.Create(ctx, submarinerObj)).To(Succeed())
		Expect(t.fakeProducer.GeneralClient.Create(ctx, brokerObj.DeepCopy())).To(Succeed())
	})

	When("the Broker is found on the same cluster as Submariner", func() {
		It("should use it to recover the broker info", func() {
			t.assertCmdSuccess()
			Expect(recoverDataCalled).To(BeTrue())
		})

		When("and broker retrieval fails", func() {
			BeforeEach(func() {
				t.fakeProducer.GeneralClient = controllerfake.NewClientBuilder().WithScheme(scheme.Scheme).WithInterceptorFuncs(
					fake.FailingGetInterceptor[*v1alpha1.Broker]()).Build()
			})

			t.testCmdFailure("mock", "error")
		})
	})

	When("the Broker is not found on the same cluster as Submariner", func() {
		JustBeforeEach(func() {
			Expect(t.fakeProducer.GeneralClient.Delete(ctx, brokerObj)).To(Succeed())
			Expect(brokerProducer.GeneralClient.Create(ctx, brokerObj.DeepCopy())).To(Succeed())
		})

		It("should retrieve it from a remote cluster", func() {
			Expect(t.cmd.Execute()).To(Succeed())
			t.status.AssertFailureCount(0)
			t.status.AssertWarningCount(1)
			Expect(t.exited).To(BeFalse())
			Expect(recoverDataCalled).To(BeTrue())
		})

		When("and broker retrieval fails on the remote cluster", func() {
			BeforeEach(func() {
				brokerProducer.GeneralClient = controllerfake.NewClientBuilder().WithScheme(scheme.Scheme).WithInterceptorFuncs(
					fake.FailingGetInterceptor[*v1alpha1.Broker]()).Build()
			})

			t.testCmdFailure("mock", "error")
		})

		When("and is not found on the remote cluster", func() {
			JustBeforeEach(func() {
				Expect(brokerProducer.GeneralClient.Delete(ctx, brokerObj)).To(Succeed())
			})

			t.testCmdFailure("No Broker")
		})
	})

	When("recovery of the broker info fails", func() {
		BeforeEach(func() {
			subctl.RecoverData = func(submCluster *cluster.Info, broker *v1alpha1.Broker, brokerNamespace, brokerURL string,
				brokerRestConfig *rest.Config, status reporter.Interface,
			) error {
				return errors.New("the recovery failed")
			}
		})

		t.testCmdFailure("recovery failed")
	})

	When("the broker-url flag is specified", func() {
		const customBrokerURL = "https://custom-broker:8443"

		BeforeEach(func() {
			t.args = []string{"--broker-url=" + customBrokerURL}

			subctl.RecoverData = func(submCluster *cluster.Info, broker *v1alpha1.Broker, brokerNamespace, brokerURL string,
				brokerRestConfig *rest.Config, status reporter.Interface,
			) error {
				recoverDataCalled = true
				Expect(brokerURL).To(Equal(customBrokerURL))

				return nil
			}
		})

		It("should use it to recover the broker info", func() {
			t.assertCmdSuccess()
			Expect(recoverDataCalled).To(BeTrue())
		})
	})
})
