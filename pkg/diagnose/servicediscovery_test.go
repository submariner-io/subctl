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
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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

	t := newTestDriver()

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
		JustBeforeEach(func() {
			t.createService(service)
			t.createServiceExport(serviceExport)
			t.createServiceImport(localServiceImport)
			t.createServiceImport(aggregatedServiceImport)
			t.creatEndpointSlice(endpointSlice)
		})

		It("should succeed", func() {
			Expect(t.runServiceDiscovery()).To(Succeed())
			t.statusTracker.AssertFailureCount(0)
			t.statusTracker.AssertWarningCount(0)
		})
	})

	When("a service is exported", func() {
		JustBeforeEach(func() {
			t.createServiceExport(serviceExport)
			t.createService(service)
		})

		Context("but the ServiceExport Valid condition isn't present", func() {
			BeforeEach(func() {
				meta.RemoveStatusCondition(&serviceExport.Status.Conditions, string(mcsv1a1.ServiceExportConditionValid))
				meta.RemoveStatusCondition(&serviceExport.Status.Conditions, string(mcsv1a1.ServiceExportConditionReady))
			})

			It("should fail", func() {
				Expect(t.runServiceDiscovery()).NotTo(Succeed())
				t.statusTracker.AssertFailureCount(1)
			})
		})

		Context("but the ServiceExport Valid condition status is False", func() {
			BeforeEach(func() {
				meta.FindStatusCondition(serviceExport.Status.Conditions,
					string(mcsv1a1.ServiceExportConditionValid)).Status = metav1.ConditionFalse
				meta.RemoveStatusCondition(&serviceExport.Status.Conditions, string(mcsv1a1.ServiceExportConditionReady))
			})

			It("should fail", func() {
				Expect(t.runServiceDiscovery()).NotTo(Succeed())
				t.statusTracker.AssertFailureCount(1)
			})
		})

		Context("but the ServiceExport Ready condition status is False", func() {
			BeforeEach(func() {
				meta.FindStatusCondition(serviceExport.Status.Conditions,
					string(mcsv1a1.ServiceExportConditionReady)).Status = metav1.ConditionFalse
			})

			It("should fail", func() {
				Expect(t.runServiceDiscovery()).NotTo(Succeed())
				t.statusTracker.AssertHasFailure()
			})
		})

		Context("but no EndpointSlice exists", func() {
			BeforeEach(func() {
				t.createServiceImport(localServiceImport)
				t.createServiceImport(aggregatedServiceImport)
			})

			It("should fail", func() {
				Expect(t.runServiceDiscovery()).NotTo(Succeed())
				t.statusTracker.AssertFailureCount(1)
			})
		})

		Context("but no local ServiceImport exists", func() {
			It("should fail", func() {
				Expect(t.runServiceDiscovery()).NotTo(Succeed())
				t.statusTracker.AssertFailureCount(1)
			})
		})

		Context("but no aggregate ServiceImport exists", func() {
			BeforeEach(func() {
				t.createServiceImport(localServiceImport)
				t.creatEndpointSlice(endpointSlice)
			})

			It("should fail", func() {
				Expect(t.runServiceDiscovery()).NotTo(Succeed())
				t.statusTracker.AssertFailureCount(1)
			})
		})
	})

	When("a ServiceExport exists but the Service resource is missing", func() {
		JustBeforeEach(func() {
			meta.FindStatusCondition(serviceExport.Status.Conditions,
				string(mcsv1a1.ServiceExportConditionValid)).Status = metav1.ConditionFalse
			t.createServiceExport(serviceExport)
		})

		It("should succeed but emit a warning", func() {
			Expect(t.runServiceDiscovery()).To(Succeed())
			t.statusTracker.AssertFailureCount(0)
			t.statusTracker.AssertWarningCount(1)
		})
	})

	When("no services are exported", func() {
		It("should succeed", func() {
			Expect(t.runServiceDiscovery()).To(Succeed())
			t.statusTracker.AssertFailureCount(0)
			t.statusTracker.AssertWarningCount(0)
		})
	})

	When("ServiceExports retrieval fails", func() {
		BeforeEach(func() {
			t.createServiceExport(serviceExport)
			fake.FailOnAction(&t.fakeProducer.DynamicClient.(*dynamicfake.FakeDynamicClient).Fake, "serviceexports", "list", nil, false)
		})

		It("should fail", func() {
			Expect(t.runServiceDiscovery()).NotTo(Succeed())
			t.statusTracker.AssertFailureCount(1)
		})
	})

	When("Service retrieval fails", func() {
		BeforeEach(func() {
			t.createService(service)
			t.createServiceExport(serviceExport)
			fake.FailOnAction(&t.fakeProducer.KubeClient.(*k8sfake.Clientset).Fake, "services", "get", nil, false)
		})

		It("should fail", func() {
			Expect(t.runServiceDiscovery()).NotTo(Succeed())
			t.statusTracker.AssertFailureCount(1)
		})
	})

	When("ServiceImport retrieval fails", func() {
		BeforeEach(func() {
			t.createService(service)
			t.createServiceExport(serviceExport)
			t.createServiceImport(localServiceImport)
			fake.FailOnAction(&t.fakeProducer.DynamicClient.(*dynamicfake.FakeDynamicClient).Fake, "serviceimports", "list", nil, true)
		})

		It("should fail", func() {
			Expect(t.runServiceDiscovery()).NotTo(Succeed())
			t.statusTracker.AssertFailureCount(1)
		})
	})

	When("EndpointSlice retrieval fails", func() {
		BeforeEach(func() {
			t.createService(service)
			t.createServiceExport(serviceExport)
			t.createServiceImport(localServiceImport)
			t.createServiceImport(aggregatedServiceImport)
			t.creatEndpointSlice(endpointSlice)
			fake.FailOnAction(&t.fakeProducer.KubeClient.(*k8sfake.Clientset).Fake, "endpointslices", "list", nil, false)
		})

		It("should fail", func() {
			Expect(t.runServiceDiscovery()).NotTo(Succeed())
			t.statusTracker.AssertFailureCount(1)
		})
	})
}

