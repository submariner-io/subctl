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

package subctl_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	lhlabels "github.com/submariner-io/lighthouse/test/e2e/labels"
	"github.com/submariner-io/subctl/cmd/subctl"
	"github.com/submariner-io/subctl/pkg/cluster"
	submlabels "github.com/submariner-io/submariner/test/e2e/labels"
)

var _ = Describe("Verify", func() {
	Context("with no arguments", func() {
		t := newVerifyTestDriver()

		Context("and the disruptive verification prompt is confirmed", func() {
			BeforeEach(func() {
				setupPrompts(map[string]any{
					"disruptive verifications": true,
				})
			})

			It("should invoke verify with all defaults", func() {
				t.assertCmdSuccess()
				Expect(t.runVerifyCalled).To(BeTrue())
				Expect(t.verifyOptions.OperationTimeout).To(Equal(subctl.DefaultOperationTimeout))
				Expect(t.verifyOptions.ConnectionTimeout).To(Equal(subctl.DefaultConnectionTimeout))
				Expect(t.verifyOptions.ConnectionAttempts).To(Equal(subctl.DefaultConnectionAttempts))
				Expect(t.fromClusterName).To(Equal(clusterName))
				Expect(t.toClusterName).To(Equal(remoteCluster))
				Expect(t.specLabels).To(ContainElements(lhlabels.ServiceDiscovery, submlabels.Compliance, submlabels.Redundancy,
					ContainSubstring(submlabels.Dataplane)))
				Expect(t.extraClusterName).To(BeEmpty())
			})
		})

		Context("and the disruptive verification prompt is not confirmed", func() {
			BeforeEach(func() {
				setupPrompts(map[string]any{
					"disruptive verifications": false,
				})
			})

			It("should invoke verify with only non-disruptive spec labels", func() {
				t.assertCmdSuccess()
				Expect(t.runVerifyCalled).To(BeTrue())
				Expect(t.specLabels).To(ContainElements(lhlabels.ServiceDiscovery, submlabels.Compliance,
					ContainSubstring(submlabels.Dataplane)))
				Expect(t.specLabels).NotTo(ContainElements(submlabels.Redundancy))
			})
		})
	})

	Context("with --extracontext specified", func() {
		t := newVerifyTestDriver()

		BeforeEach(func() {
			t.args = append(t.args, "--extracontext="+remoteCluster2, "--only=connectivity")
		})

		It("should invoke verify with three clusters", func() {
			t.assertCmdSuccess()
			Expect(t.runVerifyCalled).To(BeTrue())
			Expect(t.fromClusterName).To(Equal(clusterName))
			Expect(t.toClusterName).To(Equal(remoteCluster))
			Expect(t.extraClusterName).To(Equal(remoteCluster2))
		})
	})

	Context("with various flags specified", func() {
		t := newVerifyTestDriver()

		BeforeEach(func() {
			t.args = append(t.args, "--only=connectivity", "--verbose", "--operation-timeout=300", "--connection-timeout=90",
				"--connection-attempts=5", "--packet-size=1500", "--junit-report=/tmp/report.xml", "--skip-src-ip-check",
				"--skip-intra-cluster-connectivity-tests")
		})

		It("should invoke verify with the specified values", func() {
			t.assertCmdSuccess()
			Expect(t.runVerifyCalled).To(BeTrue())
			Expect(t.specLabels).To(ContainElements(ContainSubstring(submlabels.Dataplane)))
			Expect(t.verifyOptions.VerboseConnectivityVerification).To(BeTrue())
			Expect(t.verifyOptions.OperationTimeout).To(Equal(uint(300)))
			Expect(t.verifyOptions.ConnectionTimeout).To(Equal(uint(90)))
			Expect(t.verifyOptions.ConnectionAttempts).To(Equal(uint(5)))
			Expect(t.verifyOptions.PacketSize).To(Equal(uint(1500)))
			Expect(t.verifyOptions.JunitReport).To(Equal("/tmp/report.xml"))
			Expect(t.verifyOptions.SkipConnectorSrcIPCheck).To(BeTrue())
			Expect(t.verifyOptions.SkipIntraClusterConnectivityTests).To(BeTrue())
		})
	})

	Context("with --disruptive-tests specified", func() {
		t := newVerifyTestDriver()

		BeforeEach(func() {
			t.args = append(t.args, "--disruptive-tests", "--only=gateway-failover")
		})

		It("should invoke verify without prompting", func() {
			t.assertCmdSuccess()
			Expect(t.verifyOptions.DisruptiveTests).To(BeTrue())
			Expect(t.specLabels).To(Equal([]string{submlabels.Redundancy}))
		})
	})

	Context("with --only specified with multiple verifications", func() {
		t := newVerifyTestDriver()

		BeforeEach(func() {
			t.args = append(t.args, "--only=connectivity,service-discovery")
		})

		It("should invoke verify with the correct spec labels", func() {
			t.assertCmdSuccess()
			Expect(t.specLabels).To(ConsistOf(ContainSubstring(submlabels.Dataplane), lhlabels.ServiceDiscovery))
		})
	})

	Context("without --tocontext specified", func() {
		t := newVerifyTestDriver()

		BeforeEach(func() {
			t.args = []string{}
		})

		It("should exit with an error message", func() {
			Expect(t.cmd.Execute()).To(Succeed())
			Expect(t.exited).To(BeTrue())
		})
	})

	Context("with invalid connection-attempts", func() {
		t := newVerifyTestDriver()

		BeforeEach(func() {
			t.args = append(t.args, "--connection-attempts=0")
		})

		It("should fail argument validation", func() {
			Expect(t.cmd.Execute()).NotTo(Succeed())
		})
	})

	Context("with invalid connection-timeout", func() {
		t := newVerifyTestDriver()

		BeforeEach(func() {
			t.args = append(t.args, "--connection-timeout=10")
		})

		It("should fail argument validation", func() {
			Expect(t.cmd.Execute()).NotTo(Succeed())
		})
	})

	Context("with invalid verification name", func() {
		t := newVerifyTestDriver()

		BeforeEach(func() {
			t.args = append(t.args, "--only=invalid-verification")
		})

		It("should fail argument validation", func() {
			Expect(t.cmd.Execute()).NotTo(Succeed())
		})
	})
})

