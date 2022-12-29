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

	"github.com/AlecAivazis/survey/v2"
	"github.com/spf13/cobra"
	"github.com/submariner-io/admiral/pkg/reporter"
	"github.com/submariner-io/subctl/internal/cli"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/internal/exit"
	"github.com/submariner-io/subctl/internal/restconfig"
	"github.com/submariner-io/subctl/pkg/cluster"
	"github.com/submariner-io/subctl/pkg/uninstall"
)

var uninstallOptions struct {
	noPrompt bool
}

var uninstallRestConfigProducer = restconfig.NewProducer().WithDefaultNamespace(constants.OperatorNamespace)

var uninstallCmd = &cobra.Command{
	Use:     "uninstall",
	Short:   "Uninstall Submariner and its components",
	Long:    "This command uninstalls Submariner and its components",
	Run: func(cmd *cobra.Command, args []string) {
		exit.OnError(uninstallRestConfigProducer.RunOnSelectedContext(uninstallInContext, cli.NewReporter()))
	},
}

func init() {
	uninstallCmd.Flags().BoolVarP(&uninstallOptions.noPrompt, "yes", "y", false, "automatically answer yes to confirmation prompt")
	uninstallRestConfigProducer.SetupFlags(uninstallCmd.Flags())
	rootCmd.AddCommand(uninstallCmd)
}

func uninstallInContext(clusterInfo *cluster.Info, namespace string, status reporter.Interface) error {
	if !uninstallOptions.noPrompt {
		result := false
		prompt := &survey.Confirm{
			Message: fmt.Sprintf(
				"This will completely uninstall Submariner from the cluster %q. Are you sure you want to continue?",
				clusterInfo.Name),
		}

		_ = survey.AskOne(prompt, &result)

		if !result {
			return nil
		}
	}

	return uninstall.All( //nolint:wrapcheck // No need to wrap errors here.
		clusterInfo.ClientProducer, clusterInfo.Name, namespace, status)
}
