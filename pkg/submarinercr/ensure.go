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

package submarinercr

import (
	"context"

	"github.com/pkg/errors"
	"github.com/submariner-io/admiral/pkg/resource"
	"github.com/submariner-io/admiral/pkg/util"
	operatorv1alpha1 "github.com/submariner-io/submariner-operator/api/submariner/v1alpha1"
	"github.com/submariner-io/submariner-operator/pkg/names"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	controllerClient "sigs.k8s.io/controller-runtime/pkg/client"
)

func Ensure(client controllerClient.Client, namespace string, submarinerSpec *operatorv1alpha1.SubmarinerSpec) error {
	submarinerCR := &operatorv1alpha1.Submariner{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.SubmarinerCrName,
			Namespace: namespace,
		},
		Spec: *submarinerSpec,
	}

	propagationPolicy := metav1.DeletePropagationForeground

	_, err := util.CreateAnew(context.TODO(), resource.ForControllerClient(client, namespace, &operatorv1alpha1.Submariner{}),
		submarinerCR, metav1.CreateOptions{}, metav1.DeleteOptions{PropagationPolicy: &propagationPolicy})

	return errors.Wrap(err, "error creating Submariner resource")
}
