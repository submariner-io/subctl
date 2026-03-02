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

package show_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"github.com/submariner-io/admiral/pkg/fake"
	reportertest "github.com/submariner-io/admiral/pkg/reporter/test"
	"github.com/submariner-io/subctl/internal/cli"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/pkg/client"
	"github.com/submariner-io/subctl/pkg/cluster"
	"github.com/submariner-io/submariner-operator/api/v1alpha1"
	submarinerv1 "github.com/submariner-io/submariner/pkg/apis/submariner.io/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	controllerfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	testRepository = "test-repo"
	testVersion    = "v0.23.0"
	brokerName     = "submariner-broker"
	podCIDR        = "10.244.0.0/16"
	serviceCIDR    = "10.96.0.0/12"
)

var _ = BeforeSuite(func() {
	Expect(v1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())
	Expect(submarinerv1.AddToScheme(scheme.Scheme)).To(Succeed())
})

func TestShow(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Show Suite")
}

type testDriver struct {
	clusterInfo  *cluster.Info
	k8sClient    *k8sfake.Clientset
	status       *reportertest.Tracker
	outputReader io.Reader
}

func newTestDriver() *testDriver {
	t := &testDriver{}

	BeforeEach(func() {
		oldStdout := os.Stdout
		DeferCleanup(func() {
			os.Stdout = oldStdout
		})

		var err error

		t.outputReader, os.Stdout, err = os.Pipe()
		Expect(err).NotTo(HaveOccurred())

		t.k8sClient = k8sfake.NewClientset()
		fake.AddBasicReactors(&t.k8sClient.Fake)

		t.clusterInfo = &cluster.Info{
			Name:       "test-cluster",
			RestConfig: &rest.Config{},
			ClientProducer: &client.DefaultProducer{
				KubeClient:    fake.WithRESTClient(t.k8sClient, nil),
				GeneralClient: controllerfake.NewClientBuilder().WithScheme(scheme.Scheme).Build(),
			},
		}

		t.status = &reportertest.Tracker{Interface: cli.NewReporter()}
	})

	return t
}

func (t *testDriver) getOutput() string {
	Expect(os.Stdout.Close()).To(Succeed())

	var buf bytes.Buffer
	_, err := io.Copy(&buf, t.outputReader)
	Expect(err).ToNot(HaveOccurred())

	out := buf.String()
	fmt.Fprint(os.Stderr, out)

	return out
}

func (t *testDriver) assertTableOutput(rowMatchers ...any) {
	Expect(strings.Split(strings.TrimSpace(t.getOutput()), "\n")[1:]).To(ConsistOf(rowMatchers...))
}

func (t *testDriver) createDaemonSet(ctx context.Context, name string) *appsv1.DaemonSet {
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: constants.OperatorNamespace,
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": name,
				},
			},
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  name,
							Image: fmt.Sprintf("%s/submariner/%s:%s", testRepository, name, testVersion),
						},
					},
				},
			},
		},
	}

	result, err := t.k8sClient.AppsV1().DaemonSets(constants.OperatorNamespace).Create(ctx, ds, metav1.CreateOptions{})
	Expect(err).NotTo(HaveOccurred())

	return result
}

func (t *testDriver) createDeployment(ctx context.Context, name string) *appsv1.Deployment {
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: constants.OperatorNamespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": name,
				},
			},
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  name,
							Image: fmt.Sprintf("%s/submariner/%s:%s", testRepository, name, testVersion),
						},
					},
				},
			},
		},
	}

	result, err := t.k8sClient.AppsV1().Deployments(constants.OperatorNamespace).Create(ctx, deployment, metav1.CreateOptions{})
	Expect(err).NotTo(HaveOccurred())

	return result
}

func (t *testDriver) createPod(ctx context.Context, name, nodeName string, labels map[string]string) *corev1.Pod {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: constants.OperatorNamespace,
			Labels:    labels,
		},
		Spec: corev1.PodSpec{
			NodeName: nodeName,
			Containers: []corev1.Container{
				{
					Name: "container",
				},
			},
		},
	}

	result, err := t.k8sClient.CoreV1().Pods(constants.OperatorNamespace).Create(ctx, pod, metav1.CreateOptions{})
	Expect(err).NotTo(HaveOccurred())

	return result
}

func (t *testDriver) createNode(ctx context.Context, name, arch string) *corev1.Node {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				corev1.LabelArchStable: arch,
			},
		},
	}

	result, err := t.k8sClient.CoreV1().Nodes().Create(ctx, node, metav1.CreateOptions{})
	Expect(err).NotTo(HaveOccurred())

	return result
}

func (t *testDriver) createGateway(ctx context.Context, name string, haStatus submarinerv1.HAStatus, sf string,
	cs ...submarinerv1.ConnectionStatus,
) *submarinerv1.Gateway {
	gateway := &submarinerv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: constants.OperatorNamespace,
		},
		Status: submarinerv1.GatewayStatus{
			HAStatus:      haStatus,
			StatusFailure: sf,
			LocalEndpoint: submarinerv1.EndpointSpec{
				Hostname: "node1",
			},
			Connections: make([]submarinerv1.Connection, 0, len(cs)),
		},
	}

	for _, c := range cs {
		gateway.Status.Connections = append(gateway.Status.Connections, submarinerv1.Connection{Status: c})
	}

	Expect(t.clusterInfo.ClientProducer.ForGeneral().Create(ctx, gateway)).To(Succeed())

	return gateway
}

func tableRowMatcher(values ...string) types.GomegaMatcher {
	return WithTransform(func(line string) []string {
		parts := strings.Fields(line)
		for len(parts) < len(values) {
			parts = append(parts, "")
		}

		return parts
	}, Equal(values))
}
