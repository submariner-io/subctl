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
	. "github.com/onsi/ginkgo/v2"
	"github.com/submariner-io/admiral/pkg/fake"
	"github.com/submariner-io/admiral/pkg/syncer/test"
	"github.com/submariner-io/subctl/pkg/diagnose"
	submarinerv1 "github.com/submariner-io/submariner/pkg/apis/submariner.io/v1"
	"github.com/submariner-io/submariner/pkg/globalnet/constants"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	controllerfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	mcsv1a1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"
)

var _ = Describe("GlobalnetConfig", func() {
	t := newGlobalnetTestDriver()

	BeforeEach(func() {
		t.submariner.Spec.GlobalCIDR = "169.254.0.0/16"
	})

	Context("ClusterGlobalEgressIP", t.testClusterGlobalEgressIPs)
	Context("GlobalEgressIP", t.testGlobalEgressIPs)
	Context("GlobalIngressIP", t.testGlobalIngressIPs)

	When("Globalnet is not configured", func() {
		BeforeEach(func() {
			t.submariner.Spec.GlobalCIDR = ""
		})

		t.testSuccess(t.run)
	})
})

type globalnetTestDriver struct {
	*testDriver
	clusterGlobalEgressIP *submarinerv1.ClusterGlobalEgressIP
}

func newGlobalnetTestDriver() *globalnetTestDriver {
	t := &globalnetTestDriver{testDriver: newTestDriver()}

	BeforeEach(func() {
		t.clusterGlobalEgressIP = &submarinerv1.ClusterGlobalEgressIP{
			ObjectMeta: metav1.ObjectMeta{
				Name: constants.ClusterGlobalEgressIPName,
			},
			Spec: submarinerv1.ClusterGlobalEgressIPSpec{
				NumberOfIPs: ptr.To(1),
			},
			Status: submarinerv1.GlobalEgressIPStatus{
				AllocatedIPs: []string{"169.254.1.100"},
				Conditions: []metav1.Condition{
					{
						Type:   string(submarinerv1.GlobalEgressIPAllocated),
						Status: metav1.ConditionTrue,
					},
				},
			},
		}
	})

	JustBeforeEach(func() {
		if t.clusterGlobalEgressIP != nil {
			t.createResource(t.clusterGlobalEgressIP)
		}
	})

	return t
}

func (t *globalnetTestDriver) testClusterGlobalEgressIPs() {
	When("the resource is properly configured", func() {
		t.testSuccess(t.run)
	})

	t.testMismatchedAllocatedIPs(func() {
		t.clusterGlobalEgressIP.Status.AllocatedIPs = nil
	})

	t.testMissingAllocatedCondition(func() {
		t.clusterGlobalEgressIP.Status.Conditions = nil
	})

	t.testAllocatedConditionFalse(func() {
		t.clusterGlobalEgressIP.Status.Conditions[0] = falseAllocatedStatusCondition()
	})

	t.testListResourcesFailure(fake.FailingListInterceptor[*submarinerv1.ClusterGlobalEgressIPList]())

	When("the default instance is missing", func() {
		BeforeEach(func() {
			t.clusterGlobalEgressIP = nil
		})

		t.testFailure(t.run, constants.ClusterGlobalEgressIPName)
	})

	When("multiple instances exist", func() {
		BeforeEach(func() {
			t.createResource(&submarinerv1.ClusterGlobalEgressIP{
				ObjectMeta: metav1.ObjectMeta{
					Name: "other-instance",
				},
			})
		})

		t.testFailure(t.run, "Found 2")
	})
}

