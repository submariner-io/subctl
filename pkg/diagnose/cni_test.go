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
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/submariner-io/admiral/pkg/fake"
	"github.com/submariner-io/subctl/pkg/diagnose"
	submarinerv1 "github.com/submariner-io/submariner/pkg/apis/submariner.io/v1"
	"github.com/submariner-io/submariner/pkg/cni"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

var _ = Describe("CNIConfig", func() {
	t := cniTestDriver{testDriver: newTestDriver()}

	DescribeTableSubtree("",
		func(plugin string) {
			BeforeEach(func() {
				t.submariner.Status.NetworkPlugin = plugin
			})

			Specify("network plugin should succeed", func(ctx SpecContext) {
				t.assertSuccess(ctx, t.run)
			})
		},
		Entry("the Canal Flannel", cni.CanalFlannel),
		Entry("the WeaveNet", cni.WeaveNet),
		Entry("the OpenShiftSDN", cni.OpenShiftSDN),
		Entry("the KindNet", cni.KindNet),
	)

	When("the network plugin is Generic", func() {
		BeforeEach(func() {
			t.submariner.Status.NetworkPlugin = cni.Generic
		})

		t.testSuccessWithWarning(t.run, cni.Generic)
	})

	When("the network plugin is OVNKubernetes", t.testOVNKubernetesCNIPlugin)

	When("the network plugin is Calico", t.testCalicoCNIPlugin)

	When("the network plugin hasn't been determined", func() {
		BeforeEach(func() {
			t.submariner.Status.NetworkPlugin = ""
		})

		t.testSuccessWithWarning(t.run, "network plugin")
	})

	When("the network plugin is unsupported", func() {
		BeforeEach(func() {
			t.submariner.Status.NetworkPlugin = "unsupported-plugin"
		})

		t.testFailure(t.run)
	})

	When("the Submariner resource doesn't exist", func() {
		BeforeEach(func() {
			t.submariner = nil
		})

		It("should panic", func(ctx SpecContext) {
			Expect(func() {
				_ = diagnose.CNIConfig(ctx, newClusterInfo(ctx), "", t.statusTracker)
			}).To(Panic())
		})
	})
})

type cniTestDriver struct {
	*testDriver
}

func (t *cniTestDriver) testOVNKubernetesCNIPlugin() {
	BeforeEach(func() {
		t.submariner.Status.NetworkPlugin = cni.OVNKubernetes
	})

	JustBeforeEach(func() {
		t.fakeProducer.KubeClient = fake.WithRESTClient(t.fakeProducer.KubeClient.(*k8sfake.Clientset), nil)
	})

	Context("and the OVN version is supported", func() {
		BeforeEach(func(ctx SpecContext) {
			t.createOVNPod(ctx, diagnose.MinOVNNBVersion.String(), "nb-ovsdb")
		})

		t.testSuccess(t.run)
	})

	Context("and the OVN version is not supported", func() {
		BeforeEach(func(ctx SpecContext) {
			t.createOVNPod(ctx, "6.0.0", "nb-ovsdb")
		})

		t.testFailure(t.run, "supported version")
	})

	Context("and the OVN pod doesn't exist", func() {
		t.testFailure(t.run, "no pod found")
	})

	Context("and the OVN pod does not have the expected container", func() {
		BeforeEach(func(ctx SpecContext) {
			t.createOVNPod(ctx, diagnose.MinOVNNBVersion.String(), "unexpected")
		})

		t.testFailure(t.run, "container")
	})

	Context("and the output from the OVN pod command does not have the expected format", func() {
		BeforeEach(func(ctx SpecContext) {
			t.createOVNPod(ctx, diagnose.MinOVNNBVersion.String(), "nbdb")
			fake.SetSPDYExecutor("invalid", "", nil)
		})

		t.testFailure(t.run, "invalid")
	})

	Context("and the OVN pod command execution fails", func() {
		BeforeEach(func(ctx SpecContext) {
			t.createOVNPod(ctx, diagnose.MinOVNNBVersion.String(), "nbdb")
			fake.SetSPDYExecutor("", "", errors.New("mock error"))
		})

		t.testFailure(t.run, "mock error")
	})

	Context("listing of Pod resources fails", func() {
		BeforeEach(func() {
			fake.FailOnAction(&t.fakeProducer.KubeClient.(*k8sfake.Clientset).Fake, "pods", "list", nil, false)
		})

		t.testFailure(t.run, "error listing Pods")
	})
}

