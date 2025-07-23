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
	submarinermetricsreader "github.com/submariner-io/submariner-operator/config/openshift/rbac/submariner-metrics-reader"
	submarinerdiagnose "github.com/submariner-io/submariner-operator/config/rbac/submariner-diagnose"
	submarinergateway "github.com/submariner-io/submariner-operator/config/rbac/submariner-gateway"
	submarinerglobalnet "github.com/submariner-io/submariner-operator/config/rbac/submariner-globalnet"
	submarinerrouteagent "github.com/submariner-io/submariner-operator/config/rbac/submariner-route-agent"
	"golang.org/x/net/context"
	"k8s.io/client-go/kubernetes"
)

// Ensure functions updates or installs the operator CRDs in the cluster.
func Ensure(ctx context.Context, kubeClient kubernetes.Interface, namespace string) (bool, error) {
	created := false

	for _, applier := range serviceAccountRelatedYAMLs {
		for _, ref := range applier.refs {
			iterCreated, err := applier.applier(ctx, kubeClient, namespace, ref.content)
			if err != nil {
				return created, err
			}

			created = created || iterCreated
		}
	}

	return created, nil
}

type embeddedYAMLRef struct {
	name    string
	content []byte
}

type embeddedYAMLRefsApplier struct {
	applier func(ctx context.Context, kubeClient kubernetes.Interface, namespace string, yaml []byte) (bool, error)
	refs    []embeddedYAMLRef
}

var serviceAccountRelatedYAMLs = []embeddedYAMLRefsApplier{
	{
		serviceaccount.EnsureFromYAML,
		[]embeddedYAMLRef{
			{"gateway ServiceAccount", submarinergateway.ServiceAccount},
			{"route agent ServiceAccount", submarinerrouteagent.ServiceAccount},
			{"globalnet ServiceAccount", submarinerglobalnet.ServiceAccount},
			{"diagnose ServiceAccount", submarinerdiagnose.ServiceAccount},
		},
	},
	{
		func(ctx context.Context, kubeClient kubernetes.Interface, _ string, yaml []byte) (bool, error) {
			return clusterrole.EnsureFromYAML(ctx, kubeClient, yaml)
		},
		[]embeddedYAMLRef{
			{"gateway ClusterRole", submarinergateway.ClusterRole},
			{"route agent ClusterRole", submarinerrouteagent.ClusterRole},
			{"globalnet ClusterRole", submarinerglobalnet.ClusterRole},
			{"diagnose ClusterRole", submarinerdiagnose.ClusterRole},
		},
	},
	{
		clusterrolebinding.EnsureFromYAML,
		[]embeddedYAMLRef{
			{"gateway ClusterRoleBinding", submarinergateway.ClusterRoleBinding},
			{"route agent ClusterRoleBinding", submarinerrouteagent.ClusterRoleBinding},
			{"globalnet ClusterRoleBinding", submarinerglobalnet.ClusterRoleBinding},
			{"diagnose ClusterRoleBinding", submarinerdiagnose.ClusterRoleBinding},
		},
	},
	{
		role.EnsureFromYAML,
		[]embeddedYAMLRef{
			{"gateway Role", submarinergateway.Role},
			{"route agent Role", submarinerrouteagent.Role},
			{"globalnet Role", submarinerglobalnet.Role},
			{"diagnose Role", submarinerdiagnose.Role},
			{"metrics reader Role", submarinermetricsreader.Role},
		},
	},
	{
		func(ctx context.Context, kubeClient kubernetes.Interface, namespace string, yaml []byte) (bool, error) {
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
			{"gateway RoleBinding", submarinergateway.RoleBinding},
			{"route agent RoleBinding", submarinerrouteagent.RoleBinding},
			{"globalnet RoleBinding", submarinerglobalnet.RoleBinding},
			{"diagnose RoleBinding", submarinerdiagnose.RoleBinding},
			{"metrics reader RoleBinding", submarinermetricsreader.RoleBinding},
		},
	},
}
