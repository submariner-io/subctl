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

	"github.com/pkg/errors"
	"github.com/submariner-io/admiral/pkg/reporter"
	"github.com/submariner-io/admiral/pkg/resource"
	lhconstants "github.com/submariner-io/lighthouse/pkg/constants"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/internal/gvr"
	"github.com/submariner-io/subctl/internal/restconfig"
	"github.com/submariner-io/subctl/pkg/client"
	"github.com/submariner-io/subctl/pkg/cluster"
	corev1 "k8s.io/api/core/v1"
	discovery "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	mcsv1a1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"
)

func ServiceDiscovery(ctx context.Context, clusterInfo *cluster.Info, _ string, status reporter.Interface) error {
	status.Start("Checking that services have been exported/imported properly")
	defer status.End()

	tracker := reporter.NewTracker(status)

	checkServiceExports(ctx, clusterInfo, tracker)
	checkServiceImports(ctx, clusterInfo, tracker)

	if tracker.HasFailures() {
		return errors.New("failures while diagnosing service discovery")
	}

	return nil
}

// This function checks if all ServiceExports have a matching ServiceImport and if an EndpointSlice has been created for the service.
func checkServiceExports(ctx context.Context, clusterInfo *cluster.Info, status reporter.Interface) {
	serviceExportGVR := gvr.FromMetaGroupVersion(mcsv1a1.GroupVersion, "serviceexports")

	serviceExports, err := clusterInfo.ClientProducer.ForDynamic().Resource(serviceExportGVR).Namespace(corev1.NamespaceAll).
		List(ctx, metav1.ListOptions{})
	if err != nil {
		status.Failure("Error listing ServiceExport resources: %v", err)
		return
	}

	for i := range serviceExports.Items {
		se := resource.MustFromUnstructured(&serviceExports.Items[i], &mcsv1a1.ServiceExport{})

		_, err := clusterInfo.ClientProducer.ForKubernetes().CoreV1().Services(se.Namespace).Get(ctx, se.Name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			status.Warning("Exported Service \"%s/%s\" not found", se.Namespace, se.Name)
			verifyStatusCondition(se, mcsv1a1.ServiceExportConditionValid, metav1.ConditionFalse, status)

			continue
		}

		if err != nil {
			status.Failure("Error retrieving Service \"%s/%s\": %v", se.Namespace, se.Name, err)
			return
		}

		if !verifyStatusCondition(se, mcsv1a1.ServiceExportConditionValid, metav1.ConditionTrue, status) {
			continue
		}

		verifyStatusCondition(se, mcsv1a1.ServiceExportConditionReady, metav1.ConditionTrue, status)
		verifyStatusCondition(se, mcsv1a1.ServiceExportConditionConflict, metav1.ConditionFalse, status)

		serviceImportClient := clusterInfo.ClientProducer.ForDynamic().Resource(serviceImportGVR())

		if !localServiceImportExists(ctx, serviceImportClient, se.Name, se.Namespace, status) {
			continue
		}

		_, err = serviceImportClient.Namespace(se.Namespace).Get(ctx, se.Name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				status.Failure("No ServiceImport found for exported service \"%s/%s\"", se.Namespace, se.Name)
			} else {
				status.Failure("Error retrieving ServiceImport for exported service \"%s/%s\": %v", se.Namespace, se.Name, err)
			}
		}

		ep := clusterInfo.ClientProducer.ForKubernetes().DiscoveryV1().EndpointSlices(se.Namespace)

		epsList, err := ep.List(ctx, metav1.ListOptions{
			LabelSelector: labels.SelectorFromSet(map[string]string{
				discovery.LabelManagedBy:   lhconstants.LabelValueManagedBy,
				mcsv1a1.LabelServiceName:   se.Name,
				mcsv1a1.LabelSourceCluster: clusterInfo.Submariner.Spec.ClusterID,
			}).String(),
		})
		if err != nil {
			status.Failure("Error retrieving EndpointSlices for exported service \"%s/%s\": %v", se.Namespace, se.Name, err)
			return
		}

		if len(epsList.Items) == 0 {
			status.Failure("No EndpointSlice found for exported service \"%s/%s\"", se.Namespace, se.Name)
		}
	}
}

