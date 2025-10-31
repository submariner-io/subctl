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
	. "github.com/onsi/ginkgo/v2"
	"github.com/submariner-io/admiral/pkg/fake"
	"github.com/submariner-io/subctl/pkg/diagnose"
	"github.com/submariner-io/submariner/pkg/cni"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

var _ = Describe("KubeProxyMode", func() {
	t := kubeProxyTestDriver{testDriver: newTestDriver()}

	Context("when the network plugin is OVNKubernetes", func() {
		BeforeEach(func() {
			t.submariner.Status.NetworkPlugin = cni.OVNKubernetes
		})

		It("should succeed without checking kube-proxy mode", func() {
			t.assertSuccess(t.run)
		})
	})

	When("pod scheduling succeeds", func() {
		var podTerminatedMsg string

		JustBeforeEach(func() {
			t.fakeProducer.KubeClient.(*k8sfake.Clientset).Fake.PrependReactor("create", "pods",
				func(action k8stesting.Action) (bool, runtime.Object, error) {
					action.(k8stesting.CreateAction).GetObject().(*corev1.Pod).Status = corev1.PodStatus{
						Phase: corev1.PodSucceeded,
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

					return false, nil, nil
				})
		})

		Context("and kube-proxy is not deployed with ipvs mode (output indicated missing interface)", func() {
			BeforeEach(func() {
				podTerminatedMsg = diagnose.KubeProxyMissingInterface
			})

			t.testSuccess(t.run)
		})

		Context("and kube-proxy is not deployed with ipvs mode (output indicated device does not exist)", func() {
			BeforeEach(func() {
				podTerminatedMsg = diagnose.KubeProxyNotEnabled
			})

			t.testSuccess(t.run)
		})

		Context("and kube-proxy is deployed with ipvs mode", func() {
			BeforeEach(func() {
				podTerminatedMsg = "23: kube-ipvs0: <BROADCAST,NOARP> mtu 1500 qdisc noop state DOWN group default\nlink/" +
					"ether 00:00:00:00:00:00 brd ff:ff:ff:ff:ff:ff"
			})

			t.testFailure(t.run, "ipvs", "support")
		})
	})

	When("pod creation fails", func() {
		BeforeEach(func() {
			fake.FailOnAction(&t.fakeProducer.KubeClient.(*k8sfake.Clientset).Fake, "pods", "create", errFake, false)
		})

		t.testFailure(t.run, errFake.Error())
	})
})

type kubeProxyTestDriver struct {
	*testDriver
}

func (t *kubeProxyTestDriver) run() error {
	return diagnose.KubeProxyMode(newClusterInfo(), "", []string{}, t.statusTracker)
}
