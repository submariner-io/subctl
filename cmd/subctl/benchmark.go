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
	"github.com/onsi/ginkgo/v2"
	"github.com/spf13/cobra"
	"github.com/submariner-io/admiral/pkg/reporter"
	"github.com/submariner-io/shipyard/test/e2e/framework"
	"github.com/submariner-io/subctl/internal/benchmark"
	"github.com/submariner-io/subctl/internal/cli"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/internal/exit"
	"github.com/submariner-io/subctl/internal/restconfig"
	"github.com/submariner-io/subctl/pkg/cluster"
	"k8s.io/client-go/rest"
)

var (
	intraCluster bool
	verbose      bool

	benchmarkRestConfigProducer = restconfig.NewProducer().WithPrefixedContext("to")

	benchmarkCmd = &cobra.Command{
		Use:   "benchmark",
		Short: "Benchmark tests",
		Long:  "This command runs various benchmark tests",
	}
	benchmarkThroughputCmd = &cobra.Command{
		Use:   "throughput --context <kubeContext1> [--tocontext <kubeContext2>]",
		Short: "Benchmark throughput",
		Long:  "This command runs throughput tests within a cluster or between two clusters",
		Args:  checkBenchmarkArguments,
		Run:   buildBenchmarkRunner(benchmark.StartThroughputTests),
	}
	benchmarkLatencyCmd = &cobra.Command{
		Use:   "latency --context <kubeContext1> [--tocontext <kubeContext2>]",
		Short: "Benchmark latency",
		Long:  "This command runs latency benchmark tests within a cluster or between two clusters",
		Args:  checkBenchmarkArguments,
		Run:   buildBenchmarkRunner(benchmark.StartLatencyTests),
	}
)

func init() {
	addBenchmarkFlags(benchmarkCmd)

	benchmarkCmd.AddCommand(benchmarkThroughputCmd)
	benchmarkCmd.AddCommand(benchmarkLatencyCmd)
	rootCmd.AddCommand(benchmarkCmd)

	addImageOverrideFlag(benchmarkCmd.PersistentFlags())
	framework.AddBeforeSuite(setupTestFrameworkBeforeSuite)
}

func addBenchmarkFlags(cmd *cobra.Command) {
	benchmarkRestConfigProducer.SetupFlags(cmd.PersistentFlags())

	cmd.PersistentFlags().BoolVar(&verbose, "verbose", false, "produce verbose logs during benchmark tests")
}

func checkBenchmarkArguments(cmd *cobra.Command, args []string) error {
	err := checkImageOverrides(cmd, args)
	if err != nil {
		return err
	}

	return checkNoArguments(cmd, args)
}

func buildBenchmarkRunner(run func(intraCluster, verbose bool) error) func(command *cobra.Command, args []string) {
	return func(_ *cobra.Command, _ []string) {
		exit.OnError(benchmarkRestConfigProducer.RunOnSelectedContext(
			func(fromClusterInfo *cluster.Info, _ string, status reporter.Interface) error {
				// Try to run using the "to" context
				toContextPresent, err := benchmarkRestConfigProducer.RunOnSelectedPrefixedContext(
					"to",
					func(toClusterInfo *cluster.Info, _ string, _ reporter.Interface) error {
						return runBenchmark(run, fromClusterInfo, toClusterInfo, verbose)
					}, status)

				if toContextPresent {
					return err //nolint:wrapcheck // No need to wrap errors here.
				}

				return runBenchmark(run, fromClusterInfo, nil, verbose)
			}, cli.NewReporter()))
	}
}

func runBenchmark(
	run func(intraCluster, verbose bool) error, fromClusterInfo, toClusterInfo *cluster.Info, verbose bool,
) error {
	framework.RestConfigs = []*rest.Config{fromClusterInfo.RestConfig}
	framework.TestContext.ClusterIDs = []string{fromClusterInfo.Name}

	if toClusterInfo != nil {
		framework.RestConfigs = append(framework.RestConfigs, toClusterInfo.RestConfig)
		framework.TestContext.ClusterIDs = append(framework.TestContext.ClusterIDs, toClusterInfo.Name)
		intraCluster = false
	} else {
		intraCluster = true
	}

	framework.TestContext.OperationTimeout = operationTimeout
	framework.TestContext.ConnectionTimeout = connectionTimeout
	framework.TestContext.ConnectionAttempts = connectionAttempts
	framework.TestContext.SubmarinerNamespace = constants.OperatorNamespace

	_, reporterConfig := ginkgo.GinkgoConfiguration()
	reporterConfig.Verbose = verboseConnectivityVerification
	reporterConfig.JUnitReport = junitReport
	framework.TestContext.ReporterConfig = &reporterConfig

	return run(intraCluster, verbose)
}
