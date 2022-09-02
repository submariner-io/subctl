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
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"
	"github.com/submariner-io/admiral/pkg/reporter"
	lhconstants "github.com/submariner-io/lighthouse/pkg/constants"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/internal/gvr"
	"github.com/submariner-io/subctl/pkg/cluster"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	mcsv1a1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"
)

func ServiceDiscovery(clusterInfo *cluster.Info, _ string, status reporter.Interface) error {
	status.Start("Checking if services have been exported properly")
	defer status.End()

	tracker := reporter.NewTracker(status)

	checkServiceExport(clusterInfo, tracker)

	if tracker.HasFailures() {
		return errors.New("failures while diagnosing service discovery")
	}

	status.Success("All services have been exported properly")

	return nil
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

		verifyStatusCondition(se, mcsv1a1.ServiceExportValid, status)
		verifyStatusCondition(se, lhconstants.ServiceExportSynced, status)

		ep := clusterInfo.ClientProducer.ForKubernetes().DiscoveryV1().EndpointSlices(se.Namespace)
		_, err = ep.Get(context.TODO(), fmt.Sprintf("%s-%s", se.Name, clusterInfo.Submariner.Spec.ClusterID), metav1.GetOptions{})

		if err != nil {
			if apierrors.IsNotFound(err) {
				status.Failure("No EndpointSlice found for exported service %s/%s", se.Namespace, se.Name)
			} else {
				status.Failure("Error retrieving EndPointSlice for exported service %s/%s", se.Namespace, se.Name)
				return
			}
		}

		_, err := clusterInfo.ClientProducer.ForDynamic().Resource(serviceImportsGVR).
			Namespace(constants.OperatorNamespace).Get(context.TODO(),
			fmt.Sprintf("%s-%s-%s", se.Name, se.Namespace, clusterInfo.Submariner.Spec.ClusterID), metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				status.Failure("No ServiceImport found for exported service %s/%s", se.Namespace, se.Name)
			} else {
				status.Failure("Error retrieving ServiceImport for exported service %s/%s", se.Namespace, se.Name)
			}
		}
	}
}

func verifyStatusCondition(se *mcsv1a1.ServiceExport, condType mcsv1a1.ServiceExportConditionType, status reporter.Interface) {
	for i := range se.Status.Conditions {
		condition := &se.Status.Conditions[i]
		if condition.Type == condType {
			if condition.Status != corev1.ConditionTrue {
				out, _ := json.MarshalIndent(condition, "", "  ")
				status.Failure("The ServiceExport %q status condition type for %s/%s is not satisfied:\n%s",
					condType, se.Namespace, se.Name, out)
			}

			return
		}
	}

	status.Failure("The ServiceExport for %s/%s is missing the %q status condition type", se.Namespace, se.Name, condType)
}
