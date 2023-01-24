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

package broker

import (
	"context"
	"encoding/base64"

	"github.com/submariner-io/admiral/pkg/reporter"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/internal/rbac"
	"github.com/submariner-io/subctl/pkg/cluster"
	"github.com/submariner-io/submariner-operator/api/v1alpha1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func RecoverData(
	brokerCluster, submCluster *cluster.Info, broker *v1alpha1.Broker, namespace string, status reporter.Interface,
) error {
	status.Start("Retrieving data to reconstruct broker-info.subm")
	defer status.End()

	data := &Info{}
	var err error

	data.BrokerURL = brokerCluster.RestConfig.Host + brokerCluster.RestConfig.APIPath

	data.ClientToken, err = rbac.GetClientTokenSecret(
		context.TODO(), brokerCluster.ClientProducer.ForKubernetes(), namespace,
		constants.SubmarinerBrokerAdminSA,
	)
	if err != nil {
		return status.Error(err, "error getting broker client secret")
	}

	data.Components = broker.Spec.Components
	data.ServiceDiscovery = data.IsServiceDiscoveryEnabled()
	data.CustomDomains = &broker.Spec.DefaultCustomDomains

	status.Success("Retrieving IPSec PSK secret from Submariner found on cluster %s", submCluster.Name)

	decodedPSKSecret, err := base64.StdEncoding.DecodeString(submCluster.Submariner.Spec.CeIPSecPSK)
	if err != nil {
		return status.Error(err, "error decoding the secret")
	}

	data.IPSecPSK = &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: ipsecPSKSecretName,
		},
		Data: map[string][]byte{"psk": decodedPSKSecret},
	}

	status.Success("Successfully retrieved the data. Writing it to broker-info.subm")

	err = data.writeToFile("broker-info.subm")

	return status.Error(err, "error reconstructing broker-info.subm")
}
