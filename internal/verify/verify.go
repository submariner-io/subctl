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

package verify

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	_ "github.com/submariner-io/lighthouse/test/e2e/discovery"
	"github.com/submariner-io/shipyard/test/e2e/framework"
	"github.com/submariner-io/subctl/cmd/subctl"
	"github.com/submariner-io/subctl/pkg/cluster"
	_ "github.com/submariner-io/submariner/test/e2e/compliance"
	_ "github.com/submariner-io/submariner/test/e2e/dataplane"
	submlabels "github.com/submariner-io/submariner/test/e2e/labels"
	_ "github.com/submariner-io/submariner/test/e2e/redundancy"
	"k8s.io/client-go/rest"
)

func init() {
	subctl.RunVerify = runVerify
}

func runVerify(options subctl.VerifyOptions, fromClusterInfo, toClusterInfo, extraClusterInfo *cluster.Info, namespace string,
	specLabels []string,
) error {
	framework.RestConfigs = []*rest.Config{fromClusterInfo.RestConfig, toClusterInfo.RestConfig}
	framework.TestContext.ClusterIDs = []string{fromClusterInfo.Name, toClusterInfo.Name}
	framework.TestContext.KubeContexts = []string{fromClusterInfo.Name, toClusterInfo.Name}

	if extraClusterInfo != nil {
		framework.RestConfigs = append(framework.RestConfigs, extraClusterInfo.RestConfig)
		framework.TestContext.ClusterIDs = append(framework.TestContext.ClusterIDs, extraClusterInfo.Name)
		framework.TestContext.KubeContexts = append(framework.TestContext.KubeContexts, extraClusterInfo.Name)
	}

	framework.TestContext.OperationTimeout = options.OperationTimeout
	framework.TestContext.ConnectionTimeout = options.ConnectionTimeout
	framework.TestContext.ConnectionAttempts = options.ConnectionAttempts
	framework.TestContext.SubmarinerNamespace = namespace
	framework.TestContext.PacketSize = options.PacketSize
	framework.TestContext.SkipConnectorSrcIPCheck = options.SkipConnectorSrcIPCheck
	framework.TestContext.SkipIntraClusterConnectivityTests = options.SkipIntraClusterConnectivityTests

	// This field isn't used for verify so set it to some non-empty string to bypass shipyard's validation checking.
	framework.TestContext.KubeConfig = "not-used"

	suiteConfig, reporterConfig := ginkgo.GinkgoConfiguration()
	suiteConfig.RandomSeed = 1
	suiteConfig.LabelFilter = strings.Join(specLabels, "||")

	if fromClusterInfo.Submariner != nil && fromClusterInfo.Submariner.Spec.GlobalCIDR != "" {
		suiteConfig.LabelFilter = strings.ReplaceAll(suiteConfig.LabelFilter, "!"+submlabels.Globalnet, submlabels.Globalnet)
	}

	reporterConfig.Verbose = true
	reporterConfig.JUnitReport = options.JunitReport

	if options.VerboseConnectivityVerification {
		framework.SetStatusFunction(func(text string, _ ...func()) {
			fmt.Println(time.Now().Format(time.StampMilli) + ": " + text)
		})
	} else {
		framework.SetStatusFunction(func(_ string, _ ...func()) {
		})
	}

	framework.SetFailFunction(func(text string, _ ...int) {
		ginkgo.Fail(text)
	})

	framework.SetUserAgentFunction(func() string {
		return fmt.Sprintf("%v -- %v", rest.DefaultKubernetesUserAgent(), ginkgo.CurrentSpecReport().FullText())
	})

	gomega.RegisterFailHandler(ginkgo.Fail)

	framework.BeforeSuite()

	defer framework.RunCleanupActions()

	if !ginkgo.RunSpecs(&testing.T{}, "Submariner E2E suite", suiteConfig, reporterConfig) {
		return errors.New("E2E failed")
	}

	return nil
}
