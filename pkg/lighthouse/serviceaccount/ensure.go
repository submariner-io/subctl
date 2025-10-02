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
	"github.com/submariner-io/subctl/pkg/apply"
	"github.com/submariner-io/subctl/pkg/clusterrole"
	"github.com/submariner-io/subctl/pkg/clusterrolebinding"
	"github.com/submariner-io/subctl/pkg/role"
	"github.com/submariner-io/subctl/pkg/rolebinding"
	"github.com/submariner-io/subctl/pkg/serviceaccount"
	lighthouseagent "github.com/submariner-io/submariner-operator/config/rbac/lighthouse-agent"
	lighthousecoredns "github.com/submariner-io/submariner-operator/config/rbac/lighthouse-coredns"
	"golang.org/x/net/context"
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
			{Name: "agent ServiceAccount", Content: lighthouseagent.ServiceAccount},
			{Name: "coredns ServiceAccount", Content: lighthousecoredns.ServiceAccount},
		},
	},
	{
		Applier: func(ctx context.Context, kubeClient kubernetes.Interface, _ string, yaml []byte) (bool, error) {
			return clusterrole.EnsureFromYAML(ctx, kubeClient, yaml)
		},
		Refs: []apply.EmbeddedYAMLRef{
			{Name: "agent ClusterRole", Content: lighthouseagent.ClusterRole},
			{Name: "coredns ClusterRole", Content: lighthousecoredns.ClusterRole},
		},
	},
	{
		Applier: clusterrolebinding.EnsureFromYAML,
		Refs: []apply.EmbeddedYAMLRef{
			{Name: "agent ClusterRoleBinding", Content: lighthouseagent.ClusterRoleBinding},
			{Name: "coredns ClusterRoleBinding", Content: lighthousecoredns.ClusterRoleBinding},
		},
	},
	{
		Applier: role.EnsureFromYAML,
		Refs: []apply.EmbeddedYAMLRef{
			{Name: "agent Role", Content: lighthouseagent.Role},
			{Name: "coredns Role", Content: lighthousecoredns.Role},
		},
	},
	{
		Applier: rolebinding.EnsureFromYAML,
		Refs: []apply.EmbeddedYAMLRef{
			{Name: "agent RoleBinding", Content: lighthouseagent.RoleBinding},
			{Name: "coredns RoleBinding", Content: lighthousecoredns.RoleBinding},
		},
	},
}