func (t *cniTestDriver) testCalicoCNIPlugin() {
	const (
		remoteSubnet = "192.168.0.0/16"
		localSubnet  = "10.0.0.0/16"
	)

	var gateway *submarinerv1.Gateway

	BeforeEach(func() {
		t.submariner.Status.NetworkPlugin = cni.Calico

		gateway = newGateway(submarinerv1.HAStatusActive, submarinerv1.Connection{
			Endpoint: submarinerv1.EndpointSpec{
				CableName: "remote-endpoint",
				Subnets:   []string{remoteSubnet},
			},
		})
		gateway.Status.LocalEndpoint = submarinerv1.EndpointSpec{
			Subnets: []string{localSubnet},
		}
	})

	JustBeforeEach(func() {
		if gateway != nil {
			t.createResource(gateway)
		}
	})

	Context("and IPPools are configured correctly", func() {
		BeforeEach(func(ctx SpecContext) {
			t.createCalicoIPPool(ctx, "remote-pool", remoteSubnet, true, false, "Always")
			t.createCalicoIPPool(ctx, "local-pool", localSubnet, false, true, "Always")
		})

		t.testSuccess(t.run)

		Context("and there's a passive gateway", func() {
			BeforeEach(func() {
				other := newGateway(submarinerv1.HAStatusPassive, submarinerv1.Connection{
					Endpoint: submarinerv1.EndpointSpec{
						CableName: "remote-endpoint2",
						Subnets:   []string{"172.123.0.0/16"},
					},
				})
				other.Name = "other"
				t.createResource(other)
			})

			It("should ignore it and succeed", func(ctx SpecContext) {
				t.assertSuccess(ctx, t.run)
			})
		})
	})

	Context("and no gateways are found", func() {
		BeforeEach(func() {
			gateway = nil
		})

		t.testFailure(t.run, "gateways")
	})

	Context("and no IPPools are found", func() {
		t.testFailure(t.run, "IPPools")
	})

	Context("and a remote subnet IPPool is not disabled", func() {
		BeforeEach(func(ctx SpecContext) {
			t.createCalicoIPPool(ctx, "invalid-pool", remoteSubnet, false, false, "")
		})

		t.testFailure(t.run, "invalid-pool")
	})

	Context("and a remote subnet IPPool has natOutgoing enabled", func() {
		BeforeEach(func(ctx SpecContext) {
			t.createCalicoIPPool(ctx, "invalid-pool", remoteSubnet, true, true, "")
		})

		t.testFailure(t.run, "natOutgoing")
	})

	Context("and an IPPool has no CIDR set", func() {
		BeforeEach(func(ctx SpecContext) {
			t.createCalicoIPPool(ctx, "invalid-pool", "", true, true, "")
		})

		t.testFailure(t.run, "IPPool")
	})

	Context("and a local IPPool has incorrect vxlanMode", func() {
		BeforeEach(func(ctx SpecContext) {
			t.createCalicoIPPool(ctx, "local-pool", localSubnet, false, true, "Never")
		})

		t.testFailure(t.run, "vxlanMode")
	})

	Context("listing of IPPool resources fails", func() {
		BeforeEach(func() {
			fake.FailOnAction(&t.fakeProducer.DynamicClient.(*dynamicfake.FakeDynamicClient).Fake, diagnose.CalicoGVR.Resource,
				"list", nil, false)
		})

		t.testFailure(t.run, "IPPools")
	})
}

func (t *cniTestDriver) createCalicoIPPool(ctx context.Context, name, cidr string, disabled, natOutgoing bool, vxlanMode string) {
	pool := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": diagnose.CalicoGVR.GroupVersion().String(),
			"kind":       "IPPool",
			"metadata": map[string]any{
				"name": name,
			},
			"spec": map[string]any{
				"cidr":        cidr,
				"disabled":    disabled,
				"natOutgoing": natOutgoing,
				"vxlanMode":   vxlanMode,
			},
		},
	}

	_, err := t.fakeProducer.DynamicClient.Resource(diagnose.CalicoGVR).Create(ctx, pool, metav1.CreateOptions{})
	Expect(err).NotTo(HaveOccurred())
}

func (t *cniTestDriver) createOVNPod(ctx context.Context, version, containerName string) {
	// Create OVN pod with proper labels and container
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ovnkube-db-test",
			Namespace: "ovn-kubernetes",
			Labels: map[string]string{
				"ovn-db-pod": "true",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: containerName,
				},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}

	_, err := t.fakeProducer.KubeClient.CoreV1().Pods(pod.Namespace).Create(ctx, pod, metav1.CreateOptions{})
	Expect(err).NotTo(HaveOccurred())

	fake.SetSPDYExecutor("DB Schema "+version, "", nil)
}

func (t *cniTestDriver) run(ctx context.Context) error {
	return diagnose.CNIConfig(ctx, newClusterInfo(ctx), "", t.statusTracker)
}
