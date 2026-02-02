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
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/submariner-io/admiral/pkg/reporter"
	"github.com/submariner-io/subctl/cmd/subctl"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/pkg/client"
)

var _ = Describe("Uninstall", func() {
	t := newTestDriver()

	var (
		uninstallCalled   bool
		uninstallError    error
		expectedNamespace string
	)

	BeforeEach(func() {
		t.cmd = subctl.NewUninstallCmd()
		uninstallCalled = false
		uninstallError = nil
		expectedNamespace = constants.OperatorNamespace

		subctl.UninstallAll = func(_ context.Context, clientProducer client.Producer, localClusterName, namespace string,
			status reporter.Interface,
		) error {
			uninstallCalled = true
			Expect(localClusterName).To(Equal(clusterName))
			Expect(namespace).To(Equal(expectedNamespace))

			return uninstallError
		}
	})

	When("the --yes flag is specified", func() {
		BeforeEach(func() {
			t.args = []string{"--yes"}
		})

		It("should uninstall without prompting", func() {
			t.assertCmdSuccess()
			Expect(uninstallCalled).To(BeTrue())
		})
	})

	When("the -y flag is specified", func() {
		BeforeEach(func() {
			t.args = []string{"-y"}
		})

		It("should uninstall without prompting", func() {
			t.assertCmdSuccess()
			Expect(uninstallCalled).To(BeTrue())
		})
	})

	When("the user confirms the uninstall prompt", func() {
		BeforeEach(func() {
			setupPrompts(map[string]any{
				"completely uninstall": true,
			})
		})

		It("should uninstall Submariner", func() {
			t.assertCmdSuccess()
			Expect(uninstallCalled).To(BeTrue())
		})
	})

	When("user declines the uninstall prompt", func() {
		BeforeEach(func() {
			setupPrompts(map[string]any{
				"completely uninstall": false,
			})
		})

		It("should not uninstall Submariner", func() {
			t.assertCmdSuccess()
			Expect(uninstallCalled).To(BeFalse())
		})
	})

	When("uninstall fails", func() {
		BeforeEach(func() {
			t.args = []string{"--yes"}
			uninstallError = errors.New("uninstall failed")
		})

		It("should exit with error", func() {
			Expect(t.cmd.Execute()).To(Succeed())
			Expect(t.exited).To(BeTrue())
			Expect(uninstallCalled).To(BeTrue())
		})
	})

	When("a custom namespace is specified", func() {
		BeforeEach(func() {
			t.args = []string{"--yes", "--namespace=custom-namespace"}
			expectedNamespace = "custom-namespace"
		})

		It("should use the custom namespace", func() {
			t.assertCmdSuccess()
			Expect(uninstallCalled).To(BeTrue())
		})
	})
})
