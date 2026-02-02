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
	"encoding/base64"
	"path"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/submariner-io/subctl/internal/component"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/pkg/broker"
	"github.com/submariner-io/subctl/pkg/brokercr"
	"github.com/submariner-io/subctl/pkg/cluster"
	"github.com/submariner-io/submariner-operator/api/v1alpha1"
	opnames "github.com/submariner-io/submariner-operator/pkg/names"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

var _ = Describe("RecoverData", func() {
	const (
		testBrokerURL   = "https://broker.example.com:6443"
		testIPSecPSKRaw = "my-secret-psk"
	)

	t := newTestDriver()

	var (
		clusterInfo      *cluster.Info
		brokerObj        *v1alpha1.Broker
		brokerRestConfig *rest.Config
	)

	BeforeEach(func() {
		setupInfoFileDir()

		clusterInfo = &cluster.Info{
			Name: "test-cluster",
			Submariner: &v1alpha1.Submariner{
				ObjectMeta: metav1.ObjectMeta{
					Name:      opnames.SubmarinerCrName,
					Namespace: constants.OperatorNamespace,
				},
				Spec: v1alpha1.SubmarinerSpec{
					CeIPSecPSK: base64.StdEncoding.EncodeToString([]byte(testIPSecPSKRaw)),
				},
			},
		}

		brokerObj = &v1alpha1.Broker{
			ObjectMeta: metav1.ObjectMeta{
				Name:      brokercr.Name,
				Namespace: testBrokerNamespace,
			},
			Spec: v1alpha1.BrokerSpec{
				Components:           []string{component.Connectivity, component.ServiceDiscovery},
				DefaultCustomDomains: []string{"test-custom-domain"},
			},
		}

		brokerRestConfig = &rest.Config{}
	})

	JustBeforeEach(func(ctx SpecContext) {
		_, err := t.kubeClient.CoreV1().Secrets(testBrokerNamespace).Create(ctx, newTokenSecret(), metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
	})

	It("should write the recovered broker info to a file", func() {
		Expect(broker.RecoverData(clusterInfo, brokerObj, testBrokerNamespace, testBrokerURL, brokerRestConfig,
			t.statusReporter)).To(Succeed())

		info, err := broker.ReadInfoFromFile(path.Join(broker.InfoFileDir, broker.InfoFileName))
		Expect(err).NotTo(HaveOccurred())
		Expect(info.BrokerURL).To(Equal(testBrokerURL))
		Expect(info.Components).To(ContainElements(component.Connectivity, component.ServiceDiscovery))
		Expect(info.CustomDomains).NotTo(BeNil())
		Expect(*info.CustomDomains).To(Equal(brokerObj.Spec.DefaultCustomDomains))
		Expect(info.ClientToken).NotTo(BeNil())
		Expect(info.ClientToken.Name).To(Equal(newTokenSecret().Name))
		Expect(info.IPSecPSK).NotTo(BeNil())
		Expect(info.IPSecPSK.Data["psk"]).To(Equal([]byte(testIPSecPSKRaw)))
	})

	When("the IPSec PSK is not a valid base64 string", func() {
		BeforeEach(func() {
			clusterInfo.Submariner.Spec.CeIPSecPSK = "invalid"
		})

		It("should return an error", func() {
			Expect(broker.RecoverData(clusterInfo, brokerObj, testBrokerNamespace,
				testBrokerURL, brokerRestConfig, t.statusReporter)).NotTo(Succeed())
		})
	})
})
