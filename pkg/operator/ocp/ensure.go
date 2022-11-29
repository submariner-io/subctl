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

package ocp

import (
	"context"

	securityv1 "github.com/openshift/api/security/v1"
	"github.com/pkg/errors"
	"github.com/submariner-io/subctl/pkg/clusterrole"
	"github.com/submariner-io/subctl/pkg/clusterrolebinding"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

type RbacInfo struct {
	ComponentName          string
	ClusterRoleFile        string
	ClusterRoleBindingFile string
}

func EnsureRBAC(ctx context.Context,
	dynClient dynamic.Interface, kubeClient kubernetes.Interface, namespace string, componentsRbac []RbacInfo) (
	bool, error,
) {
	if !IsOcpPlatform(ctx, dynClient) {
		return false, nil
	}

	updateRbac := false

	for _, componentRbac := range componentsRbac {
		createdCR, err := clusterrole.EnsureFromYAML(ctx, kubeClient, componentRbac.ClusterRoleFile)
		if err != nil {
			return false, errors.Wrapf(err, "error provisioning the %s OCP ClusterRole resource", componentRbac.ComponentName)
		}

		createdCRB, err := clusterrolebinding.EnsureFromYAML(ctx, kubeClient, namespace, componentRbac.ClusterRoleBindingFile)
		if err != nil {
			return false, errors.Wrapf(err, "error provisioning the %s OCP ClusterRoleBinding resource", componentRbac.ComponentName)
		}

		updateRbac = updateRbac || createdCR || createdCRB
	}

	return updateRbac, nil
}

func IsOcpPlatform(ctx context.Context, dynClient dynamic.Interface) bool {
	sccClient := dynClient.Resource(securityv1.GroupVersion.WithResource("securitycontextconstraints"))
	_, err := sccClient.Get(ctx, "privileged", metav1.GetOptions{})

	return err == nil
}
