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
	"context"
	"fmt"
	"slices"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/submariner-io/admiral/pkg/fake"
	"github.com/submariner-io/admiral/pkg/names"
	"github.com/submariner-io/admiral/pkg/resource"
	"github.com/submariner-io/admiral/pkg/util"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/pkg/diagnose"
	submarinerv1 "github.com/submariner-io/submariner/pkg/apis/submariner.io/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	k8stesting "k8s.io/client-go/testing"
	controllerfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("Deployments", func() {
	t := newDeploymentTestDriver()

	When("all connectivity components are healthy", func() {
		JustBeforeEach(func() {
			// Create non-overlapping endpoints for CIDR check
			t.createEndpoint("endpoint1", "cluster1", []string{"10.0.0.0/16"})
			t.createEndpoint("endpoint2", "cluster2", []string{"192.168.0.0/16"})
		})

		t.testSuccess(t.run)
	})

	When("overlapping CIDRs exist", func() {
		JustBeforeEach(func() {
			// Create overlapping endpoints
			t.createEndpoint("endpoint1", "cluster1", []string{"10.0.0.0/16"})
			t.createEndpoint("endpoint2", "cluster2", []string{"10.0.0.0/16"})
		})

		t.testFailure(t.run, "overlaps")
	})

	When("multiple endpoints exist for the same cluster", func() {
		JustBeforeEach(func() {
			t.createEndpoint("endpoint1", "cluster1", []string{"10.0.0.0/16"})
			t.createEndpoint("endpoint2", "cluster1", []string{"192.168.0.0/16"})
		})

		t.testFailure(t.run, "multiple", "endpoints")
	})

	When("there's no active gateway pod", func() {
		JustBeforeEach(func() {
			Expect(util.MustUpdate(context.TODO(), resource.ForPod(t.fakeProducer.KubeClient, constants.OperatorNamespace),
				&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: names.GatewayComponent}}, func(existing *corev1.Pod) (*corev1.Pod, error) {
					existing.Labels["gateway.submariner.io/status"] = string(submarinerv1.HAStatusPassive)
					return existing, nil
				})).To(Succeed())
		})

		t.testSuccessWithWarning(t.run, "active")
	})

	When("there's no gateway pod", func() {
		JustBeforeEach(func() {
			Expect(t.fakeProducer.KubeClient.CoreV1().Pods(constants.OperatorNamespace).Delete(context.TODO(),
				names.GatewayComponent, metav1.DeleteOptions{})).To(Succeed())
		})

		t.testSuccessWithWarning(t.run, "active")
	})

	DescribeTableSubtree("with connectivity enabled",
		t.testMissingDaemonset,
		Entry("", names.GatewayComponent),
		Entry("", names.RouteAgentComponent),
		Entry("", names.MetricsProxyComponent))

	DescribeTableSubtree("with connectivity enabled",
		t.testUnhealthyDaemonset,
		Entry("", names.GatewayComponent),
		Entry("", names.RouteAgentComponent),
		Entry("", names.MetricsProxyComponent))

	DescribeTableSubtree("with connectivity enabled",
		t.testUnhealthyPod,
		Entry("", names.GatewayComponent),
		Entry("", names.RouteAgentComponent),
		Entry("", names.MetricsProxyComponent))

	Context("with globalnet enabled", func() {
		BeforeEach(func() {
			t.submariner.Spec.GlobalCIDR = "242.0.0.0/8"
		})

		JustBeforeEach(func() {
			t.ensureDaemonSet(names.GlobalnetComponent, 1, 1)
		})

		Context("and the components are healthy", func() {
			JustBeforeEach(func() {
				t.ensurePodWithStatus(names.GlobalnetComponent, nil, corev1.PodStatus{Phase: corev1.PodRunning})
			})

			t.testSuccess(t.run)
		})

		t.testMissingDaemonset(names.GlobalnetComponent)
		t.testUnhealthyDaemonset(names.GlobalnetComponent)
		t.testUnhealthyPod(names.GlobalnetComponent)
	})

	Context("with service discovery enabled", func() {
		JustBeforeEach(func() {
			t.createResource(t.serviceDiscovery)
			t.ensureDeployment(names.ServiceDiscoveryComponent, 1, 1)
			t.ensureDeployment(names.LighthouseCoreDNSComponent, 1, 1)
		})

		Context("and the components are healthy", func() {
			JustBeforeEach(func() {
				t.ensurePodWithStatus(names.ServiceDiscoveryComponent, nil, corev1.PodStatus{Phase: corev1.PodRunning})
				t.ensurePodWithStatus(names.LighthouseCoreDNSComponent, nil, corev1.PodStatus{Phase: corev1.PodRunning})
			})

			t.testSuccess(t.run)
		})

		t.testMissingDeployment(names.ServiceDiscoveryComponent)
		t.testMissingDeployment(names.LighthouseCoreDNSComponent)

		t.testUnhealthyDeployment(names.ServiceDiscoveryComponent)
		t.testUnhealthyDeployment(names.LighthouseCoreDNSComponent)

		t.testUnhealthyPod(names.ServiceDiscoveryComponent)
		t.testUnhealthyPod(names.LighthouseCoreDNSComponent)
	})

	Context("with connectivity disabled", func() {
		BeforeEach(func() {
			t.submariner = nil
		})

		t.testSuccess(t.run)
	})

	Context("with a multiple node environment", t.testMultipleNodes)

	When("creation of broker REST config fails", func() {
		BeforeEach(func() {
			resource.NewDynamicClient = func(_ *rest.Config) (dynamic.Interface, error) {
				return nil, errFake
			}
		})

		t.testFailure(t.run, "REST config", errFake.Error())
	})

	When("listing of Endpoint resources fails", func() {
		BeforeEach(func() {
			t.fakeProducer.GeneralClient = controllerfake.NewClientBuilder().WithScheme(scheme.Scheme).WithInterceptorFuncs(
				fake.FailingListInterceptor[*submarinerv1.EndpointList]()).Build()
		})

		t.testFailure(t.run, "endpoints", "error")
	})

	When("listing of Pod resources fails", func() {
		BeforeEach(func() {
			fake.FailOnAction(&t.fakeProducer.KubeClient.(*k8sfake.Clientset).Fake, "pods", "list", errFake, false)
		})

		t.testFailure(t.run, "Pods", errFake.Error())
	})
})

