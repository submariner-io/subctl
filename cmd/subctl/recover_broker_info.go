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
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/submariner-io/admiral/pkg/reporter"
	"github.com/submariner-io/subctl/internal/cli"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/internal/exit"
	"github.com/submariner-io/subctl/internal/restconfig"
	"github.com/submariner-io/subctl/pkg/broker"
	"github.com/submariner-io/subctl/pkg/cluster"
	"github.com/submariner-io/submariner-operator/api/v1alpha1"
	submarinerv1 "github.com/submariner-io/submariner/pkg/apis/submariner.io/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

var recoverRestConfigProducer = restconfig.NewProducer().WithPrefixedContext("broker").
	WithDefaultNamespace(constants.DefaultBrokerNamespace).
	WithPrefixedNamespace("broker", constants.DefaultBrokerNamespace)

// recoverBrokerInfo represents the reconstruct command.
var recoverBrokerInfo = &cobra.Command{
	Use:   "recover-broker-info",
	Short: "Recovers the broker-info.subm file from the installed Broker",
	Run: func(cmd *cobra.Command, args []string) {
		status := cli.NewReporter()
		// if --brokerconfig flag provided, get the broker and proceed to get Submariner
		contextFound, err := recoverRestConfigProducer.RunOnSelectedPrefixedContext("broker",
			recoverBrokerFromConfigContext, status)
		exit.OnError(err)

		// if --brokerconfig not provided, search for broker on current context and proceed to get Submariner
		if !contextFound {
			err = recoverRestConfigProducer.RunOnSelectedContext(recoverBrokerFromCurrentContext, status)
		}
		exit.OnError(err)
	},
}

func init() {
	recoverRestConfigProducer.SetupFlags(recoverBrokerInfo.Flags())
	rootCmd.AddCommand(recoverBrokerInfo)
}

func recoverBrokerFromConfigContext(brokerCluster *cluster.Info, brokerNamespace string, status reporter.Interface) error {
	brokerObj, err := getBroker(brokerCluster, brokerNamespace, "Please try again with another cluster", status)
	if err != nil {
		return err
	}

	clusters, err := brokerCluster.GetClusters(brokerNamespace)
	if err != nil {
		return status.Error(err, "error listing joined clusters")
	}

	ok, err := tryToRecoverFromBroker(brokerCluster, brokerObj, brokerNamespace, clusters, status)
	if ok || err != nil {
		return err
	}

	status.Warning("Submariner is not installed on the same cluster as Broker")
	status.Start("Checking if Submariner is installed on a different cluster")
	//nolint:wrapcheck // No need to wrap errors here.
	return recoverRestConfigProducer.RunOnSelectedContext(
		func(submCluster *cluster.Info, namespace string, status reporter.Interface) error {
			if isSubmJoinedToBroker(clusters, submCluster) {
				status.Success("Found a Submariner installation on cluster %q joined to the Broker", submCluster.Name)
				//nolint:wrapcheck // No need to wrap errors here.
				return broker.RecoverData(brokerCluster, submCluster, brokerObj, namespace, status)
			}

			return status.Error(
				fmt.Errorf("submariner is not installed on cluster %s. "+
					"Please specify the cluster where Submariner is installed via `--kubeconfig` or `--context` flag"+
					"", submCluster.Name), "")
		}, status)
}

func recoverBrokerFromCurrentContext(clusterInfo *cluster.Info, namespace string, status reporter.Interface) error {
	// if --brokerconfig not provided, search for broker on current context and proceed to get Submariner
	brokerObj, err := getBroker(clusterInfo, namespace, "Please specify the cluster where the Broker is installed "+
		"via the `--brokerconfig` or `--brokercontext` flag and the namespace via the `--brokernamespace` flag", status)
	if err != nil {
		return err
	}

	clusters, err := clusterInfo.GetClusters(namespace)
	if err != nil {
		return status.Error(err, "error listing joined clusters")
	}

	ok, err := tryToRecoverFromBroker(clusterInfo, brokerObj, namespace, clusters, status)
	if ok || err != nil {
		return err
	}

	return status.Error(
		fmt.Errorf("submariner is not installed on cluster %q. "+
			"Please specify the cluster where Submariner is installed via `--kubeconfig` or `--context` flag"+
			"", clusterInfo.Name), "")
}

func getBroker(clusterInfo *cluster.Info, namespace, notFoundMsg string, status reporter.Interface) (*v1alpha1.Broker, error) {
	status.Start("Checking if the Broker is installed on cluster %q in namespace %q", clusterInfo.Name, namespace)

	brokerObj, err := clusterInfo.GetBroker(namespace)
	if apierrors.IsNotFound(err) {
		return nil, status.Error(fmt.Errorf("the Broker is not installed on the specified cluster in namespace %s. %s", notFoundMsg,
			namespace), "")
	}

	if err != nil {
		return nil, status.Error(err, "")
	}

	status.End()

	return brokerObj, nil
}

func tryToRecoverFromBroker(brokerCluster *cluster.Info, brokerObj *v1alpha1.Broker,
	brokerNamespace string, clusters []submarinerv1.Cluster, status reporter.Interface,
) (bool, error) {
	status.Start("Checking if there are any clusters joined to the Broker")

	if len(clusters) == 0 {
		return false, status.Error(
			errors.New(
				"no clusters are joined to the Broker. Please re-run the `deploy-broker` command to regenerate the broker-info."+
					"subm file"), "")
	}

	status.Success("Found %d cluster(s) joined to the Broker", len(clusters))
	status.End()

	if isSubmJoinedToBroker(clusters, brokerCluster) {
		status.Success("Found a local Submariner installation joined to the Broker")
		//nolint:wrapcheck // No need to wrap errors here.
		return true, broker.RecoverData(brokerCluster, brokerCluster, brokerObj, brokerNamespace, status)
	}

	return false, nil
}

func isSubmJoinedToBroker(clusters []submarinerv1.Cluster, clusterInfo *cluster.Info) bool {
	if clusterInfo.Submariner != nil {
		for i := range clusters {
			if clusters[i].Spec.ClusterID == clusterInfo.Submariner.Spec.ClusterID {
				return true
			}
		}
	}

	return false
}
