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

package gather_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/submariner-io/admiral/pkg/fake"
	"github.com/submariner-io/subctl/internal/gather"
	"github.com/submariner-io/subctl/pkg/client"
	"github.com/submariner-io/subctl/pkg/cluster"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

var _ = Describe("Cluster summary", func() {
	const clusterName = "test-cluster"

	var (
		k8sClient   *k8sfake.Clientset
		clusterInfo *cluster.Info
		options     gather.Options
	)

	BeforeEach(func() {
		k8sClient = k8sfake.NewClientset(
			newNode("gateway-node", map[string]string{"submariner.io/gateway": "true"}),
			newNode("master-node", map[string]string{"node-role.kubernetes.io/master": ""}),
			newNode("worker-node", nil),
		)

		clusterInfo = &cluster.Info{
			Name:           clusterName,
			ClientProducer: &client.DefaultProducer{KubeClient: k8sClient},
		}

		options = gather.Options{Directory: GinkgoT().TempDir()}
	})

	readSummary := func() string {
		summary, err := os.ReadFile(filepath.Join(options.Directory, clusterName, "summary.html"))
		Expect(err).NotTo(HaveOccurred())

		return string(summary)
	}

	It("should report the number of nodes", func(ctx SpecContext) {
		Expect(gather.Data(ctx, clusterInfo, options)).To(Succeed())

		summary := readSummary()
		Expect(summary).To(MatchRegexp(`Total node\(s\)</td>\s*<td>3</td>`))
		Expect(summary).To(MatchRegexp(`Master node\(s\)</td>\s*<td>1</td>`))
		Expect(summary).To(MatchRegexp(`Gateway node\(s\)</td>\s*<td>1</td>`))
	})

	When("the nodes can't be listed", func() {
		BeforeEach(func() {
			fake.FailOnAction(&k8sClient.Fake, "nodes", "list", nil, false)
		})

		It("should still write the summary", func(ctx SpecContext) {
			Expect(gather.Data(ctx, clusterInfo, options)).To(Succeed())

			summary := readSummary()
			Expect(summary).To(MatchRegexp(`Total node\(s\)</td>\s*<td>0</td>`))
			Expect(summary).To(MatchRegexp(`Master node\(s\)</td>\s*<td>0</td>`))
			Expect(summary).To(MatchRegexp(`Gateway node\(s\)</td>\s*<td>0</td>`))
		})
	})
})

func newNode(name string, labels map[string]string) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},
	}
}
