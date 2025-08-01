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
	"github.com/submariner-io/admiral/pkg/reporter"
	"github.com/submariner-io/admiral/pkg/reporter/test"
	"github.com/submariner-io/subctl/cmd/subctl"
	"github.com/submariner-io/subctl/internal/cli"
	"github.com/submariner-io/subctl/internal/component"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/pkg/broker"
	"github.com/submariner-io/subctl/pkg/client"
	clientfake "github.com/submariner-io/subctl/pkg/client/fake"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd/api"
)

const (
	clusterName = "east"
	brokerURL   = "https://broker-api-server"
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

	kubeConfigFileName = clientfake.CreateKubeConfigFile(clientConfig)
})

func TestSubctl(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Subctl Suite")
}

type FakePrompt struct {
	value any
}

func (f *FakePrompt) Prompt(_ *survey.PromptConfig) (interface{}, error) {
	return f.value, nil
}

func (f *FakePrompt) Cleanup(_ *survey.PromptConfig, _ interface{}) error {
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
	cmd          *cobra.Command
	args         []string
	fakeProducer *client.DefaultProducer
	status       *test.Tracker
}

func newTestDriver() *testDriver {
	t := &testDriver{}

	BeforeEach(func() {
		t.args = []string{}
		t.fakeProducer = clientfake.New()
		t.status = &test.Tracker{Interface: cli.NewReporter()}

		subctl.NewReporter = func() reporter.Interface {
			return t.status
		}

		subctl.NewInputPrompt = func(from survey.Prompt) survey.Prompt {
			Fail(fmt.Sprintf("Unexpected prompt: %#v", from))
			return nil
		}
	})

	JustBeforeEach(func() {
		Expect(t.cmd.Flags().Set("kubeconfig", kubeConfigFileName)).To(Succeed())
		t.cmd.SetArgs(t.args)
	})

	return t
}

func (t *testDriver) setupNetworkDiscovery(clusterCIDR, serviceCIDR string) {
	name := "kube-controller-manager"

	err := t.fakeProducer.GeneralClient.Create(context.TODO(), &corev1.Pod{
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

func (t *testDriver) createGatewayNode() {
	_, err := t.fakeProducer.KubeClient.CoreV1().Nodes().Create(context.TODO(), &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "worker1",
			Labels: map[string]string{constants.SubmarinerGatewayLabel: constants.TrueLabel},
		},
	}, metav1.CreateOptions{})
	Expect(err).NotTo(HaveOccurred())
}

func (t *testDriver) createNodes(names ...string) {
	for _, name := range names {
		_, err := t.fakeProducer.KubeClient.CoreV1().Nodes().Create(context.TODO(), &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
		}, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
	}
}

func (t *testDriver) getNode(name string) *corev1.Node {
	node, err := t.fakeProducer.KubeClient.CoreV1().Nodes().Get(context.TODO(), name, metav1.GetOptions{})
	Expect(err).NotTo(HaveOccurred())

	return node
}
