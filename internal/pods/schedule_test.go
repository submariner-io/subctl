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

package pods_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/submariner-io/admiral/pkg/fake"
	"github.com/submariner-io/shipyard/test/e2e/framework"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/internal/pods"
	"github.com/submariner-io/subctl/pkg/image"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/utils/ptr"
)

const podTerminatedMsg = "pod terminated"

var _ = Describe("", func() {
	Describe("Schedule", testSchedule)
	Describe("Scheduled", func() {
		Describe("AwaitCompletion", testAwaitCompletion)
		Describe("Delete", testDelete)
	})
})

func testSchedule() {
	t := newScheduleTestDriver()

	Context("with defaults", func() {
		It("should create a pod to be scheduled on a gateway node", func(ctx SpecContext) {
			scheduled, err := pods.Schedule(ctx, t.config)
			Expect(err).NotTo(HaveOccurred())
			Expect(scheduled).NotTo(BeNil())

			pod, err := t.k8sClient.CoreV1().Pods(constants.OperatorNamespace).Get(ctx, scheduled.Pod.Name, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(pod).To(Equal(scheduled.Pod))

			Expect(scheduled.Pod.Spec.HostNetwork).To(BeFalse())
			Expect(scheduled.Pod.Spec.Containers).To(HaveLen(1))
			container := &scheduled.Pod.Spec.Containers[0]
			Expect(container.Image).To(HavePrefix(t.config.ImageRepositoryInfo.Name))
			Expect(scheduled.Pod.Spec.Containers[0].Env).To(ContainElement(Satisfy(func(envVar corev1.EnvVar) bool {
				return envVar.Name == "COMMAND" && envVar.Value == t.config.Command
			})))
			Expect(container.SecurityContext).To(BeNil())

			assertGatewayNodeAffinity(scheduled.Pod.Spec.Affinity, corev1.NodeSelectorOpIn)
		})
	})

	Context("with CustomNode scheduling", func() {
		BeforeEach(func() {
			t.config.Scheduling.ScheduleOn = pods.CustomNode
		})

		It("should create a pod for the specified NodeName", func(ctx SpecContext) {
			t.config.Scheduling.NodeName = "test-node"

			scheduled, err := pods.Schedule(ctx, t.config)
			Expect(err).NotTo(HaveOccurred())
			Expect(scheduled).NotTo(BeNil())
			Expect(scheduled.Pod.Spec.NodeName).To(Equal(t.config.Scheduling.NodeName))
		})

		It("should return an error when NodeName is missing", func(ctx SpecContext) {
			_, err := pods.Schedule(ctx, t.config)
			Expect(err).To(HaveOccurred())
		})
	})

	Context("with HostNetworking", func() {
		It("should create a pod with host network and security context", func(ctx SpecContext) {
			t.config.Scheduling.Networking = pods.HostNetworking

			scheduled, err := pods.Schedule(ctx, t.config)
			Expect(err).NotTo(HaveOccurred())
			Expect(scheduled).NotTo(BeNil())
			Expect(scheduled.Pod.Spec.HostNetwork).To(BeTrue())

			Expect(scheduled.Pod.Spec.Containers).To(HaveLen(1))
			securityContext := scheduled.Pod.Spec.Containers[0].SecurityContext
			Expect(securityContext).NotTo(BeNil())
			Expect(ptr.Deref(securityContext.Privileged, false)).To(BeTrue())
			Expect(ptr.Deref(securityContext.RunAsUser, -1)).To(Equal(int64(0)))
			Expect(securityContext.Capabilities).NotTo(BeNil())
			Expect(securityContext.Capabilities.Add).To(ContainElements(
				corev1.Capability("NET_ADMIN"),
				corev1.Capability("NET_RAW"),
			))
			Expect(securityContext.Capabilities.Drop).To(ContainElement(corev1.Capability("all")))
		})
	})

	Context("with a custom namespace", func() {
		BeforeEach(func(ctx SpecContext) {
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "custom-namespace",
					Labels: map[string]string{
						"pod-security.kubernetes.io/enforce": "privileged",
					},
				},
			}
			_, err := t.k8sClient.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			t.config.Namespace = ns.Name
		})

		It("should create the pod in the namespace", func(ctx SpecContext) {
			scheduled, err := pods.Schedule(ctx, t.config)
			Expect(err).NotTo(HaveOccurred())
			Expect(scheduled).NotTo(BeNil())

			_, err = t.k8sClient.CoreV1().Pods(t.config.Namespace).Get(ctx, scheduled.Pod.Name, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
		})

		Context("and namespace retrieval fails", func() {
			BeforeEach(func() {
				fake.FailOnAction(&t.k8sClient.Fake, "namespaces", "get", nil, false)
			})

			It("should return an error", func(ctx SpecContext) {
				_, err := pods.Schedule(ctx, t.config)
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Context("with NonGatewayNode scheduling", func() {
		It("should create a pod with non-gateway node affinity", func(ctx SpecContext) {
			t.config.Scheduling.ScheduleOn = pods.NonGatewayNode

			scheduled, err := pods.Schedule(ctx, t.config)
			Expect(err).NotTo(HaveOccurred())
			Expect(scheduled).NotTo(BeNil())

			assertGatewayNodeAffinity(scheduled.Pod.Spec.Affinity, corev1.NodeSelectorOpNotIn)
		})
	})

	When("the created pod is not scheduled", func() {
		BeforeEach(func() {
			t.podPhase = corev1.PodPending
		})

		It("should return an error", func(ctx SpecContext) {
			_, err := pods.Schedule(ctx, t.config)
			Expect(err).To(HaveOccurred())

			list, err := t.k8sClient.CoreV1().Pods(t.config.Namespace).List(ctx, metav1.ListOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(list.Items).To(BeEmpty())
		})
	})

	When("the created pod fails", func() {
		BeforeEach(func() {
			t.podPhase = corev1.PodFailed
		})

		It("should return an error", func(ctx SpecContext) {
			_, err := pods.Schedule(ctx, t.config)
			Expect(err).To(HaveOccurred())
		})
	})

	Context("pod creation fails", func() {
		BeforeEach(func() {
			fake.FailOnAction(&t.k8sClient.Fake, "pods", "create", nil, false)
		})

		It("should return an error", func(ctx SpecContext) {
			_, err := pods.Schedule(ctx, t.config)
			Expect(err).To(HaveOccurred())
		})
	})
}

func testAwaitCompletion() {
	t := newScheduledTestDriver()

	DescribeTableSubtree("",
		func(phase corev1.PodPhase) {
			BeforeEach(func() {
				t.podPhase = phase
			})

			It("should capture the pod output and delete the pod", func(ctx SpecContext) {
				Expect(t.scheduled.AwaitCompletion(ctx)).To(Succeed())
				Expect(t.scheduled.PodOutput).To(Equal(podTerminatedMsg))
			})
		},
		Entry("when the pod succeeds", corev1.PodSucceeded),
		Entry("when the pod fails", corev1.PodFailed),
	)

	When("the pod doesn't complete", func() {
		BeforeEach(func() {
			t.podPhase = corev1.PodRunning
		})

		It("should fail", func(ctx SpecContext) {
			Expect(t.scheduled.AwaitCompletion(ctx)).NotTo(Succeed())
		})
	})
}

func testDelete() {
	t := newScheduledTestDriver()

	It("should delete the pod", func(ctx SpecContext) {
		t.scheduled.Delete(ctx)

		_, err := t.k8sClient.CoreV1().Pods(t.scheduled.Pod.Namespace).Get(ctx, t.scheduled.Pod.Name, metav1.GetOptions{})
		Expect(err).To(Satisfy(apierrors.IsNotFound))
	})
}

type scheduleTestDriver struct {
	k8sClient *k8sfake.Clientset
	config    *pods.Config
	podPhase  corev1.PodPhase
}

func newScheduleTestDriver() *scheduleTestDriver {
	t := &scheduleTestDriver{}

	BeforeEach(func() {
		t.podPhase = corev1.PodRunning
		t.k8sClient = k8sfake.NewClientset()
		fake.AddBasicReactors(&t.k8sClient.Fake)

		t.config = &pods.Config{
			Name:                "test-pod",
			ClientSet:           t.k8sClient,
			Command:             "echo test",
			Timeout:             30,
			ImageRepositoryInfo: *image.NewRepositoryInfo("quay.io/submariner", "latest", nil),
		}
	})

	JustBeforeEach(func() {
		t.k8sClient.PrependReactor("create", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
			pod := action.(k8stesting.CreateAction).GetObject().(*corev1.Pod)
			pod.Status.Phase = t.podPhase

			return false, pod, nil
		})
	})

	return t
}

func assertGatewayNodeAffinity(affinity *corev1.Affinity, op corev1.NodeSelectorOperator) {
	Expect(affinity).NotTo(BeNil())
	Expect(affinity.NodeAffinity).NotTo(BeNil())
	Expect(affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution).NotTo(BeNil())

	terms := affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms
	Expect(terms).To(ContainElement(Satisfy(func(t corev1.NodeSelectorTerm) bool {
		Expect(t.MatchExpressions).To(HaveLen(1))
		e := t.MatchExpressions[0]

		return e.Key == framework.GatewayLabel && e.Operator == op && e.Values[0] == "true"
	})))
}

type scheduledTestDriver struct {
	*scheduleTestDriver
	scheduled *pods.Scheduled
}

func newScheduledTestDriver() *scheduledTestDriver {
	t := &scheduledTestDriver{scheduleTestDriver: newScheduleTestDriver()}

	BeforeEach(func() {
		t.podPhase = corev1.PodSucceeded
	})

	JustBeforeEach(func(ctx SpecContext) {
		t.scheduled = &pods.Scheduled{
			Config: t.config,
			Pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: t.config.Namespace,
				},
			},
		}

		pod := t.scheduled.Pod.DeepCopy()
		pod.Status = corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{
				{
					State: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							Message: podTerminatedMsg,
						},
					},
				},
			},
		}

		_, err := t.k8sClient.CoreV1().Pods(t.scheduled.Pod.Namespace).Create(ctx, pod, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
	})

	return t
}
