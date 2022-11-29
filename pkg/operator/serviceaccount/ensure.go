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
	"golang.org/x/net/context"
	"k8s.io/client-go/kubernetes"
)

// Ensure functions updates or installs the operator CRDs in the cluster.
func Ensure(ctx context.Context, kubeClient kubernetes.Interface, namespace string) (bool, error) {
	createdSA, err := ensureServiceAccounts(ctx, kubeClient, namespace)
	if err != nil {
		return false, err
	}

	createdRole, err := ensureRoles(ctx, kubeClient, namespace)
	if err != nil {
		return false, err
	}

	createdRB, err := ensureRoleBindings(ctx, kubeClient, namespace)
	if err != nil {
		return false, err
	}

	createdCR, err := ensureClusterRoles(ctx, kubeClient)
	if err != nil {
		return false, err
	}

	createdCRB, err := ensureClusterRoleBindings(ctx, kubeClient, namespace)
	if err != nil {
		return false, err
	}

	return createdSA || createdRole || createdRB || createdCR || createdCRB, nil
}

func ensureServiceAccounts(ctx context.Context, kubeClient kubernetes.Interface, namespace string) (bool, error) {
	createdOperatorSA, err := serviceaccount.EnsureFromYAML(ctx, kubeClient, namespace,
		embeddedyamls.Config_rbac_submariner_operator_service_account_yaml)
	return createdOperatorSA, errors.Wrap(err, "error provisioning operator ServiceAccount resource")
}

func ensureClusterRoles(ctx context.Context, kubeClient kubernetes.Interface) (bool, error) {
	createdOperatorCR, err := clusterrole.EnsureFromYAML(ctx, kubeClient, embeddedyamls.Config_rbac_submariner_operator_cluster_role_yaml)
	return createdOperatorCR, errors.Wrap(err, "error provisioning operator ClusterRole resource")
}

func ensureClusterRoleBindings(ctx context.Context, kubeClient kubernetes.Interface, namespace string) (bool, error) {
	createdOperatorCRB, err := clusterrolebinding.EnsureFromYAML(ctx, kubeClient, namespace,
		embeddedyamls.Config_rbac_submariner_operator_cluster_role_binding_yaml)
	return createdOperatorCRB, errors.Wrap(err, "error provisioning operator ClusterRoleBinding resource")
}

func ensureRoles(ctx context.Context, kubeClient kubernetes.Interface, namespace string) (bool, error) {
	createdOperatorRole, err := role.EnsureFromYAML(ctx, kubeClient, namespace,
		embeddedyamls.Config_rbac_submariner_operator_role_yaml)
	return createdOperatorRole, errors.Wrap(err, "error provisioning operator Role resource")
}

func ensureRoleBindings(ctx context.Context, kubeClient kubernetes.Interface, namespace string) (bool, error) {
	createdOperatorRB, err := rolebinding.EnsureFromYAML(ctx, kubeClient, namespace,
		embeddedyamls.Config_rbac_submariner_operator_role_binding_yaml)
	return createdOperatorRB, errors.Wrap(err, "error provisioning operator RoleBinding resource")
}
