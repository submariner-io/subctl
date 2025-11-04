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

package diagnose_test

import (
	"errors"
	"slices"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/submariner-io/admiral/pkg/fake"
	"github.com/submariner-io/admiral/pkg/names"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/pkg/cluster"
	"github.com/submariner-io/subctl/pkg/diagnose"
	submarinerv1 "github.com/submariner-io/submariner/pkg/apis/submariner.io/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	k8stesting "k8s.io/client-go/testing"
)

const (
	lbEncapsPort = 111
	lbNatPort    = 222
	clientPodIP  = "10.244.1.5"
)

type firewallTestDriver struct {
	*testDriver
	localClusterInfo  *cluster.Info
	remoteClusterInfo *cluster.Info
	localEndpoint     *submarinerv1.Endpoint
	localGatewayPod   *corev1.Pod
	remoteGateway     *submarinerv1.Gateway
	loadBalancerSvc   *corev1.Service
	options           diagnose.FirewallOptions
	workerNodeName    string
}

func newFirewallTestDriver() *firewallTestDriver {
	t := &firewallTestDriver{testDriver: newTestDriver()}

	BeforeEach(func() {
		t.workerNodeName = "worker-1"
		t.options = diagnose.FirewallOptions{
			ValidationTimeout: 5,
			VerboseOutput:     true,
		}

		t.localEndpoint = &submarinerv1.Endpoint{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "local-endpoint",
				Namespace: constants.OperatorNamespace,
			},
			Spec: submarinerv1.EndpointSpec{
				ClusterID:     localCluster,
				CableName:     "cable-local",
				Hostname:      "raiders",
				PrivateIPs:    []string{"10.253.6.1"},
				Backend:       diagnose.Libreswan,
				BackendConfig: map[string]string{},
			},
		}

		t.localGatewayPod = &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: names.GatewayComponent,
				Labels: map[string]string{
					diagnose.GatewayHAStatusLabel: string(submarinerv1.HAStatusActive),
					diagnose.GatewayNodeLabel:     "master",
				},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				PodIP: "10.0.0.1",
				Conditions: []corev1.PodCondition{
					{
						Type:   corev1.PodReady,
						Status: corev1.ConditionTrue,
					},
				},
			},
		}

		t.remoteGateway = &submarinerv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "remote-gateway",
				Namespace: constants.OperatorNamespace,
			},
			Status: submarinerv1.GatewayStatus{
				HAStatus: submarinerv1.HAStatusActive,
				Connections: []submarinerv1.Connection{
					{
						Endpoint: submarinerv1.EndpointSpec{
							ClusterID: localCluster,
						},
						UsingIP: t.localEndpoint.Spec.PrivateIPs[0],
					},
				},
			},
		}

		t.loadBalancerSvc = &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      diagnose.LoadBalancerName,
				Namespace: constants.OperatorNamespace,
			},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{
					{
						Name:     diagnose.LoadBalancerEncapsPortName,
						NodePort: lbEncapsPort,
					},
					{
						Name:     diagnose.LoadBalancerNattPortName,
						NodePort: lbNatPort,
					},
				},
			},
		}
	})

	JustBeforeEach(func() {
		var err error

		t.localClusterInfo, err = cluster.NewInfo(localCluster, &rest.Config{})
		Expect(err).NotTo(HaveOccurred())

		t.remoteClusterInfo, err = cluster.NewInfo(remoteCluster, &rest.Config{})
		Expect(err).NotTo(HaveOccurred())

		t.remoteClusterInfo.Submariner.Status.ClusterID = remoteCluster

		if t.workerNodeName != "" {
			t.createNode(t.workerNodeName)
		}

		if t.localEndpoint != nil {
			t.createResource(t.localEndpoint)
		}

		if t.localGatewayPod != nil {
			t.ensurePodWithStatus(t.localGatewayPod.Name, t.localGatewayPod.Labels, t.localGatewayPod.Status)
		}

		if t.remoteGateway != nil {
			t.createResource(t.remoteGateway)
		}

		if t.loadBalancerSvc != nil {
			t.createService(t.loadBalancerSvc)
		}
	})

	return t
}

