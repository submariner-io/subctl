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

package cli_test

import (
	"bytes"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	admreporter "github.com/submariner-io/admiral/pkg/reporter"
	"github.com/submariner-io/subctl/internal/cli"
	"github.com/submariner-io/subctl/internal/env"
)

var _ = Describe("Reporter", func() {
	const (
		startMsg   = "Deploying my operator"
		successMsg = "Operation succeeded"
		failureMsg = "Operation failed"
		warningMsg = "Operation warned"
		successStr = "✓ "
		failureStr = "✗ "
		warningStr = "⚠ "
	)

	var (
		output           *testWriter
		reporter         admreporter.Interface
		useSmartTerminal bool
	)

	BeforeEach(func() {
		useSmartTerminal = false
		output = &testWriter{buf: &bytes.Buffer{}}
	})

	JustBeforeEach(func() {
		env.IsSmartTerminal = func(_ io.Writer) bool {
			return useSmartTerminal
		}

		reporter = cli.NewReporterWithWriter(output)
	})

	Context("Start", func() {
		It("should output the status message indicating in progress", func() {
			reporter.Start(startMsg)
			Expect(output.String()).To(And(ContainSubstring(startMsg), ContainSubstring("...")))
		})

		It("should output the status message with args", func() {
			reporter.Start("%s again", startMsg)
			Expect(output.String()).To(And(ContainSubstring(startMsg + " again")))
		})

		Context("followed by End", func() {
			It("should output the status message indicating complete", func() {
				reporter.Start(startMsg)
				reporter.End()
				Expect(output.String()).To(ContainSubstring(successStr + startMsg))
			})
		})

		It("should end a previous status before starting a new one", func() {
			otherMsg := "Starting something else"

			reporter.Start(startMsg)
			reporter.Start(otherMsg)

			out := output.String()
			Expect(out).To(ContainSubstring(successStr + startMsg))
			Expect(out).To(ContainSubstring(otherMsg))
		})
	})

	DescribeTableSubtree("",
		func(msg, completionStr string, msgFn func(string, ...any)) {
			Specify("after Start should queue the message until End", func() {
				reporter.Start(startMsg)
				msgFn(msg)
				Expect(output.String()).NotTo(ContainSubstring(msg))

				reporter.End()
				s := strings.Split(output.String(), "\n")
				Expect(s).To(ContainElements(ContainSubstring(completionStr+startMsg), ContainSubstring(completionStr+msg)))
			})

			It("should output immediately when no status is active", func() {
				msgFn(msg)
				Expect(output.String()).To(ContainSubstring(completionStr + msg))
			})

			It("should format the message with args", func() {
				msgFn("%s again", msg)
				Expect(output.String()).To(ContainSubstring(msg + " again"))
			})

			It("should ignore an empty message", func() {
				msgFn("")
				Expect(output.String()).To(BeEmpty())
			})
		},
		Entry("Success", successMsg, successStr, func(m string, a ...any) {
			reporter.Success(m, a...)
		}),
		Entry("Failure", failureMsg, failureStr, func(m string, a ...any) {
			reporter.Failure(m, a...)
		}),
		Entry("Warning", warningMsg, warningStr, func(m string, a ...any) {
			reporter.Warning(m, a...)
		}),
	)

	Context("End", func() {
		It("should do nothing when no status is active", func() {
			reporter.End()
			Expect(output.String()).To(BeEmpty())
		})

		It("should clear messages and reset status", func() {
			reporter.Start(startMsg)
			reporter.Success(successMsg)
			reporter.End()

			output.Reset()

			reporter.Start("second operation")
			reporter.End()

			Expect(output.String()).NotTo(And(ContainSubstring(startMsg), ContainSubstring(successMsg)))
		})

		Context("after multiple messages", func() {
			It("should indicate failure over warning and success", func() {
				reporter.Start(startMsg)
				reporter.Success(successMsg)
				reporter.Warning(warningMsg)
				reporter.Failure(failureMsg)
				reporter.End()

				out := output.String()
				Expect(out).To(ContainSubstring(failureStr + startMsg))
				Expect(out).To(ContainSubstring(successMsg))
				Expect(out).To(ContainSubstring(warningMsg))
				Expect(out).To(ContainSubstring(failureMsg))
			})

			It("should indicate warning over success", func() {
				reporter.Start(startMsg)
				reporter.Success(successMsg)
				reporter.Warning(warningMsg)
				reporter.End()

				out := output.String()
				Expect(out).To(ContainSubstring(warningStr + startMsg))
				Expect(out).To(ContainSubstring(successMsg))
				Expect(out).To(ContainSubstring(warningMsg))
			})

			It("should output all messages in order", func() {
				reporter.Start(startMsg)
				reporter.Success(successMsg)
				reporter.Warning(warningMsg)
				reporter.Failure(failureMsg)
				reporter.End()

				outputBytes := output.Bytes()
				firstIdx := bytes.Index(outputBytes, []byte(successMsg))
				secondIdx := bytes.Index(outputBytes, []byte(warningMsg))
				thirdIdx := bytes.Index(outputBytes, []byte(failureMsg))

				Expect(firstIdx).To(BeNumerically("<", secondIdx))
				Expect(secondIdx).To(BeNumerically("<", thirdIdx))
			})
		})
	})

	Context("with spinner", func() {
		BeforeEach(func() {
			useSmartTerminal = true
		})

		It("should output progress", func() {
			reporter.Start(startMsg)

			time.Sleep(300 * time.Millisecond) // Sleep a little to ensure the spinner outputs progress
			Expect(output.String()).To(ContainSubstring(startMsg))

			reporter.End()
			Expect(output.String()).To(And(ContainSubstring(strings.TrimSpace(successStr)), ContainSubstring(startMsg)))

			output.Reset()
			time.Sleep(200 * time.Millisecond)
			Expect(output.String()).To(BeEmpty())
		})
	})
})

type testWriter struct {
	sync.Mutex
	buf *bytes.Buffer
}

func (w *testWriter) Write(b []byte) (int, error) {
	_, _ = os.Stderr.Write(b)

	w.Lock()
	defer w.Unlock()

	return w.buf.Write(b)
}

func (w *testWriter) String() string {
	w.Lock()
	defer w.Unlock()

	return w.buf.String()
}

func (w *testWriter) Bytes() []byte {
	w.Lock()
	defer w.Unlock()

	return w.buf.Bytes()
}

func (w *testWriter) Reset() {
	w.Lock()
	defer w.Unlock()

	w.buf.Reset()
}
