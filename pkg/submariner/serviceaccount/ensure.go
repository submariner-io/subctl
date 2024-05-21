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
	"github.com/submariner-io/admiral/pkg/resource"
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
	created := false

	for _, applier := range serviceAccountRelatedYAMLs {
		for _, ref := range applier.refs {
			iterCreated, err := applier.applier(ctx, kubeClient, namespace, ref.name)
			if err != nil {
				return created, err
			}

			created = created || iterCreated
		}
	}

	return created, nil
}

type embeddedYAMLRef struct {
	name        string
	description string
}

type embeddedYAMLRefsApplier struct {
	applier func(ctx context.Context, kubeClient kubernetes.Interface, namespace, yaml string) (bool, error)
	refs    []embeddedYAMLRef
}

var serviceAccountRelatedYAMLs = []embeddedYAMLRefsApplier{
	{
		serviceaccount.EnsureFromYAML,
		[]embeddedYAMLRef{
			{embeddedyamls.Config_rbac_submariner_gateway_service_account_yaml, "gateway ServiceAccount"},
			{embeddedyamls.Config_rbac_submariner_route_agent_service_account_yaml, "route agent ServiceAccount"},
			{embeddedyamls.Config_rbac_submariner_globalnet_service_account_yaml, "globalnet ServiceAccount"},
			{embeddedyamls.Config_rbac_submariner_diagnose_service_account_yaml, "diagnose ServiceAccount"},
		},
	},
	{
		func(ctx context.Context, kubeClient kubernetes.Interface, _, yaml string) (bool, error) {
			return clusterrole.EnsureFromYAML(ctx, kubeClient, yaml)
		},
		[]embeddedYAMLRef{
			{embeddedyamls.Config_rbac_submariner_gateway_cluster_role_yaml, "gateway ClusterRole"},
			{embeddedyamls.Config_rbac_submariner_route_agent_cluster_role_yaml, "route agent ClusterRole"},
			{embeddedyamls.Config_rbac_submariner_globalnet_cluster_role_yaml, "globalnet ClusterRole"},
			{embeddedyamls.Config_rbac_submariner_diagnose_cluster_role_yaml, "diagnose ClusterRole"},
		},
	},
	{
		clusterrolebinding.EnsureFromYAML,
		[]embeddedYAMLRef{
			{embeddedyamls.Config_rbac_submariner_gateway_cluster_role_binding_yaml, "gateway ClusterRoleBinding"},
			{embeddedyamls.Config_rbac_submariner_route_agent_cluster_role_binding_yaml, "route agent ClusterRoleBinding"},
			{embeddedyamls.Config_rbac_submariner_globalnet_cluster_role_binding_yaml, "globalnet ClusterRoleBinding"},
			{embeddedyamls.Config_rbac_submariner_diagnose_cluster_role_binding_yaml, "diagnose ClusterRoleBinding"},
		},
	},
	{
		role.EnsureFromYAML,
		[]embeddedYAMLRef{
			{embeddedyamls.Config_rbac_submariner_gateway_role_yaml, "gateway Role"},
			{embeddedyamls.Config_rbac_submariner_route_agent_role_yaml, "route agent Role"},
			{embeddedyamls.Config_rbac_submariner_globalnet_role_yaml, "globalnet Role"},
			{embeddedyamls.Config_rbac_submariner_diagnose_role_yaml, "diagnose Role"},
			{embeddedyamls.Config_openshift_rbac_submariner_metrics_reader_role_yaml, "metrics reader Role"},
		},
	},
	{
		func(ctx context.Context, kubeClient kubernetes.Interface, namespace, yaml string) (bool, error) {
			created, err := rolebinding.EnsureFromYAML(ctx, kubeClient, namespace, yaml)

			// If a RoleBinding has its own namespace, consider that as a gate: if the namespace
			// doesn't exist, the RoleBinding shouldn't be created, so namespace errors on
			// RoleBinding-specified namespaces are ignored
			if resource.IsMissingNamespaceErr(err) && resource.ExtractMissingNamespaceFromErr(err) != namespace {
				err = nil
			}

			return created, err
		},
		[]embeddedYAMLRef{
			{embeddedyamls.Config_rbac_submariner_gateway_role_binding_yaml, "gateway RoleBinding"},
			{embeddedyamls.Config_rbac_submariner_route_agent_role_binding_yaml, "route agent RoleBinding"},
			{embeddedyamls.Config_rbac_submariner_globalnet_role_binding_yaml, "globalnet RoleBinding"},
			{embeddedyamls.Config_rbac_submariner_diagnose_role_binding_yaml, "diagnose RoleBinding"},
			{embeddedyamls.Config_openshift_rbac_submariner_metrics_reader_role_binding_yaml, "metrics reader RoleBinding"},
		},
	},
}
