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

package namespace_test

import (
	"context"
	"errors"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/submariner-io/admiral/pkg/fake"
	"github.com/submariner-io/subctl/pkg/namespace"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestNamespace(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Namespace Suite")
}

var _ = Describe("Ensure", func() {
	ctx := context.TODO()
	testNS := "test-namespace"
	testLabels := map[string]string{
		"test-label1": "test-value1",
		"test-label2": "test-value2",
	}

	var kubeClient *k8sfake.Clientset

	BeforeEach(func() {
		kubeClient = k8sfake.NewClientset()
	})

	assertEnsure := func(labels map[string]string, expCreated bool) {
		created, err := namespace.Ensure(ctx, kubeClient, testNS, labels)
		Expect(err).NotTo(HaveOccurred())
		Expect(created).To(Equal(expCreated))
	}

	assertNamespace := func() *corev1.Namespace {
		ns, err := kubeClient.CoreV1().Namespaces().Get(ctx, testNS, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())

		return ns
	}

	When("creating a new namespace", func() {
		It("should successfully create it with the specified labels", func() {
			assertEnsure(testLabels, true)
			Expect(assertNamespace().Labels).To(Equal(testLabels))
		})

		It("should successfully create it with nil labels", func() {
			assertEnsure(nil, true)
			Expect(assertNamespace().Labels).To(BeEmpty())
		})

		It("should successfully create it with empty labels", func() {
			assertEnsure(map[string]string{}, true)
			Expect(assertNamespace().Labels).To(BeEmpty())
		})
	})

	When("updating an existing namespace", func() {
		var existingNS *corev1.Namespace

		BeforeEach(func() {
			existingNS = &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: testNS,
				},
			}
		})

		JustBeforeEach(func() {
			_, err := kubeClient.CoreV1().Namespaces().Create(ctx, existingNS, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
		})

		Context("with existing labels", func() {
			BeforeEach(func() {
				existingNS.Labels = map[string]string{
					"existing-label": "existing-value",
					"common-label":   "orig-value",
				}
			})

			It("should merge the new labels with the existing labels", func() {
				assertEnsure(map[string]string{
					"new-label":    "new-value",
					"common-label": "updated-value",
				}, false)

				Expect(assertNamespace().Labels).To(Equal(map[string]string{
					"existing-label": "existing-value", // preserved
					"new-label":      "new-value",      // added
					"common-label":   "updated-value",  // updated
				}))
			})
		})

		Context("with nil existing labels", func() {
			It("should add the new labels", func() {
				assertEnsure(testLabels, false)
				Expect(assertNamespace().Labels).To(Equal(testLabels))
			})
		})
	})

	When("namespace creation fails", func() {
		BeforeEach(func() {
			fake.FailOnAction(&kubeClient.Fake, "namespaces", "create", errors.New("creation failed"), false)
		})

		It("should return an error", func() {
			_, err := namespace.Ensure(ctx, kubeClient, testNS, testLabels)
			Expect(err).To(HaveOccurred())
		})
	})
})
