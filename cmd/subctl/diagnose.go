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
	"context"
	goerrors "errors"
	"fmt"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/submariner-io/admiral/pkg/reporter"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/internal/exit"
	"github.com/submariner-io/subctl/internal/restconfig"
	"github.com/submariner-io/subctl/pkg/cluster"
	"github.com/submariner-io/subctl/pkg/diagnose"
)

var (
	DiagnoseCNIConfig                        = diagnose.CNIConfig
	DiagnoseConnections                      = diagnose.Connections
	DiagnoseDeployments                      = diagnose.Deployments
	DiagnoseGlobalnetConfig                  = diagnose.GlobalnetConfig
	DiagnoseK8sVersion                       = diagnose.K8sVersion
	DiagnoseKubeProxyMode                    = diagnose.KubeProxyMode
	DiagnoseServiceDiscovery                 = diagnose.ServiceDiscovery
	DiagnoseFirewallIntraVxLANConfig         = diagnose.FirewallIntraVxLANConfig
	DiagnoseTunnelConfigAcrossClusters       = diagnose.TunnelConfigAcrossClusters
	DiagnoseNatDiscoveryConfigAcrossClusters = diagnose.NatDiscoveryConfigAcrossClusters
)

type diagnoseCommand struct {
	firewallOptions                        diagnose.FirewallOptions
	restConfigProducer                     *restconfig.Producer
	firewallTunnelRestConfigProducer       *restconfig.Producer
	firewallNatDiscoveryRestConfigProducer *restconfig.Producer
}

func NewDiagnoseCmd() *cobra.Command {
	diagnoseCommand := &diagnoseCommand{
		restConfigProducer: restconfig.NewProducer().WithDefaultNamespace(constants.OperatorNamespace).WithInClusterFlag(),
		firewallTunnelRestConfigProducer: restconfig.NewProducer().
			WithDefaultNamespace(constants.OperatorNamespace).WithPrefixedContext("remote"),
		firewallNatDiscoveryRestConfigProducer: restconfig.NewProducer().
			WithDefaultNamespace(constants.OperatorNamespace).WithPrefixedContext("remote"),
	}

	cmd := &cobra.Command{
		Use:   "diagnose",
		Short: "Run diagnostic checks on the Submariner deployment and report any issues",
		Long:  "This command runs various diagnostic checks on the Submariner deployment and reports any issues",
	}

	diagnoseCommand.restConfigProducer.SetupFlags(cmd.PersistentFlags())
	diagnoseCommand.addDiagnoseSubCommands(cmd)

	return cmd
}

func init() {
	rootCmd.AddCommand(NewDiagnoseCmd())
}

