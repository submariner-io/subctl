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

package deploy_test

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/submariner-io/admiral/pkg/reporter/test"
	"github.com/submariner-io/subctl/internal/cli"
	"github.com/submariner-io/subctl/internal/component"
	"github.com/submariner-io/subctl/pkg/broker"
	"github.com/submariner-io/subctl/pkg/client"
	clientfake "github.com/submariner-io/subctl/pkg/client/fake"
	"github.com/submariner-io/subctl/pkg/image"
	operatorv1alpha1 "github.com/submariner-io/submariner-operator/api/v1alpha1"
	"github.com/submariner-io/submariner-operator/pkg/discovery/clustersetip"
	v1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
)

const (
	testBrokerNamespace = "broker-namespace"
	testRepository      = "quay.io/submariner"
	testImageVersion    = "devel"
)

var ctx = context.TODO()

var _ = BeforeSuite(func() {
	Expect(operatorv1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())
	Expect(apiextensionsv1.AddToScheme(scheme.Scheme)).To(Succeed())
})

func TestDeploy(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Deploy Suite")
}

type testDriver struct {
	fakeProducer     *client.DefaultProducer
	brokerInfo       *broker.Info
	brokerSecret     *v1.Secret
	clustersetConfig clustersetip.Config
	repositoryInfo   *image.RepositoryInfo
	statusReporter   *test.Tracker
}

func newTestDriver() *testDriver {
	t := &testDriver{}

	BeforeEach(func() {
		t.fakeProducer = clientfake.New()
		t.statusReporter = &test.Tracker{Interface: cli.NewReporter()}

		t.brokerInfo = &broker.Info{
			BrokerURL: "https://broker.example.com:8443",
			IPSecPSK: &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: "ipsec-psk"},
				Data:       map[string][]byte{"psk": []byte("test-psk")},
			},
			Components: []string{component.ServiceDiscovery, component.Connectivity},
		}

		t.brokerSecret = &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "broker-secret"},
			Data: map[string][]byte{
				"ca.crt":    []byte("test-ca-cert"),
				"namespace": []byte(testBrokerNamespace),
				"token":     []byte("test-token"),
			},
		}

		t.repositoryInfo = &image.RepositoryInfo{
			Name:    testRepository,
			Version: testImageVersion,
		}

		t.clustersetConfig = clustersetip.Config{
			ClustersetIPCIDR: "243.0.0.0/8",
		}
	})

	return t
}
