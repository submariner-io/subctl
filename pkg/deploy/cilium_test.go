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

package deploy_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/submariner-io/subctl/pkg/deploy"
	"github.com/submariner-io/submariner-operator/pkg/ciliumcm"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("SetCiliumClusterIdentity", func() {
	t := newTestDriver()

	BeforeEach(func(ctx SpecContext) {
		_, err := t.fakeProducer.KubeClient.CoreV1().ConfigMaps("kube-system").Create(ctx, &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: ciliumcm.CiliumConfigMapName, Namespace: "kube-system"},
			Data:       map[string]string{"cluster-id": "0", "cluster-name": "default"},
		}, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
	})

	It("should update cluster-id and cluster-name", func(ctx SpecContext) {
		Expect(deploy.SetCiliumClusterIdentity(ctx, t.fakeProducer.ForKubernetes(), "kube-system",
			"1", "test-cluster")).To(Succeed())

		cm, err := t.fakeProducer.KubeClient.CoreV1().ConfigMaps("kube-system").Get(ctx,
			ciliumcm.CiliumConfigMapName, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(cm.Data["cluster-id"]).To(Equal("1"))
		Expect(cm.Data["cluster-name"]).To(Equal("test-cluster"))
	})

	It("should reject an invalid identity", func(ctx SpecContext) {
		Expect(deploy.SetCiliumClusterIdentity(ctx, t.fakeProducer.ForKubernetes(), "kube-system",
			"0", "default")).NotTo(Succeed())
	})
})

var _ = Describe("RestartCiliumAgents", func() {
	t := newTestDriver()

	BeforeEach(func(ctx SpecContext) {
		_, err := t.fakeProducer.KubeClient.AppsV1().DaemonSets("kube-system").Create(ctx, &appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{Name: ciliumcm.CiliumDaemonSetName, Namespace: "kube-system"},
			Spec: appsv1.DaemonSetSpec{
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{}},
				},
			},
			Status: appsv1.DaemonSetStatus{
				DesiredNumberScheduled: 1,
				UpdatedNumberScheduled: 1,
				NumberAvailable:        1,
				NumberUnavailable:      0,
			},
		}, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
	})

	It("should annotate the Cilium DaemonSet for a rolling restart", func(ctx SpecContext) {
		Expect(deploy.RestartCiliumAgents(ctx, t.fakeProducer.ForKubernetes(), "kube-system",
			t.statusReporter)).To(Succeed())

		ds, err := t.fakeProducer.KubeClient.AppsV1().DaemonSets("kube-system").Get(ctx,
			ciliumcm.CiliumDaemonSetName, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())
		Expect(ds.Spec.Template.Annotations).To(HaveKey("kubectl.kubernetes.io/restartedAt"))
	})
})

var _ = Describe("FormatCiliumIdentityProblems", func() {
	It("should list problems without repeating the ConfigMap name", func() {
		msg := deploy.FormatCiliumIdentityProblems("0", "default")
		Expect(msg).To(ContainSubstring(`Problems found in ConfigMap "cilium-config"`))
		Expect(msg).To(ContainSubstring(`  - cluster-id is "0"`))
		Expect(msg).To(ContainSubstring(`  - cluster-name is "default"`))
		Expect(msg).NotTo(ContainSubstring("  - cilium-config cluster-id"))
	})
})
