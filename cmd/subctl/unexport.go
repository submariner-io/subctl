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
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/submariner-io/admiral/pkg/reporter"
	"github.com/submariner-io/subctl/internal/cli"
	"github.com/submariner-io/subctl/internal/exit"
	"github.com/submariner-io/subctl/internal/restconfig"
	"github.com/submariner-io/subctl/pkg/cluster"
	"github.com/submariner-io/subctl/pkg/service"
	mcsclient "sigs.k8s.io/mcs-api/pkg/client/clientset/versioned/typed/apis/v1alpha1"
)

var (
	unexportRestConfigProducer = restconfig.NewProducer().WithNamespace()

	unexportCmd = &cobra.Command{
		Use:   "unexport",
		Short: "Stop a resource from being exported to other clusters",
		Long:  "This command stops exporting a resource so that it's no longer accessible to other clusters",
	}
	unexportServiceCmd = &cobra.Command{
		Use:   "service <serviceName>",
		Short: "Stop a Service from being exported to other clusters",
		Long: "This command removes the ServiceExport resource with the given name which in turn stops the Service " +
			"of the same name from being exported to other clusters",
		Run: func(cmd *cobra.Command, args []string) {
			err := validateUnexportArguments(args)
			exit.OnErrorWithMessage(err, "Insufficient arguments")

			exit.OnError(unexportRestConfigProducer.RunOnSelectedContext(
				func(clusterInfo *cluster.Info, namespace string, status reporter.Interface) error {
					mcsClient, err := mcsclient.NewForConfig(clusterInfo.RestConfig)
					if err != nil {
						return status.Error(err, "Error creating client")
					}

					return service.Unexport(mcsClient, namespace, args[0], status) //nolint:wrapcheck // No need to wrap errors here.
				}, cli.NewReporter()))
		},
	}
)

func init() {
	unexportRestConfigProducer.SetupFlags(unexportCmd.Flags())
	unexportCmd.AddCommand(unexportServiceCmd)
	rootCmd.AddCommand(unexportCmd)
}

func validateUnexportArguments(args []string) error {
	if len(args) == 0 || args[0] == "" {
		return errors.New("name of the Service to be removed must be specified")
	}

	return nil
}
