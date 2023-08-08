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
	"fmt"
	"os/exec"

	"github.com/coreos/go-semver/semver"
	"github.com/spf13/cobra"
	"github.com/submariner-io/admiral/pkg/reporter"
	"github.com/submariner-io/subctl/internal/cli"
	"github.com/submariner-io/subctl/internal/exit"
	"github.com/submariner-io/subctl/internal/restconfig"
	"github.com/submariner-io/subctl/pkg/version"
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
	},
}

func init() {
	upgradeCmd.Flags().StringVar(&to, "to", "", "the version of Submariner to upgrade to")
	upgradeRestConfigProducer.SetupFlags(upgradeCmd.Flags())
	rootCmd.AddCommand(upgradeCmd)
}

func upgradeSubctl(status reporter.Interface) error {
	status.Start("Getting the current subctl version")
	defer status.End()

	// TODO Address cases where current version is devel-*
	currentVersion, err := semver.NewVersion(version.Version)
	fmt.Printf("current version is %s and smever is %s", version.Version, currentVersion)

	if currentVersion == nil {
		return status.Error(err, "Error getting current subctl version")
	}

	status.Success("Current subctl version is %s", currentVersion)

	if to == "" {
		to = currentVersion.String()
	}

	toVersion, _ := semver.NewVersion(to)
	if toVersion.LessThan(*currentVersion) || toVersion.Equal(*currentVersion) {
		status.Success("Installed version %s of subctl is either equal or greater than the intended one %s. Exiting",
			currentVersion, toVersion)
		return nil
	}

	status.Success(fmt.Sprintf("Installed version of subctl %s is less than the intended one. Upgrading subctl to %s", currentVersion,
		toVersion))

	url := "https://get.submariner.io"

	_, err = exec.Command("sh", "-c", "curl "+url+" | VERSION=v"+to+" bash").CombinedOutput()
	if err != nil {
		return status.Error(err, "Error upgrading subctl")
	}

	status.Success("Upgraded and installed subctl version: %s", to)

	return nil
}
