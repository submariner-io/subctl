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
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/pkg/client"
	clientfake "github.com/submariner-io/subctl/pkg/client/fake"
	"github.com/submariner-io/subctl/pkg/cluster"
	"github.com/submariner-io/submariner-operator/api/v1alpha1"
	submarinerv1 "github.com/submariner-io/submariner/pkg/apis/submariner.io/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	cntrollerclient "sigs.k8s.io/controller-runtime/pkg/client"
	mcsv1a1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"
)

var _ = BeforeSuite(func() {
	Expect(v1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())
	Expect(submarinerv1.AddToScheme(scheme.Scheme)).To(Succeed())
	Expect(mcsv1a1.AddToScheme(scheme.Scheme)).To(Succeed())
})

func TestDiagnose(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Diagnose Suite")
}

type testDriver struct {
	fakeProducer *client.DefaultProducer
}

func (t *testDriver) createResource(resource cntrollerclient.Object) {
	err := t.fakeProducer.GeneralClient.Create(context.TODO(), resource)
	Expect(err).NotTo(HaveOccurred())
}

func newTestDriver() *testDriver {
	t := &testDriver{}

	BeforeEach(func() {
		t.fakeProducer = clientfake.New()
	})

	return t
}

func newClusterInfo() *cluster.Info {
	info, err := cluster.NewInfo("east", &rest.Config{})
	Expect(err).NotTo(HaveOccurred())

	return info
}

func newGateway(haStatus submarinerv1.HAStatus, conns ...submarinerv1.Connection) *submarinerv1.Gateway {
	return &submarinerv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: constants.OperatorNamespace,
		},
		Status: submarinerv1.GatewayStatus{
			HAStatus:    haStatus,
			Connections: conns,
		},
	}
}

func newRouteAgent(remoteEPs ...submarinerv1.RemoteEndpoint) *submarinerv1.RouteAgent {
	return &submarinerv1.RouteAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: constants.OperatorNamespace,
		},
		Status: submarinerv1.RouteAgentStatus{
			RemoteEndpoints: remoteEPs,
		},
	}
}
