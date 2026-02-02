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
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/pkg/deploy"
	operatorv1alpha1 "github.com/submariner-io/submariner-operator/api/v1alpha1"
	"github.com/submariner-io/submariner-operator/pkg/names"
	"k8s.io/client-go/kubernetes/scheme"
	ctrlClient "sigs.k8s.io/controller-runtime/pkg/client"
	controllerfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

var _ = Describe("ServiceDiscovery", func() {
	t := newTestDriver()

	var options *deploy.ServiceDiscoveryOptions

	BeforeEach(func() {
		options = &deploy.ServiceDiscoveryOptions{
			SubmarinerDebug:     true,
			BrokerK8sInsecure:   true,
			ClustersetIPEnabled: true,
			ClusterID:           "test-cluster",
			Repository:          testRepository,
			ImageVersion:        testImageVersion,
		}
	})

	runDepoy := func(ctx context.Context) error {
		return deploy.ServiceDiscovery(ctx, t.fakeProducer, options, t.brokerInfo, t.brokerSecret,
			t.clustersetConfig, t.repositoryInfo, t.statusReporter)
	}

	expectServiceDiscoverySuccess := func(ctx context.Context) *operatorv1alpha1.ServiceDiscoverySpec {
		Expect(runDepoy(ctx)).To(Succeed())

		serviceDiscoveryCR := &operatorv1alpha1.ServiceDiscovery{}
		Expect(t.fakeProducer.ForGeneral().Get(ctx, ctrlClient.ObjectKey{
			Name:      names.ServiceDiscoveryCrName,
			Namespace: constants.OperatorNamespace,
		}, serviceDiscoveryCR)).To(Succeed())

		t.statusReporter.AssertWarningCount(0)
		t.statusReporter.AssertFailureCount(0)

		return &serviceDiscoveryCR.Spec
	}

	It("should successfully deploy the ServiceDiscovery resource", func(ctx SpecContext) {
		spec := expectServiceDiscoverySuccess(ctx)
		Expect(spec.Repository).To(Equal(options.Repository))
		Expect(spec.Version).To(Equal(options.ImageVersion))
		Expect(spec.BrokerK8sCA).To(Equal(base64.StdEncoding.EncodeToString(t.brokerSecret.Data["ca.crt"])))
		Expect(spec.BrokerK8sRemoteNamespace).To(Equal(string(t.brokerSecret.Data["namespace"])))
		Expect(spec.BrokerK8sApiServerToken).To(Equal(string(t.brokerSecret.Data["token"])))
		Expect(spec.BrokerK8sApiServer).To(Equal("broker.example.com:8443"))
		Expect(spec.BrokerK8sSecret).To(Equal(t.brokerSecret.ObjectMeta.Name))
		Expect(spec.BrokerK8sInsecure).To(Equal(options.BrokerK8sInsecure))
		Expect(spec.Debug).To(Equal(options.SubmarinerDebug))
		Expect(spec.ClusterID).To(Equal(options.ClusterID))
		Expect(spec.Namespace).To(Equal(constants.OperatorNamespace))
		Expect(spec.ImageOverrides).To(Equal(t.repositoryInfo.Overrides))
		Expect(spec.ClustersetIPEnabled).To(Equal(options.ClustersetIPEnabled))
		Expect(spec.ClustersetIPCIDR).To(Equal(t.clustersetConfig.ClustersetIPCIDR))
		Expect(spec.CustomDomains).To(Equal(options.CustomDomains))
		Expect(spec.CoreDNSCustomConfig).To(BeNil())
	})

	When("the CoreDNS custom config map is provided", func() {
		BeforeEach(func() {
			options.CoreDNSCustomConfigMap = "test-namespace/test-configmap"
		})

		It("should set the CoreDNSCustomConfig field", func(ctx SpecContext) {
			Expect(expectServiceDiscoverySuccess(ctx).CoreDNSCustomConfig).To(Equal(&operatorv1alpha1.CoreDNSCustomConfig{
				ConfigMapName: "test-configmap",
				Namespace:     "test-namespace",
			}))
		})
	})

	When("custom domains are provided", func() {
		BeforeEach(func() {
			options.CustomDomains = []string{"custom.domain"}
		})

		It("should set the CustomDomains field", func(ctx SpecContext) {
			Expect(expectServiceDiscoverySuccess(ctx).CustomDomains).To(Equal(options.CustomDomains))
		})
	})

	Context("when ServiceDiscovery resource creation fails", func() {
		BeforeEach(func() {
			t.fakeProducer.GeneralClient = controllerfake.NewClientBuilder().WithScheme(scheme.Scheme).WithInterceptorFuncs(interceptor.Funcs{
				Create: func(_ context.Context, _ ctrlClient.WithWatch, _ ctrlClient.Object, _ ...ctrlClient.CreateOption) error {
					return errors.New("mock create error")
				},
			}).Build()
		})

		It("should return an error", func(ctx SpecContext) {
			Expect(runDepoy(ctx)).NotTo(Succeed())
			t.statusReporter.AssertHasFailure()
		})
	})
})
