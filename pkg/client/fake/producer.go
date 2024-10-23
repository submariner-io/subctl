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

package fake

import (
	"github.com/submariner-io/subctl/pkg/client"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	controllerfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func New() *client.DefaultProducer {
	p := &client.DefaultProducer{
		KubeClient:    k8sfake.NewClientset(),
		DynamicClient: dynamicfake.NewSimpleDynamicClient(scheme.Scheme),
		GeneralClient: controllerfake.NewClientBuilder().WithScheme(scheme.Scheme).Build(),
	}

	client.NewProducerFromRestConfig = func(_ *rest.Config) (client.Producer, error) {
		return p, nil
	}

	return p
}
