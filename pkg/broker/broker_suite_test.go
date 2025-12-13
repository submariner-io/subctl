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

package broker_test

import (
	"context"
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/submariner-io/admiral/pkg/reporter/test"
	"github.com/submariner-io/subctl/internal/cli"
	"github.com/submariner-io/subctl/pkg/broker"
	"github.com/submariner-io/submariner-operator/pkg/crd"
	corev1 "k8s.io/api/core/v1"
	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	k8stesting "k8s.io/client-go/testing"
	controllerfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var ctx = context.TODO()

var _ = BeforeSuite(func() {
	Expect(apiextensions.AddToScheme(scheme.Scheme)).To(Succeed())
})

func TestBroker(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Broker Suite")
}

type testDriver struct {
	kubeClient     kubernetes.Interface
	crdUpdater     crd.Updater
	statusReporter *test.Tracker
}

func newTestDriver() *testDriver {
	t := &testDriver{}

	BeforeEach(func() {
		t.kubeClient = k8sfake.NewClientset()
		controllerClient := controllerfake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
		t.crdUpdater = crd.UpdaterFromControllerClient(controllerClient)
		t.statusReporter = &test.Tracker{Interface: cli.NewReporter()}

		// Add reactor to automatically generate tokens for secrets
		t.kubeClient.(*k8sfake.Clientset).PrependReactor("create", "secrets",
			func(a k8stesting.Action) (bool, runtime.Object, error) {
				s := a.(k8stesting.CreateAction).GetObject().(*corev1.Secret)
				if s.Data == nil {
					s.Data = map[string][]byte{}
				}

				if len(s.Data["token"]) == 0 {
					if s.Name != "" {
						s.Data["token"] = []byte(s.Name + "-token")
					} else {
						s.Data["token"] = []byte(s.GenerateName + "-token")
					}
				}

				if len(s.Data["namespace"]) == 0 {
					s.Data["namespace"] = []byte(a.GetNamespace())
				}

				return false, nil, nil
			})

		broker.NewKubeClient = func(_ *rest.Config) (kubernetes.Interface, error) {
			return t.kubeClient, nil
		}
	})

	return t
}

func setupInfoFileDir() {
	var err error

	broker.InfoFileDir, err = os.MkdirTemp("", "broker-info-test")
	Expect(err).NotTo(HaveOccurred())

	DeferCleanup(func() {
		_ = os.RemoveAll(broker.InfoFileDir)
	})
}
