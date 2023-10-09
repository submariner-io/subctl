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
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/coreos/go-semver/semver"
	"github.com/google/go-github/v54/github"
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
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	upgradeSubctlVersion      string
	upgradeOperatorVersion    string
	upgradeSubmarinerVersion  string
	upgradeRestConfigProducer = restconfig.NewProducer()
)

// upgradeCmd represents the upgrade command.
var upgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Upgrades Submariner",
	Run:   upgrade,
}

func init() {
	upgradeCmd.Flags().StringVar(&upgradeSubctlVersion, "to-version", "", "the version of subctl and Submariner to which to upgrade")
	upgradeCmd.Flags().StringVar(&upgradeOperatorVersion, "to-operator-version", "", "the version of the operator to which to upgrade")
	_ = upgradeCmd.Flags().MarkHidden("to-operator-version")
	upgradeCmd.Flags().StringVar(&upgradeSubmarinerVersion, "to-submariner-version", "", "the version of Submariner to which to upgrade")
	_ = upgradeCmd.Flags().MarkHidden("to-submariner-version")
	upgradeRestConfigProducer.SetupFlags(upgradeCmd.Flags())
	rootCmd.AddCommand(upgradeCmd)
}

func upgrade(_ *cobra.Command, _ []string) {
	status := cli.NewReporter()

	// Step 1: upgrade subctl to match the requested version
	command, err := upgradeSubctl(status)
	exit.OnError(err)

	if command != "" {
		// Step 2a: subctl was upgraded, so run it instead of continuing
		cmd := exec.Cmd{
			Path:   command,
			Args:   os.Args,
			Stdin:  os.Stdin,
			Stdout: os.Stdout,
			Stderr: os.Stderr,
		}
		// exit.OnError outputs the version of subctl, which ends up being confusing here
		if err := cmd.Run(); err != nil {
			os.Exit(1)
		}
	} else {
		// Step 2b: this subctl is already the requested version, run it
		exit.OnError(upgradeRestConfigProducer.RunOnAllContexts(upgradeSubmariner, status))
	}
}

// upgradeSubctl upgrades the local copy of subctl, if necessary.
// Returns the path to the upgraded subctl if subctl was upgraded, nil if it wasn't.
func upgradeSubctl(status reporter.Interface) (string, error) {
	// Default to downloading the latest version
	targetVersionString := "latest"

	// If the user hasn't specified a version, try to find the latest release on GitHub
	if upgradeSubctlVersion == "" {
		client := github.NewClient(nil)
		latestRelease, _, err := client.Repositories.GetLatestRelease(context.TODO(), "submariner-io", "releases")

		// If we can't determine the latest release, we'll force a download and delegate to get.submariner.io
		if err == nil {
			upgradeSubctlVersion = *latestRelease.TagName
		}
	}

	if upgradeSubctlVersion == version.Version {
		// Already running the right version
		return "", nil
	}

	if upgradeSubctlVersion != "" {
		upgradeSubctlVersion = strings.TrimPrefix(upgradeSubctlVersion, "v")

		toVersion, err := semver.NewVersion(upgradeSubctlVersion)
		if toVersion == nil {
			return "", status.Error(err, "Invalid target version")
		}

		// semver needs a dotted triplet, which is at least five characters;
		// on development or unknown versions, assume we need to upgrade
		if len(version.Version) >= 5 && !strings.HasPrefix(version.Version, "devel") && !strings.HasPrefix(version.Version, "release") {
			currentVersion, err := semver.NewVersion(strings.TrimPrefix(version.Version, "v"))
			if currentVersion == nil {
				return "", status.Error(err, "Error parsing current subctl version")
			}

			if toVersion.LessThan(*currentVersion) || toVersion.Equal(*currentVersion) {
				return "", nil
			}
		}

		targetVersionString = "v" + toVersion.String()
	}

	status.Start("Upgrading subctl from %s to %s, replacing %s", version.Version, targetVersionString, os.Args[0])

	url := "https://get.submariner.io"

	absolutePath, err := filepath.Abs(os.Args[0])
	if err != nil {
		return "", status.Error(err, "Error determining the installation path")
	}

	_, err = exec.Command( //nolint:gosec // The user-controlled variables are sanitised above
		"sh", "-c", "curl "+url+" | VERSION="+targetVersionString+" DESTDIR="+filepath.Dir(absolutePath)+" bash").CombinedOutput()
	if err != nil {
		return "", status.Error(err, "Error upgrading subctl")
	}

	status.End()

	return absolutePath, nil
}

