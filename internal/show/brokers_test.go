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

package show_test

import (
	"context"
	"strconv"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/submariner-io/admiral/pkg/fake"
	"github.com/submariner-io/subctl/internal/component"
	"github.com/submariner-io/subctl/internal/show"
	"github.com/submariner-io/subctl/pkg/client"
	"github.com/submariner-io/submariner-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	controllerfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("Brokers", func() {
	t := newTestDriver()

	doBrokers := func(ctx context.Context) error {
		return show.Brokers(ctx, t.clusterInfo, "", t.status)
	}

	It("should display the broker information", func(ctx SpecContext) {
		broker := &v1alpha1.Broker{
			ObjectMeta: metav1.ObjectMeta{
				Name:      brokerName,
				Namespace: "broker-ns",
			},
			Spec: v1alpha1.BrokerSpec{
				Components:                  []string{component.Connectivity},
				GlobalnetEnabled:            true,
				GlobalnetCIDRRange:          "242.0.0.0/8",
				DefaultGlobalnetClusterSize: uint(65536),
				DefaultCustomDomains:        []string{"my-domain.com"},
			},
		}
		Expect(t.clusterInfo.ClientProducer.ForGeneral().Create(ctx, broker)).To(Succeed())

		Expect(doBrokers(ctx)).To(Succeed())

		t.assertTableOutput(tableRowMatcher(broker.Namespace, broker.Name, strings.Join(broker.Spec.Components, ","), "yes",
			broker.Spec.GlobalnetCIDRRange, strconv.Itoa(int(broker.Spec.DefaultGlobalnetClusterSize)),
			strings.Join(broker.Spec.DefaultCustomDomains, ",")))
	})

	When("no brokers exist", func() {
		It("should not return an error", func(ctx SpecContext) {
			Expect(doBrokers(ctx)).To(Succeed())
		})
	})

	When("broker retrieval fails", func() {
		BeforeEach(func() {
			cp := t.clusterInfo.ClientProducer.(*client.DefaultProducer)
			cp.GeneralClient = controllerfake.NewClientBuilder().WithScheme(scheme.Scheme).WithInterceptorFuncs(
				fake.FailingListInterceptor[*v1alpha1.BrokerList]()).Build()
		})

		It("should return an error", func(ctx SpecContext) {
			Expect(doBrokers(ctx)).To(HaveOccurred())
		})
	})
})
