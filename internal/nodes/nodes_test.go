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

package nodes_test

import (
	"context"
	"strconv"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/submariner-io/admiral/pkg/fake"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/internal/nodes"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

const (
	workerLabel = "node-role.kubernetes.io/worker"
	masterLabel = "node-role.kubernetes.io/master"
)

var (
	_ = Describe("LabelAsGateway", testLabelAsGateway)
	_ = Describe("GetAllWorkerNames", testGetAllWorkerNames)
	_ = Describe("ListGateways", testListGateways)
)

func testLabelAsGateway() {
	const (
		nodeName      = "test-node"
		existingLabel = "existing-label"
		existingValue = "existing-value"
	)

	t := newTestDriver()

	BeforeEach(func(ctx SpecContext) {
		t.createNode(ctx, nodeName, map[string]string{existingLabel: existingValue})
	})

	When("labeling a node as a gateway", func() {
		It("should add the gateway label", func(ctx SpecContext) {
			err := nodes.LabelAsGateway(ctx, t.k8sClient, nodeName)
			Expect(err).NotTo(HaveOccurred())

			node, err := t.k8sClient.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(node.Labels).To(HaveKeyWithValue(constants.SubmarinerGatewayLabel, constants.TrueLabel))
			Expect(node.Labels).To(HaveKeyWithValue(existingLabel, existingValue))
		})
	})

	When("the node does not exist", func() {
		It("should return an error", func(ctx SpecContext) {
			err := nodes.LabelAsGateway(ctx, t.k8sClient, "nonexistent-node")
			Expect(err).To(HaveOccurred())
		})
	})

	When("there is a conflict during patching", func() {
		It("should retry and succeed", func(ctx SpecContext) {
			fake.FailOnAction(&t.k8sClient.Fake, "nodes", "patch",
				apierrors.NewConflict(corev1.Resource("nodes"), nodeName, nil), true)

			err := nodes.LabelAsGateway(ctx, t.k8sClient, nodeName)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	When("patching fails with a non-conflict error", func() {
		It("should return the error", func(ctx SpecContext) {
			fake.FailOnAction(&t.k8sClient.Fake, "nodes", "patch", nil, false)

			err := nodes.LabelAsGateway(ctx, t.k8sClient, nodeName)
			Expect(err).To(HaveOccurred())
		})
	})
}

func testGetAllWorkerNames() {
	t := newTestDriver()

	When("worker nodes exist", func() {
		It("should return all worker node names", func(ctx SpecContext) {
			t.createNode(ctx, "worker1", map[string]string{workerLabel: ""})
			t.createNode(ctx, "worker2", map[string]string{workerLabel: ""})
			t.createNode(ctx, "master", map[string]string{masterLabel: ""})

			workers, err := nodes.GetAllWorkerNames(ctx, t.k8sClient)
			Expect(err).NotTo(HaveOccurred())
			Expect(workers).To(ConsistOf("worker1", "worker2"))
		})
	})

	When("no worker nodes exist but non-master nodes exist", func() {
		It("should return non-master node names", func(ctx SpecContext) {
			t.createNode(ctx, "master", map[string]string{masterLabel: ""})
			t.createNode(ctx, "non-master1", nil)
			t.createNode(ctx, "non-master2", nil)

			workers, err := nodes.GetAllWorkerNames(ctx, t.k8sClient)
			Expect(err).NotTo(HaveOccurred())
			Expect(workers).To(ConsistOf("non-master1", "non-master2"))
		})
	})

	When("only master nodes exist", func() {
		It("should return an empty list", func(ctx SpecContext) {
			t.createNode(ctx, "master1", map[string]string{masterLabel: ""})
			t.createNode(ctx, "master2", map[string]string{masterLabel: ""})

			workers, err := nodes.GetAllWorkerNames(ctx, t.k8sClient)
			Expect(err).NotTo(HaveOccurred())
			Expect(workers).To(BeEmpty())
		})
	})

	When("no nodes exist", func() {
		It("should return an empty list", func(ctx SpecContext) {
			workers, err := nodes.GetAllWorkerNames(ctx, t.k8sClient)
			Expect(err).NotTo(HaveOccurred())
			Expect(workers).To(BeEmpty())
		})
	})

	When("listing nodes fails", func() {
		It("should return the error", func(ctx SpecContext) {
			fake.FailOnAction(&t.k8sClient.Fake, "nodes", "list", nil, false)

			_, err := nodes.GetAllWorkerNames(ctx, t.k8sClient)
			Expect(err).To(HaveOccurred())
		})
	})
}

func testListGateways() {
	t := newTestDriver()

	When("gateway nodes exist", func() {
		It("should return all gateway node names", func(ctx SpecContext) {
			t.createNode(ctx, "gateway1", map[string]string{constants.SubmarinerGatewayLabel: constants.TrueLabel})
			t.createNode(ctx, "gateway2", map[string]string{constants.SubmarinerGatewayLabel: constants.TrueLabel})
			t.createNode(ctx, "non-gateway", map[string]string{constants.SubmarinerGatewayLabel: strconv.FormatBool(false)})
			t.createNode(ctx, "worker1", map[string]string{workerLabel: ""})

			gateways, err := nodes.ListGateways(ctx, t.k8sClient)
			Expect(err).NotTo(HaveOccurred())
			Expect(gateways).To(ConsistOf("gateway1", "gateway2"))
		})
	})

	When("no gateway nodes exist", func() {
		It("should return an empty list", func(ctx SpecContext) {
			t.createNode(ctx, "worker1", map[string]string{workerLabel: ""})
			t.createNode(ctx, "worker2", map[string]string{workerLabel: ""})

			gateways, err := nodes.ListGateways(ctx, t.k8sClient)
			Expect(err).NotTo(HaveOccurred())
			Expect(gateways).To(BeEmpty())
		})
	})

	When("listing nodes fails", func() {
		It("should return the error", func(ctx SpecContext) {
			fake.FailOnAction(&t.k8sClient.Fake, "nodes", "list", nil, false)

			_, err := nodes.ListGateways(ctx, t.k8sClient)
			Expect(err).To(HaveOccurred())
		})
	})
}

type testDriver struct {
	k8sClient *k8sfake.Clientset
}

func newTestDriver() *testDriver {
	t := &testDriver{}

	BeforeEach(func() {
		t.k8sClient = k8sfake.NewClientset()
		fake.AddBasicReactors(&t.k8sClient.Fake)
	})

	return t
}

func (t *testDriver) createNode(ctx context.Context, name string, labels map[string]string) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},
	}
	_, err := t.k8sClient.CoreV1().Nodes().Create(ctx, node, metav1.CreateOptions{})
	Expect(err).NotTo(HaveOccurred())
}
