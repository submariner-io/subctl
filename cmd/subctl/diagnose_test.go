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
	"context"
	"slices"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/submariner-io/admiral/pkg/reporter"
	"github.com/submariner-io/subctl/cmd/subctl"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/pkg/cluster"
	"github.com/submariner-io/subctl/pkg/diagnose"
	"github.com/submariner-io/submariner-operator/api/v1alpha1"
)

var _ = Describe("Diagnose", func() {
	testDiagnoseSubCommand("cni", func(t *diagnoseTestDriver) {
		t.testDiagnosisInvoked(&t.diagnoseCNICalled, true)
	})

	testDiagnoseSubCommand("connections", func(t *diagnoseTestDriver) {
		t.testDiagnosisInvoked(&t.diagnoseConnections, true)
	})

	testDiagnoseSubCommand("deployment", func(t *diagnoseTestDriver) {
		t.testDiagnosisInvoked(&t.diagnoseDeploymentsCalled, true)
		t.testImageOverrides()
	})

	testDiagnoseSubCommand("k8s-version", func(t *diagnoseTestDriver) {
		t.testDiagnosisInvoked(&t.diagnoseK8sVersionCalled, false)
	})

	testDiagnoseSubCommand("kube-proxy-mode", func(t *diagnoseTestDriver) {
		t.testDiagnosisInvoked(&t.diagnoseKubeProxyModeCalled, true)
		t.testImageOverrides()
	})

	testDiagnoseSubCommand("service-discovery", func(t *diagnoseTestDriver) {
		t.testDiagnosisInvoked(&t.diagnoseServiceDiscoveryCalled, false)
	})

	testDiagnoseSubCommand("firewall intra-cluster", func(t *diagnoseTestDriver) {
		t.testDiagnosisInvoked(&t.diagnoseFirewallIntraVxLANCalled, true)
		t.testImageOverrides()
		t.testFirewallOptions()
	})

	testDiagnoseSubCommand("firewall inter-cluster", func(t *diagnoseTestDriver) {
		BeforeEach(func() {
			t.args = append(t.args, "--remotecontext="+remoteCluster)
		})

		t.testDiagnosisInvoked(&t.diagnoseTunnelConfigCalled, false)
		t.testImageOverrides()
		t.testFirewallOptions()
	})

	testDiagnoseSubCommand("firewall nat-discovery", func(t *diagnoseTestDriver) {
		BeforeEach(func() {
			t.args = append(t.args, "--remotecontext="+remoteCluster)
		})

		t.testDiagnosisInvoked(&t.diagnoseNatDiscoveryCalled, false)
		t.testImageOverrides()
		t.testFirewallOptions()
	})
})

type diagnoseTestDriver struct {
	*testDriver
	diagnoseCNICalled                bool
	diagnoseConnections              bool
	diagnoseDeploymentsCalled        bool
	diagnoseK8sVersionCalled         bool
	diagnoseKubeProxyModeCalled      bool
	diagnoseServiceDiscoveryCalled   bool
	diagnoseFirewallIntraVxLANCalled bool
	diagnoseTunnelConfigCalled       bool
	diagnoseNatDiscoveryCalled       bool
	imageOverrides                   []string
	firewallOptions                  *diagnose.FirewallOptions
}

