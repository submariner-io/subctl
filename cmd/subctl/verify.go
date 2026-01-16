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
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"

	"github.com/AlecAivazis/survey/v2"
	"github.com/spf13/cobra"
	"github.com/submariner-io/admiral/pkg/reporter"
	lhlabels "github.com/submariner-io/lighthouse/test/e2e/labels"
	"github.com/submariner-io/shipyard/test/e2e/framework"
	"github.com/submariner-io/subctl/internal/cli"
	"github.com/submariner-io/subctl/internal/component"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/internal/exit"
	"github.com/submariner-io/subctl/internal/restconfig"
	"github.com/submariner-io/subctl/pkg/cluster"
	submlabels "github.com/submariner-io/submariner/test/e2e/labels"
)

type VerifyOptions struct {
	VerboseConnectivityVerification   bool
	DisruptiveTests                   bool
	SkipConnectorSrcIPCheck           bool
	SkipIntraClusterConnectivityTests bool
	OperationTimeout                  uint
	ConnectionTimeout                 uint
	ConnectionAttempts                uint
	PacketSize                        uint
	JunitReport                       string
	VerifyOnly                        string
}

var RunVerify func(options VerifyOptions, fromClusterInfo, toClusterInfo, extraClusterInfo *cluster.Info,
	namespace string, specLabels []string) error

type verifyCommand struct {
	cmd                *cobra.Command
	flags              VerifyOptions
	restConfigProducer *restconfig.Producer
}

func NewVerifyCmd() *cobra.Command {
	verifyCmd := &verifyCommand{
		restConfigProducer: restconfig.NewProducer().
			WithPrefixedContext("to").
			WithPrefixedContext("extra").
			WithDefaultNamespace(constants.OperatorNamespace),
	}

	verifyCmd.cmd = &cobra.Command{
		Use:   "verify --context <kubeContext1> --tocontext <kubeContext2> [--extracontext <kubeContext3>]",
		Short: "Run verifications between two clusters",
		Long: `This command performs various tests to verify that a Submariner deployment between two clusters,
specified via the --context and --tocontext args, is functioning properly. Some Service Discovery tests require a third cluster,
specified via the --extracontext arg, to verify additional functionality. If the third cluster is not specified,
those tests are skipped. The verifications performed are controlled by the --only and --enable-disruptive flags.
All verifications listed in --only are performed with special handling for those deemed as disruptive. A disruptive
verification is one that changes the state of the clusters as a side effect. If running the command interactively,
you will be prompted for confirmation to perform disruptive verifications unless the --enable-disruptive flag is
also specified. If running non-interactively (that is with no stdin), --enable-disruptive must be specified otherwise
disruptive verifications are skipped.

The following verifications are deemed disruptive:

    ` + strings.Join(disruptiveVerificationNames(), "\n    "),
		Args: verifyCmd.checkVerifyArguments,
		Run: func(cmd *cobra.Command, _ []string) {
			exit.OnError(verifyCmd.restConfigProducer.RunOnSelectedContext(
				func(fromClusterInfo *cluster.Info, namespace string, status reporter.Interface) error {
					// Try to run using the "to" context
					toContextPresent, err := verifyCmd.restConfigProducer.RunOnSelectedPrefixedContext(
						"to",
						func(toClusterInfo *cluster.Info, _ string, status reporter.Interface) error {
							extraContextPresent, err := verifyCmd.restConfigProducer.RunOnSelectedPrefixedContext(
								"extra",
								func(extraClusterInfo *cluster.Info, _ string, _ reporter.Interface) error {
									return RunVerify(verifyCmd.flags, fromClusterInfo, toClusterInfo, extraClusterInfo, namespace,
										verifyCmd.determineSpecLabelsToVerify())
								}, status)
							if extraContextPresent {
								return err //nolint:wrapcheck // No need to wrap errors here.
							}

							return RunVerify(verifyCmd.flags, fromClusterInfo, toClusterInfo, nil, namespace,
								verifyCmd.determineSpecLabelsToVerify())
						}, status)

					if toContextPresent {
						return err //nolint:wrapcheck // No need to wrap errors here.
					}

					exit.WithMessage("This command requires two kube contexts corresponding to the two clusters to verify.\n" +
						cmd.UsageString())

					return nil
				}, cli.NewReporter()))
		},
	}

	verifyCmd.restConfigProducer.SetupFlags(verifyCmd.cmd.Flags())
	verifyCmd.addFlags()
	addImageOverrideFlag(verifyCmd.cmd.PersistentFlags(), &imageOverrides)

	return verifyCmd.cmd
}

