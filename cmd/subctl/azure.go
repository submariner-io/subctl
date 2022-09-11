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

//nolint:dupl // Similar code in aws.go, azure.go, rhos.go, but not duplicate
package subctl

import (
	"github.com/spf13/cobra"
	"github.com/submariner-io/admiral/pkg/reporter"
	"github.com/submariner-io/subctl/internal/cli"
	"github.com/submariner-io/subctl/internal/exit"
	"github.com/submariner-io/subctl/pkg/cloud/azure"
	"github.com/submariner-io/subctl/pkg/cloud/cleanup"
	"github.com/submariner-io/subctl/pkg/cloud/prepare"
	"github.com/submariner-io/subctl/pkg/cluster"
)

var (
	azureConfig azure.Config

	azurePrepareCmd = &cobra.Command{
		Use:     "azure",
		Short:   "Prepare an OpenShift Azure cloud",
		Long:    "This command prepares an OpenShift installer-provisioned infrastructure (IPI) on Azure cloud for Submariner installation.",
		PreRunE: checkAzureFlags,
		Run: func(cmd *cobra.Command, args []string) {
			exit.OnError(cloudRestConfigProducer.RunOnSelectedContext(
				func(clusterInfo *cluster.Info, namespace string, status reporter.Interface) error {
					return prepare.Azure(clusterInfo, &cloudPorts, &azureConfig, cloudOptions.useLoadBalancer, status) //nolint:wrapcheck // Not needed.
				}, cli.NewReporter()))
		},
	}

	azureCleanupCmd = &cobra.Command{
		Use:   "azure",
		Short: "Clean up an Azure cloud",
		Long: "This command cleans up an OpenShift installer-provisioned infrastructure (IPI) on Azure-based" +
			" cloud after Submariner uninstallation.",
		PreRunE: checkAzureFlags,
		Run: func(cmd *cobra.Command, args []string) {
			exit.OnError(cloudRestConfigProducer.RunOnSelectedContext(
				func(clusterInfo *cluster.Info, namespace string, status reporter.Interface) error {
					return cleanup.Azure(clusterInfo, &azureConfig, status) //nolint:wrapcheck // No need to wrap errors here.
				}, cli.NewReporter()))
		},
	}
)

func init() {
	addGeneralAzureFlags := func(command *cobra.Command) {
		command.Flags().StringVar(&azureConfig.InfraID, infraIDFlag, "", "Azure infra ID")
		command.Flags().StringVar(&azureConfig.Region, regionFlag, "", "Azure region")
		command.Flags().StringVar(&azureConfig.OcpMetadataFile, "ocp-metadata", "",
			"OCP metadata.json file (or directory containing it) to read Azure infra ID and region from (Takes precedence over the flags)")
		command.Flags().StringVar(&azureConfig.AuthFile, "auth-file", "", "Azure authorization file to be used")
	}

	addGeneralAzureFlags(azurePrepareCmd)
	azurePrepareCmd.Flags().IntVar(&azureConfig.Gateways, "gateways", defaultNumGateways, "Number of gateways to deploy")
	// `Standard_F4s_v2` matches the most to `cd5.large` of AWS.
	azurePrepareCmd.Flags().StringVar(&azureConfig.GWInstanceType, "gateway-instance", "Standard_F4s_v2", "Type of gateways instance machine")
	azurePrepareCmd.Flags().BoolVar(&azureConfig.DedicatedGateway, "dedicated-gateway", true,
		"Whether a dedicated gateway node has to be deployed")
	cloudPrepareCmd.AddCommand(azurePrepareCmd)

	addGeneralAzureFlags(azureCleanupCmd)
	cloudCleanupCmd.AddCommand(azureCleanupCmd)
}

func checkAzureFlags(cmd *cobra.Command, args []string) error {
	if azureConfig.OcpMetadataFile == "" {
		expectFlag(infraIDFlag, azureConfig.InfraID)
		expectFlag(regionFlag, azureConfig.Region)
	}

	expectFlag("auth-file", azureConfig.AuthFile)

	return nil
}
