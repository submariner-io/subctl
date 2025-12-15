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
	"strings"

	"github.com/pkg/errors"
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
	"github.com/submariner-io/submariner-operator/pkg/discovery/clustersetip"
	"github.com/submariner-io/submariner-operator/pkg/discovery/globalnet"
	"k8s.io/utils/set"
)

var (
	defaultComponents      = []string{component.ServiceDiscovery, component.Connectivity}
	DeployBroker           = deploy.Broker
	ReadBrokerInfoFromFile = broker.ReadInfoFromFile
	WriteBrokerInfoToFile  = broker.WriteInfoToFile
)

type DeployBrokerCommand struct {
	cmd                *cobra.Command
	flags              deploy.BrokerOptions
	ipsecPSKFile       string
	restConfigProducer *restconfig.Producer
}

// NewDeployBrokerCmd returns the deploy-broker command.
func NewDeployBrokerCmd() *cobra.Command {
	deployCmd := &DeployBrokerCommand{
		restConfigProducer: restconfig.NewProducer().WithDefaultNamespace(constants.DefaultBrokerNamespace),
	}

	deployCmd.cmd = &cobra.Command{
		Use:   "deploy-broker",
		Short: "Deploys the broker",
		Run: func(_ *cobra.Command, _ []string) {
			exit.OnError(deployCmd.restConfigProducer.RunOnSelectedContext(deployCmd.deployBrokerInContext, cli.NewReporter()))
		},
	}

	deployCmd.addFlags()
	deployCmd.restConfigProducer.SetupFlags(deployCmd.cmd.Flags())
	addHTTPProxyFlags(deployCmd.cmd.Flags(), &deployCmd.flags.HTTPProxyConfig)

	return deployCmd.cmd
}

// deployBroker represents the deployBroker command.
var deployBroker = NewDeployBrokerCmd()

func init() {
	rootCmd.AddCommand(deployBroker)
}

func (c *DeployBrokerCommand) addFlags() {
	c.cmd.Flags().BoolVar(&c.flags.BrokerSpec.GlobalnetEnabled, "globalnet", false,
		"enable support for Overlapping CIDRs in connecting clusters (default disabled)")
	c.cmd.Flags().StringVar(&c.flags.BrokerSpec.GlobalnetCIDRRange, "globalnet-cidr-range",
		globalnet.DefaultGlobalnetCIDR, "GlobalCIDR supernet range for allocating GlobalCIDRs to each cluster")
	c.cmd.Flags().UintVar(&c.flags.BrokerSpec.DefaultGlobalnetClusterSize, "globalnet-cluster-size",
		globalnet.DefaultGlobalnetClusterSize, "default cluster size for GlobalCIDR allocated to each cluster (amount of global IPs)")

	c.cmd.Flags().StringVar(&c.ipsecPSKFile, "ipsec-psk-from", "",
		"import IPsec PSK from existing submariner broker file, like broker-info.subm")

	c.cmd.Flags().StringSliceVar(&c.flags.BrokerSpec.DefaultCustomDomains, "custom-domains", nil,
		"list of domains to use for multicluster service discovery")

	c.cmd.Flags().StringSliceVar(&c.flags.BrokerSpec.Components, "components", defaultComponents,
		"The components to be installed - any of "+strings.Join(deploy.ValidComponents, ","))

	c.cmd.Flags().StringVar(&c.flags.Repository, "repository", "", "image repository")
	c.cmd.Flags().StringVar(&c.flags.ImageVersion, "version", "", "image version")

	c.cmd.Flags().BoolVar(&c.flags.OperatorDebug, "operator-debug", false, "enable operator debugging (verbose logging)")

	c.cmd.Flags().StringVar(&c.flags.BrokerURL, "broker-url", "",
		"broker API endpoint URL (stored in the broker information file, defaults to the context URL)")
	c.cmd.Flags().BoolVar(&c.flags.BrokerSpec.ClustersetIPEnabled, "enable-clusterset-ip", false,
		"set default support for use of clusterset IP for exported services in connecting clusters (default disabled)")
	c.cmd.Flags().StringVar(&c.flags.BrokerSpec.ClustersetIPCIDRRange, "clusterset-ip-cidr-range",
		clustersetip.DefaultCIDR, "Clusterset IP CIDR supernet range for allocating Clusterset IP CIDRs to each cluster")
}

func (c *DeployBrokerCommand) deployBrokerInContext(clusterInfo *cluster.Info, namespace string, status reporter.Interface) error {
	c.flags.BrokerNamespace = namespace

	if err := DeployBroker(&c.flags, clusterInfo.ClientProducer, status); err != nil {
		return err
	}

	ipsecPSK := []byte{}
	var err error

	if c.ipsecPSKFile != "" {
		ipsecData, err := ReadBrokerInfoFromFile(c.ipsecPSKFile)
		if err != nil {
			return errors.Wrapf(err, "error importing IPsec PSK from file %q", c.ipsecPSKFile)
		}

		ipsecPSK = ipsecData.IPSecPSK.Data["psk"]
	}

	if len(ipsecPSK) == 0 {
		ipsecPSK, err = broker.GenerateRandomPSK()
		if err != nil {
			return err //nolint:wrapcheck // No need to wrap errors here.
		}
	}

	return WriteBrokerInfoToFile(
		clusterInfo.RestConfig, namespace, c.flags.BrokerURL, ipsecPSK,
		set.New(c.flags.BrokerSpec.Components...), c.flags.BrokerSpec.DefaultCustomDomains, status)
}
