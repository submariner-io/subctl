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

//nolint:dupl // Similar code in ensureClusterRoleBindings, ensureRoles, ensureRoleBindings but not duplicated
func ensureServiceAccounts(ctx context.Context, kubeClient kubernetes.Interface, namespace string) (bool, error) {
	createdSubmarinerSA, err := serviceaccount.EnsureFromYAML(ctx, kubeClient, namespace,
		embeddedyamls.Config_rbac_submariner_gateway_service_account_yaml)
	if err != nil {
		return false, errors.Wrap(err, "error provisioning gateway ServiceAccount resource")
	}

	createdRouteAgentSA, err := serviceaccount.EnsureFromYAML(ctx, kubeClient, namespace,
		embeddedyamls.Config_rbac_submariner_route_agent_service_account_yaml)
	if err != nil {
		return false, errors.Wrap(err, "error provisioning route agent ServiceAccount resource")
	}

	createdGlobalnetSA, err := serviceaccount.EnsureFromYAML(ctx, kubeClient, namespace,
		embeddedyamls.Config_rbac_submariner_globalnet_service_account_yaml)
	if err != nil {
		return false, errors.Wrap(err, "error provisioning globalnet ServiceAccount resource")
	}

	createdDiagnoseSA, err := serviceaccount.EnsureFromYAML(ctx, kubeClient, namespace,
		embeddedyamls.Config_rbac_submariner_diagnose_service_account_yaml)
	if err != nil {
		return false, errors.Wrap(err, "error provisioning diagnose ServiceAccount resource")
	}

	createdNPSyncerSA, err := serviceaccount.EnsureFromYAML(ctx, kubeClient, namespace,
		embeddedyamls.Config_rbac_networkplugin_syncer_service_account_yaml)

	return createdSubmarinerSA || createdRouteAgentSA || createdGlobalnetSA || createdNPSyncerSA || createdDiagnoseSA,
		errors.Wrap(err, "error provisioning operator networkplugin syncer resource")
}

func ensureClusterRoles(ctx context.Context, kubeClient kubernetes.Interface) (bool, error) {
	createdSubmarinerCR, err := clusterrole.EnsureFromYAML(ctx, kubeClient, embeddedyamls.Config_rbac_submariner_gateway_cluster_role_yaml)
	if err != nil {
		return false, errors.Wrap(err, "error provisioning gateway ClusterRole resource")
	}

	createdRouteAgentCR, err := clusterrole.EnsureFromYAML(ctx, kubeClient, embeddedyamls.Config_rbac_submariner_route_agent_cluster_role_yaml)
	if err != nil {
		return false, errors.Wrap(err, "error provisioning route agent ClusterRole resource")
	}

	createdGlobalnetCR, err := clusterrole.EnsureFromYAML(ctx, kubeClient, embeddedyamls.Config_rbac_submariner_globalnet_cluster_role_yaml)
	if err != nil {
		return false, errors.Wrap(err, "error provisioning globalnet ClusterRole resource")
	}

	createdDiagnoseCR, err := clusterrole.EnsureFromYAML(ctx, kubeClient, embeddedyamls.Config_rbac_submariner_diagnose_cluster_role_yaml)
	if err != nil {
		return false, errors.Wrap(err, "error provisioning diagnose ClusterRole resource")
	}

	createdNPSyncerCR, err := clusterrole.EnsureFromYAML(ctx, kubeClient, embeddedyamls.Config_rbac_networkplugin_syncer_cluster_role_yaml)

	return createdSubmarinerCR || createdRouteAgentCR || createdGlobalnetCR || createdNPSyncerCR || createdDiagnoseCR,
		errors.Wrap(err, "error provisioning networkplugin syncer ClusterRole resource")
}

//nolint:dupl // Similar code in ensureServiceAccounts, ensureRoles, ensureRoleBindings but not duplicated
func ensureClusterRoleBindings(ctx context.Context, kubeClient kubernetes.Interface, namespace string) (bool, error) {
	createdSubmarinerCRB, err := clusterrolebinding.EnsureFromYAML(ctx, kubeClient, namespace,
		embeddedyamls.Config_rbac_submariner_gateway_cluster_role_binding_yaml)
	if err != nil {
		return false, errors.Wrap(err, "error provisioning gateway ClusterRoleBinding resource")
	}

	createdRouteAgentCRB, err := clusterrolebinding.EnsureFromYAML(ctx, kubeClient, namespace,
		embeddedyamls.Config_rbac_submariner_route_agent_cluster_role_binding_yaml)
	if err != nil {
		return false, errors.Wrap(err, "error provisioning route agent ClusterRoleBinding resource")
	}

	createdGlobalnetCRB, err := clusterrolebinding.EnsureFromYAML(ctx, kubeClient, namespace,
		embeddedyamls.Config_rbac_submariner_globalnet_cluster_role_binding_yaml)
	if err != nil {
		return false, errors.Wrap(err, "error provisioning globalnet ClusterRoleBinding resource")
	}

	createdDiagnoseCRB, err := clusterrolebinding.EnsureFromYAML(ctx, kubeClient, namespace,
		embeddedyamls.Config_rbac_submariner_diagnose_cluster_role_binding_yaml)
	if err != nil {
		return false, errors.Wrap(err, "error provisioning diagnose ClusterRoleBinding resource")
	}

	createdNPSyncerCRB, err := clusterrolebinding.EnsureFromYAML(ctx, kubeClient, namespace,
		embeddedyamls.Config_rbac_networkplugin_syncer_cluster_role_binding_yaml)

	return createdSubmarinerCRB || createdRouteAgentCRB || createdGlobalnetCRB || createdNPSyncerCRB || createdDiagnoseCRB,
		errors.Wrap(err, "error provisioning networkplugin syncer ClusterRoleBinding resource")
}

