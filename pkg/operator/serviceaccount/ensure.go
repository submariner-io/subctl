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

package serviceaccount

import (
	"github.com/pkg/errors"
	"github.com/submariner-io/subctl/pkg/clusterrole"
	"github.com/submariner-io/subctl/pkg/clusterrolebinding"
	"github.com/submariner-io/subctl/pkg/role"
	"github.com/submariner-io/subctl/pkg/rolebinding"
	"github.com/submariner-io/subctl/pkg/serviceaccount"
	"github.com/submariner-io/submariner-operator/pkg/embeddedyamls"
	"k8s.io/client-go/kubernetes"
)

// Ensure functions updates or installs the operator CRDs in the cluster.
func Ensure(kubeClient kubernetes.Interface, namespace string) (bool, error) {
	createdSA, err := ensureServiceAccounts(kubeClient, namespace)
	if err != nil {
		return false, err
	}

	createdRole, err := ensureRoles(kubeClient, namespace)
	if err != nil {
		return false, err
	}

	createdRB, err := ensureRoleBindings(kubeClient, namespace)
	if err != nil {
		return false, err
	}

	createdCR, err := ensureClusterRoles(kubeClient)
	if err != nil {
		return false, err
	}

	createdCRB, err := ensureClusterRoleBindings(kubeClient, namespace)
	if err != nil {
		return false, err
	}

	return createdSA || createdRole || createdRB || createdCR || createdCRB, nil
}

func ensureServiceAccounts(kubeClient kubernetes.Interface, namespace string) (bool, error) {
	createdOperatorSA, err := serviceaccount.EnsureFromYAML(kubeClient, namespace,
		embeddedyamls.Config_rbac_submariner_operator_service_account_yaml)
	return createdOperatorSA, errors.Wrap(err, "error provisioning operator ServiceAccount resource")
}

func ensureClusterRoles(kubeClient kubernetes.Interface) (bool, error) {
	createdOperatorCR, err := clusterrole.EnsureFromYAML(kubeClient, embeddedyamls.Config_rbac_submariner_operator_cluster_role_yaml)
	return createdOperatorCR, errors.Wrap(err, "error provisioning operator ClusterRole resource")
}

func ensureClusterRoleBindings(kubeClient kubernetes.Interface, namespace string) (bool, error) {
	createdOperatorCRB, err := clusterrolebinding.EnsureFromYAML(kubeClient, namespace,
		embeddedyamls.Config_rbac_submariner_operator_cluster_role_binding_yaml)
	return createdOperatorCRB, errors.Wrap(err, "error provisioning operator ClusterRoleBinding resource")
}

func ensureRoles(kubeClient kubernetes.Interface, namespace string) (bool, error) {
	createdOperatorRole, err := role.EnsureFromYAML(kubeClient, namespace,
		embeddedyamls.Config_rbac_submariner_operator_role_yaml)
	return createdOperatorRole, errors.Wrap(err, "error provisioning operator Role resource")
}

func ensureRoleBindings(kubeClient kubernetes.Interface, namespace string) (bool, error) {
	createdOperatorRB, err := rolebinding.EnsureFromYAML(kubeClient, namespace,
		embeddedyamls.Config_rbac_submariner_operator_role_binding_yaml)
	return createdOperatorRB, errors.Wrap(err, "error provisioning operator RoleBinding resource")
}
