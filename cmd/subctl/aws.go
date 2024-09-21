//go:build !non_deploy

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
	cpaws "github.com/submariner-io/cloud-prepare/pkg/aws"
	"github.com/submariner-io/subctl/internal/cli"
	"github.com/submariner-io/subctl/internal/exit"
	cloudaws "github.com/submariner-io/subctl/pkg/cloud/aws"
	"github.com/submariner-io/subctl/pkg/cloud/cleanup"
	"github.com/submariner-io/subctl/pkg/cloud/prepare"
	"github.com/submariner-io/subctl/pkg/cluster"
)

var (
	awsConfig cloudaws.Config

	awsPrepareCmd = &cobra.Command{
		Use:     "aws",
		Short:   "Prepare an OpenShift AWS cloud",
		Long:    "This command prepares an OpenShift installer-provisioned infrastructure (IPI) on AWS cloud for Submariner installation.",
		PreRunE: checkAWSFlags,
		Run: func(cmd *cobra.Command, args []string) {
			exit.OnError(cloudRestConfigProducer.RunOnSelectedContext(
				func(clusterInfo *cluster.Info, namespace string, status reporter.Interface) error {
					return prepare.AWS( //nolint:wrapcheck // Not needed.
						clusterInfo, &cloudOptions.ports, &awsConfig, cloudOptions.useLoadBalancer, status)
				}, cli.NewReporter()))
		},
	}

	awsCleanupCmd = &cobra.Command{
		Use:   "aws",
		Short: "Clean up an AWS cloud",
		Long: "This command cleans up an OpenShift installer-provisioned infrastructure (IPI) on " +
			"AWS-based cloud after Submariner uninstallation.",
		PreRunE: checkAWSFlags,
		Run: func(cmd *cobra.Command, args []string) {
			exit.OnError(cloudRestConfigProducer.RunOnSelectedContext(
				func(clusterInfo *cluster.Info, namespace string, status reporter.Interface) error {
					return cleanup.AWS(clusterInfo, &awsConfig, status) //nolint:wrapcheck // No need to wrap errors here.
				}, cli.NewReporter()))
		},
	}
)

func init() {
	addGeneralAWSFlags := func(command *cobra.Command) {
		command.Flags().StringVar(&awsConfig.InfraID, infraIDFlag, "", "AWS infra ID")
		command.Flags().StringVar(&awsConfig.Region, regionFlag, "", "AWS region")
		command.Flags().StringVar(&awsConfig.OcpMetadataFile, "ocp-metadata", "",
			"OCP metadata.json file (or directory containing it) to read AWS infra ID and region from (Takes precedence over the flags)")
		command.Flags().StringVar(&awsConfig.Profile, "profile", cpaws.DefaultProfile(), "AWS profile to use for credentials")
		command.Flags().StringVar(&awsConfig.CredentialsFile, "credentials", cpaws.DefaultCredentialsFile(), "AWS credentials configuration file")

		command.Flags().StringVar(&awsConfig.ControlPlaneSecurityGroup, "control-plane-security-group", "",
			"Custom AWS control plane security group name if the default is not used while provisioning")
		command.Flags().StringVar(&awsConfig.WorkerSecurityGroup, "worker-security-group", "",
			"Custom AWS worker security group name if the default is not used while provisioning")
		command.Flags().StringVar(&awsConfig.VpcName, "vpc-name", "",
			"Custom AWS VPC name if the default is not used while provisioning")
		command.Flags().StringSliceVar(&awsConfig.SubnetNames, "subnet-names", nil,
			"Custom AWS subnet names if the default is not used while provisioning (comma-separated list)")
	}

	addGeneralAWSFlags(awsPrepareCmd)
	awsPrepareCmd.Flags().StringVar(&awsConfig.GWInstanceType, "gateway-instance", "c5d.large", "Type of gateways instance machine")
	awsPrepareCmd.Flags().IntVar(&awsConfig.Gateways, "gateways", defaultNumGateways,
		"Number of dedicated gateways to deploy (Set to `0` when using --load-balancer mode)")

	cloudPrepareCmd.AddCommand(awsPrepareCmd)

	addGeneralAWSFlags(awsCleanupCmd)
	cloudCleanupCmd.AddCommand(awsCleanupCmd)
}

func checkAWSFlags(_ *cobra.Command, _ []string) error {
	if awsConfig.OcpMetadataFile == "" {
		expectFlag(infraIDFlag, awsConfig.InfraID)
		expectFlag(regionFlag, awsConfig.Region)
	}

	return nil
}
