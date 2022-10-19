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

// Package aws provides common functionality to run cloud prepare/cleanup on AWS.
package aws

import (
	"github.com/submariner-io/admiral/pkg/reporter"
	"github.com/submariner-io/admiral/pkg/util"
	"github.com/submariner-io/cloud-prepare/pkg/api"
	"github.com/submariner-io/cloud-prepare/pkg/aws"
	"github.com/submariner-io/cloud-prepare/pkg/ocp"
	"github.com/submariner-io/subctl/pkg/cloud"
	"github.com/submariner-io/subctl/pkg/cluster"
)

type Config struct {
	Gateways        int
	InfraID         string
	Region          string
	Profile         string
	CredentialsFile string
	OcpMetadataFile string
	GWInstanceType  string
}

// RunOn runs the given function on AWS, supplying it with a cloud instance connected to AWS and a reporter that writes to CLI.
// The functions makes sure that infraID and region are specified, and extracts the credentials from a secret in order to connect to AWS.
func RunOn(clusterInfo *cluster.Info, config *Config, status reporter.Interface,
	function func(api.Cloud, api.GatewayDeployer, reporter.Interface) error,
) error {
	if config.OcpMetadataFile != "" {
		var err error

		config.InfraID, config.Region, err = readMetadataFile(config.OcpMetadataFile)
		if err != nil {
			return status.Error(err, "Failed to read AWS information from OCP metadata file %q", config.OcpMetadataFile)
		}

		status.Success("Obtained infra ID %q and region %q from OCP metadata file %q", config.InfraID, config.Region, config.OcpMetadataFile)
	}

	status.Start("Initializing AWS connectivity")

	awsCloud, err := aws.NewCloudFromSettings(config.CredentialsFile, config.Profile, config.InfraID, config.Region)
	if err != nil {
		return status.Error(err, "error loading default config")
	}

	status.End()

	restMapper, err := util.BuildRestMapper(clusterInfo.RestConfig)
	if err != nil {
		return status.Error(err, "error creating REST mapper")
	}

	dynamicClient := clusterInfo.ClientProducer.ForDynamic()
	msDeployer := ocp.NewK8sMachinesetDeployer(restMapper, dynamicClient)

	gwDeployer, err := aws.NewOcpGatewayDeployer(awsCloud, msDeployer, config.GWInstanceType)
	if err != nil {
		return status.Error(err, "error creating the gateway deployer")
	}

	return function(awsCloud, gwDeployer, status)
}

func readMetadataFile(fileName string) (string, string, error) {
	var metadata struct {
		InfraID string `json:"infraID"`
		AWS     struct {
			Region string `json:"region"`
		} `json:"aws"`
	}

	err := cloud.ReadMetadataFile(fileName, &metadata)

	return metadata.InfraID, metadata.AWS.Region, err //nolint:wrapcheck // No need to wrap here
}
