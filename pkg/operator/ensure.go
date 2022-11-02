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

package operator

import (
	"github.com/submariner-io/admiral/pkg/reporter"
	"github.com/submariner-io/subctl/pkg/client"
	"github.com/submariner-io/subctl/pkg/lighthouse"
	"github.com/submariner-io/subctl/pkg/namespace"
	opcrds "github.com/submariner-io/subctl/pkg/operator/crds"
	"github.com/submariner-io/subctl/pkg/operator/deployment"
	"github.com/submariner-io/subctl/pkg/operator/ocp"
	"github.com/submariner-io/subctl/pkg/operator/serviceaccount"
	"github.com/submariner-io/subctl/pkg/submariner"
	"github.com/submariner-io/submariner-operator/pkg/crd"
	"github.com/submariner-io/submariner-operator/pkg/embeddedyamls"
	"github.com/submariner-io/submariner-operator/pkg/names"
)

//nolint:wrapcheck // No need to wrap errors here.
func Ensure(status reporter.Interface, clientProducer client.Producer, operatorNamespace, operatorImage string, debug bool) error {
	if created, err := opcrds.Ensure(crd.UpdaterFromControllerClient(clientProducer.ForGeneral())); err != nil {
		return err
	} else if created {
		status.Success("Created operator CRDs")
	}

	operatorNamespaceLabels := map[string]string{
		"pod-security.kubernetes.io/enforce": "privileged",
	}

	if created, err := namespace.Ensure(clientProducer.ForKubernetes(), operatorNamespace, operatorNamespaceLabels); err != nil {
		return err
	} else if created {
		status.Success("Created operator namespace: %s", operatorNamespace)
	}

	if created, err := serviceaccount.Ensure(clientProducer.ForKubernetes(), operatorNamespace); err != nil {
		return err
	} else if created {
		status.Success("Created operator service account and role")
	}

	componentsRbac := []ocp.RbacInfo{
		{
			ComponentName:          names.OperatorComponent,
			ClusterRoleFile:        embeddedyamls.Config_rbac_submariner_operator_cluster_role_yaml,
			ClusterRoleBindingFile: embeddedyamls.Config_rbac_submariner_operator_ocp_cluster_role_binding_yaml,
		},
	}

	if created, err := ocp.EnsureRBAC(clientProducer.ForDynamic(), clientProducer.ForKubernetes(),
		operatorNamespace, componentsRbac); err != nil {
		return err
	} else if created {
		status.Success("Updated the OCP roles")
	}

	if err := submariner.Ensure(status, clientProducer.ForKubernetes(), clientProducer.ForDynamic(), operatorNamespace); err != nil {
		return err
	}

	if err := lighthouse.Ensure(status, clientProducer.ForKubernetes(), clientProducer.ForDynamic(), operatorNamespace); err != nil {
		return err
	}

	if created, err := deployment.Ensure(clientProducer.ForKubernetes(), operatorNamespace, operatorImage, debug); err != nil {
		return err
	} else if created {
		status.Success("Deployed the operator successfully")
	}

	return nil
}
