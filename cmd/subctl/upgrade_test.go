//go:build !non_deploy

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

package subctl_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/submariner-io/admiral/pkg/fake"
	"github.com/submariner-io/admiral/pkg/names"
	"github.com/submariner-io/subctl/cmd/subctl"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/pkg/broker"
	"github.com/submariner-io/subctl/pkg/version"
	"github.com/submariner-io/submariner-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	controllerfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("Upgrade", func() {
	t := newUpgradeTestDriver()

	It("should upgrade installed components", func() {
		t.assertCmdSuccess()
		Expect(t.getSubmariner().UID).NotTo(Equal(t.submariner.UID))
		t.assertOperatorDeploymentUpgraded(version.Version)
	})

	When("the broker secret is stale", func() {
		var staleSecret *corev1.Secret

		BeforeEach(func() {
			secretName := "broker-secret-stale"

			var err error

			_, err = t.fakeProducer.ForKubernetes().CoreV1().Secrets(constants.OperatorNamespace).Create(
				ctx, &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Name: secretName,
					},
					Data: map[string][]byte{"foo": []byte("bar")},
				}, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			staleSecret, err = t.fakeProducer.ForKubernetes().CoreV1().Secrets(constants.OperatorNamespace).Get(
				ctx, secretName, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())

			t.submarinerSpec.BrokerK8sSecret = staleSecret.Name
		})

		It("should migrate it", func() {
			t.assertCmdSuccess()

			migrated, err := t.fakeProducer.ForKubernetes().CoreV1().Secrets(constants.OperatorNamespace).Get(
				ctx, broker.LocalClientBrokerSecretName, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(migrated.Data).To(Equal(staleSecret.Data))

			_, err = t.fakeProducer.ForKubernetes().CoreV1().Secrets(constants.OperatorNamespace).Get(
				ctx, staleSecret.Name, metav1.GetOptions{})
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
		})

		Context("secret retrieval fails", func() {
			BeforeEach(func() {
				fake.FailOnAction(&t.fakeProducer.ForKubernetes().(*k8sfake.Clientset).Fake, "secrets", "get", nil, false)
			})

			t.testCmdFailure("retrieving", "secret")
		})
	})

	When("only ServiceDiscovery is installed", func() {
		BeforeEach(func() {
			t.submarinerSpec = nil
			t.serviceDiscoverySpec = &v1alpha1.ServiceDiscoverySpec{}
		})

		It("should upgrade it", func() {
			t.assertCmdSuccess()
			Expect(t.getServiceDiscovery().UID).NotTo(Equal(t.serviceDiscovery.UID))
			t.assertOperatorDeploymentUpgraded(version.Version)
		})
	})

	When("the broker is installed", func() {
		BeforeEach(func() {
			t.brokerSpec = &v1alpha1.BrokerSpec{}
		})

		It("should upgrade it", func() {
			t.assertCmdSuccess()
			Expect(t.getBroker().UID).NotTo(Equal(t.broker.UID))
			t.assertOperatorDeploymentUpgraded(version.Version)
		})
	})

	When("the --to-version flag is specified", func() {
		var toVersion string

		BeforeEach(func() {
			oldVersion := version.Version
			version.Version = "100.0.0"

			DeferCleanup(func() {
				version.Version = oldVersion
			})
		})

		Context("and the specified version is newer", func() {
			BeforeEach(func() {
				toVersion = "101.0.0"
				t.args = []string{"--to-version=" + toVersion}
			})

			It("should upgrade subctl to the specified version and restart it", func() {
				t.assertCmdSuccess()
				t.cmdExecutor.AwaitCommand(nil, ContainElement(ContainSubstring("curl")),
					ContainElement(ContainSubstring("VERSION=v"+toVersion)))
				t.cmdExecutor.AwaitCommand(nil, ContainElement(ContainSubstring(os.Args[0])))
			})
		})

		Context("and the specified version is older", func() {
			BeforeEach(func() {
				toVersion = "99.0.0"
				t.args = []string{"--to-version=" + toVersion}
			})

			It("should upgrade installed components but not subctl", func() {
				t.assertCmdSuccess()
				t.cmdExecutor.EnsureNoCommand(nil)
				t.assertOperatorDeploymentUpgraded(toVersion)
			})
		})
	})

	When("no Submariner components are installed", func() {
		BeforeEach(func() {
			t.submarinerSpec = nil
			t.serviceDiscoverySpec = nil
			t.brokerSpec = nil
		})

		It("should succeed", func() {
			t.assertCmdSuccess()
		})
	})

	When("Broker retrieval fails", func() {
		BeforeEach(func() {
			t.fakeProducer.GeneralClient = controllerfake.NewClientBuilder().WithScheme(scheme.Scheme).WithInterceptorFuncs(
				fake.FailingGetInterceptor[*v1alpha1.Broker]()).Build()
		})

		t.testCmdFailure("retrieving Broker")
	})

	When("operator Deployment retrieval fails", func() {
		BeforeEach(func() {
			fake.FailOnAction(&t.fakeProducer.ForKubernetes().(*k8sfake.Clientset).Fake, "deployments", "get", nil, false)
		})

		t.testCmdFailure("retrieving", "deployment")
	})

	When("retrieval of latest release tag fails", func() {
		BeforeEach(func() {
			t.httpHandler = func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusRequestTimeout)
			}
		})

		It("should upgrade subctl to the latest version", func() {
			Expect(t.cmd.Execute()).To(Succeed())
			t.status.AssertWarningCount(1)
			t.cmdExecutor.AwaitCommand(nil, ContainElement(ContainSubstring("curl")),
				ContainElement(ContainSubstring("VERSION=latest")))
		})
	})
})

type upgradeTestDriver struct {
	*testDriver
	operatorDeployment *appsv1.Deployment
	httpHandler        http.HandlerFunc
}

func newUpgradeTestDriver() *upgradeTestDriver {
	t := &upgradeTestDriver{testDriver: newTestDriver()}

	BeforeEach(func() {
		t.cmd = subctl.NewUpgradeCmd()

		t.httpHandler = func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, err := fmt.Fprintf(w, "{\"tag_name\": \"%s\"}", version.Version)
			Expect(err).NotTo(HaveOccurred())
		}

		t.submarinerSpec = &v1alpha1.SubmarinerSpec{}

		t.operatorDeployment = &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name: names.OperatorComponent,
			},
		}
	})

	JustBeforeEach(func() {
		server := httptest.NewServer(t.httpHandler)

		DeferCleanup(func() {
			server.Close()
		})

		subctl.LatestReleaseURL = server.URL

		if t.operatorDeployment != nil {
			_, err := t.fakeProducer.ForKubernetes().AppsV1().Deployments(constants.OperatorNamespace).Create(ctx, t.operatorDeployment,
				metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())
		}
	})

	return t
}

func (t *upgradeTestDriver) assertOperatorDeploymentUpgraded(toVersion string) {
	deployment, err := t.fakeProducer.ForKubernetes().AppsV1().Deployments(constants.OperatorNamespace).Get(
		ctx, names.OperatorComponent, metav1.GetOptions{})
	Expect(err).NotTo(HaveOccurred())
	Expect(deployment.Spec.Template.Spec.Containers).To(HaveLen(1))
	Expect(deployment.Spec.Template.Spec.Containers[0].Image).To(ContainSubstring(toVersion))
}
