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

package cluster_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/submariner-io/admiral/pkg/fake"
	"github.com/submariner-io/admiral/pkg/names"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/pkg/cluster"
	"github.com/submariner-io/submariner-operator/api/v1alpha1"
	submarinerv1 "github.com/submariner-io/submariner/pkg/apis/submariner.io/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	controllerfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("Info", func() {
	Describe("NewInfo", testNewInfo)
	Describe("GetGateways", testGetGateways)
	Describe("GetRouteAgents", testGetRouteAgents)
	Describe("HasSingleNode", testHasSingleNode)
	Describe("GetLocalEndpoint", testGetLocalEndpoint)
	Describe("GetAnyRemoteEndpoint", testGetAnyRemoteEndpoint)
	Describe("GetImageRepositoryInfo", testGetImageRepositoryInfo)
	Describe("OperatorNamespace", testOperatorNamespace)
	Describe("GetClusters", testGetClusters)
	Describe("MergeImageOverrides", testMergeImageOverrides)
})

func testNewInfo() {
	t := newTestDriver()

	When("the Submariner and ServiceDiscovery resources exist", func() {
		It("should succeed", func(ctx SpecContext) {
			info, err := cluster.NewInfo(ctx, localCluster, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(info).NotTo(BeNil())
			Expect(info.Submariner).To(Equal(t.submariner))
			Expect(info.ServiceDiscovery).To(Equal(t.serviceDisc))
		})
	})

	When("neither Submariner nor ServiceDiscovery resources exist", func() {
		BeforeEach(func() {
			t.submariner = nil
			t.serviceDisc = nil
		})

		It("should succeed", func(ctx SpecContext) {
			info, err := cluster.NewInfo(ctx, localCluster, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(info).NotTo(BeNil())
			Expect(info.Submariner).To(BeNil())
			Expect(info.ServiceDiscovery).To(BeNil())
		})
	})

	When("Submariner resource retrieval fails", func() {
		BeforeEach(func() {
			t.clients.GeneralClient = controllerfake.NewClientBuilder().WithScheme(scheme.Scheme).WithInterceptorFuncs(
				fake.FailingGetInterceptor[*v1alpha1.Submariner]()).Build()
		})

		It("should return an error", func(ctx SpecContext) {
			_, err := cluster.NewInfo(ctx, localCluster, nil)
			Expect(err).To(HaveOccurred())
		})
	})

	When("ServiceDiscovery resource retrieval fails", func() {
		BeforeEach(func() {
			t.clients.GeneralClient = controllerfake.NewClientBuilder().WithScheme(scheme.Scheme).WithInterceptorFuncs(
				fake.FailingGetInterceptor[*v1alpha1.ServiceDiscovery]()).Build()
		})

		It("should return an error", func(ctx SpecContext) {
			_, err := cluster.NewInfo(ctx, localCluster, nil)
			Expect(err).To(HaveOccurred())
		})
	})

	When("Gateway resource retrieval fails", func() {
		BeforeEach(func() {
			t.clients.GeneralClient = controllerfake.NewClientBuilder().WithScheme(scheme.Scheme).WithInterceptorFuncs(
				fake.FailingListInterceptor[*submarinerv1.GatewayList]()).Build()
		})

		It("should return an error", func(ctx SpecContext) {
			_, err := cluster.NewInfo(ctx, localCluster, nil)
			Expect(err).To(HaveOccurred())
		})
	})
}

func testGetGateways() {
	t := newTestDriver()

	When("gateways exist", func() {
		BeforeEach(func(ctx SpecContext) {
			Expect(t.clients.ForGeneral().Create(ctx, &submarinerv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gateway1",
					Namespace: constants.OperatorNamespace,
				},
			})).To(Succeed())

			Expect(t.clients.ForGeneral().Create(ctx, &submarinerv1.Gateway{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "gateway2",
					Namespace: constants.OperatorNamespace,
				},
			})).To(Succeed())
		})

		It("should return all gateways", func(ctx SpecContext) {
			gateways, err := t.newInfo(ctx).GetGateways(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(gateways).To(HaveLen(2))
		})
	})

	When("no gateways exist", func() {
		It("should return an empty list", func(ctx SpecContext) {
			gateways, err := t.newInfo(ctx).GetGateways(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(gateways).To(BeEmpty())
		})
	})
}

func testGetRouteAgents() {
	t := newTestDriver()

	When("route agents exist", func() {
		BeforeEach(func(ctx SpecContext) {
			Expect(t.clients.ForGeneral().Create(ctx, &submarinerv1.RouteAgent{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "node1",
					Namespace: constants.OperatorNamespace,
				},
			})).To(Succeed())

			Expect(t.clients.ForGeneral().Create(ctx, &submarinerv1.RouteAgent{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "node2",
					Namespace: constants.OperatorNamespace,
				},
			})).To(Succeed())
		})

		It("should return all route agents", func(ctx SpecContext) {
			routeAgents, err := t.newInfo(ctx).GetRouteAgents()
			Expect(err).NotTo(HaveOccurred())
			Expect(routeAgents).To(HaveLen(2))
		})
	})

	When("no route agents exist", func() {
		It("should return empty list", func(ctx SpecContext) {
			routeAgents, err := t.newInfo(ctx).GetRouteAgents()
			Expect(err).NotTo(HaveOccurred())
			Expect(routeAgents).To(BeEmpty())
		})
	})

	When("resource retrieval fails", func() {
		BeforeEach(func() {
			t.clients.GeneralClient = controllerfake.NewClientBuilder().WithScheme(scheme.Scheme).WithInterceptorFuncs(
				fake.FailingListInterceptor[*submarinerv1.RouteAgentList]()).Build()
		})

		It("should return an error", func(ctx SpecContext) {
			_, err := t.newInfo(ctx).GetRouteAgents()
			Expect(err).To(HaveOccurred())
		})
	})
}

