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
	"github.com/onsi/ginkgo/config"
	"github.com/pkg/errors"
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

	benchmarkRestConfigProducer = restconfig.NewProducer().
					WithDeprecatedKubeContexts("use --context and --tocontext instead").WithPrefixedContext("to")

	benchmarkCmd = &cobra.Command{
		Use:   "benchmark",
		Short: "Benchmark tests",
		Long:  "This command runs various benchmark tests",
	}
	benchmarkThroughputCmd = &cobra.Command{
		Use:   "throughput --context <kubeContext1> [--tocontext <kubeContext2>]",
		Short: "Benchmark throughput",
		Long:  "This command runs throughput tests within a cluster or between two clusters",
		Run:   buildBenchmarkRunner(benchmark.StartThroughputTests),
	}
	benchmarkLatencyCmd = &cobra.Command{
		Use:   "latency --context <kubeContext1> [--tocontext <kubeContext2>]",
		Short: "Benchmark latency",
		Long:  "This command runs latency benchmark tests within a cluster or between two clusters",
		Run:   buildBenchmarkRunner(benchmark.StartLatencyTests),
	}
)

func init() {
	addBenchmarkFlags(benchmarkCmd)

	benchmarkCmd.AddCommand(benchmarkThroughputCmd)
	benchmarkCmd.AddCommand(benchmarkLatencyCmd)
	rootCmd.AddCommand(benchmarkCmd)

	addTestImageOverrideFlag(benchmarkCmd.PersistentFlags())
	framework.AddBeforeSuite(setupTestFrameworkBeforeSuite)
}

func addBenchmarkFlags(cmd *cobra.Command) {
	benchmarkRestConfigProducer.SetupFlags(cmd.PersistentFlags())

	// TODO Remove in 0.15
	cmd.PersistentFlags().BoolVar(&intraCluster, "intra-cluster", false, "run the test within a single cluster")
	_ = cmd.PersistentFlags().MarkDeprecated("intra-cluster",
		"specify a single context for intra-cluster benchmarks, two contexts for inter-cluster benchmarks")

	cmd.PersistentFlags().BoolVar(&verbose, "verbose", false, "produce verbose logs during benchmark tests")
}

func buildBenchmarkRunner(run func(intraCluster, verbose bool) error) func(command *cobra.Command, args []string) {
	return func(command *cobra.Command, args []string) {
		// Deprecated variants:
		// - kubeconfigs on the command line
		if len(args) == 2 {
			exit.OnError(restconfig.NewProducerFrom(args[0], "").RunOnSelectedContext(
				func(fromClusterInfo *cluster.Info, namespace string, status reporter.Interface) error {
					return restconfig.NewProducerFrom(args[1], "").RunOnSelectedContext( //nolint:wrapcheck // No need to wrap errors here.
						func(toClusterInfo *cluster.Info, namespace string, status reporter.Interface) error {
							return runBenchmark(run, fromClusterInfo, toClusterInfo, verbose)
						}, status)
				}, cli.NewReporter()))

			return
		}

		// - kubecontext(s)
		selectedContextsPresent, err := benchmarkRestConfigProducer.RunOnSelectedContexts(
			func(clusterInfos []*cluster.Info, namespaces []string, status reporter.Interface) error {
				if len(clusterInfos) >= 2 {
					return runBenchmark(run, clusterInfos[0], clusterInfos[1], verbose)
				} else if len(clusterInfos) >= 1 {
					return runBenchmark(run, clusterInfos[0], nil, verbose)
				}
				return errors.New("no contexts were specified")
			}, cli.NewReporter())

		if selectedContextsPresent {
			exit.OnError(err)
			return
		}

		// Explicit kubeconfigs and/or contexts
		exit.OnError(benchmarkRestConfigProducer.RunOnSelectedContext(
			func(fromClusterInfo *cluster.Info, _ string, status reporter.Interface) error {
				// Try to run using the "to" context
				toContextPresent, err := benchmarkRestConfigProducer.RunOnSelectedPrefixedContext(
					"to",
					func(toClusterInfo *cluster.Info, _ string, status reporter.Interface) error {
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
	framework.TestContext.JunitReport = junitReport
	framework.TestContext.SubmarinerNamespace = constants.OperatorNamespace

	config.DefaultReporterConfig.Verbose = verboseConnectivityVerification
	config.DefaultReporterConfig.SlowSpecThreshold = 60

	return run(intraCluster, verbose)
}