type deploymentTestDriver struct {
	*testDriver
	imageOverrides []string
}

func newDeploymentTestDriver() *deploymentTestDriver {
	t := &deploymentTestDriver{testDriver: newTestDriver()}

	BeforeEach(func() {
		t.imageOverrides = []string{}
	})

	JustBeforeEach(func() {
		if t.submariner == nil {
			return
		}

		// Setup broker namespace for CIDR overlap checks.
		t.createNamespace(t.submariner.Spec.BrokerK8sRemoteNamespace)

		// Create healthy daemonsets.
		t.ensureDaemonSet(names.GatewayComponent, 1, 1)
		t.ensureDaemonSet(names.RouteAgentComponent, 3, 3)
		t.ensureDaemonSet(names.MetricsProxyComponent, 1, 1)

		// Create healthy gateway pod.
		t.ensurePodWithStatus(names.GatewayComponent, map[string]string{
			"gateway.submariner.io/status": string(submarinerv1.HAStatusActive),
			"gateway.submariner.io/node":   "node1",
		}, corev1.PodStatus{Phase: corev1.PodRunning})
	})

	return t
}

func (t *deploymentTestDriver) testMissingDaemonset(name string) {
	Context(fmt.Sprintf("and the %q daemonset doesn't exist", name), func() {
		JustBeforeEach(func() {
			Expect(t.fakeProducer.KubeClient.AppsV1().DaemonSets(constants.OperatorNamespace).Delete(
				context.Background(), name, metav1.DeleteOptions{})).To(Succeed())
		})

		t.testFailure(t.run, "Daemonset", "not found")
	})
}

func (t *deploymentTestDriver) testUnhealthyDaemonset(name string) {
	Context(fmt.Sprintf("and the %q daemonset has insufficient pods scheduled", name), func() {
		JustBeforeEach(func() {
			t.ensureDaemonSet(name, 2, 1)
		})

		t.testFailure(t.run, "number of", "DaemonSet")
	})
}

func (t *deploymentTestDriver) testMissingDeployment(name string) {
	Context(fmt.Sprintf("and the %q deployment doesn't exist", name), func() {
		JustBeforeEach(func() {
			Expect(t.fakeProducer.KubeClient.AppsV1().Deployments(constants.OperatorNamespace).Delete(
				context.Background(), name, metav1.DeleteOptions{})).To(Succeed())
		})

		t.testFailure(t.run, "Deployment", "not found")
	})
}