func (c *diagnoseCommand) addDiagnoseSubCommands(toCmd *cobra.Command) {
	diagnoseAllCmd := &cobra.Command{
		Use:   "all",
		Short: "Run all diagnostic checks (except those requiring two kubecontexts)",
		Long:  "This command runs all diagnostic checks (except those requiring two kubecontexts) and reports any issues",
		Args:  checkDiagnoseArguments,
		Run: func(cmd *cobra.Command, _ []string) {
			exit.OnError(c.diagnoseAll(cmd.Context(), NewReporter()))
		},
	}
	toCmd.AddCommand(diagnoseAllCmd)

	c.addDiagnoseFWConfigFlags(diagnoseAllCmd)
	addImageOverrideFlag(diagnoseAllCmd.Flags(), &imageOverrides)

	diagnoseCNICmd := &cobra.Command{
		Use:   "cni",
		Short: "Check the CNI network plugin",
		Long:  "This command checks if the detected CNI network plugin is supported by Submariner.",
		Run: func(cmd *cobra.Command, _ []string) {
			exit.OnError(
				c.restConfigProducer.RunOnAllContexts(cmd.Context(), restconfig.IfConnectivityInstalled(DiagnoseCNIConfig), NewReporter()))
		},
	}
	toCmd.AddCommand(diagnoseCNICmd)

	diagnoseConnectionsCmd := &cobra.Command{
		Use:   "connections",
		Short: "Check the Gateway connections",
		Long:  "This command checks that the Gateway connections to other clusters are all established",
		Run: func(cmd *cobra.Command, _ []string) {
			exit.OnError(
				c.restConfigProducer.RunOnAllContexts(cmd.Context(),
					restconfig.IfConnectivityInstalled(DiagnoseConnections), NewReporter()))
		},
	}
	toCmd.AddCommand(diagnoseConnectionsCmd)

	diagnoseDeploymentCmd := &cobra.Command{
		Use:   "deployment",
		Short: "Check the Submariner deployment",
		Long:  "This command checks that the Submariner components are properly deployed and running with no overlapping CIDRs.",
		Args:  checkDiagnoseArguments,
		Run: func(cmd *cobra.Command, _ []string) {
			exit.OnError(
				c.restConfigProducer.RunOnAllContexts(cmd.Context(),
					func(ctx context.Context, clusterInfo *cluster.Info, ns string, status reporter.Interface) error {
						if clusterInfo.Submariner == nil && clusterInfo.ServiceDiscovery == nil {
							status.Warning(constants.SubmarinerNotInstalled)

							return nil
						}

						return deployments(ctx, clusterInfo, ns, status)
					}, NewReporter()))
		},
	}
	toCmd.AddCommand(diagnoseDeploymentCmd)
	addImageOverrideFlag(diagnoseDeploymentCmd.Flags(), &imageOverrides)

	diagnoseVersionCmd := &cobra.Command{
		Use:   "k8s-version",
		Short: "Check the Kubernetes version",
		Long:  "This command checks if Submariner can be deployed on the Kubernetes version.",
		Run: func(cmd *cobra.Command, _ []string) {
			exit.OnError(c.restConfigProducer.RunOnAllContexts(cmd.Context(), DiagnoseK8sVersion, NewReporter()))
		},
	}
	toCmd.AddCommand(diagnoseVersionCmd)

	diagnoseKubeProxyModeCmd := &cobra.Command{
		Use:   "kube-proxy-mode",
		Short: "Check the kube-proxy mode",
		Long:  "This command checks if the kube-proxy mode is supported by Submariner.",
		Args:  checkDiagnoseArguments,
		Run: func(cmd *cobra.Command, _ []string) {
			exit.OnError(
				c.restConfigProducer.RunOnAllContexts(cmd.Context(), restconfig.IfConnectivityInstalled(kubeProxyMode), NewReporter()))
		},
	}
	toCmd.AddCommand(diagnoseKubeProxyModeCmd)

	addImageOverrideFlag(diagnoseKubeProxyModeCmd.Flags(), &imageOverrides)

	diagnoseServiceDiscoveryCmd := &cobra.Command{
		Use:   "service-discovery",
		Short: "Check service discovery functionality",
		Long:  "This command checks if service discovery is functioning properly.",
		Run: func(cmd *cobra.Command, _ []string) {
			exit.OnError(c.restConfigProducer.RunOnAllContexts(cmd.Context(), restconfig.IfServiceDiscoveryInstalled(DiagnoseServiceDiscovery),
				NewReporter()))
		},
	}
	toCmd.AddCommand(diagnoseServiceDiscoveryCmd)

	diagnoseFirewallCmd := &cobra.Command{
		Use:   "firewall",
		Short: "Check the firewall configuration",
		Long:  "This command checks if the firewall is configured as per Submariner pre-requisites.",
	}
	toCmd.AddCommand(diagnoseFirewallCmd)

	c.addDiagnoseFirewallSubCommands(diagnoseFirewallCmd)
}

func (c *diagnoseCommand) addDiagnoseFirewallSubCommands(diagnoseFirewallCmd *cobra.Command) {
	diagnoseFirewallVxLANCmd := &cobra.Command{
		Use:   "intra-cluster",
		Short: "Check firewall access for intra-cluster Submariner VxLAN traffic",
		Long:  "This command checks if the firewall configuration allows traffic over vx-submariner interface.",
		Args:  checkFirewallArguments,
		Run: func(cmd *cobra.Command, _ []string) {
			exit.OnError(c.restConfigProducer.RunOnAllContexts(cmd.Context(), restconfig.IfConnectivityInstalled(c.firewallIntraVxLANConfig),
				NewReporter()))
		},
	}
	diagnoseFirewallCmd.AddCommand(diagnoseFirewallVxLANCmd)

	c.addDiagnoseFWConfigFlags(diagnoseFirewallVxLANCmd)
	addImageOverrideFlag(diagnoseFirewallVxLANCmd.Flags(), &imageOverrides)

	diagnoseFirewallTunnelCmd := &cobra.Command{
		Use:   "inter-cluster --context <localcontext> --remotecontext <remotecontext>",
		Short: "Check firewall access to setup tunnels between the Gateway node",
		Long:  "This command checks if the firewall configuration allows tunnels to be configured on the Gateway nodes.",
		Args:  checkFirewallArguments,
		Run: func(cmd *cobra.Command, _ []string) {
			c.runLocalRemoteFirewallCommand(cmd.Context(), c.firewallTunnelRestConfigProducer, DiagnoseTunnelConfigAcrossClusters)
		},
	}
	diagnoseFirewallCmd.AddCommand(diagnoseFirewallTunnelCmd)

	c.addDiagnoseFWConfigFlags(diagnoseFirewallTunnelCmd)
	c.firewallTunnelRestConfigProducer.SetupFlags(diagnoseFirewallTunnelCmd.Flags())
	addImageOverrideFlag(diagnoseFirewallTunnelCmd.Flags(), &imageOverrides)

	diagnoseFirewallNatDiscovery := &cobra.Command{
		Use:   "nat-discovery --context <localcontext> --remotecontext <remotecontext>",
		Short: "Check firewall access for nat-discovery to function properly",
		Long:  "This command checks if the firewall configuration allows nat-discovery between the configured Gateway nodes.",
		Args:  checkFirewallArguments,
		Run: func(cmd *cobra.Command, _ []string) {
			c.runLocalRemoteFirewallCommand(cmd.Context(), c.firewallNatDiscoveryRestConfigProducer, DiagnoseNatDiscoveryConfigAcrossClusters)
		},
	}
	diagnoseFirewallCmd.AddCommand(diagnoseFirewallNatDiscovery)

	c.addDiagnoseFWConfigFlags(diagnoseFirewallNatDiscovery)
	c.firewallNatDiscoveryRestConfigProducer.SetupFlags(diagnoseFirewallNatDiscovery.Flags())
	addImageOverrideFlag(diagnoseFirewallNatDiscovery.Flags(), &imageOverrides)
}

