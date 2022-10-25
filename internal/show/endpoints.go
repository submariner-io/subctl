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
	"errors"

	"github.com/submariner-io/admiral/pkg/reporter"
	"github.com/submariner-io/subctl/internal/show/table"
	"github.com/submariner-io/subctl/pkg/cluster"
)

func Endpoints(clusterInfo *cluster.Info, _ string, status reporter.Interface) error {
	status.Start("Showing Endpoints")

	gateways, err := clusterInfo.GetGateways()
	if err != nil {
		return status.Error(err, "Error retrieving gateways")
	}

	if len(gateways) == 0 {
		return status.Error(errors.New("no gateways detected"), "")
	}

	printer := table.Printer{Columns: []table.Column{
		{Name: "CLUSTER", MaxLength: 24},
		{Name: "ENDPOINT IP"},
		{Name: "PUBLIC IP"},
		{Name: "CABLE DRIVER"},
		{Name: "TYPE"},
	}}

	for i := range gateways {
		gateway := &gateways[i]
		printer.Add(
			gateway.Status.LocalEndpoint.ClusterID,
			gateway.Status.LocalEndpoint.PrivateIP,
			gateway.Status.LocalEndpoint.PublicIP,
			gateway.Status.LocalEndpoint.Backend,
			"local",
		)

		for i := range gateway.Status.Connections {
			connection := &gateway.Status.Connections[i]
			printer.Add(
				connection.Endpoint.ClusterID,
				connection.Endpoint.PrivateIP,
				connection.Endpoint.PublicIP,
				connection.Endpoint.Backend,
				"remote",
			)
		}
	}

	status.End()
	printer.Print()

	return nil
}