func newDiagnoseTestDriver(commands ...string) *diagnoseTestDriver {
	t := &diagnoseTestDriver{testDriver: newTestDriver()}

	BeforeEach(func(ctx SpecContext) {
		t.cmd = subctl.NewDiagnoseCmd()
		t.args = commands
		t.imageOverrides = nil
		t.firewallOptions = nil
		t.submarinerSpec = &v1alpha1.SubmarinerSpec{}

		if slices.Contains(commands, "service-discovery") {
			t.serviceDiscoverySpec = &v1alpha1.ServiceDiscoverySpec{}
		}

		t.diagnoseCNICalled = false
		t.diagnoseConnections = false
		t.diagnoseDeploymentsCalled = false
		t.diagnoseK8sVersionCalled = false
		t.diagnoseKubeProxyModeCalled = false
		t.diagnoseServiceDiscoveryCalled = false
		t.diagnoseFirewallIntraVxLANCalled = false
		t.diagnoseTunnelConfigCalled = false
		t.diagnoseNatDiscoveryCalled = false

		subctl.DiagnoseCNIConfig = func(_ context.Context, _ *cluster.Info, _ string, _ reporter.Interface) error {
			t.diagnoseCNICalled = true
			return nil
		}

		subctl.DiagnoseConnections = func(_ context.Context, _ *cluster.Info, _ string, _ reporter.Interface) error {
			t.diagnoseConnections = true
			return nil
		}

		subctl.DiagnoseDeployments = func(_ context.Context, _ *cluster.Info, _ string, io []string, _ reporter.Interface) error {
			t.diagnoseDeploymentsCalled = true
			t.imageOverrides = io

			return nil
		}

		subctl.DiagnoseK8sVersion = func(_ context.Context, _ *cluster.Info, _ string, _ reporter.Interface) error {
			t.diagnoseK8sVersionCalled = true
			return nil
		}

		subctl.DiagnoseKubeProxyMode = func(_ context.Context, _ *cluster.Info, _ string, io []string, _ reporter.Interface) error {
			t.diagnoseKubeProxyModeCalled = true
			t.imageOverrides = io

			return nil
		}

		subctl.DiagnoseServiceDiscovery = func(_ context.Context, _ *cluster.Info, _ string, _ reporter.Interface) error {
			t.diagnoseServiceDiscoveryCalled = true
			return nil
		}

		subctl.DiagnoseFirewallIntraVxLANConfig = func(_ context.Context, _ *cluster.Info, _ string, fo diagnose.FirewallOptions,
			_ reporter.Interface,
		) error {
			t.diagnoseFirewallIntraVxLANCalled = true
			t.imageOverrides = fo.ImageOverrides
			t.firewallOptions = &fo

			return nil
		}

		subctl.DiagnoseTunnelConfigAcrossClusters = func(_ context.Context, local, remote *cluster.Info, ns string,
			fo diagnose.FirewallOptions, _ reporter.Interface,
		) error {
			Expect(local.Name).To(Equal(clusterName))
			Expect(remote.Name).To(Equal(remoteCluster))
			Expect(ns).To(Equal(constants.OperatorNamespace))

			t.diagnoseTunnelConfigCalled = true
			t.imageOverrides = fo.ImageOverrides
			t.firewallOptions = &fo

			return nil
		}

		subctl.DiagnoseNatDiscoveryConfigAcrossClusters = func(_ context.Context, local, remote *cluster.Info, ns string,
			fo diagnose.FirewallOptions, _ reporter.Interface,
		) error {
			Expect(local.Name).To(Equal(clusterName))
			Expect(remote.Name).To(Equal(remoteCluster))
			Expect(ns).To(Equal(constants.OperatorNamespace))

			t.diagnoseNatDiscoveryCalled = true
			t.imageOverrides = fo.ImageOverrides
			t.firewallOptions = &fo

			return nil
		}
	})

	return t
}

func (t *diagnoseTestDriver) testDiagnosisInvoked(invokedFlag *bool, onConnectivityInstalled bool) {
	It("should invoke diagnosis", func() {
		t.assertCmdSuccess()
		Expect(*invokedFlag).To(BeTrue())
	})

	if onConnectivityInstalled {
		When("connectivity isn't installed", func() {
			BeforeEach(func() {
				t.submarinerSpec = nil
			})

			It("should log a warning", func() {
				Expect(t.cmd.Execute()).To(Succeed())
				Expect(*invokedFlag).To(BeFalse())
				t.status.AssertWarningCount(1)
			})
		})
	}
}

func (t *diagnoseTestDriver) testImageOverrides() {
	const override = "submariner-nettest=1.0.0"

	BeforeEach(func() {
		t.args = append(t.args, "--image-override="+override)
	})

	When("the --image-override flag is specified", func() {
		It("should propagate it", func() {
			t.assertCmdSuccess()
			Expect(t.imageOverrides).To(Equal([]string{override}))
		})
	})
}

func (t *diagnoseTestDriver) testFirewallOptions() {
	BeforeEach(func() {
		t.args = append(t.args, "--validation-timeout=9", "--verbose")
	})

	When("the --validation-timeout and --verbose flags are specified", func() {
		It("should propagate them", func() {
			t.assertCmdSuccess()
			Expect(t.firewallOptions).NotTo(BeNil())
			Expect(t.firewallOptions.ValidationTimeout).To(Equal(uint(9)))
			Expect(t.firewallOptions.VerboseOutput).To(BeTrue())
		})
	})
}

func testDiagnoseSubCommand(command string, run func(*diagnoseTestDriver)) {
	Describe(command, func() {
		run(newDiagnoseTestDriver(strings.Split(command, " ")...))
	})
}
