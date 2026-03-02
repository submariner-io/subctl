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
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"github.com/submariner-io/admiral/pkg/fake"
	"github.com/submariner-io/admiral/pkg/names"
	"github.com/submariner-io/subctl/internal/show"
	"github.com/submariner-io/subctl/pkg/client"
)

var _ = Describe("Versions", func() {
	const (
		gatewayRunningVersion    = "v0.23.1"
		routeAgentRunningVersion = "v0.23.2"
		amdArch                  = "amd64"
	)

	t := newTestDriver()

	JustBeforeEach(func(ctx SpecContext) {
		node := t.createNode(ctx, "test-node", amdArch)

		ds := t.createDaemonSet(ctx, names.GatewayComponent)
		t.createPod(ctx, names.GatewayComponent, node.Name, ds.Spec.Selector.MatchLabels)

		ds = t.createDaemonSet(ctx, names.RouteAgentComponent)
		t.createPod(ctx, names.RouteAgentComponent, "", ds.Spec.Selector.MatchLabels)

		t.createDeployment(ctx, names.OperatorComponent)

		dep := t.createDeployment(ctx, names.ServiceDiscoveryComponent)
		t.createPod(ctx, names.ServiceDiscoveryComponent, "", dep.Spec.Selector.MatchLabels)

		fake.SetSPDYExecutors(fake.SPDYExecutor{
			Stderr:     names.GatewayComponent + " version: " + gatewayRunningVersion,
			URLMatcher: And(ContainSubstring(names.GatewayComponent), ContainSubstring("&command=--version")),
		})

		cp := t.clusterInfo.ClientProducer.(*client.DefaultProducer)
		cp.KubeClient = fake.WrapClientWithPodLogs(cp.KubeClient, map[string]string{
			names.RouteAgentComponent: names.RouteAgentComponent + " version: " + routeAgentRunningVersion,
		})
	})

	doVersions := func(ctx context.Context) error {
		return show.Versions(ctx, t.clusterInfo, "", t.status)
	}

	componentMatcher := func(name, runningVersion, arch string) types.GomegaMatcher {
		return tableRowMatcher(append([]string{name, testRepository, testVersion}, runningVersion, arch)...)
	}

	It("should display version information", func(ctx SpecContext) {
		Expect(doVersions(ctx)).To(Succeed())

		t.assertTableOutput(
			componentMatcher(names.GatewayComponent, gatewayRunningVersion, amdArch),
			componentMatcher(names.RouteAgentComponent, routeAgentRunningVersion, ""),
			componentMatcher(names.OperatorComponent, "", ""),
			componentMatcher(names.ServiceDiscoveryComponent, show.Unavailable, show.Unavailable))
	})

	When("no components exist", func() {
		It("should not return an error", func(ctx SpecContext) {
			Expect(doVersions(ctx)).To(Succeed())
		})
	})

	When("DaemonSet retrieval fails", func() {
		BeforeEach(func() {
			fake.FailOnAction(&t.k8sClient.Fake, "daemonsets", "get", nil, false)
		})

		It("should return an error", func(ctx SpecContext) {
			Expect(doVersions(ctx)).To(HaveOccurred())
		})
	})

	When("Deployment retrieval fails", func() {
		BeforeEach(func() {
			fake.FailOnAction(&t.k8sClient.Fake, "deployments", "get", nil, false)
		})

		It("should return an error", func(ctx SpecContext) {
			Expect(doVersions(ctx)).To(HaveOccurred())
		})
	})

	When("listing pods fails", func() {
		BeforeEach(func() {
			fake.FailOnAction(&t.k8sClient.Fake, "pods", "list", nil, false)
		})

		It("should return an error", func(ctx SpecContext) {
			Expect(doVersions(ctx)).To(HaveOccurred())
		})
	})

	When("Node retrieval fails", func() {
		BeforeEach(func() {
			fake.FailOnAction(&t.k8sClient.Fake, "nodes", "get", nil, false)
		})

		It("should return an error about node retrieval", func(ctx SpecContext) {
			Expect(doVersions(ctx)).To(HaveOccurred())
		})
	})
})
