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

package join_test

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/submariner-io/admiral/pkg/fake"
	compnames "github.com/submariner-io/admiral/pkg/names"
	"github.com/submariner-io/admiral/pkg/reporter/test"
	"github.com/submariner-io/subctl/internal/cli"
	"github.com/submariner-io/subctl/internal/component"
	"github.com/submariner-io/subctl/internal/constants"
	"github.com/submariner-io/subctl/pkg/broker"
	"github.com/submariner-io/subctl/pkg/client"
	clientfake "github.com/submariner-io/subctl/pkg/client/fake"
	"github.com/submariner-io/subctl/pkg/join"
	"github.com/submariner-io/submariner-operator/api/v1alpha1"
	"github.com/submariner-io/submariner-operator/pkg/discovery/clustersetip"
	"github.com/submariner-io/submariner-operator/pkg/discovery/globalnet"
	"github.com/submariner-io/submariner-operator/pkg/names"
	submarinerv1 "github.com/submariner-io/submariner/pkg/apis/submariner.io/v1"
	submclient "github.com/submariner-io/submariner/pkg/client/clientset/versioned"
	fakesubmclient "github.com/submariner-io/submariner/pkg/client/clientset/versioned/fake"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/version"
	fakedisc "k8s.io/client-go/discovery/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	k8stesting "k8s.io/client-go/testing"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	mcsv1a1 "sigs.k8s.io/mcs-api/pkg/apis/v1alpha1"
)

const (
	brokerNamespace = "broker-ns"
	brokerApiServer = "broker-api-server"
)

var _ = BeforeSuite(func() {
	Expect(v1alpha1.AddToScheme(scheme.Scheme)).To(Succeed())
	Expect(submarinerv1.AddToScheme(scheme.Scheme)).To(Succeed())
	Expect(submarinerv1.AddToScheme(scheme.Scheme)).To(Succeed())
	Expect(apiextensions.AddToScheme(scheme.Scheme)).To(Succeed())
	Expect(mcsv1a1.Install(scheme.Scheme)).To(Succeed())
})

func TestJoin(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Join Suite")
}

type testDriver struct {
	fakeProducer        *client.DefaultProducer
	brokerInfo          broker.Info
	options             join.Options
	clustersetIPEnabled bool
	globalnetEnabled    bool
	status              *test.Tracker
}

func newTestDriver() *testDriver {
	t := &testDriver{}

	BeforeEach(func() {
		t.fakeProducer = clientfake.New()
		t.status = &test.Tracker{Interface: cli.NewReporter()}
		t.clustersetIPEnabled = false
		t.globalnetEnabled = false

		fake.AddCreateReactor(&t.fakeProducer.KubeClient.(*k8sfake.Clientset).Fake)

		t.options = join.Options{
			ClusterID: "east",
		}

		t.brokerInfo = broker.Info{
			BrokerURL: "https://" + brokerApiServer,
			ClientToken: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: constants.SubmarinerBrokerAdminSA + "-token-",
					Annotations: map[string]string{
						corev1.ServiceAccountNameKey: constants.SubmarinerBrokerAdminSA,
					},
				},
				Type: corev1.SecretTypeServiceAccountToken,
				Data: map[string][]byte{"namespace": []byte(brokerNamespace), "token": {1, 2, 3}},
			},
			IPSecPSK: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "submariner-ipsec-psk",
				},
				Data: map[string][]byte{"psk": {1, 2, 3}},
			},
			Components: []string{component.ServiceDiscovery, component.Connectivity},
		}

		t.setFakedServerVersion("1", "33")

		broker.NewSubmarinerClientset = func(_ *rest.Config) (submclient.Interface, error) {
			return fakesubmclient.NewClientset(), nil
		}

		t.fakeProducer.KubeClient.(*k8sfake.Clientset).PrependReactor("create", "deployments",
			func(a k8stesting.Action) (bool, runtime.Object, error) {
				d := a.(k8stesting.CreateAction).GetObject().(*appsv1.Deployment)
				d.Status.Conditions = []appsv1.DeploymentCondition{
					{
						Type:   appsv1.DeploymentAvailable,
						Status: corev1.ConditionTrue,
					},
				}

				return false, nil, nil
			})

		t.fakeProducer.KubeClient.(*k8sfake.Clientset).PrependReactor("create", "secrets",
			func(a k8stesting.Action) (bool, runtime.Object, error) {
				s := a.(k8stesting.CreateAction).GetObject().(*corev1.Secret)
				if s.Data == nil {
					s.Data = map[string][]byte{}
				}

				if len(s.Data["token"]) == 0 {
					if s.Name != "" {
						s.Data["token"] = []byte(s.Name)
					} else {
						s.Data["token"] = []byte(s.GenerateName)
					}
				}

				if len(s.Data["namespace"]) == 0 {
					s.Data["namespace"] = []byte(a.GetNamespace())
				}

				return false, nil, nil
			})
	})

	JustBeforeEach(func() {
		Expect(clustersetip.CreateConfigMap(context.TODO(), t.fakeProducer.GeneralClient, t.clustersetIPEnabled,
			clustersetip.DefaultCIDR, clustersetip.DefaultAllocationSize, brokerNamespace)).To(Succeed())

		Expect(globalnet.CreateConfigMap(context.TODO(), t.fakeProducer.GeneralClient, t.globalnetEnabled,
			globalnet.DefaultGlobalnetCIDR, globalnet.DefaultGlobalnetClusterSize, brokerNamespace)).To(Succeed())
	})

	return t
}

