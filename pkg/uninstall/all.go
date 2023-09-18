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

package uninstall

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/submariner-io/admiral/pkg/finalizer"
	"github.com/submariner-io/admiral/pkg/names"
	"github.com/submariner-io/admiral/pkg/reporter"
	"github.com/submariner-io/admiral/pkg/resource"
	"github.com/submariner-io/admiral/pkg/util"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/pkg/client"
	"github.com/submariner-io/subctl/pkg/operator/deployment"
	operatorv1alpha1 "github.com/submariner-io/submariner-operator/api/v1alpha1"
	opnames "github.com/submariner-io/submariner-operator/pkg/names"
	submarinerv1 "github.com/submariner-io/submariner/pkg/apis/submariner.io/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	controller "sigs.k8s.io/controller-runtime/pkg/client"
)

const componentReadyTimeout = time.Minute * 2

func All(clients client.Producer, clusterName, submarinerNamespace string,
	status reporter.Interface,
) error {
	found, err := ensureSubmarinerDeleted(clients, clusterName, submarinerNamespace, status)
	if err != nil {
		return err
	}

	if !found {
		err = ensureServiceDiscoveryDeleted(clients, clusterName, submarinerNamespace, status)
		if err != nil {
			return err
		}
	}

	brokerNS, err := findBrokerNamespace(clients.ForGeneral(), clusterName, status)
	if err != nil {
		return err
	}

	deleted, err := deleteBrokerIfUnused(clients, brokerNS, clusterName, status)
	if err != nil {
		return err
	}

	err = deleteClusterRolesAndBindings(clients, clusterName, status, !deleted)
	if err != nil {
		return err
	}

	if deleted {
		status.Start("Deleting the Submariner namespace %q on cluster %q", submarinerNamespace, clusterName)
		defer status.End()

		err = clients.ForKubernetes().CoreV1().Namespaces().Delete(context.TODO(), submarinerNamespace, metav1.DeleteOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return status.Error(err, "Error deleting the Submariner namespace")
		}

		err = deleteCRDs(clients.ForGeneral(), clusterName, status)
		if err != nil {
			return err
		}
	}

	return unlabelGatewayNodes(clients, clusterName, status)
}

func unlabelGatewayNodes(clients client.Producer, clusterName string, status reporter.Interface) error {
	status.Start("Unlabeling gateway nodes on cluster %q", clusterName)
	defer status.End()

	list, err := clients.ForKubernetes().CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{
		LabelSelector: labels.SelectorFromSet(map[string]string{constants.SubmarinerGatewayLabel: "true"}).String(),
	})
	if err != nil {
		return status.Error(err, "Error listing Nodes")
	}

	//nolint:wrapcheck // Let the caller wrap errors
	nodeInterface := &resource.InterfaceFuncs{
		GetFunc: func(ctx context.Context, name string, options metav1.GetOptions) (runtime.Object, error) {
			return clients.ForKubernetes().CoreV1().Nodes().Get(ctx, name, options)
		},
		UpdateFunc: func(ctx context.Context, obj runtime.Object, options metav1.UpdateOptions) (runtime.Object, error) {
			return clients.ForKubernetes().CoreV1().Nodes().Update(ctx, obj.(*corev1.Node), options)
		},
	}

	for i := range list.Items {
		err = util.Update(context.TODO(), nodeInterface, &list.Items[i], func(existing runtime.Object) (runtime.Object, error) {
			delete(existing.(*corev1.Node).Labels, constants.SubmarinerGatewayLabel)
			return existing, nil
		})
		if err != nil {
			return status.Error(err, "Error updating Node %q", list.Items[i].Name)
		}
	}

	return nil
}

func deleteCRDs(controllerClient controller.Client, clusterName string, status reporter.Interface) error {
	status.Start("Deleting the Submariner custom resource definitions on cluster %q", clusterName)
	defer status.End()

	list := &apiextensionsv1.CustomResourceDefinitionList{}

	err := controllerClient.List(context.TODO(), list)
	if err != nil {
		return status.Error(err, "Error listing CustomResourceDefinitions")
	}

	for i := range list.Items {
		if !strings.HasSuffix(list.Items[i].Name, ".submariner.io") {
			continue
		}

		err = controllerClient.Delete(context.TODO(), &list.Items[i])
		if err != nil {
			return status.Error(err, "Error deleting CustomResourceDefinition %q", list.Items[i].Name)
		}

		status.Success("Deleted the %q custom resource definition", list.Items[i].Name)
	}

	return nil
}