func upgradeSubmariner(clusterInfo *cluster.Info, _ string, status reporter.Interface) error {
	ctx := context.TODO()

	// We only expect users to specify a subctl version, if any ("--to-version"). In such scenarios,
	// the versions are expected to align, so subctl vX installs the operator image tagged with vX,
	// and that operator defaults to the appropriate Submariner version.
	// Other versions can be set for debugging purposes (to test installation with development versions,
	// before tags are aligned).
	// If the operator version isn't specified, it should match the version of subctl.
	// If the Submariner version isn't specified, it should be left blank so that the operator uses
	// its defaults.
	if upgradeOperatorVersion == "" {
		upgradeOperatorVersion = upgradeSubctlVersion
	}

	// Upgrade Broker if installed; role updates are part of Broker redeploy
	brokerUpgraded, err := upgradeBroker(ctx, clusterInfo, status)
	if err != nil {
		return err
	}

	var repository string
	var debug bool
	var imageOverride map[string]string

	if clusterInfo.Submariner != nil {
		repository = clusterInfo.Submariner.Spec.Repository
		imageOverride = clusterInfo.Submariner.Spec.ImageOverrides
		debug = clusterInfo.Submariner.Spec.Debug
	} else if clusterInfo.ServiceDiscovery != nil {
		repository = clusterInfo.ServiceDiscovery.Spec.Repository
		imageOverride = clusterInfo.ServiceDiscovery.Spec.ImageOverrides
		debug = clusterInfo.ServiceDiscovery.Spec.Debug
	} else {
		// Nothing further to do
		return nil
	}

	// If a Broker was upgraded in this context, the Operator has already been upgraded
	if !brokerUpgraded {
		// Upgrade Operator if deployed
		if err := upgradeOperator(ctx, clusterInfo, repository, debug, imageOverride, status); err != nil {
			return err
		}
	}

	// We want to show the user a version; use the most specific one
	logVersion := upgradeSubctlVersion
	if upgradeOperatorVersion != "" {
		logVersion = upgradeOperatorVersion
	}

	if upgradeSubmarinerVersion != "" {
		logVersion = upgradeSubmarinerVersion
	}

	// Upgrade Submariner
	if err := upgradeConnectivity(ctx, clusterInfo, logVersion, status); err != nil {
		return err
	}

	// Upgrade Service discovery
	return upgradeServiceDiscovery(ctx, clusterInfo, logVersion, status)
}

func upgradeBroker(ctx context.Context, clusterInfo *cluster.Info, status reporter.Interface) (bool, error) {
	status.Start("Checking if the Broker is installed")
	defer status.End()

	brokerObj, brokerFound, err := getBroker(ctx, clusterInfo.RestConfig, constants.DefaultBrokerNamespace)
	if err != nil {
		return false, status.Error(err, "Error checking for the Broker")
	}

	if !brokerFound {
		return false, nil
	}

	status.Start("Upgrading the Broker to %s", upgradeOperatorVersion)
	options := &deploy.BrokerOptions{
		ImageVersion:    upgradeOperatorVersion,
		BrokerNamespace: brokerObj.Namespace,
		BrokerSpec:      brokerObj.Spec,
	}

	err = deploy.Deploy(ctx, options, status, clusterInfo.ClientProducer)

	return err == nil, status.Error(err, "Error upgrading the Broker")
}

func upgradeOperator(ctx context.Context, clusterInfo *cluster.Info, repository string, debug bool, imageOverride map[string]string,
	status reporter.Interface,
) error {
	status.Start("Checking if the Operator is installed")
	defer status.End()

	_, err := clusterInfo.ClientProducer.ForKubernetes().AppsV1().Deployments(constants.OperatorNamespace).
		Get(ctx, names.OperatorComponent, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		return nil
	}

	if err != nil {
		return status.Error(err, "Error retrieving Operator deployment")
	}

	status.Start("Upgrading the Operator to %s", upgradeOperatorVersion)

	repositoryInfo := image.NewRepositoryInfo(repository, upgradeOperatorVersion, imageOverride)

	err = operator.Ensure(
		ctx, status, clusterInfo.ClientProducer, constants.OperatorNamespace, repositoryInfo.GetOperatorImage(), debug)

	return status.Error(err, "Error upgrading the Operator")
}

func upgradeConnectivity(ctx context.Context, clusterInfo *cluster.Info, logVersion string, status reporter.Interface) error {
	if clusterInfo.Submariner != nil {
		status.Start("Upgrading the Connectivity component to %s", logVersion)
		defer status.End()

		clusterInfo.Submariner.Spec.Version = upgradeSubmarinerVersion

		err := deploy.SubmarinerFromSpec(ctx, clusterInfo.ClientProducer.ForGeneral(), &clusterInfo.Submariner.Spec)

		return status.Error(err, "Error upgrading the Connectivity component")
	}

	return nil
}

func upgradeServiceDiscovery(ctx context.Context, clusterInfo *cluster.Info, logVersion string, status reporter.Interface) error {
	if clusterInfo.ServiceDiscovery != nil {
		status.Start("Upgrading Service Discovery to %s", logVersion)
		defer status.End()

		clusterInfo.ServiceDiscovery.Spec.Version = upgradeSubmarinerVersion

		err := deploy.ServiceDiscoveryFromSpec(ctx, clusterInfo.ClientProducer.ForGeneral(), &clusterInfo.ServiceDiscovery.Spec)

		return status.Error(err, "Error upgrading Service Discovery")
	}

	return nil
}
