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
	"encoding/base64"

	"github.com/submariner-io/admiral/pkg/reporter"
	"github.com/submariner-io/subctl/pkg/cluster"
	"github.com/submariner-io/submariner-operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/util/sets"
)

func RecoverData(
	brokerCluster, submCluster *cluster.Info, broker *v1alpha1.Broker, namespace string, status reporter.Interface,
) error {
	status.Start("Retrieving data to reconstruct broker-info.subm")
	defer status.End()

	status.Success("Retrieving IPSec PSK secret from Submariner found on cluster %s", submCluster.Name)

	decodedPSKSecret, err := base64.StdEncoding.DecodeString(submCluster.Submariner.Spec.CeIPSecPSK)
	if err != nil {
		return status.Error(err, "error decoding the secret")
	}

	status.Success("Successfully retrieved the data. Writing it to broker-info.subm")

	err = WriteInfoToFile(brokerCluster.RestConfig, namespace, decodedPSKSecret,
		sets.New(broker.Spec.Components...), broker.Spec.DefaultCustomDomains, status)

	return status.Error(err, "error reconstructing broker-info.subm")
}
