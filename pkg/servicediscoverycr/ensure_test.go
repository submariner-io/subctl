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

package servicediscoverycr_test

import (
	"context"
	"errors"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/pkg/servicediscoverycr"
	operatorv1alpha1 "github.com/submariner-io/submariner-operator/api/v1alpha1"
	"github.com/submariner-io/submariner-operator/pkg/names"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	ctrlClient "sigs.k8s.io/controller-runtime/pkg/client"
	controllerfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

var _ = BeforeSuite(func() {
	Expect(operatorv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())
})

func TestSubmarinerCr(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "SubmarinerCr Suite")
}

var _ = Describe("Ensure", func() {
	const (
		clusterID = "test-cluster"
		brokerURL = "https://test-broker:8443"
	)

	ctx := context.TODO()
	serviceDiscoveryKey := ctrlClient.ObjectKey{
		Name:      names.ServiceDiscoveryCrName,
		Namespace: constants.OperatorNamespace,
	}

	var (
		client               ctrlClient.Client
		clientBuilder        *controllerfake.ClientBuilder
		serviceDiscoverySpec *operatorv1alpha1.ServiceDiscoverySpec
	)

	BeforeEach(func() {
		clientBuilder = controllerfake.NewClientBuilder().WithScheme(scheme.Scheme)

		serviceDiscoverySpec = &operatorv1alpha1.ServiceDiscoverySpec{
			ClusterID:                clusterID,
			BrokerK8sApiServer:       brokerURL,
			BrokerK8sApiServerToken:  "test-token",
			BrokerK8sCA:              "test-ca",
			BrokerK8sRemoteNamespace: "test-broker-namespace",
			Repository:               operatorv1alpha1.DefaultRepo,
			Version:                  operatorv1alpha1.DefaultSubmarinerVersion,
		}
	})

	JustBeforeEach(func() {
		client = clientBuilder.Build()
	})

	When("the ServiceDiscovery resource doesn't exist", func() {
		It("should successfully create it with the correct properties", func() {
			err := servicediscoverycr.Ensure(ctx, client, constants.OperatorNamespace, serviceDiscoverySpec)
			Expect(err).NotTo(HaveOccurred())

			serviceDiscoveryCR := &operatorv1alpha1.ServiceDiscovery{}
			err = client.Get(ctx, serviceDiscoveryKey, serviceDiscoveryCR)
			Expect(err).NotTo(HaveOccurred())

			Expect(serviceDiscoveryCR.Spec).To(Equal(*serviceDiscoverySpec))
		})
	})

	When("the ServiceDiscovery resource already exists", func() {
		BeforeEach(func() {
			clientBuilder.WithInterceptorFuncs(interceptor.Funcs{
				Create: func(_ context.Context, _ ctrlClient.WithWatch, _ ctrlClient.Object, _ ...ctrlClient.CreateOption) error {
					return errors.New("no more creates allowed")
				},
				Delete: func(_ context.Context, _ ctrlClient.WithWatch, _ ctrlClient.Object, _ ...ctrlClient.DeleteOption) error {
					return errors.New("no deletes allowed")
				},
			}).WithObjects(&operatorv1alpha1.ServiceDiscovery{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceDiscoveryKey.Name,
					Namespace: serviceDiscoveryKey.Namespace,
				},
				Spec: operatorv1alpha1.ServiceDiscoverySpec{
					BrokerK8sApiServerToken: "existing-token",
				},
			})
		})

		It("should update it", func() {
			err := servicediscoverycr.Ensure(ctx, client, constants.OperatorNamespace, serviceDiscoverySpec)
			Expect(err).NotTo(HaveOccurred())

			serviceDiscoveryCR := &operatorv1alpha1.ServiceDiscovery{}
			err = client.Get(ctx, serviceDiscoveryKey, serviceDiscoveryCR)
			Expect(err).NotTo(HaveOccurred())

			Expect(serviceDiscoveryCR.Spec).To(Equal(*serviceDiscoverySpec))
		})
	})

	When("resource creation fails", func() {
		BeforeEach(func() {
			clientBuilder.WithInterceptorFuncs(interceptor.Funcs{
				Create: func(_ context.Context, _ ctrlClient.WithWatch, _ ctrlClient.Object, _ ...ctrlClient.CreateOption) error {
					return errors.New("mock")
				},
			})
		})

		It("should return an error", func() {
			err := servicediscoverycr.Ensure(ctx, client, constants.OperatorNamespace, serviceDiscoverySpec)
			Expect(err).To(HaveOccurred())
		})
	})
})
