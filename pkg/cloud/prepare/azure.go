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
	"github.com/pkg/errors"
	"github.com/submariner-io/admiral/pkg/reporter"
	"github.com/submariner-io/cloud-prepare/pkg/api"
	"github.com/submariner-io/subctl/pkg/cloud"
	"github.com/submariner-io/subctl/pkg/cloud/azure"
	"github.com/submariner-io/subctl/pkg/cluster"
)

func Azure(clusterInfo *cluster.Info, ports *cloud.Ports, config *azure.Config, status reporter.Interface) error {
	status.Start("Preparing Azure cloud for Submariner deployment")

	gwPorts, input, err := getPortConfig(clusterInfo.ClientProducer, ports, false)
	if err != nil {
		return status.Error(err, "Failed to prepare the cloud")
	}

	//nolint:wrapcheck // No need to wrap errors here.
	err = azure.RunOn(clusterInfo, config, status,
		func(cloud api.Cloud, gwDeployer api.GatewayDeployer, status reporter.Interface) error {
			if config.Gateways > 0 {
				gwInput := api.GatewayDeployInput{
					PublicPorts: gwPorts,
					Gateways:    config.Gateways,
				}

				err := gwDeployer.Deploy(gwInput, status)
				if err != nil {
					return errors.WithMessage(err, "Deployment failed ")
				}
			}

			if len(input.InternalPorts) > 0 {
				return cloud.PrepareForSubmariner(input, status)
			}

			return nil
		})

	return status.Error(err, "Failed to prepare Azure  cloud")
}