func testImportedServices() {
	t := newTestDriver()

	BeforeEach(func() {
		fake.AddVerifyNamespaceReactor(&t.fakeProducer.DynamicClient.(*dynamicfake.FakeDynamicClient).Fake, "serviceimports")
	})

	JustBeforeEach(func() {
		t.createNamespace(t.submariner.Spec.BrokerK8sRemoteNamespace)

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
		JustBeforeEach(func() {
			t.createNamespace(serviceNamespace)

			// Local aggregated ServiceImport
			t.createServiceImport(&mcsv1a1.ServiceImport{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serviceName,
					Namespace: serviceNamespace,
				},
			})
		})

		It("should succeed", func() {
			Expect(t.runServiceDiscovery()).To(Succeed())
			t.statusTracker.AssertFailureCount(0)
			t.statusTracker.AssertWarningCount(0)
		})
	})

	When("an imported service's namespace doesn't exist locally", func() {
		It("should succeed but emit a warning", func() {
			Expect(t.runServiceDiscovery()).To(Succeed())
			t.statusTracker.AssertFailureCount(0)
			t.statusTracker.AssertWarningCount(1)
		})
	})

	When("an imported service's local ServiceImport doesn't exist", func() {
		JustBeforeEach(func() {
			t.createNamespace(serviceNamespace)
		})

		It("should fail", func() {
			Expect(t.runServiceDiscovery()).NotTo(Succeed())
			t.statusTracker.AssertFailureCount(1)
			t.statusTracker.AssertWarningCount(0)
		})
	})

	When("broker ServiceImport retrieval fails", func() {
		BeforeEach(func() {
			fake.FailOnAction(&t.fakeProducer.DynamicClient.(*dynamicfake.FakeDynamicClient).Fake, "serviceimports", "list", nil, true)
		})

		It("should fail", func() {
			Expect(t.runServiceDiscovery()).NotTo(Succeed())
			t.statusTracker.AssertFailureCount(1)
		})
	})

	When("local ServiceImport retrieval fails", func() {
		BeforeEach(func() {
			fake.FailOnAction(&t.fakeProducer.DynamicClient.(*dynamicfake.FakeDynamicClient).Fake, "serviceimports", "get", nil, true)
		})

		It("should fail", func() {
			Expect(t.runServiceDiscovery()).NotTo(Succeed())
			t.statusTracker.AssertFailureCount(1)
		})
	})
}

func (t *testDriver) runServiceDiscovery() error {
	return diagnose.ServiceDiscovery(newClusterInfo(), "", t.statusTracker)
}
