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
	"os"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/submariner-io/shipyard/test/e2e/framework"
	"github.com/submariner-io/subctl/internal/exit"
	"github.com/submariner-io/subctl/pkg/cluster"
	submarineropv1a1 "github.com/submariner-io/submariner-operator/api/v1alpha1"
	submarinerv1 "github.com/submariner-io/submariner/pkg/apis/submariner.io/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
)

type suppressWarnings struct{}

func (suppressWarnings) HandleWarningHeader(code int, agent, message string) {
	if code == 299 && strings.Contains(message, "would violate PodSecurity") {
		return
	}

	rest.WarningLogger{}.HandleWarningHeader(code, agent, message)
}

func init() {
	runtime.Must(apiextensionsv1.AddToScheme(scheme.Scheme))
	runtime.Must(submarinerv1.AddToScheme(scheme.Scheme))
	runtime.Must(submarineropv1a1.AddToScheme(scheme.Scheme))

	rest.SetDefaultWarningHandler(suppressWarnings{})
}

// rootCmd represents the base command when called without any subcommands.
var rootCmd = &cobra.Command{
	Use:   "subctl",
	Short: "Deploy, manage, verify and diagnose Submariner deployments",
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

var testImageOverrides = []string{}

func addTestImageOverrideFlag(flags *pflag.FlagSet) {
	flags.StringSliceVar(&testImageOverrides, "image-override", nil, "override component image")
}

func setupTestFrameworkBeforeSuite() {
	clusterInfo, err := cluster.NewInfo(framework.TestContext.ClusterIDs[framework.ClusterA],
		framework.RestConfigs[framework.ClusterA])
	exit.OnErrorWithMessage(err, "Error initializing the cluster information")

	if clusterInfo.Submariner == nil {
		exit.WithMessage("The Submariner resource was not found which indicates submariner has not been deployed in this cluster.")
	}

	framework.TestContext.GlobalnetEnabled = clusterInfo.Submariner.Spec.GlobalCIDR != ""

	repositoryInfo, err := clusterInfo.GetImageRepositoryInfo(testImageOverrides...)
	exit.OnErrorWithMessage(err, "Error determining repository information")

	framework.TestContext.NettestImageURL = repositoryInfo.GetNettestImage()
}

// expectFlag exits with an error if the flag value is empty.
func expectFlag(flag, value string) {
	if value == "" {
		exit.WithMessage(fmt.Sprintf("You must specify the %q flag", flag))
	}
}

// checkNoArguments checks that there are no arguments.
func checkNoArguments(_ *cobra.Command, args []string) error {
	if len(args) > 0 {
		return errors.New("this command doesn't support any arguments")
	}

	return nil
}