//nolint:dupl // Similar code in ensureServiceAccounts, ensureClusterRoleBindings, ensureRoleBindings but not duplicated
func ensureRoles(ctx context.Context, kubeClient kubernetes.Interface, namespace string) (bool, error) {
	createdSubmarinerRole, err := role.EnsureFromYAML(ctx, kubeClient, namespace,
		embeddedyamls.Config_rbac_submariner_gateway_role_yaml)
	if err != nil {
		return false, errors.Wrap(err, "error provisioning gateway Role resource")
	}

	createdRouteAgentRole, err := role.EnsureFromYAML(ctx, kubeClient, namespace,
		embeddedyamls.Config_rbac_submariner_route_agent_role_yaml)
	if err != nil {
		return false, errors.Wrap(err, "error provisioning route agent Role resource")
	}

	createdGlobalnetRole, err := role.EnsureFromYAML(ctx, kubeClient, namespace,
		embeddedyamls.Config_rbac_submariner_globalnet_role_yaml)
	if err != nil {
		return false, errors.Wrap(err, "error provisioning globalnet Role resource")
	}

	createdDiagnoseRole, err := role.EnsureFromYAML(ctx, kubeClient, namespace,
		embeddedyamls.Config_rbac_submariner_diagnose_role_yaml)
	if err != nil {
		return false, errors.Wrap(err, "error provisioning operator Role resource")
	}

	createdMetricsReaderRole, err := role.EnsureFromYAML(ctx, kubeClient, namespace,
		embeddedyamls.Config_openshift_rbac_submariner_metrics_reader_role_yaml)

	return createdSubmarinerRole || createdRouteAgentRole || createdGlobalnetRole || createdMetricsReaderRole || createdDiagnoseRole,
		errors.Wrap(err, "error provisioning _metrics reader Role resource")
}

//nolint:dupl // Similar code in ensureServiceAccounts, ensureClusterRoleBindings, ensureRoles but not duplicated
func ensureRoleBindings(ctx context.Context, kubeClient kubernetes.Interface, namespace string) (bool, error) {
	createdSubmarinerRB, err := rolebinding.EnsureFromYAML(ctx, kubeClient, namespace,
		embeddedyamls.Config_rbac_submariner_gateway_role_binding_yaml)
	if err != nil {
		return false, errors.Wrap(err, "error provisioning gateway RoleBinding resource")
	}

	createdRouteAgentRB, err := rolebinding.EnsureFromYAML(ctx, kubeClient, namespace,
		embeddedyamls.Config_rbac_submariner_route_agent_role_binding_yaml)
	if err != nil {
		return false, errors.Wrap(err, "error provisioning route agent RoleBinding resource")
	}

	createdGlobalnetRB, err := rolebinding.EnsureFromYAML(ctx, kubeClient, namespace,
		embeddedyamls.Config_rbac_submariner_globalnet_role_binding_yaml)
	if err != nil {
		return false, errors.Wrap(err, "error provisioning globalnet RoleBinding resource")
	}

	createdDiagnoseRB, err := rolebinding.EnsureFromYAML(ctx, kubeClient, namespace,
		embeddedyamls.Config_rbac_submariner_diagnose_role_binding_yaml)
	if err != nil {
		return false, errors.Wrap(err, "error provisioning diagnose RoleBinding resource")
	}

	createdMetricsReaderRB, err := rolebinding.EnsureFromYAML(ctx, kubeClient, namespace,
		embeddedyamls.Config_openshift_rbac_submariner_metrics_reader_role_binding_yaml)

	return createdSubmarinerRB || createdRouteAgentRB || createdGlobalnetRB || createdMetricsReaderRB || createdDiagnoseRB,
		errors.Wrap(err, "error provisioning metrics reader RoleBinding resource")
}
