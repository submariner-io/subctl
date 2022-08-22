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

package subctl

import (
	"github.com/spf13/cobra"
	"github.com/submariner-io/subctl/internal/restconfig"
	"github.com/submariner-io/subctl/pkg/cloud"
	"github.com/submariner-io/submariner/pkg/port"
)

const (
	infraIDFlag        = "infra-id"
	regionFlag         = "region"
	defaultNumGateways = 1
	projectIDFlag      = "project-id"
	cloudEntryFlag     = "cloud-entry"
)

var (
	cloudPorts cloud.Ports

	cloudRestConfigProducer = restconfig.NewProducer()

	cloudCmd = &cobra.Command{
		Use:   "cloud",
		Short: "Cloud operations",
		Long:  `This command contains cloud operations related to Submariner installation.`,
	}

	cloudCleanupCmd = &cobra.Command{
		Use:   "cleanup",
		Short: "Clean up the cloud",
		Long:  `This command cleans up the cloud after Submariner uninstallation.`,
	}

	cloudPrepareCmd = &cobra.Command{
		Use:   "prepare",
		Short: "Prepare the cloud",
		Long:  `This command prepares the cloud for Submariner installation.`,
	}
)

func init() {
	cloudRestConfigProducer.AddKubeContextFlag(cloudCmd)
	rootCmd.AddCommand(cloudCmd)

	cloudPrepareCmd.PersistentFlags().Uint16Var(&cloudPorts.Natt, "natt-port", port.ExternalTunnel,
		"IPSec NAT traversal port")
	cloudPrepareCmd.PersistentFlags().Uint16Var(&cloudPorts.NatDiscovery, "nat-discovery-port",
		port.NATTDiscovery, "NAT discovery port")
	cloudPrepareCmd.PersistentFlags().Uint16Var(&cloudPorts.Vxlan, "vxlan-port", port.IntraClusterVxLAN, "Internal VXLAN port")

	ports := []uint16{}

	cloudPrepareCmd.PersistentFlags().Var(&uint16Slice{value: &ports}, "metrics-ports", "Metrics ports")
	_ = cloudPrepareCmd.PersistentFlags().MarkDeprecated("metrics-ports", "Metrics ports are no longer used, this flag is ignored")

	cloudCmd.AddCommand(cloudPrepareCmd)

	cloudCmd.AddCommand(cloudCleanupCmd)
}