func deleteClusterRolesAndBindings(clients client.Producer, clusterName string, status reporter.Interface,
	keepOperator bool,
) error {
	status.Start("Deleting the Submariner cluster roles and bindings on cluster %q", clusterName)
	defer status.End()

	list, err := clients.ForKubernetes().RbacV1().ClusterRoleBindings().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return status.Error(err, "Error listing ClusterRoleBindings")
	}

	for i := range list.Items {
		if !strings.HasPrefix(list.Items[i].Name, "submariner-") || (keepOperator && list.Items[i].Name == names.OperatorComponent) {
			continue
		}

		err = clients.ForKubernetes().RbacV1().ClusterRoleBindings().Delete(context.TODO(), list.Items[i].Name, metav1.DeleteOptions{})
		if err != nil {
			return status.Error(err, "Error deleting ClusterRoleBinding %q", list.Items[i].Name)
		}

		err = clients.ForKubernetes().RbacV1().ClusterRoles().Delete(context.TODO(), list.Items[i].RoleRef.Name, metav1.DeleteOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return status.Error(err, "Error deleting ClusterRole %q", list.Items[i].RoleRef.Name)
		}

		status.Success("Deleted the %q cluster role and binding", list.Items[i].Name)
	}

	return nil
}

func ensureSubmarinerDeleted(clients client.Producer, clusterName, namespace string, status reporter.Interface) (bool, error) {
	defer status.End()

	status.Start("Checking if the connectivity component is installed on cluster %q", clusterName)

	submariner := &operatorv1alpha1.Submariner{}
	err := clients.ForGeneral().Get(context.TODO(), controller.ObjectKey{
		Namespace: namespace,
		Name:      opnames.SubmarinerCrName,
	}, submariner)

	if resource.IsNotFoundErr(err) {
		status.Success("The connectivity component is not installed on cluster %q - skipping", clusterName)
		return false, nil
	}

	if err != nil {
		return false, status.Error(err, "Error retrieving the Submariner resource")
	}

	status.Success("The connectivity component is installed on cluster %q", clusterName)

	status.Start("Deleting the Submariner resource - this may take some time")

	err = ensureDeleted(clients, submariner, status)

	return true, status.Error(err, "Error deleting Submariner resource %q", submariner.Name)
}

func ensureServiceDiscoveryDeleted(clients client.Producer, clusterName, namespace string, status reporter.Interface) error {
	defer status.End()

	status.Start("Checking if the service discovery component is installed on cluster %q", clusterName)

	serviceDiscovery := &operatorv1alpha1.ServiceDiscovery{}
	err := clients.ForGeneral().Get(context.TODO(), controller.ObjectKey{
		Namespace: namespace,
		Name:      opnames.ServiceDiscoveryCrName,
	}, serviceDiscovery)

	if resource.IsNotFoundErr(err) {
		status.Success("The service discovery component is not installed on cluster %q - skipping", clusterName)
		return nil
	}

	if err != nil {
		return status.Error(err, "Error retrieving the ServiceDiscovery resource")
	}

	status.Success("The service discovery component is installed on cluster %q", clusterName)

	status.Start("Deleting the ServiceDiscovery resource - this may take some time")

	err = ensureDeleted(clients, serviceDiscovery, status)

	return status.Error(err, "Error deleting ServiceDiscovery resource %q", serviceDiscovery.Name)
}

