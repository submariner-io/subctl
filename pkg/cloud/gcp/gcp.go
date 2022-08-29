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

// Package gcp provides common functionality to run cloud prepare/cleanup on GCP Clusters.
package gcp

import (
	"context"
	"os"

	"github.com/pkg/errors"
	"github.com/submariner-io/admiral/pkg/reporter"
	"github.com/submariner-io/admiral/pkg/util"
	"github.com/submariner-io/cloud-prepare/pkg/api"
	"github.com/submariner-io/cloud-prepare/pkg/gcp"
	gcpClientIface "github.com/submariner-io/cloud-prepare/pkg/gcp/client"
	"github.com/submariner-io/cloud-prepare/pkg/k8s"
	"github.com/submariner-io/cloud-prepare/pkg/ocp"
	"github.com/submariner-io/subctl/pkg/cloud"
	"github.com/submariner-io/subctl/pkg/cluster"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/dns/v1"
	"google.golang.org/api/option"
)

type Config struct {
	DedicatedGateway bool
	Gateways         int
	InfraID          string
	Region           string
	ProjectID        string
	CredentialsFile  string
	OcpMetadataFile  string
	GWInstanceType   string
}

// RunOn runs the given function on GCP, supplying it with a cloud instance connected to GCP and a reporter that writes to CLI.
// The functions makes sure that infraID and region are specified, and extracts the credentials from a secret in order to connect to GCP.
func RunOn(clusterInfo *cluster.Info, config *Config, status reporter.Interface,
	function func(api.Cloud, api.GatewayDeployer, reporter.Interface) error,
) error {
	var err error
	if config.OcpMetadataFile != "" {
		config.InfraID, config.Region, config.ProjectID, err = readMetadataFile(config.OcpMetadataFile)
		if err != nil {
			return status.Error(err, "Failed to read GCP information from OCP metadata file %q", config.OcpMetadataFile)
		}

		status.Success("Obtained infra ID %q, region %q, and project ID %q from OCP metadata file %q", config.InfraID,
			config.Region, config.ProjectID, config.OcpMetadataFile)
	}

	status.Start("Retrieving GCP credentials from your GCP configuration")

	creds, err := getCredentials(config.CredentialsFile)
	if err != nil {
		return status.Error(err, "error retrieving GCP credentials")
	}

	status.End()

	status.Start("Initializing GCP connectivity")

	options := []option.ClientOption{
		option.WithCredentials(creds),
		option.WithUserAgent("open-cluster-management.io submarineraddon/v1"),
	}

	gcpClient, err := gcpClientIface.NewClient(config.ProjectID, options)
	if err != nil {
		return status.Error(err, "error initializing a GCP Client")
	}

	status.End()

	restConfig := clusterInfo.RestConfig
	clientSet := clusterInfo.ClientProducer.ForKubernetes()
	k8sClientSet := k8s.NewInterface(clientSet)

	restMapper, err := util.BuildRestMapper(restConfig)
	if err != nil {
		return status.Error(err, "error creating REST mapper")
	}

	dynamicClient := clusterInfo.ClientProducer.ForDynamic()

	gcpCloudInfo := gcp.CloudInfo{
		ProjectID: config.ProjectID,
		InfraID:   config.InfraID,
		Region:    config.Region,
		Client:    gcpClient,
	}
	gcpCloud := gcp.NewCloud(gcpCloudInfo)
	msDeployer := ocp.NewK8sMachinesetDeployer(restMapper, dynamicClient)
	// TODO: Ideally we should be able to specify the image for GWNode, but it was seen that
	// with certain images, the instance is not coming up. Needs to be investigated further.
	gwDeployer := gcp.NewOcpGatewayDeployer(gcpCloudInfo, msDeployer, config.GWInstanceType, "", config.DedicatedGateway, k8sClientSet)

	return function(gcpCloud, gwDeployer, status)
}

func readMetadataFile(fileName string) (string, string, string, error) {
	var metadata struct {
		InfraID string `json:"infraID"`
		GCP     struct {
			Region    string `json:"region"`
			ProjectID string `json:"projectID"`
		} `json:"gcp"`
	}

	err := cloud.ReadMetadataFile(fileName, &metadata)

	return metadata.InfraID, metadata.GCP.Region, metadata.GCP.ProjectID, err // nolint:wrapcheck // No need to wrap here
}

func getCredentials(credentialsFile string) (*google.Credentials, error) {
	authJSON, err := os.ReadFile(credentialsFile)
	if err != nil {
		return nil, errors.Wrapf(err, "error reading file %q", credentialsFile)
	}

	creds, err := google.CredentialsFromJSON(context.TODO(), authJSON, dns.CloudPlatformScope)

	return creds, errors.Wrapf(err, "error parsing credentials file")
}