func (t *globalnetTestDriver) testGlobalEgressIPs() {
	var globalEgressIP *submarinerv1.GlobalEgressIP

	BeforeEach(func() {
		globalEgressIP = &submarinerv1.GlobalEgressIP{
			ObjectMeta: metav1.ObjectMeta{
				Name: "global-egress-ip",
			},
			Spec: submarinerv1.GlobalEgressIPSpec{
				NumberOfIPs: ptr.To(1),
			},
			Status: submarinerv1.GlobalEgressIPStatus{
				AllocatedIPs: []string{"242.10.1.1"},
				Conditions: []metav1.Condition{
					{
						Type:   string(submarinerv1.GlobalEgressIPAllocated),
						Status: metav1.ConditionTrue,
					},
				},
			},
		}
	})

	JustBeforeEach(func() {
		t.createResource(globalEgressIP)
	})

	When("a resource is properly configured", func() {
		t.testSuccess(t.run)
	})

	t.testMismatchedAllocatedIPs(func() {
		globalEgressIP.Status.AllocatedIPs = nil
	})

	t.testMissingAllocatedCondition(func() {
		globalEgressIP.Status.Conditions = nil
	})

	t.testAllocatedConditionFalse(func() {
		globalEgressIP.Status.Conditions[0] = falseAllocatedStatusCondition()
	})

	t.testListResourcesFailure(fake.FailingListInterceptor[*submarinerv1.GlobalEgressIPList]())
}

func (t *globalnetTestDriver) testGlobalIngressIPs() {
	var (
		service         *corev1.Service
		serviceExport   *mcsv1a1.ServiceExport
		globalIngressIP *submarinerv1.GlobalIngressIP
		internalService *corev1.Service
	)

	BeforeEach(func() {
		service = &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-service",
				Namespace: test.LocalNamespace,
			},
			Spec: corev1.ServiceSpec{
				Type: corev1.ServiceTypeClusterIP,
			},
		}

		serviceExport = &mcsv1a1.ServiceExport{
			ObjectMeta: metav1.ObjectMeta{
				Name:      service.Name,
				Namespace: service.Namespace,
			},
		}

		globalIngressIP = &submarinerv1.GlobalIngressIP{
			ObjectMeta: metav1.ObjectMeta{
				Name:      service.Name,
				Namespace: service.Namespace,
			},
			Status: submarinerv1.GlobalIngressIPStatus{
				AllocatedIP: "242.10.1.1",
				Conditions: []metav1.Condition{
					{
						Type:   string(submarinerv1.GlobalEgressIPAllocated),
						Status: metav1.ConditionTrue,
					},
				},
			},
		}

		internalService = &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      service.Name + "-internal",
				Namespace: service.Namespace,
				Labels: map[string]string{
					"submariner.io/exportedServiceRef": service.Name,
				},
			},
			Spec: corev1.ServiceSpec{
				ExternalIPs: []string{globalIngressIP.Status.AllocatedIP},
			},
		}
	})

	When("a resource is properly configured", func() {
		JustBeforeEach(func() {
			t.createService(service)
			t.createServiceExport(serviceExport)
			t.createResource(globalIngressIP)
			t.createService(internalService)
		})

		t.testSuccess(t.run)

		Context("but listing of ServiceExport resources fails", func() {
			BeforeEach(func() {
				fake.FailOnAction(&t.fakeProducer.DynamicClient.(*dynamicfake.FakeDynamicClient).Fake, "serviceexports", "list", errFake, false)
			})

			t.testFailure(t.run, errFake.Error())
		})

		Context("but retrieval of the Service resource fails", func() {
			BeforeEach(func() {
				fake.FailOnAction(&t.fakeProducer.KubeClient.(*k8sfake.Clientset).Fake, "services", "get", errFake, false)
			})

			t.testFailure(t.run, errFake.Error())
		})

		Context("but listing of the Service resources fails", func() {
			BeforeEach(func() {
				fake.FailOnAction(&t.fakeProducer.KubeClient.(*k8sfake.Clientset).Fake, "services", "list", errFake, false)
			})

			t.testFailure(t.run, errFake.Error())
		})

		Context("but retrieval of the GlobalIngressIP resource fails", func() {
			BeforeEach(func() {
				t.fakeProducer.GeneralClient = controllerfake.NewClientBuilder().WithScheme(scheme.Scheme).WithInterceptorFuncs(
					fake.FailingGetInterceptor[*submarinerv1.GlobalIngressIP]()).Build()
			})

			t.testFailure(t.run, "mock Get error")
		})
	})

	When("the exported service doesn't exist", func() {
		JustBeforeEach(func() {
			t.createServiceExport(serviceExport)
		})

		t.testSuccessWithWarning(t.run, "Service")
	})

	When("the exported service is not of type ClusterIP", func() {
		BeforeEach(func() {
			service.Spec.Type = corev1.ServiceTypeExternalName
		})

		JustBeforeEach(func() {
			t.createService(service)
			t.createServiceExport(serviceExport)
		})

		t.testSuccess(t.run)
	})

	When("no GlobalIngressIP exists for the exported service", func() {
		JustBeforeEach(func() {
			t.createService(service)
			t.createServiceExport(serviceExport)
		})

		t.testFailure(t.run, "No matching")
	})

	When("a service is exported", func() {
		JustBeforeEach(func() {
			t.createService(service)
			t.createServiceExport(serviceExport)
			t.createResource(globalIngressIP)
		})

		Context("and no global IP was allocated", func() {
			BeforeEach(func() {
				globalIngressIP.Status.AllocatedIP = ""
			})

			t.testFailure(t.run, "No global IP")
		})

		t.testMissingAllocatedCondition(func() {
			globalIngressIP.Status.Conditions = nil
		})

		t.testAllocatedConditionFalse(func() {
			globalIngressIP.Status.Conditions[0] = falseAllocatedStatusCondition()
		})

		Context("and the internal service doesn't exist", func() {
			t.testFailure(t.run, "No internal service")
		})

		Context("and there's more than one internal service", func() {
			JustBeforeEach(func() {
				t.createService(internalService)
				t.createService(&corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "other-internal",
						Namespace: service.Namespace,
						Labels: map[string]string{
							"submariner.io/exportedServiceRef": service.Name,
						},
					},
				})
			})

			t.testFailure(t.run, "Found 2")
		})

		Context("and the internal service doesn't have an external IP", func() {
			BeforeEach(func() {
				internalService.Spec.ExternalIPs = nil
			})

			JustBeforeEach(func() {
				t.createService(internalService)
			})

			t.testFailure(t.run, "0 external IPs")
		})

		Context("and the internal service external IP doesn't match the allocated IP", func() {
			BeforeEach(func() {
				internalService.Spec.ExternalIPs = []string{"1.2.3.4"}
			})

			JustBeforeEach(func() {
				t.createService(internalService)
			})

			t.testFailure(t.run, "match allocated IP")
		})
	})
}

