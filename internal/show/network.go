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

package show

import (
	"context"
	"fmt"

	"github.com/submariner-io/admiral/pkg/reporter"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/pkg/cluster"
	"github.com/submariner-io/submariner-operator/pkg/discovery/network"
)

func Network(clusterInfo *cluster.Info, _ string, status reporter.Interface) error {
	status.Start("Showing Network details")

	var clusterNetwork *network.ClusterNetwork
	var msg string
	var err error

	if clusterInfo.Submariner != nil {
		msg = "    Discovered network details via Submariner:"
		clusterNetwork = &network.ClusterNetwork{
			PodCIDRs:         []string{clusterInfo.Submariner.Status.ClusterCIDR},
			ServiceCIDRs:     []string{clusterInfo.Submariner.Status.ServiceCIDR},
			NetworkPlugin:    clusterInfo.Submariner.Status.NetworkPlugin,
			GlobalCIDR:       clusterInfo.Submariner.Status.GlobalCIDR,
			ClustersetIPCIDR: clusterInfo.Submariner.Status.ClustersetIPCIDR,
		}
	} else {
		msg = "    Discovered network details"

		clusterNetwork, err = network.Discover(context.TODO(), clusterInfo.ClientProducer.ForGeneral(), constants.OperatorNamespace)
		if err != nil {
			return status.Error(err, "Error discovering network details for this cluster")
		}
	}

	if clusterNetwork == nil {
		status.Warning("The network details could not be determined")
	}

	status.End()

	if clusterNetwork != nil {
		fmt.Println(msg)
		clusterNetwork.Show()
	}

	return nil
}
