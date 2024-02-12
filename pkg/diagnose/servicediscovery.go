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

	"github.com/pkg/errors"
	"github.com/submariner-io/admiral/pkg/reporter"
	"github.com/submariner-io/admiral/pkg/resource"
	lhconstants "github.com/submariner-io/lighthouse/pkg/constants"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/internal/gvr"
	"github.com/submariner-io/subctl/pkg/cluster"
	corev1 "k8s.io/api/core/v1"
	discovery "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	mcsv1a1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"
)

func ServiceDiscovery(clusterInfo *cluster.Info, _ string, status reporter.Interface) error {
	status.Start("Checking that services have been exported properly")
	defer status.End()

	tracker := reporter.NewTracker(status)

	checkServiceExport(clusterInfo, tracker)

	if tracker.HasFailures() {
		return errors.New("failures while diagnosing service discovery")
	}

	return nil
}

// This function checks if all ServiceExports have a matching ServiceImport and if an EndpointSlice has been created for the service.
func checkServiceExport(clusterInfo *cluster.Info, status reporter.Interface) {
	ctx := context.TODO()

	serviceExportGVR := gvr.FromMetaGroupVersion(mcsv1a1.GroupVersion, "serviceexports")

	serviceExports, err := clusterInfo.ClientProducer.ForDynamic().Resource(serviceExportGVR).Namespace(corev1.NamespaceAll).
		List(ctx, metav1.ListOptions{})
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

		_, err := clusterInfo.ClientProducer.ForKubernetes().CoreV1().Services(se.Namespace).Get(ctx, se.Name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			status.Warning("Exported Service %s/%s not found", se.Namespace, se.Name)
			verifyStatusCondition(se, mcsv1a1.ServiceExportValid, corev1.ConditionFalse, status)

			continue
		}

		if err != nil {
			status.Failure("Error retrieving Service %s/%s: %v", se.Namespace, se.Name, err)
			continue
		}

		verifyStatusCondition(se, mcsv1a1.ServiceExportValid, corev1.ConditionTrue, status)
		verifyStatusCondition(se, lhconstants.ServiceExportReady, corev1.ConditionTrue, status)
		verifyStatusCondition(se, mcsv1a1.ServiceExportConflict, corev1.ConditionFalse, status)

		ep := clusterInfo.ClientProducer.ForKubernetes().DiscoveryV1().EndpointSlices(se.Namespace)

		epsList, err := ep.List(ctx, metav1.ListOptions{
			LabelSelector: labels.SelectorFromSet(map[string]string{
				discovery.LabelManagedBy:          lhconstants.LabelValueManagedBy,
				mcsv1a1.LabelServiceName:          se.Name,
				lhconstants.MCSLabelSourceCluster: clusterInfo.Submariner.Spec.ClusterID,
			}).String(),
		})
		if err != nil {
			status.Failure("Error retrieving EndPointSlice for exported service %s/%s: %v", se.Namespace, se.Name, err)
			return
		}

		if len(epsList.Items) == 0 {
			status.Failure("No EndpointSlice found for exported service %s/%s", se.Namespace, se.Name)
		}

		checkForAggregateSI := false

		serviceImportClient := clusterInfo.ClientProducer.ForDynamic().Resource(serviceImportsGVR)

		localSI, err := serviceImportClient.Namespace(constants.OperatorNamespace).Get(ctx,
			fmt.Sprintf("%s-%s-%s", se.Name, se.Namespace, clusterInfo.Submariner.Spec.ClusterID), metav1.GetOptions{})
		if err == nil {
			_, checkForAggregateSI = localSI.GetLabels()[mcsv1a1.LabelServiceName]
		} else if apierrors.IsNotFound(err) {
			status.Failure("No local ServiceImport in %q found for exported service %s/%s", constants.OperatorNamespace,
				se.Namespace, se.Name)
		} else {
			status.Failure("Error retrieving ServiceImport for exported service %s/%s: %v", se.Namespace, se.Name, err)
		}

		if checkForAggregateSI {
			_, err = serviceImportClient.Namespace(se.Namespace).Get(ctx, se.Name, metav1.GetOptions{})
			if err != nil {
				if apierrors.IsNotFound(err) {
					status.Failure("No ServiceImport found for exported service %s/%s", se.Namespace, se.Name)
				} else {
					status.Failure("Error retrieving ServiceImport for exported service %s/%s: %v", se.Namespace, se.Name, err)
				}
			}
		}
	}
}

func verifyStatusCondition(se *mcsv1a1.ServiceExport, condType mcsv1a1.ServiceExportConditionType, condStatus corev1.ConditionStatus,
	status reporter.Interface,
) {
	for i := range se.Status.Conditions {
		condition := &se.Status.Conditions[i]
		if condition.Type == condType || (condType == lhconstants.ServiceExportReady && condition.Type == "Synced") {
			if condition.Status != condStatus {
				status.Failure(
					"The ServiceExport %q status condition type for %s/%s is not satisfied. Expected condition status %q. Actual:\n%s",
					condition.Type, se.Namespace, se.Name, condStatus, resource.ToJSON(condition))
			}

			return
		}
	}

	if condStatus == corev1.ConditionTrue {
		status.Failure("The ServiceExport for %s/%s is missing the %q status condition type", se.Namespace, se.Name, condType)
	}
}
