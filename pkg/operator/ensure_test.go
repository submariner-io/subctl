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

package operator_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	compnames "github.com/submariner-io/admiral/pkg/names"
	"github.com/submariner-io/subctl/internal/cli"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/pkg/client"
	clientfake "github.com/submariner-io/subctl/pkg/client/fake"
	"github.com/submariner-io/subctl/pkg/operator"
	"golang.org/x/net/context"
	"golang.org/x/net/http/httpproxy"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	k8stesting "k8s.io/client-go/testing"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestOperator(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Operator Suite")
}

var _ = BeforeSuite(func() {
	Expect(apiextensions.AddToScheme(scheme.Scheme)).To(Succeed())
})

var _ = Describe("Ensure", func() {
	var fakeProducer *client.DefaultProducer

	BeforeEach(func() {
		fakeProducer = clientfake.New()

		fakeProducer.KubeClient.(*k8sfake.Clientset).PrependReactor("create", "deployments",
			func(a k8stesting.Action) (bool, runtime.Object, error) {
				d := a.(k8stesting.CreateAction).GetObject().(*appsv1.Deployment)
				d.Status.Conditions = []appsv1.DeploymentCondition{
					{
						Type:   appsv1.DeploymentAvailable,
						Status: corev1.ConditionTrue,
					},
				}

				return false, nil, nil
			})
	})

	It("should create the operator components", func() {
		proxyConfig := &httpproxy.Config{
			HTTPProxy: "http://my-proxy",
		}

		operatorImage := "quay.io/submariner-operator"

		Expect(operator.Ensure(context.TODO(), cli.NewReporter(), fakeProducer, constants.OperatorNamespace,
			operatorImage, true, proxyConfig)).To(Succeed())

		for _, name := range []string{
			"brokers.submariner.io",
			"servicediscoveries.submariner.io",
			"submariners.submariner.io",
		} {
			err := fakeProducer.GeneralClient.Get(context.TODO(), ctrlclient.ObjectKey{Name: name},
				&apiextensions.CustomResourceDefinition{})
			Expect(err).NotTo(HaveOccurred())
		}

		_, err := fakeProducer.KubeClient.CoreV1().Namespaces().Get(context.TODO(), constants.OperatorNamespace, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())

		for _, name := range []string{
			compnames.OperatorComponent,
			compnames.GatewayComponent,
			compnames.RouteAgentComponent,
			compnames.GlobalnetComponent,
			compnames.ServiceDiscoveryComponent,
			compnames.LighthouseCoreDNSComponent,
		} {
			_, err = fakeProducer.KubeClient.CoreV1().ServiceAccounts(constants.OperatorNamespace).Get(
				context.TODO(), name, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
		}

		deployment, err := fakeProducer.KubeClient.AppsV1().Deployments(constants.OperatorNamespace).Get(
			context.TODO(), compnames.OperatorComponent, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(deployment.Spec.Template.Spec.Containers[0].Env).To(ContainElement(corev1.EnvVar{
			Name:  "HTTP_PROXY",
			Value: proxyConfig.HTTPProxy,
		}))
		Expect(deployment.Spec.Template.Spec.Containers[0].Image).To(Equal(operatorImage))
	})
})
