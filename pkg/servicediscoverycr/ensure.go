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

package servicediscoverycr

import (
	"context"

	"github.com/pkg/errors"
	"github.com/submariner-io/admiral/pkg/resource"
	resourceutil "github.com/submariner-io/subctl/pkg/resource"
	operatorv1alpha1 "github.com/submariner-io/submariner-operator/api/v1alpha1"
	"github.com/submariner-io/submariner-operator/pkg/names"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	controllerClient "sigs.k8s.io/controller-runtime/pkg/client"
)

func init() {
	err := operatorv1alpha1.AddToScheme(scheme.Scheme)
	if err != nil {
		panic(err)
	}
}

func Ensure(ctx context.Context,
	client controllerClient.Client, namespace string, serviceDiscoverySpec *operatorv1alpha1.ServiceDiscoverySpec,
) error {
	sd := &operatorv1alpha1.ServiceDiscovery{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      names.ServiceDiscoveryCrName,
		},
		Spec: *serviceDiscoverySpec,
	}

	_, err := resourceutil.CreateOrUpdate(ctx, resource.ForControllerClient(client, namespace,
		&operatorv1alpha1.ServiceDiscovery{}), sd)

	return errors.Wrap(err, "error creating/updating ServiceDiscovery resource")
}