func (t *firewallTestDriver) setupPodOutput(podOutput string) *string {
	var podCmd string

	t.fakeProducer.KubeClient.(*k8sfake.Clientset).Fake.PrependReactor("create", "pods",
		func(action k8stesting.Action) (bool, runtime.Object, error) {
			pod := action.(k8stesting.CreateAction).GetObject().(*corev1.Pod)
			Expect(pod.Spec.Containers).NotTo(BeEmpty())

			// The sniffer pod generates a message that it expects the client pod to send. The sniffer pod
			// command runs tcpdump and searches for the message in its output and echoes to stdout so the
			// message is included in the pod command. Rather than parsing the pod command to identify the
			// message, to simplify, just return the entire pod command as the pod output since the validation
			// only verifies that the message is present in the output.
			index := slices.IndexFunc(pod.Spec.Containers[0].Env, func(envVar corev1.EnvVar) bool {
				return strings.Contains(envVar.Value, "tcpdump")
			})
			if index != -1 {
				if podOutput == "" {
					// Use pod command as default output for sniffer pods
					podOutput = pod.Spec.Containers[0].Env[index].Value
				}

				podCmd = pod.Spec.Containers[0].Env[index].Value
			}

			pod.Status = corev1.PodStatus{
				Phase: corev1.PodSucceeded,
				PodIP: clientPodIP,
				ContainerStatuses: []corev1.ContainerStatus{
					{
						State: corev1.ContainerState{
							Terminated: &corev1.ContainerStateTerminated{Message: podOutput},
						},
					},
				},
			}

			return false, nil, nil
		})

	return &podCmd
}

func (t *firewallTestDriver) testPortConnectivitySuccess(run func() error, podOutput string, verifyCmd func(string)) {
	var podCmd *string

	JustBeforeEach(func() {
		podCmd = t.setupPodOutput(podOutput)
	})

	It("should successfully run the connectivity check", func() {
		t.assertSuccess(run)
		verifyCmd(*podCmd)
	})
}

func (t *firewallTestDriver) testPortConnectivityFailure(run func() error, podOutput string, msgs ...string) {
	JustBeforeEach(func() {
		t.setupPodOutput(podOutput)
	})

	Specify("the connectivity check should fail", func() {
		t.assertFailure(run, msgs...)
	})
}

func (t *firewallTestDriver) testFirewallCheckOnSingleNode(run func() error) {
	When("the remote cluster is single-node", func() {
		BeforeEach(func() {
			t.workerNodeName = ""
		})

		It("should skip the check and succeed", func() {
			t.assertSuccess(run)
		})
	})
}

func (t *firewallTestDriver) testPodCreationFailure(run func() error) {
	When("pod creation fails", func() {
		podError := errors.New("fake pod creation error")

		JustBeforeEach(func() {
			fake.FailOnAction(&t.fakeProducer.KubeClient.(*k8sfake.Clientset).Fake, "pods", "create", podError, false)
		})

		t.testPortConnectivityFailure(run, "", podError.Error())
	})
}

func (t *firewallTestDriver) testIncompletePod(run func() error) {
	When("a pod doesn't complete", func() {
		JustBeforeEach(func() {
			t.fakeProducer.KubeClient.(*k8sfake.Clientset).Fake.PrependReactor("create", "pods",
				func(action k8stesting.Action) (bool, runtime.Object, error) {
					pod := action.(k8stesting.CreateAction).GetObject().(*corev1.Pod)

					pod.Status = corev1.PodStatus{
						Phase: corev1.PodRunning,
					}

					return false, nil, nil
				})
		})

		t.testFailure(run, string(corev1.PodRunning))
	})
}

func (t *firewallTestDriver) testActiveGatewayPodErrors(run func() error) {
	When("there is no active Gateway pod", func() {
		BeforeEach(func() {
			t.localGatewayPod.Labels[diagnose.GatewayHAStatusLabel] = string(submarinerv1.HAStatusPassive)
		})

		t.testFailure(run, "active Gateway")
	})

	When("the active Gateway pod is missing the node name label", func() {
		BeforeEach(func() {
			delete(t.localGatewayPod.Labels, diagnose.GatewayNodeLabel)
		})

		t.testFailure(run, "missing", diagnose.GatewayNodeLabel)
	})
}

func (t *firewallTestDriver) testImageRepositoryFailure(run func() error) {
	t.testImageRepositoryInfoFailure(func() {
		t.options.ImageOverrides = []string{"invalid:image:override"}
	}, run)
}