func localServiceImportExists(ctx context.Context, siClient dynamic.NamespaceableResourceInterface, serviceName, serviceNamespace string,
	status reporter.Interface,
) bool {
	list, err := siClient.Namespace(constants.OperatorNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{
			mcsv1a1.LabelServiceName:         serviceName,
			lhconstants.LabelSourceNamespace: serviceNamespace,
		}).String(),
	})
	if err != nil {
		status.Failure("Error retrieving ServiceImport for exported service \"%s/%s\": %v", serviceNamespace, serviceName, err)

		return false
	}

	if len(list.Items) > 0 {
		return true
	}

	status.Failure("No local ServiceImport in %q found for exported service \"%s/%s\"", constants.OperatorNamespace,
		serviceNamespace, serviceName)

	return false
}

func verifyStatusCondition(se *mcsv1a1.ServiceExport, condType mcsv1a1.ServiceExportConditionType, condStatus metav1.ConditionStatus,
	status reporter.Interface,
) bool {
	for i := range se.Status.Conditions {
		condition := &se.Status.Conditions[i]
		if condition.Type == string(condType) {
			if condition.Status != condStatus {
				status.Failure(
					"The ServiceExport %q status condition type for \"%s/%s\" is not satisfied. Expected condition status %q. Actual:\n%s",
					condition.Type, se.Namespace, se.Name, condStatus, resource.ToJSON(condition))

				return false
			}

			return true
		}
	}

	if condStatus == metav1.ConditionTrue {
		status.Failure("The ServiceExport for \"%s/%s\" is missing the %q status condition type", se.Namespace, se.Name, condType)
		return false
	}

	return true
}

func checkServiceImports(ctx context.Context, clusterInfo *cluster.Info, status reporter.Interface) {
	brokerRestConfig, brokerNamespace, err := restconfig.ForBroker(clusterInfo.Submariner, nil)
	if err != nil {
		status.Failure("Error getting the Broker's REST config: %v", err)
		return
	}

	clientProducer, err := client.NewProducerFromRestConfig(brokerRestConfig)
	if err != nil {
		status.Failure("Error creating broker client Producer: %v", err)
		return
	}

	serviceImports, err := clientProducer.ForDynamic().Resource(serviceImportGVR()).Namespace(brokerNamespace).List(
		ctx, metav1.ListOptions{})
	if err != nil {
		status.Failure("Error listing ServiceImports on the broker: %v", err)
		return
	}

	for i := range serviceImports.Items {
		if _, ok := serviceImports.Items[i].GetAnnotations()[mcsv1a1.LabelServiceName]; !ok {
			continue
		}

		// This is an aggregated ServiceImport on the broker - check for the local copy.
		serviceName := serviceImports.Items[i].GetAnnotations()[mcsv1a1.LabelServiceName]
		serviceNamespace := serviceImports.Items[i].GetAnnotations()[lhconstants.LabelSourceNamespace]

		_, err = clusterInfo.ClientProducer.ForDynamic().Resource(serviceImportGVR()).Namespace(serviceNamespace).Get(
			ctx, serviceName, metav1.GetOptions{})
		if resource.IsMissingNamespaceErr(err) {
			status.Warning("The namespace %q for imported service %q does not exist therefore the service will "+
				"not be accessible via DNS on this cluster. This may be intentional.", serviceNamespace, serviceName)
		} else if apierrors.IsNotFound(err) {
			status.Failure("No ServiceImport found for imported service \"%s/%s\"", serviceNamespace, serviceName)
		} else if err != nil {
			status.Failure("Error retrieving ServiceImport for imported service \"%s/%s\": %v", serviceNamespace, serviceName, err)
		}
	}
}

func serviceImportGVR() schema.GroupVersionResource {
	return gvr.FromMetaGroupVersion(mcsv1a1.GroupVersion, "serviceimports")
}
