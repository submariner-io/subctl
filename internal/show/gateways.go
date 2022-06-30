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
	"fmt"

	"github.com/submariner-io/admiral/pkg/reporter"
	"github.com/submariner-io/subctl/internal/show/table"
	"github.com/submariner-io/subctl/pkg/cluster"
	submv1 "github.com/submariner-io/submariner/pkg/apis/submariner.io/v1"
)

func Gateways(clusterInfo *cluster.Info, status reporter.Interface) bool {
	status.Start("Showing Gateways")

	gateways, err := clusterInfo.GetGateways()
	if err != nil {
		status.Failure("Error retrieving gateways: %v", err)
		status.End()

		return false
	}

	if len(gateways) == 0 {
		status.Failure("There are no gateways detected")
		status.End()

		return false
	}

	printer := table.Printer{Columns: []table.Column{
		{Name: "NODE", MaxLength: 30},
		{Name: "HA STATUS"},
		{Name: "SUMMARY"},
	}}

	for i := range gateways {
		gateway := gateways[i]
		totalConnections := len(gateway.Status.Connections)
		countConnected := 0

		for i := range gateway.Status.Connections {
			if gateway.Status.Connections[i].Status == submv1.Connected {
				countConnected++
			}
		}

		var summary string
		if gateway.Status.StatusFailure != "" {
			summary = gateway.Status.StatusFailure
		} else if totalConnections == 0 {
			summary = "There are no connections"
		} else if totalConnections == countConnected {
			summary = fmt.Sprintf("All connections (%d) are established", totalConnections)
		} else {
			summary = fmt.Sprintf("%d connections out of %d are established", countConnected, totalConnections)
		}

		printer.Add(gateway.Status.LocalEndpoint.Hostname, gateway.Status.HAStatus, summary)
	}

	status.End()
	printer.Print()

	return true
}
