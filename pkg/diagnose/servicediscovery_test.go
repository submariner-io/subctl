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
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	"github.com/submariner-io/admiral/pkg/fake"
	lhconstants "github.com/submariner-io/lighthouse/pkg/constants"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/pkg/diagnose"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	mcsv1a1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"
)

const (
	serviceName      = "nginx"
	serviceNamespace = "service-ns"
)

var _ = Describe("ServiceDiscovery", func() {
	Describe("Exported Services", testExportedServices)
	Describe("Imported Services", testImportedServices)
})

func testExportedServices() {
	var (
		service                 *corev1.Service
		serviceExport           *mcsv1a1.ServiceExport
		localServiceImport      *mcsv1a1.ServiceImport
		aggregatedServiceImport *mcsv1a1.ServiceImport
		endpointSlice           *discoveryv1.EndpointSlice
	)

	t := newServiceDiscoveryTestDriver()

	BeforeEach(func() {
		service = &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      serviceName,
				Namespace: serviceNamespace,
			},
		}

		serviceExport = &mcsv1a1.ServiceExport{
			ObjectMeta: metav1.ObjectMeta{
				Name:      serviceName,
				Namespace: serviceNamespace,
			},
			Status: mcsv1a1.ServiceExportStatus{
				Conditions: []metav1.Condition{
					{
						Type:   string(mcsv1a1.ServiceExportConditionValid),
						Status: metav1.ConditionTrue,
					},
					{
						Type:   string(mcsv1a1.ServiceExportConditionReady),
						Status: metav1.ConditionTrue,
					},
				},
			},
		}

		endpointSlice = &discoveryv1.EndpointSlice{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "eps",
				Namespace: serviceNamespace,
				Labels: map[string]string{
					discoveryv1.LabelManagedBy: lhconstants.LabelValueManagedBy,
					mcsv1a1.LabelServiceName:   serviceName,
					mcsv1a1.LabelSourceCluster: t.submariner.Spec.ClusterID,
				},
			},
		}

		localServiceImport = &mcsv1a1.ServiceImport{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-%s-%s", serviceName, serviceNamespace, t.submariner.Spec.ClusterID),
				Namespace: constants.OperatorNamespace,
				Labels: map[string]string{
					mcsv1a1.LabelServiceName:         serviceName,
					lhconstants.LabelSourceNamespace: serviceNamespace,
				},
			},
		}

		aggregatedServiceImport = &mcsv1a1.ServiceImport{
			ObjectMeta: metav1.ObjectMeta{
				Name:      serviceName,
				Namespace: serviceNamespace,
			},
		}
	})

	When("a service is exported properly", func() {
		JustBeforeEach(func(ctx SpecContext) {
			t.createService(ctx, service)
			t.createServiceExport(serviceExport)
			t.createServiceImport(localServiceImport)
			t.createServiceImport(aggregatedServiceImport)
			t.createEndpointSlice(ctx, endpointSlice)
		})

		t.testSuccess(t.run)
	})

	When("a service is exported", func() {
		JustBeforeEach(func(ctx SpecContext) {
			t.createServiceExport(serviceExport)
			t.createService(ctx, service)
		})

		Context("but the ServiceExport Valid condition isn't present", func() {
			BeforeEach(func() {
				meta.RemoveStatusCondition(&serviceExport.Status.Conditions, string(mcsv1a1.ServiceExportConditionValid))
				meta.RemoveStatusCondition(&serviceExport.Status.Conditions, string(mcsv1a1.ServiceExportConditionReady))
			})

			t.testFailure(t.run, "missing", string(mcsv1a1.ServiceExportConditionValid))
		})

		Context("but the ServiceExport Valid condition status is False", func() {
			BeforeEach(func() {
				meta.FindStatusCondition(serviceExport.Status.Conditions,
					string(mcsv1a1.ServiceExportConditionValid)).Status = metav1.ConditionFalse
				meta.RemoveStatusCondition(&serviceExport.Status.Conditions, string(mcsv1a1.ServiceExportConditionReady))
			})

			t.testFailure(t.run, string(mcsv1a1.ServiceExportConditionValid), string(metav1.ConditionFalse))
		})

		Context("but the ServiceExport Ready condition status is False", func() {
			BeforeEach(func() {
				meta.FindStatusCondition(serviceExport.Status.Conditions,
					string(mcsv1a1.ServiceExportConditionReady)).Status = metav1.ConditionFalse
			})

			t.testFailure(t.run, string(mcsv1a1.ServiceExportConditionReady), string(metav1.ConditionFalse))
		})

		Context("but no EndpointSlice exists", func() {
			BeforeEach(func() {
				t.createServiceImport(localServiceImport)
				t.createServiceImport(aggregatedServiceImport)
			})

			t.testFailure(t.run, "EndpointSlice", serviceName)
		})

		Context("but no local ServiceImport exists", func() {
			t.testFailure(t.run, "local", "ServiceImport", serviceName)
		})

		Context("but no aggregate ServiceImport exists", func() {
			BeforeEach(func(ctx SpecContext) {
				t.createServiceImport(localServiceImport)
				t.createEndpointSlice(ctx, endpointSlice)
			})

			t.testFailure(t.run, "ServiceImport", serviceName)
		})
	})

	When("a ServiceExport exists but the Service resource is missing", func() {
		JustBeforeEach(func() {
			meta.FindStatusCondition(serviceExport.Status.Conditions,
				string(mcsv1a1.ServiceExportConditionValid)).Status = metav1.ConditionFalse
			t.createServiceExport(serviceExport)
		})

		t.testSuccessWithWarning(t.run, serviceName, "not found")
	})

	When("no services are exported", func() {
		t.testSuccess(t.run)
	})

	When("ServiceExports retrieval fails", func() {
		BeforeEach(func() {
			t.createServiceExport(serviceExport)
			fake.FailOnAction(&t.fakeProducer.DynamicClient.(*dynamicfake.FakeDynamicClient).Fake, "serviceexports", "list", errFake, false)
		})

		t.testFailure(t.run, "ServiceExport", errFake.Error())
	})

	When("Service retrieval fails", func() {
		BeforeEach(func(ctx SpecContext) {
			t.createService(ctx, service)
			t.createServiceExport(serviceExport)
			fake.FailOnAction(&t.fakeProducer.KubeClient.(*k8sfake.Clientset).Fake, "services", "get", errFake, false)
		})

		t.testFailure(t.run, "Service", errFake.Error())
	})

	When("ServiceImport retrieval fails", func() {
		BeforeEach(func(ctx SpecContext) {
			t.createService(ctx, service)
			t.createServiceExport(serviceExport)
			t.createServiceImport(localServiceImport)
			fake.FailOnAction(&t.fakeProducer.DynamicClient.(*dynamicfake.FakeDynamicClient).Fake, "serviceimports", "list", errFake, true)
		})

		t.testFailure(t.run, "ServiceImport", errFake.Error())
	})

	When("EndpointSlice retrieval fails", func() {
		BeforeEach(func(ctx SpecContext) {
			t.createService(ctx, service)
			t.createServiceExport(serviceExport)
			t.createServiceImport(localServiceImport)
			t.createServiceImport(aggregatedServiceImport)
			t.createEndpointSlice(ctx, endpointSlice)
			fake.FailOnAction(&t.fakeProducer.KubeClient.(*k8sfake.Clientset).Fake, "endpointslices", "list", errFake, false)
		})

		t.testFailure(t.run, "EndpointSlice", errFake.Error())
	})
}

