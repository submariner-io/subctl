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

package secret_test

import (
	"context"
	"errors"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/submariner-io/admiral/pkg/fake"
	"github.com/submariner-io/subctl/pkg/secret"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
)

const (
	namespace   = "test-namespace"
	secretName  = "test-secret"
	secretKey   = "test-key"
	secretValue = "test-value"
)

func TestSecret(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Secret Suite")
}

var _ = Describe("Ensure", func() {
	var (
		kubeClient *fakeclientset.Clientset
		testSecret *corev1.Secret
	)

	BeforeEach(func() {
		kubeClient = fakeclientset.NewClientset()
		fake.AddCreateReactor(&kubeClient.Fake)

		testSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-secret",
			},
			Data: map[string][]byte{
				"test-key": []byte("test-value"),
			},
			Type: corev1.SecretTypeOpaque,
		}
	})

	assertEnsure := func(ctx context.Context) *corev1.Secret {
		created, err := secret.Ensure(ctx, kubeClient, namespace, testSecret)
		Expect(err).NotTo(HaveOccurred())
		Expect(created).ToNot(BeNil())
		Expect(created.Name).To(Equal(testSecret.Name))
		Expect(created.Namespace).To(Equal(namespace))
		Expect(created.Data).To(Equal(testSecret.Data))
		Expect(created.Type).To(Equal(testSecret.Type))

		actualSecret, err := kubeClient.CoreV1().Secrets(namespace).Get(ctx, testSecret.Name, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(actualSecret).To(Equal(created))

		return created
	}

	When("the Secret doesn't exist", func() {
		It("should successfully create it with the correct properties", func(ctx SpecContext) {
			assertEnsure(ctx)
		})
	})

	When("the Secret already exists", func() {
		var existingUID types.UID

		BeforeEach(func(ctx SpecContext) {
			s, err := kubeClient.CoreV1().Secrets(namespace).Create(ctx, testSecret, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			existingUID = s.UID
			testSecret.Data = map[string][]byte{
				"new-key": []byte("new-value"),
			}

			fake.FailOnAction(&kubeClient.Fake, "secrets", "update", errors.New("updates not allowed"), false)
		})

		It("should replace it with the new Secret", func(ctx SpecContext) {
			actual := assertEnsure(ctx)
			Expect(actual.UID).NotTo(Equal(existingUID))
		})
	})

	When("Secret creation fails", func() {
		BeforeEach(func() {
			fake.FailOnAction(&kubeClient.Fake, "secrets", "create", errors.New("creation failed"), false)
		})

		It("should return an error", func(ctx SpecContext) {
			_, err := secret.Ensure(ctx, kubeClient, namespace, testSecret)
			Expect(err).To(HaveOccurred())
		})
	})
})