func testHasSingleNode() {
	t := newTestDriver()

	When("the cluster has a single node", func() {
		BeforeEach(func(ctx SpecContext) {
			t.createNode(ctx, "node1")
		})

		It("should return true", func(ctx SpecContext) {
			hasSingleNode, err := t.newInfo(ctx).HasSingleNode()
			Expect(err).NotTo(HaveOccurred())
			Expect(hasSingleNode).To(BeTrue())
		})
	})

	When("cluster has multiple nodes", func() {
		BeforeEach(func(ctx SpecContext) {
			t.createNode(ctx, "node1")
			t.createNode(ctx, "node2")
		})

		It("should return false", func(ctx SpecContext) {
			hasSingleNode, err := t.newInfo(ctx).HasSingleNode()
			Expect(err).NotTo(HaveOccurred())
			Expect(hasSingleNode).To(BeFalse())
		})
	})

	When("cluster has no nodes", func() {
		It("should return false", func(ctx SpecContext) {
			hasSingleNode, err := t.newInfo(ctx).HasSingleNode()
			Expect(err).NotTo(HaveOccurred())
			Expect(hasSingleNode).To(BeFalse())
		})
	})

	When("node retrieval fails", func() {
		BeforeEach(func() {
			fake.FailOnAction(&t.clients.ForKubernetes().(*k8sfake.Clientset).Fake, "nodes", "list", nil, false)
		})

		It("should return an error", func(ctx SpecContext) {
			_, err := t.newInfo(ctx).HasSingleNode()
			Expect(err).To(HaveOccurred())
		})
	})
}

func testGetLocalEndpoint() {
	t := newTestDriver()

	When("a local endpoint exists", func() {
		BeforeEach(func(ctx SpecContext) {
			t.createEndpoint(ctx, localCluster)
			t.createEndpoint(ctx, remoteCluster)
		})

		It("should return the local endpoint", func(ctx SpecContext) {
			endpoint, err := t.newInfo(ctx).GetLocalEndpoint()
			Expect(err).NotTo(HaveOccurred())
			Expect(endpoint).NotTo(BeNil())
			Expect(endpoint.Spec.ClusterID).To(Equal(localCluster))
		})
	})

	When("a local endpoint does not exist", func() {
		BeforeEach(func(ctx SpecContext) {
			t.createEndpoint(ctx, remoteCluster)
		})

		It("should return not found error", func(ctx SpecContext) {
			_, err := t.newInfo(ctx).GetLocalEndpoint()
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
		})
	})

	When("no endpoints exist", func() {
		It("should return not found error", func(ctx SpecContext) {
			_, err := t.newInfo(ctx).GetLocalEndpoint()
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
		})
	})

	When("endpoint retrieval fails", func() {
		BeforeEach(func() {
			t.clients.GeneralClient = controllerfake.NewClientBuilder().WithScheme(scheme.Scheme).WithInterceptorFuncs(
				fake.FailingListInterceptor[*submarinerv1.EndpointList]()).Build()
		})

		It("should return an error", func(ctx SpecContext) {
			_, err := t.newInfo(ctx).GetLocalEndpoint()
			Expect(err).To(HaveOccurred())
		})
	})
}

