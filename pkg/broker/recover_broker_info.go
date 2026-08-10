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

	"github.com/pkg/errors"
	"github.com/submariner-io/admiral/pkg/reporter"
	"github.com/submariner-io/subctl/pkg/cluster"
	"github.com/submariner-io/submariner-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/utils/set"
)

func RecoverData(submCluster *cluster.Info, broker *v1alpha1.Broker, brokerNamespace, brokerURL string,
	brokerRestConfig *rest.Config, status reporter.Interface,
) error {
	status.Start("Retrieving data to reconstruct broker-info.subm")
	defer status.End()

	status.Success("Retrieving IPSec PSK secret from Submariner found on cluster %s", submCluster.Name)

	var decodedPSKSecret []byte
	var err error

	// Read PSK from Secret if configured, otherwise fall back to CR field
	if submCluster.Submariner.Spec.CeIPSecPSKSecret != "" {
		decodedPSKSecret, err = readIPSecPSKFromSecret(submCluster, submCluster.Submariner.Spec.CeIPSecPSKSecret)
		if err != nil {
			return status.Error(err, "error reading IPSec PSK from secret %q", submCluster.Submariner.Spec.CeIPSecPSKSecret)
		}
	} else {
		// Fall back to reading from CR field for backwards compatibility
		decodedPSKSecret, err = base64.StdEncoding.DecodeString(submCluster.Submariner.Spec.CeIPSecPSK)
		if err != nil {
			return status.Error(err, "error decoding the secret")
		}
	}

	status.Success("Successfully retrieved the data. Writing it to broker-info.subm")

	err = WriteInfoToFile(brokerRestConfig, brokerNamespace, brokerURL, decodedPSKSecret,
		set.New(broker.Spec.Components...), broker.Spec.DefaultCustomDomains, status)

	return status.Error(err, "error reconstructing broker-info.subm")
}

func readIPSecPSKFromSecret(submCluster *cluster.Info, secretName string) ([]byte, error) {
	secret, err := submCluster.ClientProducer.ForKubernetes().CoreV1().Secrets(submCluster.Submariner.Namespace).Get(
		context.TODO(), secretName, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "error reading IPSec PSK secret %s/%s", submCluster.OperatorNamespace(), secretName)
	}

	pskData, ok := secret.Data["psk"]
	if !ok || len(pskData) == 0 {
		return nil, errors.Errorf("IPSec PSK secret %s/%s missing 'psk' data", submCluster.OperatorNamespace(), secretName)
	}

	// Secret.Data is already decoded from base64 by Kubernetes API client
	return pskData, nil
}
