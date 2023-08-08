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

package lighthouse

import (
	"github.com/submariner-io/admiral/pkg/names"
	"github.com/submariner-io/admiral/pkg/reporter"
	"github.com/submariner-io/subctl/pkg/lighthouse/serviceaccount"
	"github.com/submariner-io/subctl/pkg/operator/ocp"
	"github.com/submariner-io/submariner-operator/pkg/embeddedyamls"
	"golang.org/x/net/context"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

func Ensure(ctx context.Context, status reporter.Interface, kubeClient kubernetes.Interface, dynClient dynamic.Interface,
	operatorNamespace string,
) error {
	if created, err := serviceaccount.Ensure(ctx, kubeClient, operatorNamespace); err != nil {
		return err //nolint:wrapcheck // No need to wrap here
	} else if created {
		status.Success("Created lighthouse service account and role")
	}

	componentsRbac := []ocp.RbacInfo{
		{
			ComponentName:          names.ServiceDiscoveryComponent,
			ClusterRoleFile:        embeddedyamls.Config_rbac_lighthouse_agent_ocp_cluster_role_yaml,
			ClusterRoleBindingFile: embeddedyamls.Config_rbac_lighthouse_agent_ocp_cluster_role_binding_yaml,
		},
		{
			ComponentName:          names.LighthouseCoreDNSComponent,
			ClusterRoleFile:        embeddedyamls.Config_rbac_lighthouse_coredns_ocp_cluster_role_yaml,
			ClusterRoleBindingFile: embeddedyamls.Config_rbac_lighthouse_coredns_ocp_cluster_role_binding_yaml,
		},
	}

	if created, err := ocp.EnsureRBAC(ctx, dynClient, kubeClient, operatorNamespace, componentsRbac); err != nil {
		return err //nolint:wrapcheck // No need to wrap here
	} else if created {
		status.Success("Updated the OCP roles")
	}

	return nil
}