func (c *diagnoseCommand) addDiagnoseFWConfigFlags(command *cobra.Command) {
	command.Flags().UintVar(&c.firewallOptions.ValidationTimeout, "validation-timeout", 90,
		"time to run in seconds while validating the firewall")
	command.Flags().BoolVar(&c.firewallOptions.VerboseOutput, "verbose", false,
		"produce verbose output while validating the firewall")
}

func (c *diagnoseCommand) firewallIntraVxLANConfig(ctx context.Context, clusterInfo *cluster.Info, namespace string,
	status reporter.Interface,
) error {
	c.firewallOptions.ImageOverrides = imageOverrides

	return DiagnoseFirewallIntraVxLANConfig(ctx, clusterInfo, namespace, c.firewallOptions, status)
}

func checkDiagnoseArguments(_ *cobra.Command, _ []string) error {
	return checkImageOverrides(imageOverrides)
}

func checkFirewallArguments(cmd *cobra.Command, args []string) error {
	err := checkImageOverrides(imageOverrides)
	if err != nil {
		return err
	}

	return checkNoArguments(cmd, args)
}

func kubeProxyMode(ctx context.Context, clusterInfo *cluster.Info, namespace string, status reporter.Interface) error {
	return DiagnoseKubeProxyMode(ctx, clusterInfo, namespace, imageOverrides, status)
}

func deployments(ctx context.Context, clusterInfo *cluster.Info, namespace string, status reporter.Interface) error {
	return DiagnoseDeployments(ctx, clusterInfo, namespace, imageOverrides, status)
}

func (c *diagnoseCommand) diagnoseAll(ctx context.Context, status reporter.Interface) error {
	allDiagnoseCommands := []restconfig.PerContextFn{
		DiagnoseK8sVersion,
		deployments,
		restconfig.IfConnectivityInstalled(
			DiagnoseCNIConfig,
			DiagnoseConnections,
			kubeProxyMode,
			c.firewallIntraVxLANConfig,
			DiagnoseGlobalnetConfig),
		restconfig.IfServiceDiscoveryInstalled(DiagnoseServiceDiscovery),
	}

	err := c.restConfigProducer.RunOnAllContexts(
		ctx,
		func(ctx context.Context, clusterInfo *cluster.Info, namespace string, status reporter.Interface) error {
			diagnoseErrors := make([]error, 0, len(allDiagnoseCommands))

			for _, command := range allDiagnoseCommands {
				diagnoseErrors = append(diagnoseErrors, command(ctx, clusterInfo, namespace, status))

				fmt.Println()
			}

			return goerrors.Join(diagnoseErrors...)
		}, status)

	fmt.Printf("Skipping inter-cluster firewall check as it requires two kubeconfigs." +
		" Please run \"subctl diagnose firewall inter-cluster\" command manually.\n")

	return err //nolint:wrapcheck // No need to wrap errors here.
}

func (c *diagnoseCommand) runLocalRemoteFirewallCommand(ctx context.Context, localRemoteRestConfigProducer *restconfig.Producer,
	function func(ctx context.Context, localClusterInfo, remoteClusterInfo *cluster.Info, namespace string,
		options diagnose.FirewallOptions, status reporter.Interface,
	) error,
) {
	status := NewReporter()

	c.firewallOptions.ImageOverrides = imageOverrides

	exit.OnErrorWithMessage(localRemoteRestConfigProducer.RunOnSelectedContext(
		ctx,
		func(ctx context.Context, localClusterInfo *cluster.Info, localNamespace string, status reporter.Interface) error {
			found, err := localRemoteRestConfigProducer.RunOnSelectedPrefixedContext(
				ctx,
				"remote",
				func(ctx context.Context, remoteClusterInfo *cluster.Info, _ string, status reporter.Interface) error {
					return function(ctx, localClusterInfo, remoteClusterInfo, localNamespace, c.firewallOptions, status)
				}, status)
			if err != nil {
				return err //nolint:wrapcheck // No need to wrap errors here.
			}

			if !found {
				return errors.New("no remote context was specified")
			}

			return nil
		}, status), "Error running command")
}