type verifyTestDriver struct {
	*testDriver
	runVerifyCalled  bool
	fromClusterName  string
	toClusterName    string
	extraClusterName string
	verifyOptions    subctl.VerifyOptions
	specLabels       []string
}

func newVerifyTestDriver() *verifyTestDriver {
	t := &verifyTestDriver{testDriver: newTestDriver()}

	BeforeEach(func() {
		t.cmd = subctl.NewVerifyCmd()
		t.runVerifyCalled = false
		t.fromClusterName = ""
		t.toClusterName = ""
		t.extraClusterName = ""
		t.verifyOptions = subctl.VerifyOptions{}
		t.specLabels = nil

		subctl.RunVerify = func(options subctl.VerifyOptions, fromClusterInfo, toClusterInfo, extraClusterInfo *cluster.Info,
			namespace string, specLabels []string,
		) error {
			t.runVerifyCalled = true
			t.verifyOptions = options
			t.fromClusterName = ""
			t.toClusterName = ""
			t.extraClusterName = ""

			if fromClusterInfo != nil {
				t.fromClusterName = fromClusterInfo.Name
			}

			if toClusterInfo != nil {
				t.toClusterName = toClusterInfo.Name
			}

			if extraClusterInfo != nil {
				t.extraClusterName = extraClusterInfo.Name
			}

			t.specLabels = specLabels

			return nil
		}

		t.args = append(t.args, "--tocontext="+remoteCluster)
	})

	return t
}