func ensureDeleted(clients client.Producer, obj controller.Object, status reporter.Interface) error {
	const maxWait = componentReadyTimeout + time.Second*30
	const checkInterval = 2 * time.Second

	awaitDeleted := func() error {
		//nolint:wrapcheck // No need to wrap
		return wait.PollUntilContextTimeout(context.Background(), checkInterval, maxWait, true, func(ctx context.Context) (bool, error) {
			err := clients.ForGeneral().Delete(ctx, obj)
			if apierrors.IsNotFound(err) {
				return true, nil
			}

			return false, err
		})
	}

	err := awaitDeleted()

	if wait.Interrupted(err) {
		labelSelector, err := deployment.GetPodLabelSelector(clients.ForKubernetes(), obj.GetNamespace())
		if err != nil {
			return errors.Wrap(err, "error obtaining the operator deployment label")
		}

		if labelSelector == "" {
			status.Warning("The Submariner operator deployment does not exist so deletion of the resource was not completed - " +
				"the resource will be force-deleted")
		} else {
			pods, err := clients.ForKubernetes().CoreV1().Pods(obj.GetNamespace()).List(context.TODO(), metav1.ListOptions{
				LabelSelector: labelSelector,
			})
			if err != nil {
				return errors.Wrap(err, "error listing pods")
			}

			var podStatusStr string
			if len(pods.Items) == 0 {
				podStatusStr = "does not exist"
			} else {
				if pods.Items[0].Status.Phase == corev1.PodRunning {
					return status.Error(fmt.Errorf("the Submariner operator pod appears to be running but did not "+
						"complete deletion of the resource. Please check the pod logs"), "")
				}

				podStatusStr = fmt.Sprintf("is not running (status is %q)", pods.Items[0].Status.Phase)
			}

			status.Warning("The Submariner operator pod %s so deletion of the resource was not completed - "+
				"the resource will be force-deleted", podStatusStr)
		}

		err = finalizer.Remove(context.TODO(), resource.ForControllerClient(clients.ForGeneral(), obj.GetNamespace(), obj),
			obj, opnames.CleanupFinalizer)
		if err != nil {
			return err //nolint:wrapcheck // No need to wrap
		}

		return awaitDeleted()
	}

	return err
}

func deleteBrokerIfUnused(clients client.Producer, namespace, clusterName string, status reporter.Interface) (bool, error) {
	if namespace == "" {
		return true, nil
	}

	_, err := clients.ForKubernetes().CoreV1().Namespaces().Get(context.TODO(), namespace, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return true, nil
		}

		return false, status.Error(err, "Error retrieving broker namespace %q", namespace)
	}

	inUse, err := brokerInUse(clients.ForGeneral(), namespace, clusterName, status)
	if err != nil {
		return false, err
	}

	if inUse {
		return false, nil
	}

	status.Start("Deleting the broker namespace %q", namespace)
	defer status.End()

	err = clients.ForKubernetes().CoreV1().Namespaces().Delete(context.TODO(), namespace, metav1.DeleteOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return false, status.Error(err, "Error deleting the broker namespace")
	}

	return true, nil
}

func brokerInUse(controllerClient controller.Client, namespace, clusterName string, status reporter.Interface) (bool, error) {
	status.Start("Verifying broker namespace %q is not in use", namespace)
	defer status.End()

	endpoints := &submarinerv1.EndpointList{}

	err := controllerClient.List(context.TODO(), endpoints, controller.InNamespace(namespace))
	if err != nil {
		return false, status.Error(err, "error retrieving Endpoints")
	}

	var remoteClusterNames []string

	for i := range endpoints.Items {
		remoteClusterName := endpoints.Items[i].Spec.ClusterID
		if remoteClusterName != clusterName {
			remoteClusterNames = append(remoteClusterNames, remoteClusterName)
		}
	}

	if len(remoteClusterNames) > 0 {
		status.Warning("Broker namespace %q appears to be in use by other clusters (%v) - keeping the broker components.",
			namespace, remoteClusterNames)

		return true, nil
	}

	return false, nil
}

func findBrokerNamespace(controllerClient controller.Client, clusterName string, status reporter.Interface) (string, error) {
	status.Start("Checking if the broker component is installed on cluster %q", clusterName)
	defer status.End()

	brokers := &operatorv1alpha1.BrokerList{}

	err := controllerClient.List(context.TODO(), brokers, controller.InNamespace(metav1.NamespaceAll))
	if err != nil && !resource.IsNotFoundErr(err) {
		return "", status.Error(err, "Error listing broker resources")
	}

	for i := range brokers.Items {
		status.Success("The broker component is installed in namespace %q", brokers.Items[i].Namespace)

		return brokers.Items[i].Namespace, nil
	}

	status.Success("The broker component is not installed on cluster %q", clusterName)

	return "", nil
}
