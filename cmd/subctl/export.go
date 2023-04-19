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
	"k8s.io/client-go/kubernetes/scheme"
	mcsv1a1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"
)

var (
	exportRestConfigProducer = restconfig.NewProducer().WithNamespace()

	exportCmd = &cobra.Command{
		Use:   "export",
		Short: "Exports a resource to other clusters",
		Long:  "This command exports a resource so it is accessible to other clusters",
	}
	exportServiceCmd = &cobra.Command{
		Use:   "service <serviceName>",
		Short: "Exports a Service to other clusters",
		Long: "This command creates a ServiceExport resource with the given name which causes the Service of the same name to be accessible" +
			" to other clusters",
		Run: func(cmd *cobra.Command, args []string) {
			err := validateArguments(args)
			exit.OnErrorWithMessage(err, "Insufficient arguments")

			exit.OnError(exportRestConfigProducer.RunOnSelectedContext(
				func(clusterInfo *cluster.Info, namespace string, status reporter.Interface) error {
					return service.Export(clusterInfo.ClientProducer, namespace, args[0], status) //nolint:wrapcheck // No need to wrap errors here.
				}, cli.NewReporter()))
		},
	}
)

func init() {
	err := mcsv1a1.Install(scheme.Scheme)
	exit.OnErrorWithMessage(err, "Failed to add to scheme")

	exportRestConfigProducer.SetupFlags(exportServiceCmd.Flags())
	exportCmd.AddCommand(exportServiceCmd)
	rootCmd.AddCommand(exportCmd)
}

func validateArguments(args []string) error {
	if len(args) == 0 {
		return errors.New("name of the Service to be exported must be specified")
	}

	return nil
}
