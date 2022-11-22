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

package brokercr

import (
	"context"

	"github.com/pkg/errors"
	"github.com/submariner-io/admiral/pkg/resource"
	"github.com/submariner-io/admiral/pkg/util"
	submariner "github.com/submariner-io/submariner-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	controllerClient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	Name = "submariner-broker"
)

func Ensure(ctx context.Context, client controllerClient.Client, namespace string, brokerSpec submariner.BrokerSpec) error {
	brokerCR := &submariner.Broker{
		ObjectMeta: metav1.ObjectMeta{
			Name:      Name,
			Namespace: namespace,
		},
		Spec: brokerSpec,
	}

	_, err := util.CreateAnew(ctx, resource.ForControllerClient(client, namespace, &submariner.Broker{}), brokerCR,
		metav1.CreateOptions{}, metav1.DeleteOptions{})

	return errors.Wrap(err, "error creating Broker resource")
}
