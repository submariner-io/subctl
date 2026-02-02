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
	"context"
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

var UninstallAll = uninstall.All

type uninstallCommand struct {
	noPrompt           bool
	restConfigProducer *restconfig.Producer
}

// NewUninstallCmd returns the uninstall command.
func NewUninstallCmd() *cobra.Command {
	uninstallCmd := &uninstallCommand{
		restConfigProducer: restconfig.NewProducer().WithDefaultNamespace(constants.OperatorNamespace),
	}

	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Uninstall Submariner and its components",
		Long:  "This command uninstalls Submariner and its components",
		Run: func(cmd *cobra.Command, _ []string) {
			exit.OnError(uninstallCmd.restConfigProducer.RunOnSelectedContext(cmd.Context(), uninstallCmd.uninstallInContext, cli.NewReporter()))
		},
	}

	cmd.Flags().BoolVarP(&uninstallCmd.noPrompt, "yes", "y", false, "automatically answer yes to confirmation prompt")
	uninstallCmd.restConfigProducer.SetupFlags(cmd.Flags())

	return cmd
}

var uninstallCmdInstance = NewUninstallCmd()

func init() {
	rootCmd.AddCommand(uninstallCmdInstance)
}

func (c *uninstallCommand) uninstallInContext(ctx context.Context, clusterInfo *cluster.Info, namespace string,
	status reporter.Interface,
) error {
	if !c.noPrompt {
		result := false
		prompt := NewInputPrompt(&survey.Confirm{
			Message: fmt.Sprintf(
				"This will completely uninstall Submariner from the cluster %q. Are you sure you want to continue?",
				clusterInfo.Name),
		})

		_ = survey.AskOne(prompt, &result)

		if !result {
			return nil
		}
	}

	return UninstallAll(ctx, clusterInfo.ClientProducer, clusterInfo.Name, namespace, status)
}
