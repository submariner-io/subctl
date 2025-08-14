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

package submarinercr_test

import (
	"context"
	"errors"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/pkg/submarinercr"
	operatorv1alpha1 "github.com/submariner-io/submariner-operator/api/v1alpha1"
	"github.com/submariner-io/submariner-operator/pkg/names"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
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
	submarinerKey := ctrlClient.ObjectKey{
		Name:      names.SubmarinerCrName,
		Namespace: constants.OperatorNamespace,
	}

	var (
		client         ctrlClient.Client
		clientBuilder  *controllerfake.ClientBuilder
		submarinerSpec *operatorv1alpha1.SubmarinerSpec
	)

	BeforeEach(func() {
		clientBuilder = controllerfake.NewClientBuilder().WithScheme(scheme.Scheme)

		submarinerSpec = &operatorv1alpha1.SubmarinerSpec{
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

	When("the Submariner resource doesn't exist", func() {
		It("should successfully create it with the correct properties", func() {
			err := submarinercr.Ensure(ctx, client, constants.OperatorNamespace, submarinerSpec)
			Expect(err).NotTo(HaveOccurred())

			submarinerCR := &operatorv1alpha1.Submariner{}
			err = client.Get(ctx, submarinerKey, submarinerCR)
			Expect(err).NotTo(HaveOccurred())

			Expect(submarinerCR.Spec).To(Equal(*submarinerSpec))
		})
	})

	When("the Submariner resource already exists", func() {
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

		JustBeforeEach(func() {
			existing := &operatorv1alpha1.Submariner{
				ObjectMeta: metav1.ObjectMeta{
					Name:      names.SubmarinerCrName,
					Namespace: constants.OperatorNamespace,
				},
				Spec: operatorv1alpha1.SubmarinerSpec{
					BrokerK8sApiServerToken: "existing-token",
				},
			}

			Expect(client.Create(ctx, existing)).To(Succeed())

			existingUID = existing.UID
		})

		It("should replace it with a new resource", func() {
			err := submarinercr.Ensure(ctx, client, constants.OperatorNamespace, submarinerSpec)
			Expect(err).NotTo(HaveOccurred())

			submarinerCR := &operatorv1alpha1.Submariner{}
			err = client.Get(ctx, submarinerKey, submarinerCR)
			Expect(err).NotTo(HaveOccurred())

			Expect(submarinerCR.UID).NotTo(Equal(existingUID))
			Expect(submarinerCR.Spec).To(Equal(*submarinerSpec))
		})
	})

	When("resource creation fails", func() {
		BeforeEach(func() {
			clientBuilder.WithInterceptorFuncs(interceptor.Funcs{
				Create: func(ctx context.Context, c ctrlClient.WithWatch, obj ctrlClient.Object, opts ...ctrlClient.CreateOption) error {
					obj.SetUID(uuid.NewUUID())
					return errors.New("mock")
				},
			})
		})

		It("should return an error", func() {
			err := submarinercr.Ensure(ctx, client, constants.OperatorNamespace, submarinerSpec)
			Expect(err).To(HaveOccurred())
		})
	})
})