func init() {
	rootCmd.AddCommand(NewVerifyCmd())
	framework.AddBeforeSuite(setupTestFrameworkBeforeSuite)
}

func (c *verifyCommand) addFlags() {
	c.cmd.Flags().BoolVar(&c.flags.VerboseConnectivityVerification, "verbose", false,
		"produce verbose logs during connectivity verification")
	c.cmd.Flags().UintVar(&c.flags.OperationTimeout, "operation-timeout", DefaultOperationTimeout,
		"operation timeout for K8s API calls")
	c.cmd.Flags().UintVar(&c.flags.ConnectionTimeout, "connection-timeout", DefaultConnectionTimeout,
		"timeout in seconds per connection attempt")
	c.cmd.Flags().UintVar(&c.flags.ConnectionAttempts, "connection-attempts", DefaultConnectionAttempts,
		"maximum number of connection attempts")
	c.cmd.Flags().StringVar(&c.flags.JunitReport, "junit-report", "", "XML report path and report name")
	c.cmd.Flags().StringVar(&c.flags.VerifyOnly, "only", strings.Join(getAllVerifyKeys(), ","),
		"comma separated verifications to be performed")
	c.cmd.Flags().BoolVar(&c.flags.DisruptiveTests, "disruptive-tests", false, "enable disruptive verifications like gateway-failover")
	c.cmd.Flags().UintVar(&c.flags.PacketSize, "packet-size", 3000, "set packet size used in TCP connectivity tests")
	c.cmd.Flags().BoolVar(&c.flags.SkipConnectorSrcIPCheck, "skip-src-ip-check", false,
		"skip source IP verification for connector pod traffic")
	c.cmd.Flags().BoolVar(&c.flags.SkipIntraClusterConnectivityTests,
		"skip-intra-cluster-connectivity-tests", false,
		"skip tests that verify intra-cluster connectivity")
}

func isNonInteractive(err error) bool {
	if errors.Is(err, io.EOF) {
		return true
	}

	var pathError *os.PathError
	if errors.As(err, &pathError) {
		var syserr syscall.Errno
		if errors.As(pathError, &syserr) {
			if pathError.Path == "/dev/stdin" && (errors.Is(syserr, syscall.EBADF) || errors.Is(syserr, syscall.EINVAL)) {
				return true
			}
		}
	}

	return false
}

func (c *verifyCommand) checkVerifyArguments(cmd *cobra.Command, args []string) error {
	if c.flags.ConnectionAttempts < 1 {
		return errors.New("--connection-attempts must be >=1")
	}

	if c.flags.ConnectionTimeout < 20 {
		return errors.New("--connection-timeout must be >=20")
	}

	if _, _, err := getVerifySpecLabels(c.flags.VerifyOnly, true); err != nil {
		return err
	}

	err := checkImageOverrides(imageOverrides)
	if err != nil {
		return err
	}

	return checkNoArguments(cmd, args)
}

var verifyE2ESpecLabels = map[string]string{
	component.Connectivity: fmt.Sprintf("%s&&!%s", submlabels.Dataplane, submlabels.Globalnet),
	fmt.Sprintf("%s-%s", framework.BasicTestLabel, component.Connectivity): fmt.Sprintf("%s&&%s&&!%s",
		submlabels.Dataplane, framework.BasicTestLabel, submlabels.Globalnet),
	component.ServiceDiscovery: lhlabels.ServiceDiscovery,
	"compliance":               submlabels.Compliance,
}

