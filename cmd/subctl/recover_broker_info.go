//go:build !non_deploy

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
	"fmt"

	"github.com/spf13/cobra"
	"github.com/submariner-io/admiral/pkg/reporter"
	"github.com/submariner-io/subctl/internal/cli"
	"github.com/submariner-io/subctl/internal/exit"
	"github.com/submariner-io/subctl/internal/restconfig"
	"github.com/submariner-io/subctl/pkg/broker"
	"github.com/submariner-io/subctl/pkg/cluster"
)

var (
	recoverRestConfigProducer = restconfig.NewProducer()
	recoverBrokerURL          string
)

// recoverBrokerInfo represents the reconstruct command.
var recoverBrokerInfo = &cobra.Command{
	Use:   "recover-broker-info",
	Short: "Recovers the broker-info.subm file from the installed Broker",
	Run: func(cmd *cobra.Command, args []string) {
		status := cli.NewReporter()

		exit.OnError(recoverRestConfigProducer.RunOnSelectedContext(restconfig.IfConnectivityInstalled(recoverBrokerInfoFromSubm), status))
	},
}

func init() {
	recoverRestConfigProducer.SetupFlags(recoverBrokerInfo.Flags())
	recoverBrokerInfo.Flags().StringVar(&recoverBrokerURL, "broker-url", "",
		"broker API endpoint URL (stored in the broker information file, defaults to the context URL)")
	rootCmd.AddCommand(recoverBrokerInfo)
}

func recoverBrokerInfoFromSubm(submCluster *cluster.Info, _ string, status reporter.Interface) error {
	brokerNamespace := submCluster.Submariner.Spec.BrokerK8sRemoteNamespace
	brokerRestConfig := submCluster.RestConfig

	status.Start("Checking if the Broker is installed on the Submariner cluster %q in namespace %q", submCluster.Name, brokerNamespace)
	defer status.End()

	brokerObj, found, err := getBroker(context.TODO(), brokerRestConfig, brokerNamespace)
	if err != nil {
		return status.Error(err, "Error getting Broker")
	}

	if !found {
		status.Warning("Broker not found. Trying to connect to the Broker installed on a different cluster")

		brokerRestConfig, brokerNamespace, err = restconfig.ForBroker(submCluster.Submariner, submCluster.ServiceDiscovery)
		if err != nil {
			return status.Error(err, "Error getting the Broker's REST config")
		}

		brokerObj, found, err = getBroker(context.TODO(), brokerRestConfig, brokerNamespace)
		if err != nil {
			return status.Error(err, "")
		}

		if !found {
			return status.Error(fmt.Errorf("no Broker resource found in namespace %q", brokerNamespace), "")
		}

		status.Success("Found Broker installed on a different cluster in namespace %s", brokerNamespace)
	}

	//nolint:wrapcheck // No need to wrap errors here.
	return broker.RecoverData(submCluster, brokerObj, brokerNamespace, recoverBrokerURL, brokerRestConfig, status)
}
