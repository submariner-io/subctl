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

package brokercr_test

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"github.com/submariner-io/subctl/internal/component"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/pkg/brokercr"
	submariner "github.com/submariner-io/submariner-operator/api/v1alpha1"
	"github.com/submariner-io/submariner-operator/pkg/discovery/globalnet"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/client-go/kubernetes/scheme"
	ctrlClient "sigs.k8s.io/controller-runtime/pkg/client"
	controllerfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

var _ = BeforeSuite(func() {
	Expect(submariner.AddToScheme(scheme.Scheme)).To(Succeed())
})

var _ = Describe("Ensure", func() {
	brokerKey := ctrlClient.ObjectKey{
		Name:      brokercr.Name,
		Namespace: constants.DefaultBrokerNamespace,
	}

	var (
		client        ctrlClient.Client
		clientBuilder *controllerfake.ClientBuilder
		brokerSpec    *submariner.BrokerSpec
	)

	BeforeEach(func() {
		clientBuilder = controllerfake.NewClientBuilder().WithScheme(scheme.Scheme)

		brokerSpec = &submariner.BrokerSpec{
			GlobalnetEnabled:            true,
			GlobalnetCIDRRange:          globalnet.DefaultGlobalnetCIDR,
			DefaultGlobalnetClusterSize: globalnet.DefaultGlobalnetClusterSize,
			Components:                  []string{component.Connectivity},
		}
	})

	JustBeforeEach(func() {
		client = clientBuilder.Build()
	})

	When("the Broker doesn't exist", func() {
		It("should create it", func(ctx SpecContext) {
			Expect(brokercr.Ensure(ctx, client, constants.DefaultBrokerNamespace, brokerSpec)).To(Succeed())

			broker := &submariner.Broker{}
			Expect(client.Get(ctx, brokerKey, broker)).To(Succeed())
			Expect(&broker.Spec).To(Equal(brokerSpec))
		})
	})

	When("the Broker already exists", func() {
		var existingUID types.UID

		BeforeEach(func() {
			clientBuilder.WithInterceptorFuncs(interceptor.Funcs{
				Create: func(ctx context.Context, c ctrlClient.WithWatch, obj ctrlClient.Object, opts ...ctrlClient.CreateOption) error {
					obj.SetUID(uuid.NewUUID())
					return c.Create(ctx, obj, opts...)
				},
				Update: func(_ context.Context, _ ctrlClient.WithWatch, _ ctrlClient.Object, _ ...ctrlClient.UpdateOption) error {
					return errors.New("no updates allowed")
				},
			})
		})

		JustBeforeEach(func(ctx SpecContext) {
			existingBroker := &submariner.Broker{
				ObjectMeta: metav1.ObjectMeta{
					Name:      brokerKey.Name,
					Namespace: brokerKey.Namespace,
				},
				Spec: submariner.BrokerSpec{
					GlobalnetCIDRRange:          "192.168.0.0/16",
					DefaultGlobalnetClusterSize: 256,
					Components:                  []string{component.ServiceDiscovery},
				},
			}
			Expect(client.Create(ctx, existingBroker)).To(Succeed())

			existingUID = existingBroker.UID
		})

		It("should replace it with a new resource", func(ctx SpecContext) {
			Expect(brokercr.Ensure(ctx, client, constants.DefaultBrokerNamespace, brokerSpec)).To(Succeed())

			broker := &submariner.Broker{}
			Expect(client.Get(ctx, brokerKey, broker)).To(Succeed())
			Expect(broker.UID).NotTo(Equal(existingUID))
			Expect(&broker.Spec).To(Equal(brokerSpec))
		})
	})

	When("resource creation fails", func() {
		BeforeEach(func() {
			clientBuilder.WithInterceptorFuncs(interceptor.Funcs{
				Create: func(ctx context.Context, c ctrlClient.WithWatch, obj ctrlClient.Object, opts ...ctrlClient.CreateOption) error {
					return errors.New("mock")
				},
			})
		})

		It("should return an error", func(ctx SpecContext) {
			Expect(brokercr.Ensure(ctx, client, constants.DefaultBrokerNamespace, brokerSpec)).NotTo(Succeed())
		})
	})
})

func TestBrokerCR(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "BrokerCR Suite")
}
