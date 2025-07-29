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
	"encoding/base64"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	reportertest "github.com/submariner-io/admiral/pkg/reporter/test"
	"github.com/submariner-io/admiral/pkg/resource"
	"github.com/submariner-io/admiral/pkg/syncer/test"
	"github.com/submariner-io/subctl/internal/cli"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/internal/gvr"
	"github.com/submariner-io/subctl/pkg/client"
	clientfake "github.com/submariner-io/subctl/pkg/client/fake"
	"github.com/submariner-io/subctl/pkg/cluster"
	"github.com/submariner-io/submariner-operator/api/v1alpha1"
	"github.com/submariner-io/submariner-operator/pkg/names"
	submarinerv1 "github.com/submariner-io/submariner/pkg/apis/submariner.io/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	controllerclient "sigs.k8s.io/controller-runtime/pkg/client"
	mcsv1a1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"
)

var _ = BeforeSuite(func() {
	Expect(v1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())
	Expect(submarinerv1.AddToScheme(scheme.Scheme)).To(Succeed())
	Expect(mcsv1a1.Install(scheme.Scheme)).To(Succeed())
})

func TestDiagnose(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Diagnose Suite")
}

type testDriver struct {
	fakeProducer  *client.DefaultProducer
	submariner    *v1alpha1.Submariner
	statusTracker *reportertest.Tracker
}

func (t *testDriver) createResource(obj controllerclient.Object) {
	err := t.fakeProducer.GeneralClient.Create(context.TODO(), obj)
	Expect(err).NotTo(HaveOccurred())
}

func (t *testDriver) createServiceExport(se *mcsv1a1.ServiceExport) {
	test.CreateResource(t.fakeProducer.DynamicClient.Resource(
		gvr.FromMetaGroupVersion(mcsv1a1.GroupVersion, "serviceexports")).Namespace(se.Namespace), se)
}

func (t *testDriver) createServiceImport(si *mcsv1a1.ServiceImport) {
	test.CreateResource(t.fakeProducer.DynamicClient.Resource(
		gvr.FromMetaGroupVersion(mcsv1a1.GroupVersion, "serviceimports")).Namespace(si.Namespace), si)
}

func (t *testDriver) createService(svc *corev1.Service) {
	_, err := t.fakeProducer.KubeClient.CoreV1().Services(svc.Namespace).Create(context.TODO(), svc, metav1.CreateOptions{})
	Expect(err).NotTo(HaveOccurred())
}

func (t *testDriver) creatEndpointSlice(eps *discoveryv1.EndpointSlice) {
	_, err := t.fakeProducer.KubeClient.DiscoveryV1().EndpointSlices(eps.Namespace).Create(context.TODO(), eps, metav1.CreateOptions{})
	Expect(err).NotTo(HaveOccurred())
}

func (t *testDriver) createNamespace(name string) {
	_, err := t.fakeProducer.KubeClient.CoreV1().Namespaces().Create(context.TODO(), &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}, metav1.CreateOptions{})
	Expect(err).NotTo(HaveOccurred())

	test.CreateResource(t.fakeProducer.DynamicClient.Resource(corev1.SchemeGroupVersion.WithResource("namespaces")),
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
		})
}

func newTestDriver() *testDriver {
	t := &testDriver{}

	BeforeEach(func() {
		t.statusTracker = &reportertest.Tracker{Interface: cli.NewReporter()}
		t.fakeProducer = clientfake.New()
		t.submariner = &v1alpha1.Submariner{
			ObjectMeta: metav1.ObjectMeta{
				Name:      names.SubmarinerCrName,
				Namespace: constants.OperatorNamespace,
			},
			Spec: v1alpha1.SubmarinerSpec{
				ClusterID:                "east",
				BrokerK8sApiServer:       "api-server",
				BrokerK8sApiServerToken:  base64.StdEncoding.EncodeToString([]byte("token")),
				BrokerK8sRemoteNamespace: constants.DefaultBrokerNamespace,
			},
		}

		resource.NewDynamicClient = func(_ *rest.Config) (dynamic.Interface, error) {
			return t.fakeProducer.DynamicClient, nil
		}
	})

	JustBeforeEach(func() {
		t.createResource(t.submariner)
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
