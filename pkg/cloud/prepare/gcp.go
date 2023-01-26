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

package prepare

import (
	"github.com/submariner-io/admiral/pkg/reporter"
	"github.com/submariner-io/cloud-prepare/pkg/api"
	"github.com/submariner-io/subctl/internal/cli"
	"github.com/submariner-io/subctl/pkg/cloud"
	"github.com/submariner-io/subctl/pkg/cloud/gcp"
	"github.com/submariner-io/subctl/pkg/cluster"
)

func GCP(clusterInfo *cluster.Info, ports *cloud.Ports, config *gcp.Config, useLoadBalancer bool, status reporter.Interface) error {
	defer status.End()

	gwPorts, input, err := getPortConfig(clusterInfo.ClientProducer, ports, false)
	if err != nil {
		return status.Error(err, "Failed to prepare the cloud")
	}

	//nolint:wrapcheck // No need to wrap errors here.
	err = gcp.RunOn(clusterInfo, config, cli.NewReporter(),
		func(cloud api.Cloud, gwDeployer api.GatewayDeployer, status reporter.Interface) error {
			if config.Gateways > 0 {
				gwInput := api.GatewayDeployInput{
					PublicPorts:     gwPorts,
					Gateways:        config.Gateways,
					UseLoadBalancer: useLoadBalancer,
				}
				err := gwDeployer.Deploy(gwInput, status)
				if err != nil {
					return err
				}
			}

			return cloud.PrepareForSubmariner(input, status)
		})

	return status.Error(err, "Failed to prepare GCP cloud")
}
