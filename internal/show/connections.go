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
	"github.com/submariner-io/admiral/pkg/reporter"
	"github.com/submariner-io/subctl/internal/show/table"
	"github.com/submariner-io/subctl/pkg/cluster"
	submv1 "github.com/submariner-io/submariner/pkg/apis/submariner.io/v1"
)

func Connections(clusterInfo *cluster.Info, status reporter.Interface) bool {
	status.Start("Showing Connections")

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
		{Name: "GATEWAY", MaxLength: 30},
		{Name: "CLUSTER", MaxLength: 24},
		{Name: "REMOTE IP"},
		{Name: "NAT"},
		{Name: "CABLE DRIVER"},
		{Name: "SUBNETS", MaxLength: 40},
		{Name: "STATUS"},
		{Name: "RTT avg."},
	}}

	for i := range gateways {
		gateway := &gateways[i]
		for i := range gateway.Status.Connections {
			connection := &gateway.Status.Connections[i]
			ip, nat := remoteIPAndNATForConnection(connection)
			printer.Add(
				connection.Endpoint.Hostname,
				connection.Endpoint.ClusterID,
				ip,
				nat,
				connection.Endpoint.Backend,
				connection.Endpoint.Subnets,
				connection.Status,
				getAverageRTTForConnection(connection),
			)
		}
	}

	if printer.Empty() {
		status.Failure("No connections found")
		status.End()

		return false
	}

	status.End()
	printer.Print()

	return true
}

func getAverageRTTForConnection(connection *submv1.Connection) string {
	rtt := ""
	if connection.LatencyRTT != nil {
		rtt = connection.LatencyRTT.Average
	}

	return rtt
}

func remoteIPAndNATForConnection(connection *submv1.Connection) (string, bool) {
	if connection.UsingIP != "" {
		return connection.UsingIP, connection.UsingNAT
	}

	if connection.Endpoint.NATEnabled {
		return connection.Endpoint.PublicIP, true
	}

	return connection.Endpoint.PrivateIP, false
}