func (t *testDriver) runJoinClusterToBroker() error {
	return join.ClusterToBroker(context.TODO(), &t.brokerInfo, &t.options, t.fakeProducer, t.status)
}

func (t *testDriver) setFakedServerVersion(major, minor string) {
	(t.fakeProducer.KubeClient.Discovery().(*fakedisc.FakeDiscovery)).FakedServerVersion = &version.Info{
		Major: major,
		Minor: minor,
	}
}

func (t *testDriver) getSubmarinerResource() (*v1alpha1.Submariner, error) {
	subm := &v1alpha1.Submariner{}
	err := t.fakeProducer.GeneralClient.Get(context.TODO(), ctrlclient.ObjectKey{
		Namespace: constants.OperatorNamespace, Name: names.SubmarinerCrName,
	}, subm)

	return subm, err
}

func (t *testDriver) assertSubmarinerResource() *v1alpha1.Submariner {
	subm, err := t.getSubmarinerResource()
	Expect(err).NotTo(HaveOccurred())

	return subm
}

func (t *testDriver) assertNoSubmarinerResource() {
	_, err := t.getSubmarinerResource()
	Expect(err).To(HaveOccurred())
}

func (t *testDriver) getServiceDiscoveryResource() (*v1alpha1.ServiceDiscovery, error) {
	sd := &v1alpha1.ServiceDiscovery{}
	err := t.fakeProducer.GeneralClient.Get(context.TODO(), ctrlclient.ObjectKey{
		Namespace: constants.OperatorNamespace, Name: names.ServiceDiscoveryCrName,
	}, sd)

	return sd, err
}

func (t *testDriver) assertServiceDiscoveryResource() *v1alpha1.ServiceDiscovery {
	sd, err := t.getServiceDiscoveryResource()
	Expect(err).NotTo(HaveOccurred())

	return sd
}

func (t *testDriver) assertNoServiceDiscoveryResource() {
	_, err := t.getServiceDiscoveryResource()
	Expect(err).To(HaveOccurred())
}

func (t *testDriver) getOperatorDeployment() *appsv1.Deployment {
	deployment, err := t.fakeProducer.KubeClient.AppsV1().Deployments(constants.OperatorNamespace).Get(
		context.TODO(), compnames.OperatorComponent, metav1.GetOptions{})
	Expect(err).NotTo(HaveOccurred())

	return deployment
}