var verifyE2EDisruptiveSpecLabels = map[string]string{
	"gateway-failover": submlabels.Redundancy,
}

type verificationType int

const (
	disruptiveVerification = iota
	normalVerification
	unknownVerification
)

func disruptiveVerificationNames() []string {
	names := make([]string, 0, len(verifyE2EDisruptiveSpecLabels))
	for n := range verifyE2EDisruptiveSpecLabels {
		names = append(names, n)
	}

	return names
}

func extractDisruptiveVerifications(csv string) []string {
	var disruptive []string

	for verification := range strings.SplitSeq(csv, ",") {
		verification = strings.Trim(strings.ToLower(verification), " ")
		if _, ok := verifyE2EDisruptiveSpecLabels[verification]; ok {
			disruptive = append(disruptive, verification)
		}
	}

	return disruptive
}

func getAllVerifyKeys() []string {
	keys := make([]string, 0, len(verifyE2ESpecLabels)+len(verifyE2EDisruptiveSpecLabels))

	for k := range verifyE2ESpecLabels {
		keys = append(keys, k)
	}

	for k := range verifyE2EDisruptiveSpecLabels {
		keys = append(keys, k)
	}

	return keys
}

func getVerifySpecLabel(key string) (verificationType, string) {
	if pattern, ok := verifyE2ESpecLabels[key]; ok {
		return normalVerification, pattern
	}

	if pattern, ok := verifyE2EDisruptiveSpecLabels[key]; ok {
		return disruptiveVerification, pattern
	}

	return unknownVerification, ""
}

func getVerifySpecLabels(csv string, includeDisruptive bool) ([]string, []string, error) {
	outputLabels := []string{}
	outputVerifications := []string{}

	for verification := range strings.SplitSeq(csv, ",") {
		verification = strings.Trim(strings.ToLower(verification), " ")

		vtype, label := getVerifySpecLabel(verification)
		switch vtype {
		case unknownVerification:
			return nil, nil, fmt.Errorf("unknown verification %q", verification)
		case normalVerification:
			outputLabels = append(outputLabels, label)
			outputVerifications = append(outputVerifications, verification)
		case disruptiveVerification:
			if includeDisruptive {
				outputLabels = append(outputLabels, label)
				outputVerifications = append(outputVerifications, verification)
			}
		}
	}

	if len(outputLabels) == 0 {
		return nil, nil, errors.New("please specify at least one verification to be performed")
	}

	return outputLabels, outputVerifications, nil
}

func (c *verifyCommand) determineSpecLabelsToVerify() []string {
	disruptive := extractDisruptiveVerifications(c.flags.VerifyOnly)
	if !c.flags.DisruptiveTests && len(disruptive) > 0 {
		err := survey.AskOne(NewInputPrompt(&survey.Confirm{
			Message: fmt.Sprintf("You have specified disruptive verifications (%s). Are you sure you want to run them?",
				strings.Join(disruptive, ",")),
		}), &c.flags.DisruptiveTests)
		if err != nil {
			if isNonInteractive(err) {
				fmt.Printf(`
You have specified disruptive verifications (%s) but subctl is running non-interactively and thus cannot
prompt for confirmation therefore you must specify --enable-disruptive to run them.`, strings.Join(disruptive, ","))
			} else {
				exit.OnErrorWithMessage(err, "Prompt failure:")
			}
		}
	}

	labels, verifications, err := getVerifySpecLabels(c.flags.VerifyOnly, c.flags.DisruptiveTests)
	if err != nil {
		exit.WithMessage(err.Error())
	}

	fmt.Printf("Performing the following verifications: %s\n", strings.Join(verifications, ", "))

	return labels
}
