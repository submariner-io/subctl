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
	"context"

	"github.com/submariner-io/admiral/pkg/resource"
	"github.com/submariner-io/subctl/pkg/apply"
	"github.com/submariner-io/subctl/pkg/clusterrole"
	"github.com/submariner-io/subctl/pkg/clusterrolebinding"
	"github.com/submariner-io/subctl/pkg/role"
	"github.com/submariner-io/subctl/pkg/rolebinding"
	"github.com/submariner-io/subctl/pkg/serviceaccount"
	submarinermetricsreader "github.com/submariner-io/submariner-operator/config/openshift/rbac/submariner-metrics-reader"
	submarinerdiagnose "github.com/submariner-io/submariner-operator/config/rbac/submariner-diagnose"
	submarinergateway "github.com/submariner-io/submariner-operator/config/rbac/submariner-gateway"
	submarinerglobalnet "github.com/submariner-io/submariner-operator/config/rbac/submariner-globalnet"
	submarinerrouteagent "github.com/submariner-io/submariner-operator/config/rbac/submariner-route-agent"
	"k8s.io/client-go/kubernetes"
)

// Ensure functions updates or installs the operator CRDs in the cluster.
func Ensure(ctx context.Context, kubeClient kubernetes.Interface, namespace string) (bool, error) {
	return apply.EmbeddedYAMLs(ctx, kubeClient, namespace, serviceAccountRelatedYAMLs) //nolint:wrapcheck // No need to wrap
}

var serviceAccountRelatedYAMLs = []apply.EmbeddedYAMLRefsApplier{
	{
		Applier: serviceaccount.EnsureFromYAML,
		Refs: []apply.EmbeddedYAMLRef{
			{Name: "gateway ServiceAccount", Content: submarinergateway.ServiceAccount},
			{Name: "route agent ServiceAccount", Content: submarinerrouteagent.ServiceAccount},
			{Name: "globalnet ServiceAccount", Content: submarinerglobalnet.ServiceAccount},
			{Name: "diagnose ServiceAccount", Content: submarinerdiagnose.ServiceAccount},
		},
	},
	{
		Applier: func(ctx context.Context, kubeClient kubernetes.Interface, _ string, yaml []byte) (bool, error) {
			return clusterrole.EnsureFromYAML(ctx, kubeClient, yaml)
		},
		Refs: []apply.EmbeddedYAMLRef{
			{Name: "gateway ClusterRole", Content: submarinergateway.ClusterRole},
			{Name: "route agent ClusterRole", Content: submarinerrouteagent.ClusterRole},
			{Name: "globalnet ClusterRole", Content: submarinerglobalnet.ClusterRole},
			{Name: "diagnose ClusterRole", Content: submarinerdiagnose.ClusterRole},
		},
	},
	{
		Applier: clusterrolebinding.EnsureFromYAML,
		Refs: []apply.EmbeddedYAMLRef{
			{Name: "gateway ClusterRoleBinding", Content: submarinergateway.ClusterRoleBinding},
			{Name: "route agent ClusterRoleBinding", Content: submarinerrouteagent.ClusterRoleBinding},
			{Name: "globalnet ClusterRoleBinding", Content: submarinerglobalnet.ClusterRoleBinding},
			{Name: "diagnose ClusterRoleBinding", Content: submarinerdiagnose.ClusterRoleBinding},
		},
	},
	{
		Applier: role.EnsureFromYAML,
		Refs: []apply.EmbeddedYAMLRef{
			{Name: "gateway Role", Content: submarinergateway.Role},
			{Name: "route agent Role", Content: submarinerrouteagent.Role},
			{Name: "globalnet Role", Content: submarinerglobalnet.Role},
			{Name: "diagnose Role", Content: submarinerdiagnose.Role},
			{Name: "metrics reader Role", Content: submarinermetricsreader.Role},
		},
	},
	{
		Applier: func(ctx context.Context, kubeClient kubernetes.Interface, namespace string, yaml []byte) (bool, error) {
			created, err := rolebinding.EnsureFromYAML(ctx, kubeClient, namespace, yaml)

			// If a RoleBinding has its own namespace, consider that as a gate: if the namespace
			// doesn't exist, the RoleBinding shouldn't be created, so namespace errors on
			// RoleBinding-specified namespaces are ignored
			if resource.IsMissingNamespaceErr(err) && resource.ExtractMissingNamespaceFromErr(err) != namespace {
				err = nil
			}

			return created, err
		},
		Refs: []apply.EmbeddedYAMLRef{
			{Name: "gateway RoleBinding", Content: submarinergateway.RoleBinding},
			{Name: "route agent RoleBinding", Content: submarinerrouteagent.RoleBinding},
			{Name: "globalnet RoleBinding", Content: submarinerglobalnet.RoleBinding},
			{Name: "diagnose RoleBinding", Content: submarinerdiagnose.RoleBinding},
			{Name: "metrics reader RoleBinding", Content: submarinermetricsreader.RoleBinding},
		},
	},
}
