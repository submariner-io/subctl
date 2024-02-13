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

package subctl

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
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
	"k8s.io/utils/set"
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
	Run: func(_ *cobra.Command, _ []string) {
		exit.OnError(deployRestConfigProducer.RunOnSelectedContext(deployBrokerInContext, cli.NewReporter()))
	},
}

func init() {
	addDeployBrokerFlags(deployBroker.Flags())
	deployRestConfigProducer.SetupFlags(deployBroker.Flags())
	rootCmd.AddCommand(deployBroker)
}

func addDeployBrokerFlags(flags *pflag.FlagSet) {
	flags.BoolVar(&deployflags.BrokerSpec.GlobalnetEnabled, "globalnet", false,
		"enable support for Overlapping CIDRs in connecting clusters (default disabled)")
	flags.StringVar(&deployflags.BrokerSpec.GlobalnetCIDRRange, "globalnet-cidr-range",
		globalnet.DefaultGlobalnetCIDR, "GlobalCIDR supernet range for allocating GlobalCIDRs to each cluster")
	flags.UintVar(&deployflags.BrokerSpec.DefaultGlobalnetClusterSize, "globalnet-cluster-size",
		globalnet.DefaultGlobalnetClusterSize, "default cluster size for GlobalCIDR allocated to each cluster (amount of global IPs)")

	flags.StringVar(&ipsecSubmFile, "ipsec-psk-from", "",
		"import IPsec PSK from existing submariner broker file, like broker-info.subm")

	flags.StringSliceVar(&deployflags.BrokerSpec.DefaultCustomDomains, "custom-domains", nil,
		"list of domains to use for multicluster service discovery")

	flags.StringSliceVar(&deployflags.BrokerSpec.Components, "components", defaultComponents,
		fmt.Sprintf("The components to be installed - any of %s", strings.Join(deploy.ValidComponents, ",")))

	flags.StringVar(&deployflags.Repository, "repository", "", "image repository")
	flags.StringVar(&deployflags.ImageVersion, "version", "", "image version")

	flags.BoolVar(&deployflags.OperatorDebug, "operator-debug", false, "enable operator debugging (verbose logging)")

	flags.StringVar(&deployflags.BrokerURL, "broker-url", "",
		"broker API endpoint URL (stored in the broker information file, defaults to the context URL)")
}

func deployBrokerInContext(clusterInfo *cluster.Info, namespace string, status reporter.Interface) error {
	deployflags.BrokerNamespace = namespace

	if err := deploy.Broker(&deployflags, clusterInfo.ClientProducer, status); err != nil {
		return err //nolint:wrapcheck // No need to wrap errors here.
	}

	ipsecPSK := []byte{}
	var err error

	if ipsecSubmFile != "" {
		ipsecData, err := broker.ReadInfoFromFile(ipsecSubmFile)
		if err != nil {
			return errors.Wrapf(err, "error importing IPsec PSK from file %q", ipsecSubmFile)
		}

		ipsecPSK = ipsecData.IPSecPSK.Data["psk"]
	}

	if len(ipsecPSK) == 0 {
		ipsecPSK, err = broker.GenerateRandomPSK()
		if err != nil {
			return err //nolint:wrapcheck // No need to wrap errors here.
		}
	}

	return broker.WriteInfoToFile( //nolint:wrapcheck // No need to wrap errors here.
		clusterInfo.RestConfig, namespace, deployflags.BrokerURL, ipsecPSK,
		set.New(deployflags.BrokerSpec.Components...), deployflags.BrokerSpec.DefaultCustomDomains, status)
}
