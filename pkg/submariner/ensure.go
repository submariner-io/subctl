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

package submariner

import (
	"github.com/submariner-io/admiral/pkg/reporter"
	"github.com/submariner-io/subctl/pkg/operator/ocp"
	"github.com/submariner-io/subctl/pkg/submariner/serviceaccount"
	"github.com/submariner-io/submariner-operator/pkg/embeddedyamls"
	"github.com/submariner-io/submariner-operator/pkg/names"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

func Ensure(status reporter.Interface, kubeClient kubernetes.Interface, dynClient dynamic.Interface,
	operatorNamespace string,
) error {
	if created, err := serviceaccount.Ensure(kubeClient, operatorNamespace); err != nil {
		return err //nolint:wrapcheck // No need to wrap here
	} else if created {
		status.Success("Created submariner service account and role")
	}

	componentsRbac := []ocp.RbacInfo{
		{
			ComponentName:          names.GatewayComponent,
			ClusterRoleFile:        embeddedyamls.Config_rbac_submariner_gateway_ocp_cluster_role_yaml,
			ClusterRoleBindingFile: embeddedyamls.Config_rbac_submariner_gateway_ocp_cluster_role_binding_yaml,
		},
		{
			ComponentName:          names.RouteAgentComponent,
			ClusterRoleFile:        embeddedyamls.Config_rbac_submariner_route_agent_ocp_cluster_role_yaml,
			ClusterRoleBindingFile: embeddedyamls.Config_rbac_submariner_route_agent_ocp_cluster_role_binding_yaml,
		},
		{
			ComponentName:          names.GatewayComponent,
			ClusterRoleFile:        embeddedyamls.Config_rbac_submariner_globalnet_ocp_cluster_role_yaml,
			ClusterRoleBindingFile: embeddedyamls.Config_rbac_submariner_globalnet_ocp_cluster_role_binding_yaml,
		},
		{
			ComponentName:          "submariner-diagnose",
			ClusterRoleFile:        embeddedyamls.Config_rbac_submariner_diagnose_ocp_cluster_role_yaml,
			ClusterRoleBindingFile: embeddedyamls.Config_rbac_submariner_diagnose_ocp_cluster_role_binding_yaml,
		},
		{
			ComponentName:          names.NetworkPluginSyncerComponent,
			ClusterRoleFile:        embeddedyamls.Config_rbac_networkplugin_syncer_ocp_cluster_role_yaml,
			ClusterRoleBindingFile: embeddedyamls.Config_rbac_networkplugin_syncer_ocp_cluster_role_binding_yaml,
		},
	}

	if created, err := ocp.EnsureRBAC(dynClient, kubeClient, operatorNamespace, componentsRbac); err != nil {
		return err //nolint:wrapcheck // No need to wrap here
	} else if created {
		status.Success("Updated the OCP roles")
	}

	return nil
}
