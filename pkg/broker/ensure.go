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

package broker

import (
	"context"

	"github.com/pkg/errors"
	"github.com/submariner-io/subctl/internal/component"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/pkg/gateway"
	"github.com/submariner-io/subctl/pkg/namespace"
	"github.com/submariner-io/subctl/pkg/role"
	"github.com/submariner-io/subctl/pkg/rolebinding"
	"github.com/submariner-io/subctl/pkg/serviceaccount"
	"github.com/submariner-io/submariner-operator/pkg/crd"
	"github.com/submariner-io/submariner-operator/pkg/embeddedyamls"
	"github.com/submariner-io/submariner-operator/pkg/lighthouse"
	"github.com/submariner-io/submariner-operator/pkg/names"
	v1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func Ensure(ctx context.Context, crdUpdater crd.Updater, kubeClient kubernetes.Interface, componentArr []string, createCRDs bool,
	brokerNS string,
) error {
	if createCRDs {
		for i := range componentArr {
			switch componentArr[i] {
			case component.Connectivity:
				err := gateway.Ensure(ctx, crdUpdater)
				if err != nil {
					return errors.Wrap(err, "error setting up the connectivity requirements")
				}
			case component.ServiceDiscovery:
				_, err := lighthouse.Ensure(ctx, crdUpdater, lighthouse.BrokerCluster)
				if err != nil {
					return errors.Wrap(err, "error setting up the service discovery requirements")
				}
			case component.Globalnet:
				// Globalnet needs the Lighthouse CRDs too
				_, err := lighthouse.Ensure(ctx, crdUpdater, lighthouse.BrokerCluster)
				if err != nil {
					return errors.Wrap(err, "error setting up the globalnet requirements")
				}
			}
		}
	}

	brokerNamespaceLabels := map[string]string{}

	// Create the namespace
	_, err := namespace.Ensure(ctx, kubeClient, brokerNS, brokerNamespaceLabels)
	if err != nil {
		return err //nolint:wrapcheck // No need to wrap here
	}

	// Create administrator SA, Role, and bind them
	if err := createBrokerAdministratorRoleAndSA(ctx, kubeClient, brokerNS); err != nil {
		return err
	}

	// Create the broker cluster role, which will also be used by any new enrolled cluster
	_, err = CreateOrUpdateClusterBrokerRole(ctx, kubeClient, brokerNS)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return errors.Wrap(err, "error creating broker role")
	}

	return nil
}

// CreateSAForCluster creates a new SA for each new cluster joined and binds it to the submariner cluster role.
func CreateSAForCluster(ctx context.Context, kubeClient kubernetes.Interface, clusterID, inNamespace string) (*v1.Secret, error) {
	saName := names.ForClusterSA(clusterID)

	err := CreateNewBrokerSA(ctx, kubeClient, saName, inNamespace)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return nil, errors.Wrap(err, "error creating cluster sa")
	}

	_, err = CreateNewBrokerRoleBinding(ctx, kubeClient, saName, submarinerBrokerClusterRole, inNamespace)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return nil, errors.Wrap(err, "error binding sa to cluster role")
	}

	clientToken, err := serviceaccount.EnsureTokenSecret(ctx, kubeClient, inNamespace, saName)
	if err != nil {
		return nil, errors.Wrap(err, "error getting cluster sa token")
	}

	return clientToken, nil
}

func createBrokerAdministratorRoleAndSA(ctx context.Context, kubeClient kubernetes.Interface, inNamespace string) error {
	// Create the SA we need for the managing the broker (from subctl, etc..).
	err := CreateNewBrokerAdminSA(ctx, kubeClient, inNamespace)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return errors.Wrap(err, "error creating the broker admin service account")
	}

	// Create the broker admin role
	_, err = CreateOrUpdateBrokerAdminRole(ctx, kubeClient, inNamespace)
	if err != nil {
		return errors.Wrap(err, "error creating subctl role")
	}

	// Create the role binding
	_, err = CreateOrUpdateBrokerAdminRoleBinding(ctx, kubeClient, inNamespace)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return errors.Wrap(err, "error creating the broker rolebinding")
	}

	return nil
}

//nolint:wrapcheck // No need to wrap here
func CreateOrUpdateClusterBrokerRole(ctx context.Context, kubeClient kubernetes.Interface, inNamespace string) (bool, error) {
	return role.EnsureFromYAML(ctx, kubeClient, inNamespace, embeddedyamls.Config_broker_broker_client_role_yaml)
}

//nolint:wrapcheck // No need to wrap here
func CreateOrUpdateBrokerAdminRole(ctx context.Context, clientset kubernetes.Interface, inNamespace string) (created bool, err error) {
	return role.EnsureFromYAML(ctx, clientset, inNamespace, embeddedyamls.Config_broker_broker_admin_role_yaml)
}

//nolint:wrapcheck // No need to wrap here
func CreateNewBrokerRoleBinding(ctx context.Context, kubeClient kubernetes.Interface, serviceAccount, roleName, inNamespace string) (
	brokerRoleBinding *rbacv1.RoleBinding, err error,
) {
	return kubeClient.RbacV1().RoleBindings(inNamespace).Create(
		ctx, NewBrokerRoleBinding(serviceAccount, roleName, inNamespace), metav1.CreateOptions{})
}

//nolint:wrapcheck // No need to wrap here
func CreateOrUpdateBrokerAdminRoleBinding(ctx context.Context, kubeClient kubernetes.Interface, inNamespace string) (
	created bool, err error,
) {
	return rolebinding.EnsureFromYAML(ctx, kubeClient, inNamespace, embeddedyamls.Config_broker_broker_admin_role_binding_yaml)
}

//nolint:wrapcheck // No need to wrap here
func CreateNewBrokerSA(ctx context.Context, kubeClient kubernetes.Interface, submarinerBrokerSA, inNamespace string) (err error) {
	sa := NewBrokerSA(submarinerBrokerSA)
	_, err = serviceaccount.Ensure(ctx, kubeClient, inNamespace, sa)

	return err
}

//nolint:wrapcheck // No need to wrap here
func CreateNewBrokerAdminSA(ctx context.Context, kubeClient kubernetes.Interface, inNamespace string) (err error) {
	_, err = serviceaccount.EnsureFromYAML(ctx, kubeClient, inNamespace, embeddedyamls.Config_broker_broker_admin_service_account_yaml)
	if err != nil {
		return err
	}

	_, err = serviceaccount.EnsureTokenSecret(ctx, kubeClient, inNamespace, constants.SubmarinerBrokerAdminSA)

	return err
}