func testGetAnyRemoteEndpoint() {
	t := newTestDriver()

	When("remote endpoints exist", func() {
		BeforeEach(func(ctx SpecContext) {
			t.createEndpoint(ctx, localCluster)
			t.createEndpoint(ctx, remoteCluster)
		})

		It("should return a remote endpoint", func(ctx SpecContext) {
			endpoint, err := t.newInfo(ctx).GetAnyRemoteEndpoint()
			Expect(err).NotTo(HaveOccurred())
			Expect(endpoint.Spec.ClusterID).To(Equal(remoteCluster))
		})
	})

	When("no remote endpoints exist", func() {
		BeforeEach(func(ctx SpecContext) {
			t.createEndpoint(ctx, localCluster)
		})

		It("should return not found error", func(ctx SpecContext) {
			_, err := t.newInfo(ctx).GetAnyRemoteEndpoint()
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
		})
	})

	When("endpoint retrieval fails", func() {
		BeforeEach(func() {
			t.clients.GeneralClient = controllerfake.NewClientBuilder().WithScheme(scheme.Scheme).WithInterceptorFuncs(
				fake.FailingListInterceptor[*submarinerv1.EndpointList]()).Build()
		})

		It("should return an error", func(ctx SpecContext) {
			_, err := t.newInfo(ctx).GetAnyRemoteEndpoint()
			Expect(err).To(HaveOccurred())
		})
	})
}

func testGetImageRepositoryInfo() {
	t := newTestDriver()

	When("the Submariner resource exists", func() {
		BeforeEach(func() {
			t.submariner.Spec.Version = "1.0.0"
			t.submariner.Spec.ImageOverrides = map[string]string{
				names.GatewayComponent: "custom-gateway:latest",
			}
		})

		It("should return repository info from the Submariner spec", func(ctx SpecContext) {
			repoInfo, err := t.newInfo(ctx).GetImageRepositoryInfo()
			Expect(err).NotTo(HaveOccurred())
			Expect(repoInfo).NotTo(BeNil())
			Expect(repoInfo.Name).To(Equal(t.submariner.Spec.Repository))
			Expect(repoInfo.Version).To(Equal(t.submariner.Spec.Version))
			Expect(repoInfo.Overrides).To(Equal(t.submariner.Spec.ImageOverrides))
		})

		It("should merge local overrides with the Submariner spec overrides", func(ctx SpecContext) {
			overrideImage := "custom-routeagent:2.0.0"
			repoInfo, err := t.newInfo(ctx).GetImageRepositoryInfo(names.RouteAgentComponent + "=" + overrideImage)
			Expect(err).NotTo(HaveOccurred())
			Expect(repoInfo).NotTo(BeNil())
			Expect(repoInfo.Overrides).To(HaveKeyWithValue(names.GatewayComponent,
				t.submariner.Spec.ImageOverrides[names.GatewayComponent]))
			Expect(repoInfo.Overrides).To(HaveKeyWithValue(names.RouteAgentComponent, overrideImage))
		})

		It("should return an error for an invalid local override format", func(ctx SpecContext) {
			_, err := t.newInfo(ctx).GetImageRepositoryInfo("invalid-format")
			Expect(err).To(HaveOccurred())
		})
	})

	When("Submariner resource does not exist", func() {
		BeforeEach(func() {
			t.submariner = nil
		})

		It("should return default repository info", func(ctx SpecContext) {
			repoInfo, err := t.newInfo(ctx).GetImageRepositoryInfo()
			Expect(err).NotTo(HaveOccurred())
			Expect(repoInfo).NotTo(BeNil())
			Expect(repoInfo.Name).To(Equal(v1alpha1.DefaultRepo))
			Expect(repoInfo.Version).To(Equal(v1alpha1.DefaultSubmarinerOperatorVersion))
			Expect(repoInfo.Overrides).To(BeEmpty())
		})

		It("should return local overrides", func(ctx SpecContext) {
			overrideImage := "custom-gateway:latest"
			repoInfo, err := t.newInfo(ctx).GetImageRepositoryInfo(names.GatewayComponent + "=" + overrideImage)
			Expect(err).NotTo(HaveOccurred())
			Expect(repoInfo).NotTo(BeNil())
			Expect(repoInfo.Name).To(Equal(v1alpha1.DefaultRepo))
			Expect(repoInfo.Version).To(Equal(v1alpha1.DefaultSubmarinerOperatorVersion))
			Expect(repoInfo.Overrides).To(HaveKeyWithValue(names.GatewayComponent, overrideImage))
		})

		It("should return an error for invalid local override format", func(ctx SpecContext) {
			_, err := t.newInfo(ctx).GetImageRepositoryInfo("invalid-format")
			Expect(err).To(HaveOccurred())
		})
	})
}

