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

package crds_test

import (
	"context"
	"errors"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/submariner-io/subctl/pkg/operator/crds"
	"github.com/submariner-io/submariner-operator/pkg/crd"
	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/client-go/kubernetes/scheme"
	controllerclient "sigs.k8s.io/controller-runtime/pkg/client"
	controllerfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func TestCrds(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "CRDs Suite")
}

var _ = BeforeSuite(func() {
	Expect(apiextensions.AddToScheme(scheme.Scheme)).To(Succeed())
})

var _ = Describe("Ensure", func() {
	var client controllerclient.Client

	BeforeEach(func() {
		client = controllerfake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	})

	It("should create operator CRDs", func() {
		created, err := crds.Ensure(context.TODO(), crd.UpdaterFromControllerClient(client))
		Expect(err).NotTo(HaveOccurred())
		Expect(created).To(BeTrue())

		for _, name := range []string{
			"brokers.submariner.io",
			"servicediscoveries.submariner.io",
			"submariners.submariner.io",
		} {
			Expect(client.Get(context.TODO(), controllerclient.ObjectKey{Name: name}, &apiextensions.CustomResourceDefinition{})).To(Succeed())
		}

		created, err = crds.Ensure(context.TODO(), crd.UpdaterFromControllerClient(client))
		Expect(err).NotTo(HaveOccurred())
		Expect(created).To(BeFalse())
	})

	When("creation fails", func() {
		BeforeEach(func() {
			client = controllerfake.NewClientBuilder().WithScheme(scheme.Scheme).
				WithInterceptorFuncs(interceptor.Funcs{
					Create: func(_ context.Context, _ controllerclient.WithWatch, _ controllerclient.Object,
						_ ...controllerclient.CreateOption,
					) error {
						return errors.New("mock")
					},
				}).Build()
		})

		It("should return an error", func() {
			_, err := crds.Ensure(context.TODO(), crd.UpdaterFromControllerClient(client))
			Expect(err).To(HaveOccurred())
		})
	})
})
