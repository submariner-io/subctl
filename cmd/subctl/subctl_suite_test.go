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
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/AlecAivazis/survey/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"
	fakecommand "github.com/submariner-io/admiral/pkg/command/fake"
	"github.com/submariner-io/admiral/pkg/log"
	"github.com/submariner-io/admiral/pkg/reporter"
	"github.com/submariner-io/admiral/pkg/reporter/test"
	"github.com/submariner-io/subctl/cmd/subctl"
	"github.com/submariner-io/subctl/internal/cli"
	"github.com/submariner-io/subctl/internal/component"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/pkg/broker"
	"github.com/submariner-io/subctl/pkg/brokercr"
	"github.com/submariner-io/subctl/pkg/client"
	clientfake "github.com/submariner-io/subctl/pkg/client/fake"
	"github.com/submariner-io/submariner-operator/api/v1alpha1"
	"github.com/submariner-io/submariner-operator/pkg/names"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/clientcmd/api"
	controllerClient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	clusterName    = "east"
	remoteCluster  = "west"
	remoteCluster2 = "north"
	brokerURL      = "https://broker-api-server"
)

var (
	brokerInfoFileName string
	kubeConfigFileName string
	brokerInfo         broker.Info
)

var _ = BeforeSuite(func() {
	file, err := os.CreateTemp("", "broker-info-")
	Expect(err).To(Succeed())

	brokerInfoFileName = file.Name()

	DeferCleanup(func() {
		_ = os.Remove(brokerInfoFileName)
	})

	psk := make([]byte, 48)
	_, err = rand.Read(psk)
	Expect(err).To(Succeed())

	brokerInfo = broker.Info{
		BrokerURL: brokerURL,
		ClientToken: &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: constants.SubmarinerBrokerAdminSA + "-token-",
				Annotations: map[string]string{
					corev1.ServiceAccountNameKey: constants.SubmarinerBrokerAdminSA,
				},
			},
			Type: corev1.SecretTypeServiceAccountToken,
		},
		IPSecPSK: &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name: "submariner-ipsec-psk",
			},
			Data: map[string][]byte{"psk": psk},
		},
		ServiceDiscovery: true,
		Components:       []string{component.ServiceDiscovery, component.Connectivity},
	}

	Expect(brokerInfo.WriteToFile(brokerInfoFileName)).To(Succeed())

	clientConfig := api.NewConfig()
	clientConfig.Clusters[clusterName] = &api.Cluster{}
	clientConfig.Contexts[clusterName] = &api.Context{
		Cluster: clusterName,
	}
	clientConfig.CurrentContext = clusterName

	clientConfig.Clusters[remoteCluster] = &api.Cluster{}
	clientConfig.Contexts[remoteCluster] = &api.Context{
		Cluster: remoteCluster,
	}

	clientConfig.Clusters[remoteCluster2] = &api.Cluster{}
	clientConfig.Contexts[remoteCluster2] = &api.Context{
		Cluster: remoteCluster2,
	}

	kubeConfigFileName = clientfake.CreateKubeConfigFile(clientConfig)
})

func TestSubctl(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Subctl Suite")
}

type FakePrompt struct {
	value any
}

func (f *FakePrompt) Prompt(_ *survey.PromptConfig) (any, error) {
	return f.value, nil
}

func (f *FakePrompt) Cleanup(_ *survey.PromptConfig, _ any) error {
	return nil
}

func (f *FakePrompt) Error(_ *survey.PromptConfig, _ error) error {
	return nil
}

func setupPrompts(respValues map[string]any) {
	subctl.NewInputPrompt = func(from survey.Prompt) survey.Prompt {
		var msg string

		switch p := from.(type) {
		case *survey.Input:
			msg = p.Message
		case *survey.Select:
			msg = p.Message
		case *survey.Confirm:
			msg = p.Message
		}

		var respValue any

		for msgStr, value := range respValues {
			if strings.Contains(msg, msgStr) {
				respValue = value
				break
			}
		}

		return &FakePrompt{value: respValue}
	}
}

type testDriver struct {
	cmd                  *cobra.Command
	args                 []string
	fakeProducer         *client.DefaultProducer
	status               *test.Tracker
	exited               bool
	cmdExecutor          *fakecommand.Executor
	submarinerSpec       *v1alpha1.SubmarinerSpec
	submariner           *v1alpha1.Submariner
	serviceDiscoverySpec *v1alpha1.ServiceDiscoverySpec
	serviceDiscovery     *v1alpha1.ServiceDiscovery
	brokerSpec           *v1alpha1.BrokerSpec
	broker               *v1alpha1.Broker
}