func testOperatorNamespace() {
	t := newTestDriver()

	When("the Submariner resource exists", func() {
		It("should return its namespace", func(ctx SpecContext) {
			Expect(t.newInfo(ctx).OperatorNamespace()).To(Equal(t.submariner.Namespace))
		})
	})

	When("only the ServiceDiscovery resource exists", func() {
		BeforeEach(func() {
			t.submariner = nil
		})

		It("should return its namespace", func(ctx SpecContext) {
			Expect(t.newInfo(ctx).OperatorNamespace()).To(Equal(t.serviceDisc.Namespace))
		})
	})

	When("neither Submariner nor ServiceDiscovery exists", func() {
		BeforeEach(func() {
			t.submariner = nil
			t.serviceDisc = nil
		})

		It("should return default operator namespace", func(ctx SpecContext) {
			Expect(t.newInfo(ctx).OperatorNamespace()).To(Equal(constants.OperatorNamespace))
		})
	})
}

func testGetClusters() {
	t := newTestDriver()

	When("clusters exist", func() {
		BeforeEach(func(ctx SpecContext) {
			Expect(t.clients.ForGeneral().Create(ctx, &submarinerv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cluster1",
					Namespace: constants.OperatorNamespace,
				},
			})).To(Succeed())

			Expect(t.clients.ForGeneral().Create(ctx, &submarinerv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cluster2",
					Namespace: constants.OperatorNamespace,
				},
			})).To(Succeed())
		})

		It("should return all clusters", func(ctx SpecContext) {
			clusters, err := t.newInfo(ctx).GetClusters(constants.OperatorNamespace)
			Expect(err).NotTo(HaveOccurred())
			Expect(clusters).To(HaveLen(2))
		})
	})

	When("no clusters exist", func() {
		It("should return an empty list", func(ctx SpecContext) {
			clusters, err := t.newInfo(ctx).GetClusters(constants.OperatorNamespace)
			Expect(err).NotTo(HaveOccurred())
			Expect(clusters).To(BeEmpty())
		})
	})

	When("cluster retrieval fails", func() {
		BeforeEach(func() {
			t.clients.GeneralClient = controllerfake.NewClientBuilder().WithScheme(scheme.Scheme).WithInterceptorFuncs(
				fake.FailingListInterceptor[*submarinerv1.ClusterList]()).Build()
		})

		It("should return an error", func(ctx SpecContext) {
			_, err := t.newInfo(ctx).GetClusters(constants.OperatorNamespace)
			Expect(err).To(HaveOccurred())
		})
	})
}

func testMergeImageOverrides() {
	It("should merge overrides", func() {
		existing := map[string]string{
			names.GatewayComponent: "gateway:v1",
		}

		result, err := cluster.MergeImageOverrides(existing, []string{
			names.RouteAgentComponent + "=routeagent:v2",
			names.GlobalnetComponent + "=globalnet:v3",
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(HaveKeyWithValue(names.GatewayComponent, "gateway:v1"))
		Expect(result).To(HaveKeyWithValue(names.RouteAgentComponent, "routeagent:v2"))
		Expect(result).To(HaveKeyWithValue(names.GlobalnetComponent, "globalnet:v3"))
		Expect(result).To(HaveLen(3))
	})

	It("should override existing values", func() {
		existing := map[string]string{
			names.GatewayComponent: "gateway:v1",
		}

		result, err := cluster.MergeImageOverrides(existing, []string{
			names.GatewayComponent + "=gateway:v2",
		})

		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(HaveKeyWithValue(names.GatewayComponent, "gateway:v2"))
	})

	It("should support all valid components", func() {
		overrides := []string{
			names.OperatorComponent + "=operator:v1",
			names.GatewayComponent + "=gateway:v1",
			names.RouteAgentComponent + "=routeagent:v1",
			names.GlobalnetComponent + "=globalnet:v1",
			names.ServiceDiscoveryComponent + "=sd:v1",
			names.LighthouseCoreDNSComponent + "=coredns:v1",
			names.NettestComponent + "=nettest:v1",
			names.MetricsProxyComponent + "=metrics:v1",
		}

		result, err := cluster.MergeImageOverrides(nil, overrides)

		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(HaveLen(8))
	})

	It("should return error for an invalid format", func() {
		_, err := cluster.MergeImageOverrides(nil, []string{
			"invalid-no-equals-sign",
		})

		Expect(err).To(HaveOccurred())
	})

	It("should return error for an invalid component", func() {
		_, err := cluster.MergeImageOverrides(nil, []string{
			"invalid-component=image:v1",
		})

		Expect(err).To(HaveOccurred())
	})
}
