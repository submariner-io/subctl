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

package subctl

import (
	"context"

	"github.com/pkg/errors"
	"github.com/submariner-io/admiral/pkg/resource"
	"github.com/submariner-io/subctl/pkg/brokercr"
	"github.com/submariner-io/subctl/pkg/client"
	"github.com/submariner-io/submariner-operator/api/v1alpha1"
	"k8s.io/client-go/rest"
	controllerClient "sigs.k8s.io/controller-runtime/pkg/client"
)

func getBroker(config *rest.Config, namespace string) (*v1alpha1.Broker, bool, error) {
	brokerClientProducer, err := client.NewProducerFromRestConfig(config)
	if err != nil {
		return nil, false, errors.Wrap(err, "error creating broker client Producer")
	}

	brokerObj := &v1alpha1.Broker{}
	err = brokerClientProducer.ForGeneral().Get(
		context.TODO(), controllerClient.ObjectKey{
			Namespace: namespace,
			Name:      brokercr.Name,
		}, brokerObj)

	if resource.IsNotFoundErr(err) {
		return nil, false, nil
	}

	if err != nil {
		return nil, false, errors.Wrap(err, "error retrieving Broker")
	}

	return brokerObj, true, nil
}
