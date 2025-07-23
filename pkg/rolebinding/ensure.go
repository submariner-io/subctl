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

package rolebinding

import (
	"context"

	"github.com/pkg/errors"
	"github.com/submariner-io/admiral/pkg/resource"
	resourceutil "github.com/submariner-io/subctl/pkg/resource"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/yaml"
)

func EnsureFromYAML(ctx context.Context, kubeClient kubernetes.Interface, namespace string, roleBindingYaml []byte) (bool, error) {
	roleBinding := &rbacv1.RoleBinding{}

	err := yaml.Unmarshal(roleBindingYaml, &roleBinding)
	if err != nil {
		return false, errors.Wrap(err, "error extracting embedded RoleBinding")
	}

	// If the embedded role binding specifies its namespace, use that
	if roleBinding.Namespace != "" {
		namespace = roleBinding.Namespace
	}

	return Ensure(ctx, kubeClient, namespace, roleBinding)
}

//nolint:wrapcheck // No need to wrap errors here.
func Ensure(ctx context.Context, kubeClient kubernetes.Interface, namespace string, roleBinding *rbacv1.RoleBinding) (bool, error) {
	return resourceutil.CreateOrUpdate(ctx, resource.ForRoleBinding(kubeClient, namespace), roleBinding)
}