func newTestDriver() *testDriver {
	t := &testDriver{}

	BeforeEach(func() {
		t.args = []string{}
		t.fakeProducer = clientfake.New()
		t.status = &test.Tracker{Interface: cli.NewReporter()}
		t.exited = false
		t.cmdExecutor = fakecommand.New()
		t.submarinerSpec = nil
		t.submariner = nil
		t.serviceDiscoverySpec = nil
		t.serviceDiscovery = nil
		t.brokerSpec = nil
		t.broker = nil

		clientfake.AddDeploymentAvailableReactor(&t.fakeProducer.KubeClient.(*k8sfake.Clientset).Fake)

		log.Exit = func(_ int) {
			t.exited = true
		}

		subctl.NewReporter = func() reporter.Interface {
			return t.status
		}

		subctl.NewInputPrompt = func(from survey.Prompt) survey.Prompt {
			Fail(fmt.Sprintf("Unexpected prompt: %#v", from))
			return nil
		}
	})

	JustBeforeEach(func(ctx SpecContext) {
		if t.submarinerSpec != nil {
			t.submariner = &v1alpha1.Submariner{
				ObjectMeta: metav1.ObjectMeta{
					Name:      names.SubmarinerCrName,
					Namespace: constants.OperatorNamespace,
				},
				Spec: *t.submarinerSpec,
			}

			Expect(t.fakeProducer.GeneralClient.Create(ctx, t.submariner)).To(Succeed())
		}

		if t.serviceDiscoverySpec != nil {
			t.serviceDiscovery = &v1alpha1.ServiceDiscovery{
				ObjectMeta: metav1.ObjectMeta{
					Name:      names.ServiceDiscoveryCrName,
					Namespace: constants.OperatorNamespace,
				},
				Spec: *t.serviceDiscoverySpec,
			}

			Expect(t.fakeProducer.GeneralClient.Create(ctx, t.serviceDiscovery)).To(Succeed())
		}

		if t.brokerSpec != nil {
			t.broker = &v1alpha1.Broker{
				ObjectMeta: metav1.ObjectMeta{
					Name:      brokercr.Name,
					Namespace: constants.DefaultBrokerNamespace,
				},
				Spec: *t.brokerSpec,
			}

			Expect(t.fakeProducer.GeneralClient.Create(ctx, t.broker)).To(Succeed())
		}

		t.args = append(t.args, "--kubeconfig="+kubeConfigFileName, "--context="+clusterName)

		t.cmd.SetArgs(t.args)
	})

	return t
}

func (t *testDriver) setupNetworkDiscovery(ctx context.Context, clusterCIDR, serviceCIDR string) {
	name := "kube-controller-manager"

	err := t.fakeProducer.GeneralClient.Create(ctx, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      name,
			Labels:    map[string]string{"component": name, "name": name, "app": name, "k8s-app": name},
		},

		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Command: []string{
						"kube-controller-manager", "--cluster-cidr=" + clusterCIDR,
						"--service-cluster-ip-range=" + serviceCIDR,
					},
				},
			},
		},
	})
	Expect(err).NotTo(HaveOccurred())
}

func (t *testDriver) createGatewayNode(ctx context.Context) {
	_, err := t.fakeProducer.KubeClient.CoreV1().Nodes().Create(ctx, &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "worker1",
			Labels: map[string]string{constants.SubmarinerGatewayLabel: constants.TrueLabel},
		},
	}, metav1.CreateOptions{})
	Expect(err).NotTo(HaveOccurred())
}

func (t *testDriver) createNodes(ctx context.Context, nodeNames ...string) {
	for _, name := range nodeNames {
		_, err := t.fakeProducer.KubeClient.CoreV1().Nodes().Create(ctx, &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
		}, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
	}
}

func (t *testDriver) getNode(ctx context.Context, name string) *corev1.Node {
	node, err := t.fakeProducer.KubeClient.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
	Expect(err).NotTo(HaveOccurred())

	return node
}

func (t *testDriver) getSubmariner(ctx context.Context) *v1alpha1.Submariner {
	submariner := &v1alpha1.Submariner{}
	Expect(t.fakeProducer.GeneralClient.Get(ctx, controllerClient.ObjectKey{
		Namespace: constants.OperatorNamespace,
		Name:      names.SubmarinerCrName,
	}, submariner)).To(Succeed())

	return submariner
}

func (t *testDriver) getServiceDiscovery(ctx context.Context) *v1alpha1.ServiceDiscovery {
	sd := &v1alpha1.ServiceDiscovery{}
	Expect(t.fakeProducer.GeneralClient.Get(ctx, controllerClient.ObjectKey{
		Namespace: constants.OperatorNamespace,
		Name:      names.ServiceDiscoveryCrName,
	}, sd)).To(Succeed())

	return sd
}

func (t *testDriver) getBroker(ctx context.Context) *v1alpha1.Broker {
	b := &v1alpha1.Broker{}
	Expect(t.fakeProducer.GeneralClient.Get(ctx, controllerClient.ObjectKey{
		Namespace: constants.DefaultBrokerNamespace,
		Name:      brokercr.Name,
	}, b)).To(Succeed())

	return b
}

func (t *testDriver) assertCmdSuccess() {
	Expect(t.cmd.Execute()).To(Succeed())
	t.status.AssertFailureCount(0)
	t.status.AssertWarningCount(0)
	Expect(t.exited).To(BeFalse())
}

func (t *testDriver) assertCmdFailed(s ...string) {
	Expect(t.cmd.Execute()).To(Succeed())
	t.status.AssertFailureCount(1)
	t.status.AssertFailureContainsStrings(s...)
	Expect(t.exited).To(BeTrue())
}

func (t *testDriver) testCmdFailure(s ...string) {
	It("should exit with an error", func() {
		t.assertCmdFailed(s...)
	})
}
