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

package azure

import (
	"encoding/json"
	"os"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/pkg/errors"
	"github.com/submariner-io/admiral/pkg/reporter"
	"github.com/submariner-io/admiral/pkg/util"
	"github.com/submariner-io/cloud-prepare/pkg/api"
	"github.com/submariner-io/cloud-prepare/pkg/azure"
	"github.com/submariner-io/cloud-prepare/pkg/k8s"
	"github.com/submariner-io/cloud-prepare/pkg/ocp"
	"github.com/submariner-io/subctl/pkg/cloud"
	"github.com/submariner-io/subctl/pkg/cluster"
)

type Config struct {
	AirGappedDeployment bool
	Gateways            int
	InfraID             string
	Region              string
	OcpMetadataFile     string
	AuthFile            string
	GWInstanceType      string
}

func RunOn(clusterInfo *cluster.Info, config *Config, status reporter.Interface,
	function func(api.Cloud, api.GatewayDeployer, reporter.Interface) error,
) error {
	if config.OcpMetadataFile != "" {
		var err error

		config.InfraID, config.Region, err = readMetadataFile(config.OcpMetadataFile)
		if err != nil {
			return status.Error(err, "Failed to read Azure information from OCP metadata file %q", config.OcpMetadataFile)
		}

		status.Success("Obtained infra ID %q and region %q from OCP metadata file %q", config.InfraID, config.Region,
			config.OcpMetadataFile)
	}

	status.Start("Retrieving Azure credentials from your Azure authorization file %q", config.AuthFile)

	err := os.Setenv("AZURE_AUTH_LOCATION", config.AuthFile)
	if err != nil {
		return status.Error(err, "Unable to set AZURE_AUTH_LOCATION env variable")
	}

	subscriptionID, err := initializeFromAuthFile(config.AuthFile)
	if err != nil {
		return status.Error(err, "Failed to read authorization information from Azure authorization file")
	}

	status.End()

	status.Start("Initializing Azure connectivity")

	credentials, err := azidentity.NewEnvironmentCredential(nil)
	if err != nil {
		return status.Error(err, "Error getting an authorizer for Azure")
	}

	restConfig := clusterInfo.RestConfig
	clientSet := clusterInfo.ClientProducer.ForKubernetes()
	k8sClientSet := k8s.NewInterface(clientSet)

	restMapper, err := util.BuildRestMapper(restConfig)
	if err != nil {
		return status.Error(err, "Failed to create restmapper")
	}

	dynamicClient := clusterInfo.ClientProducer.ForDynamic()
	msDeployer := ocp.NewK8sMachinesetDeployer(restMapper, dynamicClient)

	cloudInfo := &azure.CloudInfo{
		SubscriptionID:  subscriptionID,
		InfraID:         config.InfraID,
		Region:          config.Region,
		BaseGroupName:   config.InfraID + "-rg",
		TokenCredential: credentials,
		K8sClient:       k8sClientSet,
	}

	azureCloud := azure.NewCloud(cloudInfo)

	status.End()

	gwDeployer, err := azure.NewOcpGatewayDeployer(cloudInfo, azureCloud, msDeployer, config.GWInstanceType)
	if err != nil {
		return status.Error(err, "Failed to initialize a GatewayDeployer config")
	}

	return function(azureCloud, gwDeployer, status)
}

func readMetadataFile(fileName string) (string, string, error) {
	var metadata struct {
		InfraID string `json:"infraID"`
		Azure   struct {
			Region string `json:"region"`
		} `json:"azure"`
	}

	err := cloud.ReadMetadataFile(fileName, &metadata)

	return metadata.InfraID, metadata.Azure.Region, err //nolint:wrapcheck // No need to wrap here
}

func initializeFromAuthFile(authFile string) (string, error) {
	data, err := os.ReadFile(authFile)
	if err != nil {
		return "", errors.Wrapf(err, "error reading file %q", authFile)
	}

	var authInfo struct {
		ClientID       string
		ClientSecret   string
		SubscriptionID string
		TenantID       string
	}

	err = json.Unmarshal(data, &authInfo)
	if err != nil {
		return "", errors.Wrap(err, "error unmarshalling data")
	}

	if err := os.Setenv("AZURE_CLIENT_ID", authInfo.ClientID); err != nil {
		return "", errors.Wrapf(err, "unable to set AZURE_CLIENT_ID env variable")
	}

	if err := os.Setenv("AZURE_CLIENT_SECRET", authInfo.ClientSecret); err != nil {
		return "", errors.Wrapf(err, "unable to set AZURE_CLIENT_SECRET env variable")
	}

	if err := os.Setenv("AZURE_TENANT_ID", authInfo.TenantID); err != nil {
		return "", errors.Wrapf(err, "unable to set AZURE_TENANT_ID env variable")
	}

	return authInfo.SubscriptionID, nil
}
