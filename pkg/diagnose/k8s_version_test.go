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

package diagnose_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	"github.com/submariner-io/subctl/pkg/diagnose"
	"k8s.io/apimachinery/pkg/version"
	fakediscovery "k8s.io/client-go/discovery/fake"
)

var _ = Describe("K8sVersion", func() {
	t := newK8sVersionTestDriver()

	When("the K8s version is supported", func() {
		BeforeEach(func() {
			t.setK8sVersion("34")
		})

		t.testSuccess(t.run)
	})

	When("the K8s version is the oldest supported for connectivity", func() {
		BeforeEach(func() {
			t.setK8sVersion("19")
		})

		Context("with service discovery disabled", func() {
			BeforeEach(func() {
				t.serviceDiscovery = nil
			})

			t.testSuccess(t.run)
		})

		Context("with service discovery enabled", func() {
			t.testFailure(t.run, "service discovery requires")
		})
	})

	When("the K8s version is not supported", func() {
		BeforeEach(func() {
			t.setK8sVersion("18")
		})

		t.testFailure(t.run, "connectivity requires")
	})
})

type k8sVersionTestDriver struct {
	*testDriver
}

func newK8sVersionTestDriver() *k8sVersionTestDriver {
	t := &k8sVersionTestDriver{testDriver: newTestDriver()}

	JustBeforeEach(func() {
		if t.serviceDiscovery != nil {
			t.createResource(t.serviceDiscovery)
		}
	})

	return t
}

func (t *k8sVersionTestDriver) run() error {
	return diagnose.K8sVersion(newClusterInfo(), "", t.statusTracker)
}

func (t *k8sVersionTestDriver) setK8sVersion(minor string) {
	(t.fakeProducer.KubeClient.Discovery().(*fakediscovery.FakeDiscovery)).FakedServerVersion = &version.Info{
		Major:      "1",
		Minor:      minor,
		GitVersion: fmt.Sprintf("1.%s.0", minor),
	}
}
