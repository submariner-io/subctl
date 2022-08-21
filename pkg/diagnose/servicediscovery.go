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

package diagnose

import (
	"context"
	"fmt"

	"github.com/submariner-io/admiral/pkg/reporter"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/internal/gvr"
	"github.com/submariner-io/subctl/pkg/cluster"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	mcsv1a1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"
)

func ServiceDiscovery(clusterInfo *cluster.Info, status reporter.Interface) bool {
	status.Start("Checking if services have been exported properly")
	defer status.End()

	tracker := reporter.NewTracker(status)

	checkServiceExport(clusterInfo, tracker)

	if tracker.HasFailures() {
		return false
	}

	status.Success("All services have been exported properly")

	return true
}

// This function checks if all ServiceExports have a matching ServiceImport and if an EndpointSlice has been created for the service.
func checkServiceExport(clusterInfo *cluster.Info, status reporter.Interface) {
	serviceExportGVR := gvr.FromMetaGroupVersion(mcsv1a1.GroupVersion, "serviceexports")

	serviceExports, err := clusterInfo.ClientProducer.ForDynamic().Resource(serviceExportGVR).Namespace(corev1.NamespaceAll).
		List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		status.Failure("Error listing ServiceExport resources: %v", err)
		return
	}

	serviceImportsGVR := gvr.FromMetaGroupVersion(mcsv1a1.GroupVersion, "serviceimports")

	for i := range serviceExports.Items {
		se := &mcsv1a1.ServiceExport{}
		err = runtime.DefaultUnstructuredConverter.FromUnstructured(serviceExports.Items[i].Object, se)

		if err != nil {
			status.Failure("Error converting ServiceExport: %v", err)
			continue
		}

		ns := se.GetNamespace()
		name := se.GetName()
		conditions := se.Status.Conditions
		var serviceExportValid *mcsv1a1.ServiceExportCondition

		for i := range conditions {
			if conditions[i].Type == mcsv1a1.ServiceExportValid {
				serviceExportValid = &conditions[i]
			}
		}

		if serviceExportValid == nil {
			status.Failure("ServiceExport for %s/%s is missing the %s condition type", ns, name, mcsv1a1.ServiceExportValid)
			continue
		}

		if serviceExportValid.Status != corev1.ConditionTrue {
			status.Failure("Export failed or has not yet completed for service %s/%s: %s", ns, name, serviceExportValid.Message)
			continue
		}

		ep := clusterInfo.ClientProducer.ForKubernetes().DiscoveryV1().EndpointSlices(ns)
		_, err = ep.Get(context.TODO(), fmt.Sprintf("%s-%s", name, clusterInfo.Submariner.Spec.ClusterID), metav1.GetOptions{})

		if err != nil {
			if apierrors.IsNotFound(err) {
				status.Failure("No EndpointSlice found for exported service %s/%s", ns, name)
			} else {
				status.Failure("Error retrieving EndPointSlice for exported service %s/%s", ns, name)
				return
			}
		}

		_, err := clusterInfo.ClientProducer.ForDynamic().Resource(serviceImportsGVR).
			Namespace(constants.OperatorNamespace).Get(context.TODO(),
			fmt.Sprintf("%s-%s-%s", name, ns, clusterInfo.Submariner.Spec.ClusterID), metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				status.Failure("No ServiceImport found for exported service %s/%s", ns, name)
			} else {
				status.Failure("Error retrieving ServiceImport for exported service %s/%s", ns, name)
			}
		}
	}
}
