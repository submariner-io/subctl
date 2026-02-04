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
	"github.com/submariner-io/admiral/pkg/fake"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/internal/show"
	"github.com/submariner-io/subctl/pkg/client"
	submarinerv1 "github.com/submariner-io/submariner/pkg/apis/submariner.io/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	controllerfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("Endpoints", func() {
	const (
		localClusterID   = "local-cluster"
		localPrivateIPv4 = "10.0.1.5"
		localPrivateIPv6 = "fd00::5"
		localPublicIPv4  = "203.0.113.5"
		localPublicIPv6  = "2001:db8::5"
		localSubnetIPv4  = "10.0.1.0/24"
		localSubnetIPv6  = "fd00::/64"
		localCableDriver = "libreswan"
	)

	remoteEndpointSpec1 := submarinerv1.EndpointSpec{
		ClusterID:  "remote-cluster1",
		PrivateIPs: []string{"10.0.2.10"},
		PublicIPs:  []string{"204.0.113.10"},
		Backend:    "wireguard",
		Subnets:    []string{"10.0.2.0/24"},
	}

	remoteEndpointSpec2 := submarinerv1.EndpointSpec{
		ClusterID:  "remote-cluster2",
		PrivateIPs: []string{"10.0.3.10"},
		PublicIPs:  []string{"205.0.113.10"},
		Backend:    "vxlan",
		Subnets:    []string{"10.0.3.0/24"},
	}

	t := newTestDriver()

	doEndpoints := func(ctx context.Context) error {
		return show.Endpoints(ctx, t.clusterInfo, "", t.status)
	}

	It("should display endpoint information", func(ctx SpecContext) {
		gateway := &submarinerv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "gateway-" + localClusterID,
				Namespace: constants.OperatorNamespace,
			},
			Status: submarinerv1.GatewayStatus{
				LocalEndpoint: submarinerv1.EndpointSpec{
					ClusterID:  localClusterID,
					PrivateIPs: []string{localPrivateIPv4, localPrivateIPv6},
					PublicIPs:  []string{localPublicIPv4, localPublicIPv6},
					Backend:    localCableDriver,
					Subnets:    []string{localSubnetIPv4, localSubnetIPv6},
				},
				Connections: []submarinerv1.Connection{
					{
						Endpoint: remoteEndpointSpec1,
						UsingIP:  remoteEndpointSpec1.PrivateIPs[0],
					},
					{
						Endpoint: remoteEndpointSpec2,
						UsingIP:  remoteEndpointSpec2.PrivateIPs[0],
					},
				},
			},
		}

		Expect(t.clusterInfo.ClientProducer.ForGeneral().Create(ctx, gateway)).To(Succeed())

		Expect(doEndpoints(ctx)).To(Succeed())

		t.assertTableOutput(
			tableRowMatcher(localClusterID, localPrivateIPv4, localPublicIPv4, localCableDriver, "local"),
			tableRowMatcher(localClusterID, localPrivateIPv6, localPublicIPv6, localCableDriver, "local"),
			tableRowMatcher(remoteEndpointSpec1.ClusterID, remoteEndpointSpec1.PrivateIPs[0], remoteEndpointSpec1.PublicIPs[0],
				remoteEndpointSpec1.Backend, "remote"),
			tableRowMatcher(remoteEndpointSpec2.ClusterID, remoteEndpointSpec2.PrivateIPs[0], remoteEndpointSpec2.PublicIPs[0],
				remoteEndpointSpec2.Backend, "remote"))
	})

	When("no gateways exist", func() {
		It("should return an error", func(ctx SpecContext) {
			Expect(doEndpoints(ctx)).To(HaveOccurred())
		})
	})

	When("gateway retrieval fails", func() {
		BeforeEach(func() {
			cp := t.clusterInfo.ClientProducer.(*client.DefaultProducer)
			cp.GeneralClient = controllerfake.NewClientBuilder().WithScheme(scheme.Scheme).WithInterceptorFuncs(
				fake.FailingListInterceptor[*submarinerv1.GatewayList]()).Build()
		})

		It("should return an error", func(ctx SpecContext) {
			Expect(doEndpoints(ctx)).To(HaveOccurred())
		})
	})
})