func (t *globalnetTestDriver) run() error {
	return diagnose.GlobalnetConfig(newClusterInfo(), "", t.statusTracker)
}

func (t *globalnetTestDriver) testMismatchedAllocatedIPs(before func()) {
	When("the number of requested IPs does not match the allocated IPs", func() {
		BeforeEach(func() {
			before()
		})

		t.testFailure(t.run, "requested IPs")
	})
}

func (t *globalnetTestDriver) testMissingAllocatedCondition(before func()) {
	When("the Allocated status condition is missing", func() {
		BeforeEach(func() {
			before()
		})

		t.testFailure(t.run, "missing", "Allocated")
	})
}

func (t *globalnetTestDriver) testAllocatedConditionFalse(before func()) {
	When("the Allocated status condition is false", func() {
		BeforeEach(func() {
			before()
		})

		t.testFailure(t.run, "Allocation failed")
	})
}

//nolint:gocritic // Ignore hugeParam.
func (t *globalnetTestDriver) testListResourcesFailure(listInterceptor interceptor.Funcs) {
	When("listing of resources fails", func() {
		BeforeEach(func() {
			t.fakeProducer.GeneralClient = controllerfake.NewClientBuilder().WithScheme(scheme.Scheme).WithInterceptorFuncs(
				listInterceptor).Build()
		})

		t.testFailure(t.run, "mock List error")
	})
}

func falseAllocatedStatusCondition() metav1.Condition {
	return metav1.Condition{
		Type:    string(submarinerv1.GlobalEgressIPAllocated),
		Status:  metav1.ConditionFalse,
		Reason:  "Failure",
		Message: "Allocation failed",
	}
}
