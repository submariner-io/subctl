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
	"os"
	"path"
	"slices"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/submariner-io/subctl/internal/component"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/pkg/broker"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/utils/set"
)

var _ = Describe("File I/O", func() {
	Describe("WriteInfoToFile", testWriteInfoToFile)
	Describe("ReadInfoFromFile", testReadInfoFromFile)
})

func testWriteInfoToFile() {
	restConfig := &rest.Config{
		Host:    "https://localhost",
		APIPath: "/apis",
	}

	t := newTestDriver()

	var tokenSecret *corev1.Secret

	BeforeEach(func() {
		setupInfoFileDir()

		tokenSecret = newTokenSecret()
	})

	JustBeforeEach(func(ctx SpecContext) {
		if tokenSecret != nil {
			_, err := t.kubeClient.CoreV1().Secrets(testBrokerNamespace).Create(ctx, tokenSecret, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
		}
	})

	It("should correctly write the broker info to a file", func(ctx SpecContext) {
		components := set.New(component.Connectivity, component.ServiceDiscovery)
		customDomains := []string{"acme.com"}
		ipsecPSK := []byte{1, 2, 3}

		Expect(broker.WriteInfoToFile(ctx, restConfig, testBrokerNamespace, "", ipsecPSK, components, customDomains,
			t.statusReporter)).To(Succeed())
		t.statusReporter.AssertFailureCount(0)

		info, err := broker.ReadInfoFromFile(path.Join(broker.InfoFileDir, broker.InfoFileName))
		Expect(err).NotTo(HaveOccurred())
		Expect(info.BrokerURL).To(Equal(restConfig.Host + restConfig.APIPath))
		Expect(info.ServiceDiscovery).To(BeTrue())
		Expect(info.Components).To(ContainElements(component.Connectivity, component.ServiceDiscovery))
		Expect(info.CustomDomains).NotTo(BeNil())
		Expect(*info.CustomDomains).To(Equal(customDomains))
		Expect(info.ClientToken).NotTo(BeNil())
		Expect(info.ClientToken.Name).To(Equal(tokenSecret.Name))
		Expect(info.IPSecPSK).NotTo(BeNil())
		Expect(info.IPSecPSK.Data["psk"]).To(Equal(ipsecPSK))
	})

	When("the broker info file already exists", func() {
		BeforeEach(func() {
			err := os.WriteFile(path.Join(broker.InfoFileDir, broker.InfoFileName), []byte{1}, 0o600)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should back up the file", func(ctx SpecContext) {
			Expect(broker.WriteInfoToFile(ctx, restConfig, testBrokerNamespace, "", []byte{}, set.New(component.Connectivity), nil,
				t.statusReporter)).To(Succeed())
			t.statusReporter.AssertFailureCount(0)

			files, err := os.ReadDir(broker.InfoFileDir)
			Expect(err).NotTo(HaveOccurred())
			Expect(slices.IndexFunc(files, func(e os.DirEntry) bool {
				return strings.HasPrefix(e.Name(), broker.InfoFileName+".")
			})).To(BeNumerically(">=", 0))
		})
	})

	When("the broker URL is specified", func() {
		It("should override the default URL", func(ctx SpecContext) {
			brokerURL := "https://acme.com"
			Expect(broker.WriteInfoToFile(ctx, restConfig, testBrokerNamespace, brokerURL, nil, set.New(component.Connectivity), nil,
				t.statusReporter)).To(Succeed())
			t.statusReporter.AssertFailureCount(0)

			info, err := broker.ReadInfoFromFile(path.Join(broker.InfoFileDir, broker.InfoFileName))
			Expect(err).NotTo(HaveOccurred())
			Expect(info.BrokerURL).To(Equal(brokerURL))
		})
	})

	When("the token Secret doesn't exist", func() {
		BeforeEach(func() {
			tokenSecret = nil
		})

		It("should return an error", func(ctx SpecContext) {
			Expect(broker.WriteInfoToFile(ctx, restConfig, testBrokerNamespace, "", []byte{}, set.New(component.Connectivity), nil,
				t.statusReporter)).NotTo(Succeed())
			t.statusReporter.AssertHasFailure()
		})
	})
}

func testReadInfoFromFile() {
	When("the broker info file doesn't exist", func() {
		It("should return an error", func() {
			_, err := broker.ReadInfoFromFile("does-not-exist")
			Expect(err).To(HaveOccurred())
		})
	})

	When("the broker info file contains invalid data", func() {
		var file *os.File

		BeforeEach(func() {
			var err error

			file, err = os.CreateTemp("", "invalid-broker-file")
			Expect(err).NotTo(HaveOccurred())

			_, err = file.WriteString("invalid data")
			Expect(err).NotTo(HaveOccurred())

			DeferCleanup(func() {
				_ = os.Remove(file.Name())
			})
		})

		It("should return an error", func() {
			_, err := broker.ReadInfoFromFile(file.Name())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("error decoding"))
		})
	})
}

func newTokenSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: "token-secret",
			Annotations: map[string]string{
				corev1.ServiceAccountNameKey: constants.SubmarinerBrokerAdminSA,
			},
		},
		Type: corev1.SecretTypeServiceAccountToken,
	}
}