func testImportedServices() {
	t := newServiceDiscoveryTestDriver()

	BeforeEach(func() {
		fake.AddVerifyNamespaceReactor(&t.fakeProducer.DynamicClient.(*dynamicfake.FakeDynamicClient).Fake, "serviceimports")
	})

	JustBeforeEach(func(ctx SpecContext) {
		t.createNamespace(ctx, t.submariner.Spec.BrokerK8sRemoteNamespace)

		// Aggregated ServiceImport on the broker
		t.createServiceImport(&mcsv1a1.ServiceImport{
			ObjectMeta: metav1.ObjectMeta{
				Name:      serviceName + "-" + serviceNamespace,
				Namespace: t.submariner.Spec.BrokerK8sRemoteNamespace,
				Annotations: map[string]string{
					mcsv1a1.LabelServiceName:         serviceName,
					lhconstants.LabelSourceNamespace: serviceNamespace,
				},
			},
		})

		// Some cluster-local ServiceImport on the broker
		t.createServiceImport(&mcsv1a1.ServiceImport{
			ObjectMeta: metav1.ObjectMeta{
				Name:      serviceName + "-wdglkj",
				Namespace: t.submariner.Spec.BrokerK8sRemoteNamespace,
				Labels: map[string]string{
					mcsv1a1.LabelServiceName:         serviceName,
					lhconstants.LabelSourceNamespace: serviceNamespace,
					mcsv1a1.LabelSourceCluster:       "west",
				},
			},
		})
	})

	When("an imported service's local ServiceImport exists", func() {
		JustBeforeEach(func(ctx SpecContext) {
			t.createNamespace(ctx, serviceNamespace)

			// Local aggregated ServiceImport
			t.createServiceImport(&mcsv1a1.ServiceImport{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceName,
					Namespace: serviceNamespace,
				},
			})
		})

		t.testSuccess(t.run)
	})

	When("an imported service's namespace doesn't exist locally", func() {
		t.testSuccessWithWarning(t.run, serviceNamespace)
	})

	When("an imported service's local ServiceImport doesn't exist", func() {
		JustBeforeEach(func(ctx SpecContext) {
			t.createNamespace(ctx, serviceNamespace)
		})

		t.testFailure(t.run, "No ServiceImport")
	})

	When("broker ServiceImport retrieval fails", func() {
		BeforeEach(func() {
			fake.FailOnAction(&t.fakeProducer.DynamicClient.(*dynamicfake.FakeDynamicClient).Fake, "serviceimports", "list", errFake, true)
		})

		t.testFailure(t.run, "ServiceImports", errFake.Error())
	})

	When("local ServiceImport retrieval fails", func() {
		BeforeEach(func() {
			fake.FailOnAction(&t.fakeProducer.DynamicClient.(*dynamicfake.FakeDynamicClient).Fake, "serviceimports", "get", errFake, true)
		})

		t.testFailure(t.run, serviceName, errFake.Error())
	})
}

type serviceDiscoveryTestDriver struct {
	*testDriver
}

func newServiceDiscoveryTestDriver() *serviceDiscoveryTestDriver {
	return &serviceDiscoveryTestDriver{testDriver: newTestDriver()}
}

func (t *serviceDiscoveryTestDriver) run(ctx context.Context) error {
	return diagnose.ServiceDiscovery(ctx, newClusterInfo(ctx), "", t.statusTracker)
}
