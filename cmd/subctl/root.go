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
	"bytes"
	"fmt"
	"os"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/submariner-io/shipyard/test/e2e/framework"
	"github.com/submariner-io/subctl/internal/exit"
	"github.com/submariner-io/subctl/internal/restconfig"
	"github.com/submariner-io/subctl/pkg/cluster"
	"github.com/submariner-io/submariner-operator/pkg/client"
)

var restConfigProducer = restconfig.NewProducer()

// rootCmd represents the base command when called without any subcommands.
var rootCmd = &cobra.Command{
	Use:   "subctl",
	Short: "An installer for Submariner",
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func setupTestFrameworkBeforeSuite() {
	clientProducer, err := client.NewProducerFromRestConfig(framework.RestConfigs[framework.ClusterA])
	exit.OnErrorWithMessage(err, "Error creating client producer")

	clusterInfo, err := cluster.NewInfo(framework.TestContext.ClusterIDs[framework.ClusterA], clientProducer,
		framework.RestConfigs[framework.ClusterA])
	exit.OnErrorWithMessage(err, "Error initializing the cluster information")

	if clusterInfo.Submariner == nil {
		exit.WithMessage("The Submariner resource was not found which indicates submariner has not been deployed in this cluster.")
	}

	framework.TestContext.GlobalnetEnabled = clusterInfo.Submariner.Spec.GlobalCIDR != ""

	framework.TestContext.NettestImageURL = clusterInfo.GetImageRepositoryInfo().GetNettestImageURL()
}

func compareFiles(file1, file2 string) (bool, error) {
	first, err := os.ReadFile(file1)
	if err != nil {
		return false, errors.Wrapf(err, "error reading file %q", file1)
	}

	second, err := os.ReadFile(file2)
	if err != nil {
		return false, errors.Wrapf(err, "error reading file %q", file2)
	}

	return bytes.Equal(first, second), nil
}

// expectFlag exits with an error if the flag value is empty.
func expectFlag(flag, value string) {
	if value == "" {
		exit.WithMessage(fmt.Sprintf("You must specify the %q flag", flag))
	}
}
