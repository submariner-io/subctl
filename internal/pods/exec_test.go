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

package pods_test

import (
	"errors"
	"fmt"
	"net/url"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/submariner-io/admiral/pkg/fake"
	"github.com/submariner-io/admiral/pkg/resource"
	"github.com/submariner-io/subctl/internal/pods"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

var _ = Describe("ExecWithOptions", func() {
	const (
		stdoutOutput = "command output"
		stderrOutput = "error output"
	)

	stdoutOutputWS := fmt.Sprintf("  %s  \n", stdoutOutput)
	stderrOutputWS := fmt.Sprintf("  %s  \n", stderrOutput)

	var (
		config           pods.ExecConfig
		execOptions      pods.ExecOptions
		capturedRawQuery string
	)

	BeforeEach(func() {
		capturedRawQuery = ""
		config = pods.ExecConfig{
			RestConfig: &rest.Config{},
			ClientSet:  fake.WithRESTClient(k8sfake.NewClientset(), nil),
		}

		execOptions = pods.ExecOptionsFromPod(&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "test-namespace",
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name: "test-container",
					},
				},
			},
		})

		fake.SetSPDYExecutor(stdoutOutputWS, stderrOutputWS, nil)
	})

	JustBeforeEach(func() {
		delegate := resource.NewSPDYExecutor
		resource.NewSPDYExecutor = func(c *rest.Config, m string, url *url.URL) (remotecommand.Executor, error) {
			Expect(url.Path).To(Equal(fmt.Sprintf("/namespaces/%s/pods/%s/exec", execOptions.Namespace, execOptions.PodName)))
			Expect(url.RawQuery).To(ContainSubstring("container=" + execOptions.ContainerName))
			capturedRawQuery = url.RawQuery

			return delegate(c, m, url)
		}
	})

	Context("with defaults", func() {
		It("should return stdout and stderr with whitespace trimmed", func(ctx SpecContext) {
			stdout, stderr, err := pods.ExecWithOptions(ctx, config, &execOptions)

			Expect(err).NotTo(HaveOccurred())
			Expect(stdout).To(Equal(stdoutOutput))
			Expect(stderr).To(Equal(stderrOutput))
			Expect(capturedRawQuery).To(And(ContainSubstring("stdout=true"), ContainSubstring("stderr=true")))
		})
	})

	Context("with PreserveWhitespace enabled", func() {
		BeforeEach(func() {
			execOptions.PreserveWhitespace = true
		})

		It("should preserve whitespace in stdout and stderr", func(ctx SpecContext) {
			stdout, stderr, err := pods.ExecWithOptions(ctx, config, &execOptions)

			Expect(err).NotTo(HaveOccurred())
			Expect(stdout).To(Equal(stdoutOutputWS))
			Expect(stderr).To(Equal(stderrOutputWS))
		})
	})

	Context("with capture output disabled", func() {
		BeforeEach(func() {
			execOptions.CaptureStdout = false
			execOptions.CaptureStderr = false
		})

		It("should set the request query parameters appropriately", func(ctx SpecContext) {
			_, _, err := pods.ExecWithOptions(ctx, config, &execOptions)

			Expect(err).NotTo(HaveOccurred())
			Expect(capturedRawQuery).NotTo(ContainSubstring("stdout="))
			Expect(capturedRawQuery).NotTo(ContainSubstring("stderr="))
		})
	})

	Context("when the executor fails", func() {
		BeforeEach(func() {
			fake.SetSPDYExecutor("", "", errors.New("executor error"))
		})

		It("should return an error", func(ctx SpecContext) {
			_, _, err := pods.ExecWithOptions(ctx, config, &execOptions)
			Expect(err).To(HaveOccurred())
		})
	})
})
