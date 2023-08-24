//go:build !deploy

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
	"fmt"
	"os/exec"
	"strings"

	"github.com/coreos/go-semver/semver"
	"github.com/spf13/cobra"
	"github.com/submariner-io/admiral/pkg/names"
	"github.com/submariner-io/admiral/pkg/reporter"
	"github.com/submariner-io/subctl/internal/cli"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/internal/exit"
	"github.com/submariner-io/subctl/internal/restconfig"
	"github.com/submariner-io/subctl/pkg/cluster"
	"github.com/submariner-io/subctl/pkg/deploy"
	"github.com/submariner-io/subctl/pkg/image"
	"github.com/submariner-io/subctl/pkg/operator"
	"github.com/submariner-io/subctl/pkg/version"
	"github.com/submariner-io/submariner-operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	to                        string
	upgradeRestConfigProducer = restconfig.NewProducer()
)

// upgradeCmd represents the upgrade command.
var upgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Upgrades Submariner",
	Run: func(cmd *cobra.Command, args []string) {
		status := cli.NewReporter()
		exit.OnError(upgradeSubctl(status))
		exit.OnError(upgradeRestConfigProducer.RunOnAllContexts(upgradeSubmariner, status))
	},
}

func init() {
	upgradeCmd.Flags().StringVar(&to, "to", "", "the version of Submariner to upgrade to")
	upgradeRestConfigProducer.SetupFlags(upgradeCmd.Flags())
	rootCmd.AddCommand(upgradeCmd)
}

func upgradeSubctl(status reporter.Interface) error {
	if to == version.Version {
		// Already running the right version
		return nil
	}

	// Default to downloading the latest version
	targetVersionString := "latest"

	if to != "" {
		to = strings.TrimPrefix(to, "v")

		toVersion, err := semver.NewVersion(to)
		if toVersion == nil {
			return status.Error(err, "Invalid target version")
		}

		// semver needs a dotted triplet, which is at least five characters;
		// on development or unknown versions, assume we need to upgrade
		if len(version.Version) >= 5 && version.Version[0:5] != "devel" {
			currentVersion, err := semver.NewVersion(version.Version)
			if currentVersion == nil {
				return status.Error(err, "Error parsing current subctl version")
			}

			if toVersion.LessThan(*currentVersion) || toVersion.Equal(*currentVersion) {
				return nil
			}
		}

		targetVersionString = "v" + toVersion.String()
	}

	status.Start(fmt.Sprintf("Upgrading subctl from %s to %s", version.Version, targetVersionString))
	defer status.End()

	url := "https://get.submariner.io"

	_, err := exec.Command("sh", "-c", "curl "+url+" | VERSION="+targetVersionString+" bash").CombinedOutput()
	if err != nil {
		return status.Error(err, "Error upgrading subctl")
	}

	status.Success("Upgraded and installed subctl version: %s", to)

	return nil
}

func upgradeSubmariner(clusterInfo *cluster.Info, _ string, status reporter.Interface) error {
	ctx := context.TODO()

	status.Start("Starting upgrade process")
	defer status.End()

	brokerObj, found, err := getBroker(clusterInfo.RestConfig, constants.DefaultBrokerNamespace)
	if err != nil {
		return err
	}

	if found {
		// Role updates are part of Broker redeploy
		err = upgradeBroker(ctx, clusterInfo, status, brokerObj.Namespace, brokerObj.Spec)
		if err != nil {
			return status.Error(err, "Error upgrading Broker")
		}
	}

	var repository string
	var debug bool
	var imageOverride map[string]string

	if !found {
		if clusterInfo.Submariner != nil {
			repository = clusterInfo.Submariner.Spec.Repository
			imageOverride = clusterInfo.Submariner.Spec.ImageOverrides
			debug = clusterInfo.Submariner.Spec.Debug
		} else if clusterInfo.ServiceDiscovery != nil {
			repository = clusterInfo.ServiceDiscovery.Spec.Repository
			imageOverride = clusterInfo.ServiceDiscovery.Spec.ImageOverrides
			debug = clusterInfo.ServiceDiscovery.Spec.Debug
		}

		// Upgrade Operator if deployed
		if err := upgradeOperator(ctx, clusterInfo, repository, debug, imageOverride, status); err != nil {
			return status.Error(err, "Error upgrading Operator")
		}

		// Upgrade Submariner
		if clusterInfo.Submariner != nil {
			status.Start("Found Submariner components. Upgrading it to %s", to)

			clusterInfo.Submariner.Spec.Version = to

			err := deploy.SubmarinerFromSpec(ctx, clusterInfo.ClientProducer.ForGeneral(), &clusterInfo.Submariner.Spec)
			if err != nil {
				return status.Error(err, "Error upgrading Submariner")
			}

			status.Success("Submariner successfully upgraded")
		}

		// Upgrade Service discovery
		if clusterInfo.ServiceDiscovery != nil {
			status.Start("Found Service Discovery components. Upgrading it to %s", to)

			clusterInfo.ServiceDiscovery.Spec.Version = to

			err := deploy.ServiceDiscoveryFromSpec(ctx, clusterInfo.ClientProducer.ForGeneral(), &clusterInfo.ServiceDiscovery.Spec)
			if err != nil {
				return status.Error(err, "Error upgrading Service Discovery")
			}

			status.Success("Service discovery successfully upgraded.")
		}
	}

	return nil
}

func upgradeBroker(ctx context.Context, clusterInfo *cluster.Info, status reporter.Interface, namespace string,
	brokerSpec v1alpha1.BrokerSpec,
) error {
	status.Start("Found Broker installed. Upgrading it to %s", to)
	options := &deploy.BrokerOptions{
		ImageVersion:    to,
		BrokerNamespace: namespace,
		BrokerSpec:      brokerSpec,
	}

	if err := deploy.Deploy(ctx, options, status, clusterInfo.ClientProducer); err != nil {
		return err //nolint:wrapcheck // No need to wrap here
	}

	status.Success("Broker successfully upgraded.")

	return nil
}

func upgradeOperator(ctx context.Context, clusterInfo *cluster.Info, repository string, debug bool, imageOverride map[string]string,
	status reporter.Interface,
) error {
	status.Start("Checking if Operator is deployed")
	defer status.End()

	operatorDeployment, err := clusterInfo.ClientProducer.ForKubernetes().AppsV1().Deployments(constants.OperatorNamespace).
		Get(ctx, names.OperatorComponent, metav1.GetOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return status.Error(err, "Error retrieving Operator deployment")
	}

	if operatorDeployment != nil {
		status.Success("Operator deployed. Upgrading it")

		repositoryInfo := image.NewRepositoryInfo(repository, to, imageOverride)

		err = operator.Ensure(
			ctx, status, clusterInfo.ClientProducer, constants.OperatorNamespace, repositoryInfo.GetOperatorImage(), debug)
		if err != nil {
			return status.Error(err, "Error upgrading the Operator")
		}

		status.Success("Operator successfully upgraded")
	}

	return nil
}
