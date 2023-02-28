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
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/submariner-io/admiral/pkg/reporter"
	"github.com/submariner-io/subctl/internal/cli"
	"github.com/submariner-io/subctl/internal/component"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/internal/exit"
	"github.com/submariner-io/subctl/internal/restconfig"
	"github.com/submariner-io/subctl/pkg/broker"
	"github.com/submariner-io/subctl/pkg/cluster"
	"github.com/submariner-io/subctl/pkg/deploy"
	"github.com/submariner-io/submariner-operator/pkg/discovery/globalnet"
	"k8s.io/apimachinery/pkg/util/sets"
)

var (
	deployflags       deploy.BrokerOptions
	ipsecSubmFile     string
	defaultComponents = []string{component.ServiceDiscovery, component.Connectivity}
)

var deployRestConfigProducer = restconfig.NewProducer().
	WithDefaultNamespace(constants.DefaultBrokerNamespace)

// deployBroker represents the deployBroker command.
var deployBroker = &cobra.Command{
	Use:   "deploy-broker",
	Short: "Deploys the broker",
	Run: func(cmd *cobra.Command, args []string) {
		exit.OnError(deployRestConfigProducer.RunOnSelectedContext(deployBrokerInContext, cli.NewReporter()))
	},
}

func init() {
	addDeployBrokerFlags()
	deployRestConfigProducer.SetupFlags(deployBroker.Flags())
	rootCmd.AddCommand(deployBroker)
}

func addDeployBrokerFlags() {
	deployBroker.PersistentFlags().BoolVar(&deployflags.BrokerSpec.GlobalnetEnabled, "globalnet", false,
		"enable support for Overlapping CIDRs in connecting clusters (default disabled)")
	deployBroker.PersistentFlags().StringVar(&deployflags.BrokerSpec.GlobalnetCIDRRange, "globalnet-cidr-range",
		globalnet.DefaultGlobalnetCIDR, "GlobalCIDR supernet range for allocating GlobalCIDRs to each cluster")
	deployBroker.PersistentFlags().UintVar(&deployflags.BrokerSpec.DefaultGlobalnetClusterSize, "globalnet-cluster-size",
		globalnet.DefaultGlobalnetClusterSize, "default cluster size for GlobalCIDR allocated to each cluster (amount of global IPs)")

	deployBroker.PersistentFlags().StringVar(&ipsecSubmFile, "ipsec-psk-from", "",
		"import IPsec PSK from existing submariner broker file, like broker-info.subm")

	deployBroker.PersistentFlags().StringSliceVar(&deployflags.BrokerSpec.DefaultCustomDomains, "custom-domains", nil,
		"list of domains to use for multicluster service discovery")

	deployBroker.PersistentFlags().StringSliceVar(&deployflags.BrokerSpec.Components, "components", defaultComponents,
		fmt.Sprintf("The components to be installed - any of %s", strings.Join(deploy.ValidComponents, ",")))

	deployBroker.PersistentFlags().StringVar(&deployflags.Repository, "repository", "", "image repository")
	deployBroker.PersistentFlags().StringVar(&deployflags.ImageVersion, "version", "", "image version")

	deployBroker.PersistentFlags().BoolVar(&deployflags.OperatorDebug, "operator-debug", false, "enable operator debugging (verbose logging)")
}

func deployBrokerInContext(clusterInfo *cluster.Info, namespace string, status reporter.Interface) error {
	deployflags.BrokerNamespace = namespace

	if err := deploy.Broker(&deployflags, clusterInfo.ClientProducer, status); err != nil {
		return err //nolint:wrapcheck // No need to wrap errors here.
	}

	return broker.WriteInfoToFile( //nolint:wrapcheck // No need to wrap errors here.
		clusterInfo.RestConfig, namespace, ipsecSubmFile,
		sets.New(deployflags.BrokerSpec.Components...), deployflags.BrokerSpec.DefaultCustomDomains, status)
}
