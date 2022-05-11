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
	"github.com/spf13/cobra"
	"github.com/submariner-io/subctl/internal/cli"
	"github.com/submariner-io/subctl/pkg/diagnose"
	"github.com/submariner-io/subctl/pkg/subctl/cmd"
)

const (
	TCPSniffVxLANCommand = "tcpdump -ln -c 3 -i vx-submariner tcp and port 8080 and 'tcp[tcpflags] == tcp-syn'"
)

func init() {
	command := &cobra.Command{
		Use:   "intra-cluster",
		Short: "Check firewall access for intra-cluster Submariner VxLAN traffic",
		Long:  "This command checks if the firewall configuration allows traffic over vx-submariner interface.",
		Run: func(command *cobra.Command, args []string) {
			cmd.ExecuteMultiCluster(restConfigProducer, checkVxLANConfig)
		},
	}

	addDiagnoseFWConfigFlags(command)
	addVerboseFlag(command)
	diagnoseFirewallConfigCmd.AddCommand(command)
}

func checkVxLANConfig(c *cmd.Cluster) bool {
	return diagnose.VxLANConfig(clusterInfoFrom(c), diagnose.FirewallOptions{
		ValidationTimeout: validationTimeout,
		VerboseOutput:     verboseOutput,
		PodNamespace:      podNamespace,
	}, cli.NewStatus())
}