func (t *deploymentTestDriver) testUnhealthyDeployment(name string) {
	Context(fmt.Sprintf("and the %q deployment has insufficient replicas", name), func() {
		JustBeforeEach(func() {
			t.ensureDeployment(name, 2, 1)
		})

		t.testFailure(t.run, "number of", "Deployment")
	})
}

func (t *deploymentTestDriver) testUnhealthyPod(name string) {
	Context(fmt.Sprintf("and the %q pod is not running", name), func() {
		JustBeforeEach(func() {
			t.ensurePodWithStatus(name, nil, corev1.PodStatus{Phase: corev1.PodFailed})
		})

		t.testFailure(t.run, "Pod", "not running")
	})

	Context(fmt.Sprintf("and the %q pod has restarted excessively", name), func() {
		JustBeforeEach(func() {
			t.ensurePodWithStatus(name, nil, corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{
						RestartCount: 5,
					},
				},
			})
		})

		t.testSuccessWithWarning(t.run, "restarted")
	})
}

func (t *deploymentTestDriver) testMultipleNodes() {
	var (
		podStatus   corev1.PodStatus
		podsCreated []*corev1.Pod
	)

	BeforeEach(func() {
		podsCreated = nil
		podStatus = corev1.PodStatus{
			Phase: corev1.PodSucceeded,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					State: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{Message: "200 OK"},
					},
				},
			},
		}
	})

	JustBeforeEach(func() {
		t.createNode("worker1")

		t.fakeProducer.KubeClient.(*k8sfake.Clientset).Fake.PrependReactor("create", "pods",
			func(action k8stesting.Action) (bool, runtime.Object, error) {
				pod := action.(k8stesting.CreateAction).GetObject().(*corev1.Pod)
				podsCreated = append(podsCreated, pod)
				pod.Status = podStatus

				return false, nil, nil
			})
	})

	It("should check for gateway metrics accessibility", func() {
		t.assertSuccess(t.run)

		Expect(podsCreated).To(ContainElement(Satisfy(func(pod *corev1.Pod) bool {
			return len(pod.Spec.Containers) > 0 && slices.ContainsFunc(pod.Spec.Containers[0].Env, func(envVar corev1.EnvVar) bool {
				return envVar.Name == "COMMAND" && strings.Contains(envVar.Value, "submariner-gateway-metrics")
			})
		})))
	})

	Context("and the metrics test pod fails to create", func() {
		BeforeEach(func() {
			t.fakeProducer.KubeClient.(*k8sfake.Clientset).Fake.PrependReactor("create", "pods",
				func(action k8stesting.Action) (bool, runtime.Object, error) {
					pod := action.(k8stesting.CreateAction).GetObject().(*corev1.Pod)
					if len(pod.Spec.Containers) > 0 {
						return true, nil, errFake
					}

					return false, nil, nil
				})
		})

		t.testFailure(t.run, errFake.Error())
	})

	Context("and the metrics test pod fails to complete", func() {
		BeforeEach(func() {
			podStatus.Phase = corev1.PodRunning
		})

		t.testFailure(t.run, string(corev1.PodRunning))
	})

	Context("and the metrics test pod returns failure output", func() {
		BeforeEach(func() {
			podStatus.ContainerStatuses[0].State.Terminated.Message = "404"
		})

		t.testFailure(t.run, "404")
	})

	Context("and globalnet is enabled", func() {
		BeforeEach(func() {
			t.submariner.Spec.GlobalCIDR = "242.0.0.0/8"
		})

		JustBeforeEach(func() {
			t.ensureDaemonSet(names.GlobalnetComponent, 1, 1)
		})

		It("should check for globalnet metrics accessibility", func() {
			t.assertSuccess(t.run)

			Expect(podsCreated).To(ContainElement(Satisfy(func(pod *corev1.Pod) bool {
				return len(pod.Spec.Containers) > 0 && slices.ContainsFunc(pod.Spec.Containers[0].Env, func(envVar corev1.EnvVar) bool {
					return envVar.Name == "COMMAND" && strings.Contains(envVar.Value, "submariner-globalnet-metrics")
				})
			})))
		})
	})

	t.testImageRepositoryInfoFailure(func() {
		t.imageOverrides = []string{"invalid:image:override"}
	}, t.run)
}

func (t *deploymentTestDriver) run() error {
	return diagnose.Deployments(newClusterInfo(), "", t.imageOverrides, t.statusTracker)
}
