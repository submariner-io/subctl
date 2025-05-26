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

package diagnose

import (
	"errors"

	"github.com/submariner-io/admiral/pkg/reporter"
	"github.com/submariner-io/admiral/pkg/resource"
	"github.com/submariner-io/subctl/pkg/cluster"
	submv1 "github.com/submariner-io/submariner/pkg/apis/submariner.io/v1"
	k8snet "k8s.io/utils/net"
)

func Connections(clusterInfo *cluster.Info, _ string, status reporter.Interface) error {
	return errors.Join(
		CheckGatewayConnections(clusterInfo, status),
		CheckRouteAgentConnections(clusterInfo, status),
	)
}

func CheckGatewayConnections(clusterInfo *cluster.Info, status reporter.Interface) error {
	status.Start("Checking gateway connections")
	defer status.End()

	gateways, err := clusterInfo.GetGateways()
	if err != nil {
		return status.Error(err, "Error retrieving gateways")
	}

	if len(gateways) == 0 {
		return status.Error(errors.New("no gateways were detected"), "")
	}

	tracker := reporter.NewTracker(status)
	foundActive := false

	for i := range gateways {
		gateway := &gateways[i]
		if gateway.Status.HAStatus != submv1.HAStatusActive {
			continue
		}

		foundActive = true

		if len(gateway.Status.Connections) == 0 {
			tracker.Failure("There are no active connections on gateway %q", gateway.Name)
		}

		for j := range gateway.Status.Connections {
			connection := &gateway.Status.Connections[j]
			if connection.Status == submv1.Connecting {
				tracker.Failure("IPv%s connection to cluster %q is in progress", connection.GetFamily(),
					connection.Endpoint.ClusterID)
			} else if connection.Status == submv1.ConnectionError {
				tracker.Failure("IPv%s connection to cluster %q is not established. Connection details:\n%s",
					connection.GetFamily(), connection.Endpoint.ClusterID, resource.ToJSON(connection))
			}
		}
	}

	if !foundActive {
		tracker.Failure("No active gateway was found")
	}

	if tracker.HasFailures() {
		return errors.New("failures while diagnosing gateway connections")
	}

	return nil
}

func CheckRouteAgentConnections(clusterInfo *cluster.Info, status reporter.Interface) error {
	status.Start("Retrieving route agent connections")

	routeAgents, err := clusterInfo.GetRouteAgents()
	if err != nil {
		return status.Error(err, "Error retrieving route agents")
	}

	if len(routeAgents) == 0 {
		return status.Error(errors.New("no route agents were detected"), "")
	}

	status.End()

	tracker := reporter.NewTracker(status)

	for i := range routeAgents {
		routeAgent := &routeAgents[i]

		tracker.Start("Checking remote connections for route agent %q", routeAgent.Name)

		if len(routeAgent.Status.RemoteEndpoints) == 0 {
			tracker.Success("There are no remote endpoint connections on route agent %q", routeAgent.Name)
		}

		for j := range routeAgent.Status.RemoteEndpoints {
			remoteEndpoint := &routeAgent.Status.RemoteEndpoints[j]
			if remoteEndpoint.Status == submv1.Connecting {
				tracker.Failure("%s connection to cluster %q is in progress", getIPFamilyString(remoteEndpoint),
					remoteEndpoint.Spec.ClusterID)
			} else if remoteEndpoint.Status == submv1.ConnectionError {
				tracker.Failure("%s connection to cluster %q is not established. Connection details:\n%s",
					getIPFamilyString(remoteEndpoint), remoteEndpoint.Spec.ClusterID, resource.ToJSON(remoteEndpoint))
			}
		}

		tracker.End()
	}

	if tracker.HasFailures() {
		return errors.New("failures while diagnosing route agent connections")
	}

	return nil
}

func getIPFamilyString(ep *submv1.RemoteEndpoint) string {
	if len(ep.Spec.HealthCheckIPs) != 1 {
		return "IPv?"
	}

	return "IPv" + string(k8snet.IPFamilyOfString(ep.Spec.HealthCheckIPs[0]))
}
