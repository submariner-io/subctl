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
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
	"github.com/submariner-io/admiral/pkg/fake"
	"github.com/submariner-io/subctl/internal/show"
	"github.com/submariner-io/subctl/pkg/client"
	submarinerv1 "github.com/submariner-io/submariner/pkg/apis/submariner.io/v1"
	"k8s.io/client-go/kubernetes/scheme"
	controllerfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("Gateways", func() {
	t := newTestDriver()

	doGateways := func(ctx context.Context) error {
		return show.Gateways(ctx, t.clusterInfo, "", t.status)
	}

	gatewayMatcher := func(gw *submarinerv1.Gateway, summary string, args ...any) types.GomegaMatcher {
		return tableRowMatcher(append([]string{gw.Status.LocalEndpoint.Hostname, string(gw.Status.HAStatus)},
			strings.Split(fmt.Sprintf(summary, args...), " ")...)...)
	}

	It("should display gateway information", func(ctx SpecContext) {
		activeGW := t.createGateway(ctx, "active-gw", submarinerv1.HAStatusActive, "", submarinerv1.Connected,
			submarinerv1.Connected)
		passiveGW := t.createGateway(ctx, "passive-gw", submarinerv1.HAStatusPassive, "")

		Expect(doGateways(ctx)).To(Succeed())

		t.assertTableOutput(
			gatewayMatcher(activeGW, show.AllConnectionsFmt, len(activeGW.Status.Connections)),
			gatewayMatcher(passiveGW, show.NoConnectionsMsg),
		)
	})

	It("should display gateway information with partial connections", func(ctx SpecContext) {
		gw := t.createGateway(ctx, "gateway", submarinerv1.HAStatusActive, "", submarinerv1.Connected, submarinerv1.Connecting,
			submarinerv1.ConnectionError)

		Expect(doGateways(ctx)).To(Succeed())

		t.assertTableOutput(gatewayMatcher(gw, show.PartialConnectionsFmt, 1, 3))
	})

	It("should display gateway with status failure", func(ctx SpecContext) {
		gw := t.createGateway(ctx, "gateway", submarinerv1.HAStatusActive, "Cable driver error")

		Expect(doGateways(ctx)).To(Succeed())

		t.assertTableOutput(gatewayMatcher(gw, gw.Status.StatusFailure)) //nolint:govet // Ignore non-constant format string
	})

	When("no gateways exist", func() {
		It("should return an error", func(ctx SpecContext) {
			Expect(doGateways(ctx)).NotTo(Succeed())
		})
	})

	When("gateway retrieval fails", func() {
		BeforeEach(func() {
			cp := t.clusterInfo.ClientProducer.(*client.DefaultProducer)
			cp.GeneralClient = controllerfake.NewClientBuilder().WithScheme(scheme.Scheme).WithInterceptorFuncs(
				fake.FailingListInterceptor[*submarinerv1.GatewayList]()).Build()
		})

		It("should return an error", func(ctx SpecContext) {
			Expect(doGateways(ctx)).NotTo(Succeed())
		})
	})
})
