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
	"github.com/submariner-io/subctl/pkg/serviceaccount"
	lighthouseagent "github.com/submariner-io/submariner-operator/config/rbac/lighthouse-agent"
	lighthousecoredns "github.com/submariner-io/submariner-operator/config/rbac/lighthouse-coredns"
	"golang.org/x/net/context"
	"k8s.io/client-go/kubernetes"
)

// Ensure functions updates or installs the operator CRDs in the cluster.
func Ensure(ctx context.Context, kubeClient kubernetes.Interface, namespace string) (bool, error) {
	createdSA, err := ensureServiceAccounts(ctx, kubeClient, namespace)
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

	return createdSA || createdCR || createdCRB, nil
}

func ensureServiceAccounts(ctx context.Context, kubeClient kubernetes.Interface, namespace string) (bool, error) {
	createdAgentSA, err := serviceaccount.EnsureFromYAML(ctx, kubeClient, namespace, lighthouseagent.ServiceAccount)
	if err != nil {
		return false, errors.Wrap(err, "error provisioning the agent ServiceAccount resource")
	}

	createdCoreDNSSA, err := serviceaccount.EnsureFromYAML(ctx, kubeClient, namespace, lighthousecoredns.ServiceAccount)

	return createdAgentSA || createdCoreDNSSA, errors.Wrap(err, "error provisioning the coredns ServiceAccount resource")
}

func ensureClusterRoles(ctx context.Context, kubeClient kubernetes.Interface) (bool, error) {
	createdAgentCR, err := clusterrole.EnsureFromYAML(ctx, kubeClient, lighthouseagent.ClusterRole)
	if err != nil {
		return false, errors.Wrap(err, "error provisioning the agent ClusterRole resource")
	}

	createdCoreDNSCR, err := clusterrole.EnsureFromYAML(ctx, kubeClient, lighthousecoredns.ClusterRole)

	return createdAgentCR || createdCoreDNSCR, errors.Wrap(err, "error provisioning the coredns ClusterRole resource")
}

func ensureClusterRoleBindings(ctx context.Context, kubeClient kubernetes.Interface, namespace string) (bool, error) {
	createdAgentCRB, err := clusterrolebinding.EnsureFromYAML(ctx, kubeClient, namespace, lighthouseagent.ClusterRoleBinding)
	if err != nil {
		return false, errors.Wrap(err, "error provisioning the agent ClusterRoleBinding resource")
	}

	createdCoreDNSCRB, err := clusterrolebinding.EnsureFromYAML(ctx, kubeClient, namespace, lighthousecoredns.ClusterRoleBinding)

	return createdAgentCRB || createdCoreDNSCRB, errors.Wrap(err, "error provisioning the coredns ClusterRoleBinding resource")
}
